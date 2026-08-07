package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestUserAccountLifecycleAPI(t *testing.T) {
	repositories, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identities, err := users.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newAppHandler(repositories, identities))
	t.Cleanup(server.Close)

	created := requestUser(t, http.MethodPost, server.URL+"/users", `{"handle":"octo-cat","display_name":"Octo Cat"}`, http.StatusCreated)
	if created.ID == "" || created.Handle != "octo-cat" || created.DisplayName != "Octo Cat" {
		t.Fatalf("created = %#v", created)
	}
	inspected := requestUser(t, http.MethodGet, server.URL+"/users/"+created.ID, "", http.StatusOK)
	if inspected != created {
		t.Fatalf("inspected = %#v, created = %#v", inspected, created)
	}
	updated := requestUser(t, http.MethodPatch, server.URL+"/users/"+created.ID, `{"display_name":"The Octocat"}`, http.StatusOK)
	if updated.ID != created.ID || updated.CreatedAt != created.CreatedAt || updated.Handle != created.Handle || updated.DisplayName != "The Octocat" {
		t.Fatalf("updated = %#v", updated)
	}

	requestStatus(t, http.MethodPost, server.URL+"/users", `{"handle":"octo-cat","display_name":"Duplicate"}`, http.StatusConflict)
	requestStatus(t, http.MethodPatch, server.URL+"/users/"+created.ID, `{}`, http.StatusBadRequest)
	requestStatus(t, http.MethodGet, server.URL+"/users/not-an-id", "", http.StatusNotFound)
}

func requestUser(t *testing.T, method, url, body string, status int) users.User {
	t.Helper()
	response := requestStatus(t, method, url, body, status)
	defer response.Body.Close()
	var user users.User
	if err := json.NewDecoder(response.Body).Decode(&user); err != nil {
		t.Fatal(err)
	}
	return user
}

func requestStatus(t *testing.T, method, url, body string, status int) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		response.Body.Close()
		t.Fatalf("%s %s status = %d, want %d", method, url, response.StatusCode, status)
	}
	if status >= 400 {
		response.Body.Close()
	}
	return response
}
