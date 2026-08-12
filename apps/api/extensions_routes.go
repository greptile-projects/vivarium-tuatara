package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/extensions"
)

func registerExtensionRoutes(mux *http.ServeMux, store *extensions.Store, credentials *auth.Store) {
	registerExtensionRoutesWithVerifier(mux, store, credentials, verifyExtensionEndpoints)
}

func registerExtensionRoutesWithVerifier(mux *http.ServeMux, store *extensions.Store, credentials *auth.Store, verify func(context.Context, ...string) (time.Time, error)) {
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
		verified, err := verify(r.Context(), in.CallbackEndpoint, in.ActionEndpoint)
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
		_, ok := authenticateRequest(w, r, credentials, "profile:write", false)
		if !ok {
			return
		}
		v, e := store.Get(r.PathValue("id"))
		if errors.Is(e, extensions.ErrNotFound) {
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
	return verifyExtensionEndpointsWithNetwork(ctx, net.DefaultResolver.LookupIP, (&net.Dialer{Timeout: 5 * time.Second}).DialContext, raw...)
}

type lookupIPFunc func(context.Context, string, string) ([]net.IP, error)
type dialContextFunc func(context.Context, string, string) (net.Conn, error)

func verifyExtensionEndpointsWithNetwork(ctx context.Context, lookup lookupIPFunc, dial dialContextFunc, raw ...string) (time.Time, error) {
	challengeBytes := make([]byte, 24)
	if _, e := rand.Read(challengeBytes); e != nil {
		return time.Time{}, errors.New("could not create endpoint challenge")
	}
	challenge := hex.EncodeToString(challengeBytes)
	for _, value := range raw {
		u, e := url.Parse(value)
		if e != nil || u.Host == "" || u.User != nil || u.Scheme != "https" {
			return time.Time{}, errors.New("endpoints must use HTTPS")
		}
		addresses, e := lookup(ctx, "ip", u.Hostname())
		if e != nil || len(addresses) == 0 {
			return time.Time{}, errors.New("endpoint hostname could not be resolved")
		}
		for _, address := range addresses {
			if !publicEndpointIP(address) {
				return time.Time{}, errors.New("endpoint must resolve only to publicly routable addresses")
			}
		}
		// Dial only an address from the validated resolution. This closes the
		// DNS-rebinding window between policy validation and connection setup.
		pinned := append([]net.IP(nil), addresses...)
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			_, port, splitErr := net.SplitHostPort(address)
			if splitErr != nil {
				return nil, splitErr
			}
			var last error
			for _, ip := range pinned {
				connection, dialErr := dial(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return connection, nil
				}
				last = dialErr
			}
			return nil, last
		}
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if e != nil {
			return time.Time{}, errors.New("endpoint URL is invalid")
		}
		req.Header.Set("Vivarium-Extension-Challenge", challenge)
		client := http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
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

func publicEndpointIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range nonPublicEndpointPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

// IsGlobalUnicast includes special-purpose address space. Endpoint ownership
// verification needs the narrower Internet-public boundary from the IANA
// special-purpose registries, because routed CGNAT or benchmark space may still
// reach platform-internal services. The broader enclosing prefixes below fail
// closed for transition/relay ranges that are inappropriate extension origins.
var nonPublicEndpointPrefixes = mustEndpointPrefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
	"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24",
	"192.31.196.0/24", "192.52.193.0/24", "192.88.99.0/24",
	"192.168.0.0/16", "192.175.48.0/24", "198.18.0.0/15",
	"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "64:ff9b:1::/48", "100::/64", "2001::/23",
	"2001:db8::/32", "2002::/16", "fc00::/7", "fe80::/10", "ff00::/8",
)

func mustEndpointPrefixes(values ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, len(values))
	for i, value := range values {
		prefixes[i] = netip.MustParsePrefix(value)
	}
	return prefixes
}
