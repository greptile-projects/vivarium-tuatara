package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestContributorOpensPullRequestWithExactBranchState(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	proposalRoot := t.TempDir()
	proposalStore, _ := proposals.New(proposalRoot)
	pullRequestRoot := t.TempDir()
	pullRequestStore, _ := pullrequests.New(pullRequestRoot, gitStore)
	server := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, proposalStore, pullRequestStore))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "pull-owner")
	contributor := createTestAccount(t, server.URL, "pull-contributor")
	outsider := createTestAccount(t, server.URL, "pull-outsider")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"reviewable"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(response.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+contributor.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()

	gitRepository, err := gitStore.Open(repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldReadme, _ := gitRepository.WriteObject(storage.BlobObject, []byte("old\n"))
	newReadme, _ := gitRepository.WriteObject(storage.BlobObject, []byte("new\n"))
	added, _ := gitRepository.WriteObject(storage.BlobObject, []byte("added\n"))
	baseTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "README.md", id: oldReadme})
	headTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "README.md", id: newReadme}, testTreeEntry{mode: "100644", name: "feature.go", id: added})
	base := writeTestCommit(t, gitRepository, baseTree, nil, 1700000000, "base")
	head := writeTestCommit(t, gitRepository, headTree, []storage.ObjectID{base}, 1700000001, "candidate")
	if err := gitRepository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	if err := gitRepository.CreateReference(storage.Reference{Name: "refs/heads/feature", Target: string(head)}); err != nil {
		t.Fatal(err)
	}

	proposalResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/proposals", `{"title":"Plan feature","body":"Agree on the behavior."}`, contributor.Credential.Token, http.StatusCreated)
	var proposal proposals.Proposal
	if err := json.NewDecoder(proposalResponse.Body).Decode(&proposal); err != nil {
		t.Fatal(err)
	}
	proposalResponse.Body.Close()
	body := `{"title":"Add the feature","body":"Implements the agreed behavior.","source_branch":"feature","target_branch":"main","proposal_id":"` + proposal.ID + `"}`
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", body, contributor.Credential.Token, http.StatusCreated)
	var pullRequest pullrequests.PullRequest
	if err := json.NewDecoder(created.Body).Decode(&pullRequest); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	if pullRequest.AuthorID != contributor.User.ID || pullRequest.SourceCommitID != string(head) || pullRequest.TargetCommitID != string(base) || pullRequest.Status != pullrequests.Open || pullRequest.ProposalID == nil || *pullRequest.ProposalID != proposal.ID {
		t.Fatalf("pull request = %#v", pullRequest)
	}
	if got := created.Header.Get("Location"); got != "/repositories/"+repository.ID+"/pulls/"+pullRequest.ID {
		t.Fatalf("Location = %q", got)
	}

	advanced := writeTestCommit(t, gitRepository, headTree, []storage.ObjectID{head}, 1700000002, "more work")
	if err := gitRepository.UpdateReference(storage.Reference{Name: "refs/heads/feature", Target: string(advanced)}); err != nil {
		t.Fatal(err)
	}
	inspected := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/pulls/"+pullRequest.ID, "", owner.Credential.Token, http.StatusOK)
	var persisted pullrequests.PullRequest
	if err := json.NewDecoder(inspected.Body).Decode(&persisted); err != nil {
		t.Fatal(err)
	}
	inspected.Body.Close()
	if persisted.SourceCommitID != string(head) {
		t.Fatalf("source commit moved to %s, want creation state %s", persisted.SourceCommitID, head)
	}
	commitsResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/pulls/"+pullRequest.ID+"/commits", "", owner.Credential.Token, http.StatusOK)
	var commitSet struct {
		Commits []pullrequests.Commit `json:"commits"`
	}
	if err := json.NewDecoder(commitsResponse.Body).Decode(&commitSet); err != nil {
		t.Fatal(err)
	}
	commitsResponse.Body.Close()
	if len(commitSet.Commits) != 1 || commitSet.Commits[0].ID != string(head) || commitSet.Commits[0].Message != "candidate\n" {
		t.Fatalf("commits = %#v", commitSet.Commits)
	}
	filesResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/pulls/"+pullRequest.ID+"/files", "", contributor.Credential.Token, http.StatusOK)
	var fileSet struct {
		Files []pullrequests.FileChange `json:"files"`
	}
	if err := json.NewDecoder(filesResponse.Body).Decode(&fileSet); err != nil {
		t.Fatal(err)
	}
	filesResponse.Body.Close()
	if len(fileSet.Files) != 2 || fileSet.Files[0].Path != "README.md" || fileSet.Files[0].Status != "modified" || fileSet.Files[1].Path != "feature.go" || fileSet.Files[1].Status != "added" {
		t.Fatalf("files = %#v", fileSet.Files)
	}
	commentsURL := server.URL + "/repositories/" + repository.ID + "/pulls/" + pullRequest.ID + "/comments"
	commentResponse := authenticatedRequest(t, http.MethodPost, commentsURL, `{"body":"Please cover the edge case."}`, owner.Credential.Token, http.StatusCreated)
	var comment pullrequests.Comment
	if err := json.NewDecoder(commentResponse.Body).Decode(&comment); err != nil {
		t.Fatal(err)
	}
	commentResponse.Body.Close()
	if comment.AuthorID != owner.User.ID || comment.PullRequestID != pullRequest.ID {
		t.Fatalf("comment = %#v", comment)
	}
	conversationResponse := authenticatedRequest(t, http.MethodGet, commentsURL, "", contributor.Credential.Token, http.StatusOK)
	var conversation struct {
		Comments []pullrequests.Comment `json:"comments"`
	}
	if err := json.NewDecoder(conversationResponse.Body).Decode(&conversation); err != nil {
		t.Fatal(err)
	}
	conversationResponse.Body.Close()
	if len(conversation.Comments) != 1 || conversation.Comments[0].ID != comment.ID {
		t.Fatalf("comments = %#v", conversation.Comments)
	}
	authenticatedRequest(t, http.MethodPost, commentsURL, `{"body":"uninvited"}`, outsider.Credential.Token, http.StatusNotFound).Body.Close()

	reviewsURL := server.URL + "/repositories/" + repository.ID + "/pulls/" + pullRequest.ID + "/reviews"
	authenticatedRequest(t, http.MethodPost, reviewsURL, `{"decision":"approved"}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	synchronizeURL := server.URL + "/repositories/" + repository.ID + "/pulls/" + pullRequest.ID + "/synchronize"
	authenticatedRequest(t, http.MethodPost, synchronizeURL, "", contributor.Credential.Token, http.StatusOK).Body.Close()
	reviewResponse := authenticatedRequest(t, http.MethodPost, reviewsURL, `{"decision":"approved"}`, owner.Credential.Token, http.StatusOK)
	var review pullrequests.Review
	if err := json.NewDecoder(reviewResponse.Body).Decode(&review); err != nil {
		t.Fatal(err)
	}
	reviewResponse.Body.Close()
	if review.ReviewerID != owner.User.ID || review.Decision != pullrequests.Approved || review.ReviewedCommitID != string(advanced) || review.Stale {
		t.Fatalf("review = %#v", review)
	}
	latest := writeTestCommit(t, gitRepository, headTree, []storage.ObjectID{advanced}, 1700000003, "review response")
	if err := gitRepository.UpdateReference(storage.Reference{Name: "refs/heads/feature", Target: string(latest)}); err != nil {
		t.Fatal(err)
	}
	listedResponse := authenticatedRequest(t, http.MethodGet, reviewsURL, "", contributor.Credential.Token, http.StatusOK)
	var reviewSet struct {
		Reviews []pullrequests.Review `json:"reviews"`
	}
	if err := json.NewDecoder(listedResponse.Body).Decode(&reviewSet); err != nil {
		t.Fatal(err)
	}
	listedResponse.Body.Close()
	if len(reviewSet.Reviews) != 1 || !reviewSet.Reviews[0].Stale || reviewSet.Reviews[0].ReviewedCommitID != string(advanced) {
		t.Fatalf("stale reviews = %#v", reviewSet.Reviews)
	}
	authenticatedRequest(t, http.MethodPost, reviewsURL, `{"decision":"changes_requested"}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	authenticatedRequest(t, http.MethodPost, synchronizeURL, "", contributor.Credential.Token, http.StatusOK).Body.Close()
	replacementResponse := authenticatedRequest(t, http.MethodPost, reviewsURL, `{"decision":"changes_requested"}`, owner.Credential.Token, http.StatusOK)
	var replacement pullrequests.Review
	if err := json.NewDecoder(replacementResponse.Body).Decode(&replacement); err != nil {
		t.Fatal(err)
	}
	replacementResponse.Body.Close()
	if replacement.ID != review.ID || replacement.Decision != pullrequests.ChangesRequested || replacement.ReviewedCommitID != string(latest) {
		t.Fatalf("replacement = %#v", replacement)
	}
	authenticatedRequest(t, http.MethodDelete, reviewsURL+"/"+review.ID, "", contributor.Credential.Token, http.StatusNotFound).Body.Close()
	withdrawResponse := authenticatedRequest(t, http.MethodDelete, reviewsURL+"/"+review.ID, "", owner.Credential.Token, http.StatusOK)
	var withdrawn pullrequests.Review
	if err := json.NewDecoder(withdrawResponse.Body).Decode(&withdrawn); err != nil {
		t.Fatal(err)
	}
	withdrawResponse.Body.Close()
	if withdrawn.Decision != pullrequests.Withdrawn || withdrawn.ReviewedCommitID != string(latest) {
		t.Fatalf("withdrawn review = %#v", withdrawn)
	}
	authenticatedRequest(t, http.MethodPost, reviewsURL, `{"decision":"maybe"}`, owner.Credential.Token, http.StatusBadRequest).Body.Close()
	authenticatedRequest(t, http.MethodPost, reviewsURL, `{"decision":"approved"}`, outsider.Credential.Token, http.StatusNotFound).Body.Close()

	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/pulls", "", contributor.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", body, outsider.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", `{"title":"Bad","body":"","source_branch":"missing","target_branch":"main"}`, contributor.Credential.Token, http.StatusBadRequest).Body.Close()

	objectPath := filepath.Join(gitRepository.Path(), "objects", string(latest)[:2], string(latest)[2:])
	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", `{"title":"Unavailable branch","body":"","source_branch":"feature","target_branch":"main"}`, contributor.Credential.Token, http.StatusInternalServerError).Body.Close()
	if err := os.WriteFile(filepath.Join(proposalRoot, proposal.ID+".json"), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", body, contributor.Credential.Token, http.StatusInternalServerError).Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/pulls/00000000000000000000000000000000", "", owner.Credential.Token, http.StatusNotFound).Body.Close()
	if err := os.WriteFile(filepath.Join(pullRequestRoot, repository.ID, pullRequest.ID+".json"), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/pulls/"+pullRequest.ID, "", owner.Credential.Token, http.StatusInternalServerError).Body.Close()
}

func TestPullRequestMergeReadinessReportsRequirementsConflictsAndPermission(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	pullRequestStore, _ := pullrequests.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, nil, pullRequestStore))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "ready-owner")
	contributor := createTestAccount(t, server.URL, "ready-contributor")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"readiness"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+contributor.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()

	gitRepository, _ := gitStore.Open(repository.ID)
	baseBlob, _ := gitRepository.WriteObject(storage.BlobObject, []byte("base\n"))
	sourceBlob, _ := gitRepository.WriteObject(storage.BlobObject, []byte("source\n"))
	baseTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "file.txt", id: baseBlob})
	sourceTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "file.txt", id: sourceBlob})
	base := writeTestCommit(t, gitRepository, baseTree, nil, 1700000000, "base")
	source := writeTestCommit(t, gitRepository, sourceTree, []storage.ObjectID{base}, 1700000001, "source")
	gitRepository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)})
	gitRepository.CreateReference(storage.Reference{Name: "refs/heads/feature", Target: string(source)})
	objectsBeforeReadiness, _ := gitRepository.ListObjects()

	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", `{"title":"Ready?","body":"Report every condition.","source_branch":"feature","target_branch":"main"}`, contributor.Credential.Token, http.StatusCreated)
	var pullRequest pullrequests.PullRequest
	json.NewDecoder(created.Body).Decode(&pullRequest)
	created.Body.Close()
	readinessURL := server.URL + "/repositories/" + repository.ID + "/pulls/" + pullRequest.ID + "/merge-readiness"

	readReport := func(token string) pullrequests.MergeReadiness {
		t.Helper()
		response := authenticatedRequest(t, http.MethodGet, readinessURL, "", token, http.StatusOK)
		var report pullrequests.MergeReadiness
		if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return report
	}
	report := readReport(contributor.Credential.Token)
	if report.Mergeable || report.CanMerge || report.RequiredApprovals != 1 || report.Approvals != 0 || len(report.Blockers) != 1 || report.Blockers[0].Code != "approval_required" || report.Source.State != "current" || report.Target.State != "current" || report.HasConflicts {
		t.Fatalf("initial readiness = %#v", report)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls/"+pullRequest.ID+"/reviews", `{"decision":"approved"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	contributorReport := readReport(contributor.Credential.Token)
	ownerReport := readReport(owner.Credential.Token)
	if !contributorReport.Mergeable || contributorReport.CanMerge || !ownerReport.Mergeable || !ownerReport.CanMerge || len(ownerReport.Blockers) != 0 {
		t.Fatalf("approved readiness: contributor=%#v owner=%#v", contributorReport, ownerReport)
	}

	targetBlob, _ := gitRepository.WriteObject(storage.BlobObject, []byte("target\n"))
	targetTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "file.txt", id: targetBlob})
	target := writeTestCommit(t, gitRepository, targetTree, []storage.ObjectID{base}, 1700000002, "target")
	gitRepository.UpdateReference(storage.Reference{Name: "refs/heads/main", Target: string(target)})
	report = readReport(owner.Credential.Token)
	if report.Mergeable || report.CanMerge || !report.HasConflicts || report.Target.State != "advanced" || len(report.Blockers) != 1 || report.Blockers[0].Code != "merge_conflict" {
		t.Fatalf("conflicting readiness = %#v", report)
	}
	objectsAfterReadiness, _ := gitRepository.ListObjects()
	if len(objectsAfterReadiness) != len(objectsBeforeReadiness)+3 { // target blob, tree, and commit only
		t.Fatalf("readiness wrote repository objects: before=%d after=%d", len(objectsBeforeReadiness), len(objectsAfterReadiness))
	}
}

