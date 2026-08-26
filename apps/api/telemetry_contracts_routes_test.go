package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestPublicRepositoryReaderCannotPublishTelemetryVerification(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	credentials, _ := auth.New(t.TempDir())
	owner, reader := strings.Repeat("1", 32), strings.Repeat("2", 32)
	repository, err := catalog.Create(owner, "public-telemetry")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = catalog.SetVisibility(owner, repository.ID, repositories.Public); err != nil {
		t.Fatal(err)
	}
	issued, err := credentials.Issue(reader, auth.API, "reader", []string{"repositories:read", "repositories:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerTelemetryContractRoutes(mux, gitStore, catalog, credentials, nil, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodPost, "/repositories/"+repository.ID+"/pulls/eligible/telemetry-verifications", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
