package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/extensions"
)

func TestExtensionRegistrationVerifiesIdentityAndGrantsNothing(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vivarium-Extension-Challenge", r.Header.Get("Vivarium-Extension-Challenge"))
		w.WriteHeader(204)
	}))
	defer endpoint.Close()
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const ownerID = "0123456789abcdef0123456789abcdef"
	token, err := credentials.Issue(ownerID, auth.Session, "owner", []string{"profile:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	store, err := extensions.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerExtensionRoutesWithVerifier(mux, store, credentials, func(context.Context, ...string) (time.Time, error) {
		return time.Now().UTC(), nil
	})
	body := `{"name":"Review lens","description":"Adds review evidence","operator_contact":"ops@example.test","capabilities":["review annotations"],"callback_endpoint":"` + endpoint.URL + `/events","action_endpoint":"` + endpoint.URL + `/actions","requested_permissions":[{"resource":"pull_requests","actions":["read","comment"]}],"supported_events":["pull_request.opened"],"credential_rotation":{"interval_days":30,"overlap_hours":2}}`
	req := httptest.NewRequest("POST", "/extensions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token.Token)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != 201 {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var v extensions.Extension
	if err = json.Unmarshal(res.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.PrincipalType != "extension" || v.OwnerID != ownerID || v.CallbackEndpoint.VerifiedAt.IsZero() || v.AuthorityPreview.Installed || len(v.AuthorityPreview.Items) != 1 || len(v.AuthorityPreview.Items[0].EffectiveActions) != 0 {
		t.Fatalf("unsafe projection: %#v", v)
	}
	if v.ID == ownerID {
		t.Fatal("extension impersonated owner identity")
	}
}

func TestExtensionRegistrationRejectsUnverifiedEndpoint(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer endpoint.Close()
	credentials, _ := auth.New(t.TempDir())
	token, _ := credentials.Issue("0123456789abcdef0123456789abcdef", auth.Session, "owner", []string{"profile:write"}, time.Hour)
	store, _ := extensions.New(t.TempDir())
	mux := http.NewServeMux()
	registerExtensionRoutesWithVerifier(mux, store, credentials, func(context.Context, ...string) (time.Time, error) {
		return time.Time{}, errors.New("challenge mismatch")
	})
	body := `{"name":"Review lens","operator_contact":"ops@example.test","capabilities":["review"],"callback_endpoint":"` + endpoint.URL + `","action_endpoint":"` + endpoint.URL + `","requested_permissions":[{"resource":"pull_requests","actions":["read"]}],"supported_events":["pull_request.opened"],"credential_rotation":{"interval_days":30}}`
	req := httptest.NewRequest("POST", "/extensions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token.Token)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != 422 {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestExtensionEndpointVerificationRejectsNonPublicAddresses(t *testing.T) {
	for _, address := range []string{
		"0.0.0.0", "10.0.0.1", "100.64.0.1", "127.0.0.1", "169.254.1.1",
		"192.0.0.1", "192.0.2.1", "192.168.0.1", "198.18.0.1",
		"198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1",
		"::", "::1", "::ffff:100.64.0.1", "64:ff9b:1::1", "100::1",
		"2001:db8::1", "2002::1", "fc00::1", "fe80::1", "ff00::1",
	} {
		t.Run(address, func(t *testing.T) {
			dialed := false
			_, err := verifyExtensionEndpointsWithNetwork(context.Background(), func(context.Context, string, string) ([]net.IP, error) {
				return []net.IP{net.ParseIP(address)}, nil
			}, func(context.Context, string, string) (net.Conn, error) {
				dialed = true
				return nil, errors.New("unexpected dial")
			}, "https://extension.example/events")
			if err == nil || !strings.Contains(err.Error(), "publicly routable") {
				t.Fatalf("error = %v", err)
			}
			if dialed {
				t.Fatal("verifier dialed a protected address")
			}
		})
	}
}

func TestPublicEndpointIPAcceptsOrdinaryPublicAddresses(t *testing.T) {
	for _, address := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !publicEndpointIP(net.ParseIP(address)) {
			t.Fatalf("publicEndpointIP(%s) = false", address)
		}
	}
}

func TestExtensionEndpointVerificationAllowsOnlyOptInDevelopmentLoopback(t *testing.T) {
	t.Setenv("EXTENSION_DEVELOPMENT_ENDPOINTS", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vivarium-Extension-Challenge", r.Header.Get("Vivarium-Extension-Challenge"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if _, err := verifyExtensionEndpoints(context.Background(), server.URL); err != nil {
		t.Fatalf("development loopback verification: %v", err)
	}
	_, err := verifyExtensionEndpointsWithNetwork(context.Background(), func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.1")}, nil
	}, func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("non-loopback development endpoint must fail before dialing")
		return nil, nil
	}, "http://extension.example/events")
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("non-loopback HTTP error = %v", err)
	}
}

func TestExtensionEndpointVerificationRejectsDevelopmentLocalhostResolvingOutsideLoopback(t *testing.T) {
	t.Setenv("EXTENSION_DEVELOPMENT_ENDPOINTS", "1")
	dialed := false
	_, err := verifyExtensionEndpointsWithNetwork(context.Background(), func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.23.45.67")}, nil
	}, func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, errors.New("unexpected dial")
	}, "http://localhost:18080/ownership")
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("error = %v", err)
	}
	if dialed {
		t.Fatal("verifier dialed a non-loopback development resolution")
	}
}

func TestExtensionEndpointVerificationRejectsMixedPublicAndPrivateResolution(t *testing.T) {
	_, err := verifyExtensionEndpointsWithNetwork(context.Background(), func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("127.0.0.1")}, nil
	}, func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("mixed resolution must fail before dialing")
		return nil, nil
	}, "https://extension.example/events")
	if err == nil || !strings.Contains(err.Error(), "publicly routable") {
		t.Fatalf("error = %v", err)
	}
}

func TestExtensionRegistrationRejectsRedirectedEndpoint(t *testing.T) {
	targetRequests := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests++
		w.Header().Set("Vivarium-Extension-Challenge", r.Header.Get("Vivarium-Extension-Challenge"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	credentials, _ := auth.New(t.TempDir())
	token, _ := credentials.Issue("0123456789abcdef0123456789abcdef", auth.Session, "owner", []string{"profile:write"}, time.Hour)
	store, _ := extensions.New(t.TempDir())
	mux := http.NewServeMux()
	registerExtensionRoutes(mux, store, credentials)
	body := `{"name":"Review lens","operator_contact":"ops@example.test","capabilities":["review"],"callback_endpoint":"` + redirect.URL + `","action_endpoint":"` + redirect.URL + `","requested_permissions":[{"resource":"pull_requests","actions":["read"]}],"supported_events":["pull_request.opened"],"credential_rotation":{"interval_days":30}}`
	req := httptest.NewRequest("POST", "/extensions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token.Token)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if targetRequests != 0 {
		t.Fatalf("redirect target received %d challenge request(s)", targetRequests)
	}
}
