package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/extensions"
)

func registerExtensionRoutes(mux *http.ServeMux, store *extensions.Store, credentials *auth.Store) {
	mux.HandleFunc("POST /extensions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "profile:write", false)
		if !ok {
			return
		}
		var in struct {
			Name                 string                    `json:"name"`
			Description          string                    `json:"description"`
			OperatorContact      string                    `json:"operator_contact"`
			Capabilities         []string                  `json:"capabilities"`
			CallbackEndpoint     string                    `json:"callback_endpoint"`
			ActionEndpoint       string                    `json:"action_endpoint"`
			RequestedPermissions []extensions.Permission   `json:"requested_permissions"`
			SupportedEvents      []string                  `json:"supported_events"`
			CredentialRotation   extensions.RotationPolicy `json:"credential_rotation"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_extension", "request body must be valid JSON")
			return
		}
		verified, err := verifyExtensionEndpoints(r.Context(), in.CallbackEndpoint, in.ActionEndpoint)
		if err != nil {
			writeAPIError(w, 422, "endpoint_verification_failed", err.Error())
			return
		}
		v, err := store.Create(actor.UserID, extensions.Registration{Name: in.Name, Description: in.Description, OperatorContact: in.OperatorContact, Capabilities: in.Capabilities, CallbackURL: in.CallbackEndpoint, ActionURL: in.ActionEndpoint, RequestedPermissions: in.RequestedPermissions, SupportedEvents: in.SupportedEvents, CredentialRotation: in.CredentialRotation}, verified)
		if errors.Is(err, extensions.ErrInvalid) {
			writeAPIError(w, 422, "invalid_extension", "identity, contract, permissions, events, and rotation policy are required")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "extension_storage_unavailable", "extension could not be registered")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /extensions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "profile:write", false)
		if !ok {
			return
		}
		v, e := store.List(actor.UserID)
		if e != nil {
			writeAPIError(w, 500, "extension_storage_unavailable", "extensions could not be loaded")
			return
		}
		writeJSON(w, 200, map[string]any{"extensions": v})
	})
	mux.HandleFunc("GET /extensions/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "profile:write", false)
		if !ok {
			return
		}
		v, e := store.Get(r.PathValue("id"))
		if errors.Is(e, extensions.ErrNotFound) || e == nil && v.OwnerID != actor.UserID {
			writeAPIError(w, 404, "extension_not_found", "extension not found")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "extension_storage_unavailable", "extension could not be loaded")
			return
		}
		writeJSON(w, 200, v)
	})
}

func verifyExtensionEndpoints(ctx context.Context, raw ...string) (time.Time, error) {
	challengeBytes := make([]byte, 24)
	if _, e := rand.Read(challengeBytes); e != nil {
		return time.Time{}, errors.New("could not create endpoint challenge")
	}
	challenge := hex.EncodeToString(challengeBytes)
	for _, value := range raw {
		u, e := url.Parse(value)
		if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname()))) {
			return time.Time{}, errors.New("endpoints must use HTTPS (HTTP is accepted only for loopback development)")
		}
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if e != nil {
			return time.Time{}, errors.New("endpoint URL is invalid")
		}
		req.Header.Set("Vivarium-Extension-Challenge", challenge)
		client := http.Client{
			Timeout: 5 * time.Second,
			// The proof belongs to the declared endpoint, not a destination it
			// selects after receiving the challenge. Never forward the header.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		res, e := client.Do(req)
		if e != nil {
			return time.Time{}, errors.New("endpoint did not answer its ownership challenge")
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1024))
		_ = res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 || strings.TrimSpace(res.Header.Get("Vivarium-Extension-Challenge")) != challenge {
			return time.Time{}, errors.New("endpoint did not echo the Vivarium-Extension-Challenge response header")
		}
	}
	return time.Now().UTC().Truncate(time.Microsecond), nil
}

func isLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback() || strings.EqualFold(host, "localhost")
}
