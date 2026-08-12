package main

import (
	"encoding/json"
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
	registerExtensionRoutes(mux, store, credentials)
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
	registerExtensionRoutes(mux, store, credentials)
	body := `{"name":"Review lens","operator_contact":"ops@example.test","capabilities":["review"],"callback_endpoint":"` + endpoint.URL + `","action_endpoint":"` + endpoint.URL + `","requested_permissions":[{"resource":"pull_requests","actions":["read"]}],"supported_events":["pull_request.opened"],"credential_rotation":{"interval_days":30}}`
	req := httptest.NewRequest("POST", "/extensions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token.Token)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != 422 {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
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
