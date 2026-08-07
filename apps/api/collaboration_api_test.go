package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestPublicInterfacesSupportProposalToMergeCollaboration(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	proposalStore, _ := proposals.New(t.TempDir())
	pullStore, _ := pullrequests.New(t.TempDir(), gitStore)
	activityStore, _ := activities.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, proposalStore, pullStore, activityStore))
	defer server.Close()

	maintainer := createTestAccount(t, server.URL, "journey-maintainer")
	newcomer := createTestAccount(t, server.URL, "journey-newcomer")
	outsider := createTestAccount(t, server.URL, "journey-outsider")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"welcome"}`, maintainer.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, created, &repository)
	maintainerGit := issueGitCredential(t, server.URL, maintainer.Credential.Token, "maintainer git")
	newcomerGit := issueGitCredential(t, server.URL, newcomer.Credential.Token, "newcomer git")
	remoteWithToken := func(token string) string {
		remote, _ := url.Parse(server.URL + repository.GitRemote)
		remote.User = url.UserPassword("git", token)
		return remote.String()
	}

	maintainerCopy := filepath.Join(t.TempDir(), "maintainer")
	gitCommand(t, "", "clone", remoteWithToken(maintainerGit.Token), maintainerCopy)
	gitCommand(t, maintainerCopy, "config", "user.name", "Maintainer")
	gitCommand(t, maintainerCopy, "config", "user.email", "maintainer@example.com")
	if err := os.WriteFile(filepath.Join(maintainerCopy, "README.md"), []byte("# Welcome\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, maintainerCopy, "add", "README.md")
	gitCommand(t, maintainerCopy, "commit", "-m", "Start project")
	gitCommand(t, maintainerCopy, "push", "origin", "main")

	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+newcomer.User.ID+`"}`, maintainer.Credential.Token, http.StatusCreated).Body.Close()
	proposalResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/proposals", `{"title":"Add a greeting","body":"A first contribution for new visitors."}`, newcomer.Credential.Token, http.StatusCreated)
	var proposal proposals.Proposal
	decodeResponse(t, proposalResponse, &proposal)
	proposalComments := server.URL + "/repositories/" + repository.ID + "/proposals/" + proposal.ID + "/comments"
	authenticatedRequest(t, http.MethodPost, proposalComments, `{"body":"@journey-newcomer please include a friendly example. @journey-outsider may be interested."}`, maintainer.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, proposalComments, `{"body":"I will send that on a candidate branch."}`, newcomer.Credential.Token, http.StatusCreated).Body.Close()
	outsiderActivity := authenticatedRequest(t, http.MethodGet, server.URL+"/activity?limit=100", "", outsider.Credential.Token, http.StatusOK)
	var outsiderFeed struct {
		Events []activities.Event `json:"events"`
	}
	decodeResponse(t, outsiderActivity, &outsiderFeed)
	if len(outsiderFeed.Events) != 0 {
		t.Fatalf("private mention leaked activity to non-participant: %#v", outsiderFeed.Events)
	}

	newcomerCopy := filepath.Join(t.TempDir(), "newcomer")
	gitCommand(t, "", "clone", remoteWithToken(newcomerGit.Token), newcomerCopy)
	gitCommand(t, newcomerCopy, "config", "user.name", "Newcomer")
	gitCommand(t, newcomerCopy, "config", "user.email", "newcomer@example.com")
	gitCommand(t, newcomerCopy, "switch", "-c", "greeting")
	if err := os.WriteFile(filepath.Join(newcomerCopy, "greeting.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, newcomerCopy, "add", "greeting.txt")
	gitCommand(t, newcomerCopy, "commit", "-m", "Add greeting")
	gitCommand(t, newcomerCopy, "push", "origin", "greeting")

	pullResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", `{"title":"Add a greeting","body":"Implements the agreed welcome.","source_branch":"greeting","target_branch":"main","proposal_id":"`+proposal.ID+`"}`, newcomer.Credential.Token, http.StatusCreated)
	var pull pullrequests.PullRequest
	decodeResponse(t, pullResponse, &pull)
	reviewInboxResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/inbox?category=review&limit=100", "", maintainer.Credential.Token, http.StatusOK)
	var reviewInbox struct {
		Items []inboxItem `json:"items"`
	}
	decodeResponse(t, reviewInboxResponse, &reviewInbox)
	if len(reviewInbox.Items) != 1 || reviewInbox.Items[0].ResourceID != pull.ID || reviewInbox.Items[0].Action != "Review pull request" {
		t.Fatalf("maintainer review inbox = %#v", reviewInbox.Items)
	}
	pullURL := server.URL + "/repositories/" + repository.ID + "/pulls/" + pull.ID
	authenticatedRequest(t, http.MethodPost, pullURL+"/comments", `{"body":"Could this greet agents too?"}`, maintainer.Credential.Token, http.StatusCreated).Body.Close()
	requested := authenticatedRequest(t, http.MethodPost, pullURL+"/reviews", `{"decision":"changes_requested"}`, maintainer.Credential.Token, http.StatusOK)
	var firstReview pullrequests.Review
	decodeResponse(t, requested, &firstReview)

	if err := os.WriteFile(filepath.Join(newcomerCopy, "greeting.txt"), []byte("hello developers and agents\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, newcomerCopy, "add", "greeting.txt")
	gitCommand(t, newcomerCopy, "commit", "-m", "Address review feedback")
	gitCommand(t, newcomerCopy, "push", "origin", "greeting")
	updatedTip := strings.TrimSpace(gitCommand(t, newcomerCopy, "rev-parse", "HEAD"))
	authenticatedRequest(t, http.MethodPost, pullURL+"/reviews", `{"decision":"approved"}`, maintainer.Credential.Token, http.StatusConflict).Body.Close()
	authenticatedRequest(t, http.MethodPost, pullURL+"/synchronize", "", maintainer.Credential.Token, http.StatusNotFound).Body.Close()
	synchronized := authenticatedRequest(t, http.MethodPost, pullURL+"/synchronize", "", newcomer.Credential.Token, http.StatusOK)
	var revised pullrequests.PullRequest
	decodeResponse(t, synchronized, &revised)
	if revised.SourceCommitID != updatedTip || revised.SourceCommitID == pull.SourceCommitID {
		t.Fatalf("synchronized pull request = %#v", revised)
	}
	reviewsResponse := authenticatedRequest(t, http.MethodGet, pullURL+"/reviews", "", newcomer.Credential.Token, http.StatusOK)
	var reviews struct {
		Reviews []pullrequests.Review `json:"reviews"`
	}
	decodeResponse(t, reviewsResponse, &reviews)
	if len(reviews.Reviews) != 1 || reviews.Reviews[0].ID != firstReview.ID || !reviews.Reviews[0].Stale {
		t.Fatalf("reviews after revision = %#v", reviews.Reviews)
	}
	authenticatedRequest(t, http.MethodPost, pullURL+"/comments", `{"body":"Updated the greeting as requested."}`, newcomer.Credential.Token, http.StatusCreated).Body.Close()
	approved := authenticatedRequest(t, http.MethodPost, pullURL+"/reviews", `{"decision":"approved"}`, maintainer.Credential.Token, http.StatusOK)
	var finalReview pullrequests.Review
	decodeResponse(t, approved, &finalReview)
	if finalReview.ID != firstReview.ID || finalReview.ReviewedCommitID != updatedTip || finalReview.Stale {
		t.Fatalf("fresh approval = %#v", finalReview)
	}

	readinessResponse := authenticatedRequest(t, http.MethodGet, pullURL+"/merge-readiness", "", maintainer.Credential.Token, http.StatusOK)
	var readiness pullrequests.MergeReadiness
	decodeResponse(t, readinessResponse, &readiness)
	if !readiness.Mergeable || !readiness.CanMerge || len(readiness.Blockers) != 0 {
		t.Fatalf("merge readiness = %#v", readiness)
	}
	mergedResponse := authenticatedRequest(t, http.MethodPost, pullURL+"/merge", "", maintainer.Credential.Token, http.StatusOK)
	var merged pullrequests.PullRequest
	decodeResponse(t, mergedResponse, &merged)
	if merged.Status != pullrequests.Merged || merged.MergeCommitID == nil || merged.MergedBy == nil || *merged.MergedBy != maintainer.User.ID {
		t.Fatalf("merged pull request = %#v", merged)
	}
	gitCommand(t, maintainerCopy, "pull", "--ff-only")
	assertFile(t, filepath.Join(maintainerCopy, "greeting.txt"), "hello developers and agents\n", false)
	proposalInspection := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/proposals/"+proposal.ID, "", newcomer.Credential.Token, http.StatusOK)
	var closedProposal proposals.Proposal
	decodeResponse(t, proposalInspection, &closedProposal)
	if closedProposal.Status != proposals.Closed {
		t.Fatalf("linked proposal status = %q", closedProposal.Status)
	}
	activityResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/activity?limit=100", "", newcomer.Credential.Token, http.StatusOK)
	var feed struct {
		Events []activities.Event `json:"events"`
	}
	decodeResponse(t, activityResponse, &feed)
	wanted := map[string]bool{"access.granted": false, "proposal.created": false, "mention.created": false, "pull_request.created": false, "review.changes_requested": false, "review.approved": false, "pull_request.merged": false}
	for _, event := range feed.Events {
		if _, ok := wanted[event.Kind]; ok {
			if event.Kind != "mention.created" || (event.TargetUserID != nil && *event.TargetUserID == newcomer.User.ID) {
				wanted[event.Kind] = true
			}
		}
		if event.RepositoryID != repository.ID || event.RepositoryName != repository.Name || event.ActorID == "" || event.ResourceID == "" || event.ResourceTitle == "" {
			t.Fatalf("incomplete activity event: %#v", event)
		}
		if event.Kind == "mention.created" && event.TargetUserID != nil && *event.TargetUserID == newcomer.User.ID && event.ResourceID != proposal.ID {
			t.Fatalf("mention event = %#v", event)
		}
	}
	for kind, found := range wanted {
		if !found {
			t.Errorf("activity did not include %s: %#v", kind, feed.Events)
		}
	}
	inboxResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/inbox?limit=100", "", newcomer.Credential.Token, http.StatusOK)
	var inbox struct {
		Items []inboxItem `json:"items"`
	}
	decodeResponse(t, inboxResponse, &inbox)
	categories := map[string]bool{"response": false, "awareness": false}
	for _, item := range inbox.Items {
		categories[item.Category] = true
		if item.Action == "" || item.ActorID == newcomer.User.ID {
			t.Fatalf("non-actionable inbox item: %#v", item)
		}
	}
	if !categories["response"] || !categories["awareness"] {
		t.Fatalf("inbox did not classify response and awareness work: %#v", inbox.Items)
	}
	clearedID := inbox.Items[0].ID
	authenticatedRequest(t, http.MethodDelete, server.URL+"/inbox/"+clearedID, "", newcomer.Credential.Token, http.StatusNoContent).Body.Close()
	inboxResponse = authenticatedRequest(t, http.MethodGet, server.URL+"/inbox?limit=100", "", newcomer.Credential.Token, http.StatusOK)
	inbox.Items = nil
	decodeResponse(t, inboxResponse, &inbox)
	for _, item := range inbox.Items {
		if item.ID == clearedID {
			t.Fatalf("cleared item remained in inbox: %#v", item)
		}
	}
	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+repository.ID+"/collaborators/"+newcomer.User.ID, "", maintainer.Credential.Token, http.StatusNoContent).Body.Close()
	revokedResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/activity?limit=100", "", newcomer.Credential.Token, http.StatusOK)
	feed.Events = nil
	decodeResponse(t, revokedResponse, &feed)
	if len(feed.Events) != 0 {
		t.Fatalf("revoked collaborator retained private activity = %#v", feed.Events)
	}
}

func issueGitCredential(t *testing.T, baseURL, sessionToken, name string) auth.IssuedCredential {
	t.Helper()
	response := authenticatedRequest(t, http.MethodPost, baseURL+"/auth/credentials", `{"kind":"git","name":"`+name+`","scopes":["git:read","git:write"],"expires_in":3600}`, sessionToken, http.StatusCreated)
	var credential auth.IssuedCredential
	decodeResponse(t, response, &credential)
	return credential
}

func decodeResponse(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
