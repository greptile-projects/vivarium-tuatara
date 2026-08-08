package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
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
	checkRunStore, _ := checkruns.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, proposalStore, pullRequestStore, nil, nil, checkRunStore))
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
	checkConfig, _ := gitRepository.WriteObject(storage.BlobObject, []byte(`{"version":1,"checks":[{"name":"candidate snapshot","image":"alpine:3.22","command":"echo inspecting candidate; test \"$(cat README.md)\" = new; printf evidence > \"$VIVARIUM_OUTPUT/report.txt\""}]}`))
	checkTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "checks.json", id: checkConfig})
	baseTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "README.md", id: oldReadme})
	headTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "40000", name: ".vivarium", id: checkTree}, testTreeEntry{mode: "100644", name: "README.md", id: newReadme}, testTreeEntry{mode: "100644", name: "feature.go", id: added})
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
	checksURL := server.URL + "/repositories/" + repository.ID + "/pulls/" + pullRequest.ID + "/checks"
	var checkSet struct {
		CheckRuns []checkruns.Run `json:"check_runs"`
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		checksResponse := authenticatedRequest(t, http.MethodGet, checksURL, "", owner.Credential.Token, http.StatusOK)
		checkSet.CheckRuns = nil
		if err := json.NewDecoder(checksResponse.Body).Decode(&checkSet); err != nil {
			t.Fatal(err)
		}
		checksResponse.Body.Close()
		if len(checkSet.CheckRuns) == 1 && checkSet.CheckRuns[0].CompletedAt != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(checkSet.CheckRuns) != 1 || checkSet.CheckRuns[0].CommitID != string(head) || checkSet.CheckRuns[0].State != "succeeded" {
		t.Fatalf("check runs = %#v", checkSet.CheckRuns)
	}
	run := checkSet.CheckRuns[0]
	if len(run.Attempts) != 1 || run.Attempts[0].State != "succeeded" || len(run.Artifacts) != 1 || run.Artifacts[0].Path != "report.txt" {
		t.Fatalf("check evidence summary = %#v", run)
	}
	detailResponse := authenticatedRequest(t, http.MethodGet, checksURL+"/"+run.ID, "", owner.Credential.Token, http.StatusOK)
	detailResponse.Body.Close()
	eventsResponse := authenticatedRequest(t, http.MethodGet, checksURL+"/"+run.ID+"/events?after=0", "", owner.Credential.Token, http.StatusOK)
	var evidence struct {
		Events []checkruns.Event `json:"events"`
		Next   int64             `json:"next_sequence"`
	}
	if err := json.NewDecoder(eventsResponse.Body).Decode(&evidence); err != nil {
		t.Fatal(err)
	}
	eventsResponse.Body.Close()
	if len(evidence.Events) < 6 || evidence.Events[0].State != "queued" || evidence.Events[1].State != "running" || evidence.Events[len(evidence.Events)-1].State != "succeeded" || evidence.Next != evidence.Events[len(evidence.Events)-1].Sequence {
		t.Fatalf("check events = %#v", evidence)
	}
	resumedResponse := authenticatedRequest(t, http.MethodGet, checksURL+"/"+run.ID+"/events?after="+strconv.FormatInt(evidence.Next, 10), "", owner.Credential.Token, http.StatusOK)
	var resumed struct {
		Events []checkruns.Event `json:"events"`
	}
	if err := json.NewDecoder(resumedResponse.Body).Decode(&resumed); err != nil {
		t.Fatal(err)
	}
	resumedResponse.Body.Close()
	if len(resumed.Events) != 0 {
		t.Fatalf("resumed events = %#v", resumed.Events)
	}
	artifactResponse := authenticatedRequest(t, http.MethodGet, checksURL+"/"+run.ID+"/artifacts/"+run.Artifacts[0].ID, "", owner.Credential.Token, http.StatusOK)
	artifactBody, _ := io.ReadAll(artifactResponse.Body)
	artifactResponse.Body.Close()
	if string(artifactBody) != "evidence" {
		t.Fatalf("artifact = %q", artifactBody)
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
	if len(fileSet.Files) != 3 || fileSet.Files[0].Path != ".vivarium/checks.json" || fileSet.Files[1].Path != "README.md" || fileSet.Files[1].Status != "modified" || fileSet.Files[2].Path != "feature.go" || fileSet.Files[2].Status != "added" {
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

func TestForkOwnerOpensAndSynchronizesUpstreamPullRequest(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	pullStore, _ := pullrequests.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, nil, pullStore, nil, nil))
	defer server.Close()

	maintainer := createTestAccount(t, server.URL, "outside-maintainer")
	author := createTestAccount(t, server.URL, "outside-author")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"upstream-review"}`, maintainer.Credential.Token, http.StatusCreated)
	var upstream repositories.Repository
	if err := json.NewDecoder(created.Body).Decode(&upstream); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+upstream.ID, `{"visibility":"public"}`, maintainer.Credential.Token, http.StatusOK).Body.Close()
	upstreamGit, _ := gitStore.Open(upstream.ID)
	baseBlob, _ := upstreamGit.WriteObject(storage.BlobObject, []byte("base\n"))
	baseTree := writeTestTree(t, upstreamGit, testTreeEntry{mode: "100644", name: "README.md", id: baseBlob})
	base := writeTestCommit(t, upstreamGit, baseTree, nil, 1700000100, "base")
	if err := upstreamGit.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}

	forked := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/forks", `{"name":"outside-work"}`, author.Credential.Token, http.StatusCreated)
	var fork repositories.Repository
	if err := json.NewDecoder(forked.Body).Decode(&fork); err != nil {
		t.Fatal(err)
	}
	forked.Body.Close()
	forkGit, _ := gitStore.Open(fork.ID)
	featureBlob, _ := forkGit.WriteObject(storage.BlobObject, []byte("outside\n"))
	featureTree := writeTestTree(t, forkGit, testTreeEntry{mode: "100644", name: "README.md", id: baseBlob}, testTreeEntry{mode: "100644", name: "outside.txt", id: featureBlob})
	feature := writeTestCommit(t, forkGit, featureTree, []storage.ObjectID{base}, 1700000101, "outside feature")
	if err := forkGit.CreateReference(storage.Reference{Name: "refs/heads/contribution", Target: string(feature)}); err != nil {
		t.Fatal(err)
	}

	body := `{"title":"Outside contribution","body":"Review independently owned work.","source_repository_id":"` + fork.ID + `","source_branch":"contribution","target_branch":"main"}`
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/pulls", body, author.Credential.Token, http.StatusCreated)
	var pull pullrequests.PullRequest
	if err := json.NewDecoder(response.Body).Decode(&pull); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if pull.RepositoryID != upstream.ID || pull.SourceRepositoryID != fork.ID || pull.SourceCommitID != string(feature) || pull.TargetCommitID != string(base) {
		t.Fatalf("cross-repository pull = %#v", pull)
	}
	credentialURL := server.URL + "/repositories/" + upstream.ID + "/pulls/" + pull.ID + "/maintainer-credential"
	authenticatedRequest(t, http.MethodPost, credentialURL, "", maintainer.Credential.Token, http.StatusConflict).Body.Close()
	policyURL := server.URL + "/repositories/" + upstream.ID + "/pulls/" + pull.ID
	authenticatedRequest(t, http.MethodPatch, policyURL, `{"maintainer_edits_allowed":true}`, maintainer.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodPatch, policyURL, `{"maintainer_edits_allowed":true}`, author.Credential.Token, http.StatusOK).Body.Close()
	credentialResponse := authenticatedRequest(t, http.MethodPost, credentialURL, "", maintainer.Credential.Token, http.StatusCreated)
	var branchCredential auth.IssuedCredential
	decodeResponse(t, credentialResponse, &branchCredential)
	if branchCredential.PullRequestID != pull.ID {
		t.Fatalf("credential pull binding = %q, want %q", branchCredential.PullRequestID, pull.ID)
	}
	assertGitDiscoveryStatus(t, server.URL+fork.GitRemote+"/info/refs?service=git-receive-pack", branchCredential.Token, http.StatusOK)
	secondResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/pulls", body, author.Credential.Token, http.StatusCreated)
	var secondPull pullrequests.PullRequest
	decodeResponse(t, secondResponse, &secondPull)
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+upstream.ID+"/pulls/"+secondPull.ID, `{"maintainer_edits_allowed":true}`, author.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPatch, policyURL, `{"maintainer_edits_allowed":false}`, author.Credential.Token, http.StatusOK).Body.Close()
	assertGitDiscoveryStatus(t, server.URL+fork.GitRemote+"/info/refs?service=git-receive-pack", branchCredential.Token, http.StatusNotFound)
	commits := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+upstream.ID+"/pulls/"+pull.ID+"/commits", "", maintainer.Credential.Token, http.StatusOK)
	var commitPage struct {
		Commits []pullrequests.Commit `json:"commits"`
	}
	if err := json.NewDecoder(commits.Body).Decode(&commitPage); err != nil {
		t.Fatal(err)
	}
	commits.Body.Close()
	if len(commitPage.Commits) != 1 || commitPage.Commits[0].ID != string(feature) {
		t.Fatalf("commits = %#v", commitPage.Commits)
	}
	files := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+upstream.ID+"/pulls/"+pull.ID+"/files", "", maintainer.Credential.Token, http.StatusOK)
	var filePage struct {
		Files []pullrequests.FileChange `json:"files"`
	}
	if err := json.NewDecoder(files.Body).Decode(&filePage); err != nil {
		t.Fatal(err)
	}
	files.Body.Close()
	if len(filePage.Files) != 1 || filePage.Files[0].Path != "outside.txt" {
		t.Fatalf("files = %#v", filePage.Files)
	}

	revised := writeTestCommit(t, forkGit, featureTree, []storage.ObjectID{feature}, 1700000102, "follow-up")
	if err := forkGit.UpdateReference(storage.Reference{Name: "refs/heads/contribution", Target: string(revised)}); err != nil {
		t.Fatal(err)
	}
	synchronized := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/pulls/"+pull.ID+"/synchronize", "", author.Credential.Token, http.StatusOK)
	if err := json.NewDecoder(synchronized.Body).Decode(&pull); err != nil {
		t.Fatal(err)
	}
	synchronized.Body.Close()
	if pull.SourceCommitID != string(revised) {
		t.Fatalf("synchronized source = %s", pull.SourceCommitID)
	}
	if _, err := upstreamGit.ReadCommit(revised); err != nil {
		t.Fatalf("adopted source was not imported: %v", err)
	}

	// A private upstream revocation takes effect before the next adoption; fork
	// ownership alone is not authority to import more objects into the target.
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/collaborators", `{"user_id":"`+author.User.ID+`"}`, maintainer.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+upstream.ID, `{"visibility":"private"}`, maintainer.Credential.Token, http.StatusOK).Body.Close()
	denied := writeTestCommit(t, forkGit, featureTree, []storage.ObjectID{revised}, 1700000103, "revoked follow-up")
	if err := forkGit.UpdateReference(storage.Reference{Name: "refs/heads/contribution", Target: string(denied)}); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+upstream.ID+"/collaborators/"+author.User.ID, "", maintainer.Credential.Token, http.StatusNoContent).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/pulls/"+pull.ID+"/synchronize", "", author.Credential.Token, http.StatusNotFound).Body.Close()
	closedResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/pulls/"+pull.ID+"/close", "", maintainer.Credential.Token, http.StatusOK)
	var closed pullrequests.PullRequest
	decodeResponse(t, closedResponse, &closed)
	if closed.Status != pullrequests.Closed || closed.ClosedAt == nil || closed.ClosedBy == nil || *closed.ClosedBy != maintainer.User.ID || closed.MaintainerEditsAllowed {
		t.Fatalf("closed pull = %#v", closed)
	}
	if _, err := upstreamGit.ReadCommit(denied); err == nil {
		t.Fatal("revoked source commit was imported into private upstream")
	}
}

func TestOutsideContributionRetainsGovernanceAndProvenanceAfterForkDeletion(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	pullStore, _ := pullrequests.New(t.TempDir(), gitStore)
	checkRunStore, _ := checkruns.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, pullStore, nil, nil, checkRunStore))
	defer server.Close()

	maintainer := createTestAccount(t, server.URL, "governance-maintainer")
	author := createTestAccount(t, server.URL, "governance-author")
	observer := createTestAccount(t, server.URL, "governance-observer")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"governed-upstream"}`, maintainer.Credential.Token, http.StatusCreated)
	var upstream repositories.Repository
	decodeResponse(t, created, &upstream)
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+upstream.ID, `{"visibility":"public"}`, maintainer.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+upstream.ID+"/branches/main/required-checks", `{"checks":["outside-quality"]}`, maintainer.Credential.Token, http.StatusOK).Body.Close()

	upstreamGit, _ := gitStore.Open(upstream.ID)
	baseTree := writeTestTree(t, upstreamGit)
	base := writeTestCommit(t, upstreamGit, baseTree, nil, 1700000200, "base")
	if err := upstreamGit.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	forked := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/forks", `{"name":"governed-work"}`, author.Credential.Token, http.StatusCreated)
	var fork repositories.Repository
	decodeResponse(t, forked, &fork)
	forkGit, _ := gitStore.Open(fork.ID)
	featureBlob, _ := forkGit.WriteObject(storage.BlobObject, []byte("outside\n"))
	featureTree := writeTestTree(t, forkGit, testTreeEntry{mode: "100644", name: "outside.txt", id: featureBlob})
	feature := writeTestCommit(t, forkGit, featureTree, []storage.ObjectID{base}, 1700000201, "outside feature")
	if err := forkGit.CreateReference(storage.Reference{Name: "refs/heads/contribution", Target: string(feature)}); err != nil {
		t.Fatal(err)
	}

	pullResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/pulls", `{"title":"Governed outside work","body":"Keep its origin.","source_repository_id":"`+fork.ID+`","source_branch":"contribution","target_branch":"main"}`, author.Credential.Token, http.StatusCreated)
	var pull pullrequests.PullRequest
	decodeResponse(t, pullResponse, &pull)
	commentsURL := server.URL + "/repositories/" + upstream.ID + "/pulls/" + pull.ID + "/comments"
	authenticatedRequest(t, http.MethodPost, commentsURL, `{"body":"Public visibility does not make me a participant."}`, observer.Credential.Token, http.StatusNotFound).Body.Close()
	authorComment := authenticatedRequest(t, http.MethodPost, commentsURL, `{"body":"I authored this outside revision."}`, author.Credential.Token, http.StatusCreated)
	var comment pullrequests.Comment
	decodeResponse(t, authorComment, &comment)
	if comment.AuthorID != author.User.ID {
		t.Fatalf("comment attribution = %#v", comment)
	}

	runs, err := checkRunStore.Create(upstream.ID, pull.ID, string(feature), []checkruns.Definition{{Name: "outside-quality", Image: "alpine:3.22", Command: "true"}})
	if err != nil || len(runs) != 1 {
		t.Fatalf("create check run: %#v, %v", runs, err)
	}
	runs[0].State = "succeeded"
	if err := checkRunStore.Update(runs[0]); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+fork.ID, "", author.Credential.Token, http.StatusNoContent).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/pulls/"+pull.ID+"/reviews", `{"decision":"approved"}`, maintainer.Credential.Token, http.StatusOK).Body.Close()

	readinessResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+upstream.ID+"/pulls/"+pull.ID+"/merge-readiness", "", maintainer.Credential.Token, http.StatusOK)
	var readiness pullrequests.MergeReadiness
	decodeResponse(t, readinessResponse, &readiness)
	if !readiness.CanMerge || readiness.Source.State != "unavailable" || len(readiness.RequiredChecks) != 1 || readiness.RequiredChecks[0].Status != "passed" {
		t.Fatalf("outside readiness = %#v", readiness)
	}
	mergedResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/pulls/"+pull.ID+"/merge", "", maintainer.Credential.Token, http.StatusOK)
	var merged pullrequests.PullRequest
	decodeResponse(t, mergedResponse, &merged)
	if merged.Status != pullrequests.Merged || merged.AuthorID != author.User.ID || merged.SourceRepositoryID != fork.ID || merged.SourceCommitID != string(feature) || merged.MergeCommitID == nil {
		t.Fatalf("retained pull provenance = %#v", merged)
	}
	mergeCommit, err := upstreamGit.ReadCommit(storage.ObjectID(*merged.MergeCommitID))
	if err != nil {
		t.Fatal(err)
	}
	message := string(mergeCommit.Message)
	for _, trailer := range []string{"Source-Repository: " + fork.ID, "Source-Branch: contribution", "Source-Commit: " + string(feature), "Authored-by: " + author.User.ID, "Merged-by: " + maintainer.User.ID} {
		if !strings.Contains(message, trailer) {
			t.Fatalf("merge message %q missing %q", message, trailer)
		}
	}
	comments := authenticatedRequest(t, http.MethodGet, commentsURL, "", maintainer.Credential.Token, http.StatusOK)
	var discussion struct {
		Comments []pullrequests.Comment `json:"comments"`
	}
	decodeResponse(t, comments, &discussion)
	if len(discussion.Comments) != 1 || discussion.Comments[0].AuthorID != author.User.ID {
		t.Fatalf("retained discussion = %#v", discussion.Comments)
	}
}

