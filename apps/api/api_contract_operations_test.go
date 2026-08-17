package main

import (
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
)

func TestClientHandoffResolvesExactIntegrationWork(t *testing.T) {
	store, err := apicontracts.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour)
	app := apicontracts.Application{ID: "app", RepositoryID: "provider", ContractID: "contract", ContractVersion: 2, Status: "approved", ApprovalExpiresAt: &expires}
	preload := apicontracts.IntegrationPreload{SyntheticOnly: true}
	first, err := store.CreateIntegrationWork(app, "owner", "consumer-one", "commit-one", "task", "human", "owner", "first", preload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateIntegrationWork(app, "owner", "consumer-two", "commit-two", "task", "human", "owner", "second", preload)
	if err != nil {
		t.Fatal(err)
	}
	repositoryID, err := clientHandoffRepository(store, app, second.ID)
	if err != nil || repositoryID != "consumer-two" {
		t.Fatalf("second work resolved to %q, %v", repositoryID, err)
	}
	if repositoryID, err = clientHandoffRepository(store, app, first.ID); err != nil || repositoryID != "consumer-one" {
		t.Fatalf("first work resolved to %q, %v", repositoryID, err)
	}
	otherApp := app
	otherApp.ID = "other"
	if _, err = clientHandoffRepository(store, otherApp, second.ID); err == nil {
		t.Fatal("cross-application work was accepted")
	}
}