func TestOwnerMergesApprovedPullRequestAndClosesLinkedProposal(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	proposalStore, _ := proposals.New(t.TempDir())
	pullStore, _ := pullrequests.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, proposalStore, pullStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "merge-owner")
	contributor := createTestAccount(t, server.URL, "merge-contributor")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"mergeable"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+contributor.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	gitRepository, _ := gitStore.Open(repository.ID)
	baseTree := writeTestTree(t, gitRepository)
	featureBlob, _ := gitRepository.WriteObject(storage.BlobObject, []byte("feature\n"))
	featureTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "feature.txt", id: featureBlob})
	base := writeTestCommit(t, gitRepository, baseTree, nil, 1700000000, "base")
	feature := writeTestCommit(t, gitRepository, featureTree, []storage.ObjectID{base}, 1700000001, "feature")
	gitRepository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)})
	gitRepository.CreateReference(storage.Reference{Name: "refs/heads/feature", Target: string(feature)})
	proposalResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/proposals", `{"title":"Ship feature","body":"Collaboratively agreed."}`, contributor.Credential.Token, http.StatusCreated)
	var proposal proposals.Proposal
	json.NewDecoder(proposalResponse.Body).Decode(&proposal)
	proposalResponse.Body.Close()
	pullResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", `{"title":"Ship feature","body":"Implements our discussion.","source_branch":"feature","target_branch":"main","proposal_id":"`+proposal.ID+`"}`, contributor.Credential.Token, http.StatusCreated)
	var pull pullrequests.PullRequest
	json.NewDecoder(pullResponse.Body).Decode(&pull)
	pullResponse.Body.Close()
	mergeURL := server.URL + "/repositories/" + repository.ID + "/pulls/" + pull.ID + "/merge"
	authenticatedRequest(t, http.MethodPost, mergeURL, "", contributor.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodPost, mergeURL, "", owner.Credential.Token, http.StatusConflict).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls/"+pull.ID+"/reviews", `{"decision":"approved"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	mergedResponse := authenticatedRequest(t, http.MethodPost, mergeURL, "", owner.Credential.Token, http.StatusOK)
	var merged pullrequests.PullRequest
	json.NewDecoder(mergedResponse.Body).Decode(&merged)
	mergedResponse.Body.Close()
	if merged.Status != pullrequests.Merged || merged.MergedBy == nil || *merged.MergedBy != owner.User.ID || merged.MergeCommitID == nil || merged.MergedAt == nil {
		t.Fatalf("merged = %#v", merged)
	}
	main, _ := gitRepository.ReadReference("refs/heads/main")
	if main.Target != *merged.MergeCommitID {
		t.Fatalf("main = %s, merge = %s", main.Target, *merged.MergeCommitID)
	}
	commit, err := gitRepository.ReadCommit(storage.ObjectID(main.Target))
	if err != nil || len(commit.Parents) != 2 || commit.Parents[0] != base || commit.Parents[1] != feature || !strings.Contains(string(commit.Message), "Pull-Request: "+pull.ID) || !strings.Contains(string(commit.Message), "Proposal: "+proposal.ID) || !strings.Contains(string(commit.Message), "Authored-by: "+contributor.User.ID) || !strings.Contains(string(commit.Message), "Merged-by: "+owner.User.ID) {
		t.Fatalf("merge commit = %#v, %v", commit, err)
	}
	closed, _ := proposalStore.Get(repository.ID, proposal.ID)
	if closed.Status != proposals.Closed {
		t.Fatalf("proposal = %#v", closed)
	}
	authenticatedRequest(t, http.MethodPost, mergeURL, "", owner.Credential.Token, http.StatusOK).Body.Close()
}
