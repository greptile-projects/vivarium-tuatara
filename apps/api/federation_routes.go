package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/federation"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func registerFederationRoutes(mux *http.ServeMux, store *federation.Store, userStore *users.Store, organizationStore *organizations.Store, credentials *auth.Store) {
	mux.HandleFunc("GET /.well-known/vivarium-federation", func(w http.ResponseWriter, _ *http.Request) {
		d, err := store.Identity()
		if err != nil {
			writeAPIError(w, 503, "federation_unavailable", err.Error())
			return
		}
		writeJSON(w, 200, d)
	})
	mux.HandleFunc("GET /federation/actors/user/{id}", func(w http.ResponseWriter, r *http.Request) {
		u, err := userStore.Get(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "federated_actor_not_found", "actor not found")
			return
		}
		d, _ := store.Identity()
		writeJSON(w, 200, map[string]any{"identity": d.InstanceID + ":user:" + u.ID, "instance_id": d.InstanceID, "type": "user", "id": u.ID, "handle": u.Handle, "display_name": u.DisplayName, "profile_version": u.UpdatedAt, "identity_document_version": d.Version})
	})
	mux.HandleFunc("GET /federation/actors/agent/{id}", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.PathValue("id"), ":")
		if len(parts) != 2 || organizationStore == nil {
			writeAPIError(w, 404, "federated_actor_not_found", "actor not found")
			return
		}
		org, err := organizationStore.Get(parts[0])
		if err != nil {
			writeAPIError(w, 404, "federated_actor_not_found", "actor not found")
			return
		}
		for _, a := range org.Agents {
			if a.ID == parts[1] && a.Visibility == "public" {
				d, _ := store.Identity()
				writeJSON(w, 200, map[string]any{"identity": d.InstanceID + ":agent:" + parts[0] + ":" + a.ID, "instance_id": d.InstanceID, "type": "agent", "id": parts[0] + ":" + a.ID, "name": a.Name, "description": a.Description, "capabilities": a.Capabilities, "operators": a.OperatorIDs, "profile_version": a.Version, "identity_document_version": d.Version})
				return
			}
		}
		writeAPIError(w, 404, "federated_actor_not_found", "actor not found")
	})
	require := func(w http.ResponseWriter, r *http.Request) bool {
		_, ok := authenticateRequest(w, r, credentials, "profile:write", false)
		return ok
	}
	mux.HandleFunc("GET /federation/peers", func(w http.ResponseWriter, r *http.Request) {
		if !require(w, r) {
			return
		}
		p, e := store.List()
		if e != nil {
			writeAPIError(w, 503, "federation_unavailable", e.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"peers": p})
	})
	mux.HandleFunc("POST /federation/identity/rotate", func(w http.ResponseWriter, r *http.Request) {
		if !require(w, r) {
			return
		}
		d, e := store.Rotate()
		if e != nil {
			writeAPIError(w, 503, "federation_unavailable", e.Error())
			return
		}
		writeJSON(w, 200, d)
	})
	mux.HandleFunc("POST /federation/peers", func(w http.ResponseWriter, r *http.Request) {
		if !require(w, r) {
			return
		}
		var in struct {
			DiscoveryURL string `json:"discovery_url"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "discovery_url is required")
			return
		}
		d, e := fetchFederationDocument(in.DiscoveryURL)
		if e != nil {
			writeAPIError(w, 422, "peer_unreachable", e.Error())
			return
		}
		p, e := store.Upsert(in.DiscoveryURL, d)
		if e != nil {
			writeFederationError(w, e)
			return
		}
		writeJSON(w, 201, p)
	})
	mux.HandleFunc("POST /federation/peers/{id}/refresh", func(w http.ResponseWriter, r *http.Request) {
		if !require(w, r) {
			return
		}
		old, e := store.Get(r.PathValue("id"))
		if e != nil {
			writeFederationError(w, e)
			return
		}
		d, e := fetchFederationDocument(old.DiscoveryURL)
		if e != nil {
			p, _ := store.RecordFailure(old.InstanceID, e.Error())
			writeJSON(w, 202, p)
			return
		}
		if d.InstanceID != old.InstanceID {
			p, _ := store.RecordFailure(old.InstanceID, "discovery URL now identifies a different instance")
			writeJSON(w, 202, p)
			return
		}
		p, e := store.Upsert(old.DiscoveryURL, d)
		if e != nil {
			writeFederationError(w, e)
			return
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("PATCH /federation/peers/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !require(w, r) {
			return
		}
		var in struct {
			Version int    `json:"version"`
			Action  string `json:"action"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "version and action are required")
			return
		}
		p, e := store.Decide(r.PathValue("id"), in.Version, in.Action)
		if e != nil {
			writeFederationError(w, e)
			return
		}
		writeJSON(w, 200, p)
	})
}

func fetchFederationDocument(raw string) (federation.Document, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return federation.Document{}, fmt.Errorf("invalid discovery URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return federation.Document{}, fmt.Errorf("discovery requires HTTPS (HTTP is allowed only for loopback development)")
	}
	u.Path = "/.well-known/vivarium-federation"
	u.RawQuery = ""
	u.Fragment = ""
	c := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Get(u.String())
	if err != nil {
		return federation.Document{}, fmt.Errorf("peer unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return federation.Document{}, fmt.Errorf("peer returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return federation.Document{}, err
	}
	var d federation.Document
	if json.Unmarshal(body, &d) != nil {
		return d, fmt.Errorf("peer returned an invalid identity document")
	}
	if err = federation.Verify(d); err != nil {
		return d, fmt.Errorf("peer signature is invalid")
	}
	return d, nil
}
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func writeFederationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, federation.ErrNotFound):
		writeAPIError(w, 404, "federation_peer_not_found", err.Error())
	case errors.Is(err, federation.ErrConflict):
		writeAPIError(w, 409, "federation_conflict", err.Error())
	case errors.Is(err, federation.ErrInvalid):
		writeAPIError(w, 422, "invalid_federation_identity", err.Error())
	default:
		writeAPIError(w, 503, "federation_unavailable", err.Error())
	}
}