func TestPullRequestMergeReadinessReportsRequirementsConflictsAndPermission(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	pullRequestStore, _ := pullrequests.New(t.TempDir(), gitStore)
	checkRunStore, _ := checkruns.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, pullRequestStore, nil, nil, checkRunStore))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "ready-owner")
	contributor := createTestAccount(t, server.URL, "ready-contributor")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"readiness"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+repository.ID+"/branches/main/required-checks", `{"checks":["quality"]}`, owner.Credential.Token, http.StatusOK).Body.Close()
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
	if report.Mergeable || report.CanMerge || report.RequiredApprovals != 1 || report.Approvals != 0 || len(report.Blockers) != 2 || report.Blockers[0].Code != "approval_required" || report.Blockers[1].Code != "required_check_missing" || report.EvaluatedCommitID != string(source) || len(report.RequiredChecks) != 1 || report.RequiredChecks[0].Status != "missing" || report.Source.State != "current" || report.Target.State != "current" || report.HasConflicts {
		t.Fatalf("initial readiness = %#v", report)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls/"+pullRequest.ID+"/reviews", `{"decision":"approved"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	runs, err := checkRunStore.Create(repository.ID, pullRequest.ID, string(source), []checkruns.Definition{{Name: "quality", Image: "alpine:3.22", Command: "true"}})
	if err != nil || len(runs) != 1 {
		t.Fatalf("create required run: %#v, %v", runs, err)
	}
	runs[0].State = "succeeded"
	if err := checkRunStore.Update(runs[0]); err != nil {
		t.Fatal(err)
	}
	contributorReport := readReport(contributor.Credential.Token)
	ownerReport := readReport(owner.Credential.Token)
	if !contributorReport.Mergeable || contributorReport.CanMerge || !ownerReport.Mergeable || !ownerReport.CanMerge || len(ownerReport.Blockers) != 0 || ownerReport.RequiredChecks[0].Status != "passed" || ownerReport.RequiredChecks[0].CommitID == nil || *ownerReport.RequiredChecks[0].CommitID != string(source) {
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
	server := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, proposalStore, pullStore, nil))
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
