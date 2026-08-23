package pullrequests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

// ConflictAnalysis is read-only evidence about two exact revisions. It is
// deliberately separate from a resolution workspace: understanding a collision
// does not grant authority to change either side.
type ConflictAnalysis struct {
	RepositoryID   string                    `json:"repository_id"`
	BaseCommitID   string                    `json:"base_commit_id"`
	Source         ConflictSide              `json:"source"`
	Target         ConflictSide              `json:"target"`
	CandidateID    string                    `json:"candidate_id,omitempty"`
	Status         string                    `json:"status"`
	StaleReasons   []string                  `json:"stale_reasons"`
	Incomplete     []string                  `json:"incomplete"`
	Files          []ConflictFile            `json:"files"`
	Semantic       []SemanticIncompatibility `json:"semantic_incompatibilities"`
	AffectedChecks []CheckRequirement        `json:"affected_checks"`
}

type ConflictSide struct {
	Branch          string                 `json:"branch"`
	CommitID        string                 `json:"commit_id"`
	CurrentCommitID string                 `json:"current_commit_id,omitempty"`
	PullRequests    []ConflictPullEvidence `json:"pull_requests"`
	OwnerIDs        []string               `json:"owner_ids"`
}

type ConflictPullEvidence struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	AuthorID           string   `json:"author_id"`
	TaskID             string   `json:"task_id,omitempty"`
	ProposalID         string   `json:"proposal_id,omitempty"`
	DiscussionIDs      []string `json:"discussion_ids"`
	DecisionIDs        []string `json:"decision_ids"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type ConflictFile struct {
	Path              string   `json:"path"`
	Kinds             []string `json:"kinds"`
	SourceChange      string   `json:"source_change"`
	TargetChange      string   `json:"target_change"`
	Symbols           []string `json:"symbols"`
	SchemaOrInterface bool     `json:"schema_or_interface"`
}

type SemanticIncompatibility struct {
	Path        string `json:"path"`
	Symbol      string `json:"symbol"`
	Detector    string `json:"detector"`
	Explanation string `json:"explanation"`
}

// AnalyzePullConflict analyzes the adopted pull revision or one retained queue
// candidate. Candidate analysis remains useful after supersession and is marked
// stale instead of being silently recomputed against another base.
func (s *Store) AnalyzePullConflict(repositoryID, pullID, candidateID, ownerID string) (ConflictAnalysis, error) {
	p, err := s.Get(repositoryID, pullID)
	if err != nil {
		return ConflictAnalysis{}, err
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return ConflictAnalysis{}, err
	}
	source, target := p.SourceCommitID, p.TargetCommitID
	currentTarget, branchErr := branchCommit(repository, p.TargetBranch)
	analysisCandidate := ""
	candidateSuperseded := false
	if candidateID != "" {
		found := false
		for _, candidate := range p.IntegrationCandidates {
			if candidate.ID == candidateID {
				source, target, analysisCandidate, found = candidate.SourceCommitID, candidate.BaseCommitID, candidate.ID, true
				candidateSuperseded = candidate.SupersededAt != nil
				break
			}
		}
		if !found {
			return ConflictAnalysis{}, ErrNotFound
		}
	}
	if target == "" {
		return ConflictAnalysis{}, ErrBranchNotFound
	}
	analysis, err := s.analyzeConflict(repositoryID, p.SourceBranch, source, p.TargetBranch, target, ownerID)
	if err != nil {
		return ConflictAnalysis{}, err
	}
	analysis.CandidateID = analysisCandidate
	if candidateSuperseded {
		analysis.StaleReasons = append(analysis.StaleReasons, "integration candidate was superseded")
	}
	var evidenceIncomplete []string
	analysis.Source.PullRequests, evidenceIncomplete = s.pullEvidence(repositoryID, analysis.BaseCommitID, source)
	analysis.Incomplete = append(analysis.Incomplete, evidenceIncomplete...)
	analysis.Target.PullRequests, evidenceIncomplete = s.pullEvidence(repositoryID, analysis.BaseCommitID, target)
	analysis.Incomplete = append(analysis.Incomplete, evidenceIncomplete...)
	analysis.Source.OwnerIDs = evidenceOwners(analysis.Source.OwnerIDs, analysis.Source.PullRequests)
	analysis.Target.OwnerIDs = evidenceOwners(analysis.Target.OwnerIDs, analysis.Target.PullRequests)
	sourceRepository, sourceRepositoryErr := s.git.Open(p.SourceRepositoryID)
	if live, liveErr := func() (string, error) {
		if sourceRepositoryErr != nil {
			return "", sourceRepositoryErr
		}
		return branchCommit(sourceRepository, p.SourceBranch)
	}(); liveErr == nil {
		analysis.Source.CurrentCommitID = live
		if live != source {
			analysis.StaleReasons = append(analysis.StaleReasons, "source branch moved after the analyzed revision")
		}
	} else {
		analysis.Incomplete = append(analysis.Incomplete, "source branch is unavailable")
	}
	if branchErr == nil {
		analysis.Target.CurrentCommitID = currentTarget
		if currentTarget != target {
			analysis.StaleReasons = append(analysis.StaleReasons, "target branch moved after the analyzed revision")
		}
	} else {
		analysis.Incomplete = append(analysis.Incomplete, "target branch is unavailable")
	}
	if len(analysis.Incomplete) > 0 {
		analysis.Status = "incomplete"
	} else if len(analysis.StaleReasons) > 0 {
		analysis.Status = "stale"
	}
	return analysis, nil
}

// AnalyzeBranches provides the same public projection for two selected branch
// tips without creating a pull request or changing either reference.
func (s *Store) AnalyzeBranches(repositoryID, sourceBranch, targetBranch, ownerID string) (ConflictAnalysis, error) {
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return ConflictAnalysis{}, err
	}
	source, err := branchCommit(repository, sourceBranch)
	if err != nil {
		return ConflictAnalysis{}, err
	}
	target, err := branchCommit(repository, targetBranch)
	if err != nil {
		return ConflictAnalysis{}, err
	}
	analysis, err := s.analyzeConflict(repositoryID, sourceBranch, source, targetBranch, target, ownerID)
	if err != nil {
		return ConflictAnalysis{}, err
	}
	analysis.Source.CurrentCommitID, analysis.Target.CurrentCommitID = source, target
	var evidenceIncomplete []string
	analysis.Source.PullRequests, evidenceIncomplete = s.pullEvidence(repositoryID, analysis.BaseCommitID, source)
	analysis.Incomplete = append(analysis.Incomplete, evidenceIncomplete...)
	analysis.Target.PullRequests, evidenceIncomplete = s.pullEvidence(repositoryID, analysis.BaseCommitID, target)
	analysis.Incomplete = append(analysis.Incomplete, evidenceIncomplete...)
	analysis.Source.OwnerIDs = evidenceOwners(analysis.Source.OwnerIDs, analysis.Source.PullRequests)
	analysis.Target.OwnerIDs = evidenceOwners(analysis.Target.OwnerIDs, analysis.Target.PullRequests)
	if len(analysis.Incomplete) > 0 {
		analysis.Status = "incomplete"
	}
	return analysis, nil
}

func (s *Store) analyzeConflict(repositoryID, sourceBranch, source, targetBranch, target, ownerID string) (ConflictAnalysis, error) {
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return ConflictAnalysis{}, err
	}
	baseBytes, err := gitRead(repository, "merge-base", target, source)
	if err != nil {
		return ConflictAnalysis{}, fmt.Errorf("find conflict base: %w", err)
	}
	base := strings.TrimSpace(string(baseBytes))
	baseFiles, err := snapshotFiles(repository, base)
	if err != nil {
		return ConflictAnalysis{}, err
	}
	sourceFiles, err := snapshotFiles(repository, source)
	if err != nil {
		return ConflictAnalysis{}, err
	}
	targetFiles, err := snapshotFiles(repository, target)
	if err != nil {
		return ConflictAnalysis{}, err
	}
	sourceChanges := changedFiles(baseFiles, sourceFiles)
	targetChanges := changedFiles(baseFiles, targetFiles)
	unmerged, err := unmergedPaths(repository, base, target, source)
	if err != nil {
		return ConflictAnalysis{}, err
	}

	analysis := ConflictAnalysis{RepositoryID: repositoryID, BaseCommitID: base, Status: "current", StaleReasons: []string{}, Incomplete: []string{}, Files: []ConflictFile{}, Semantic: []SemanticIncompatibility{}, AffectedChecks: []CheckRequirement{}, Source: ConflictSide{Branch: sourceBranch, CommitID: source, OwnerIDs: uniqueStrings([]string{ownerID})}, Target: ConflictSide{Branch: targetBranch, CommitID: target, OwnerIDs: uniqueStrings([]string{ownerID})}}
	paths := []string{}
	for path := range sourceChanges {
		if _, ok := targetChanges[path]; ok {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		sc, tc := sourceChanges[path], targetChanges[path]
		kinds := []string{}
		if unmerged[path] {
			kinds = append(kinds, "textual")
		}
		structural := sc != "modified" || tc != "modified" || schemaOrInterfacePath(path)
		if structural {
			kinds = append(kinds, "structural")
		}
		symbols := sharedChangedSymbols(repository, baseFiles[path], sourceFiles[path], targetFiles[path])
		for _, symbol := range symbols {
			kinds = append(kinds, "semantic")
			analysis.Semantic = append(analysis.Semantic, SemanticIncompatibility{Path: path, Symbol: symbol, Detector: "independent_symbol_overlap", Explanation: "both revisions independently change the same declared symbol"})
		}
		if len(kinds) == 0 {
			continue
		}
		analysis.Files = append(analysis.Files, ConflictFile{Path: path, Kinds: uniqueStrings(kinds), SourceChange: sc, TargetChange: tc, Symbols: symbols, SchemaOrInterface: schemaOrInterfacePath(path)})
	}
	if s.requirements != nil {
		names, reqErr := s.requirements.RequiredChecks(repositoryID, targetBranch)
		if reqErr != nil {
			analysis.Incomplete = append(analysis.Incomplete, "affected check requirements could not be loaded")
		} else {
			for _, name := range names {
				analysis.AffectedChecks = append(analysis.AffectedChecks, CheckRequirement{Name: name, Status: "affected"})
			}
		}
	}
	if len(analysis.Incomplete) > 0 {
		analysis.Status = "incomplete"
	}
	return analysis, nil
}

type snapshotEntry struct {
	id   storage.ObjectID
	mode string
}

func snapshotFiles(repository *storage.Repository, revision string) (map[string]snapshotEntry, error) {
	commit, err := repository.ReadCommit(storage.ObjectID(revision))
	if err != nil {
		return nil, err
	}
	entries, err := repository.WalkTree(commit.Tree)
	if err != nil {
		return nil, err
	}
	result := map[string]snapshotEntry{}
	for _, entry := range entries {
		if entry.Type != storage.TreeObject {
			result[entry.Path] = snapshotEntry{id: entry.ID, mode: entry.Mode}
		}
	}
	return result, nil
}
func changedFiles(base, side map[string]snapshotEntry) map[string]string {
	result := map[string]string{}
	paths := map[string]bool{}
	for path := range base {
		paths[path] = true
	}
	for path := range side {
		paths[path] = true
	}
	for path := range paths {
		b, bok := base[path]
		x, xok := side[path]
		switch {
		case !bok && xok:
			result[path] = "added"
		case bok && !xok:
			result[path] = "deleted"
		case b != x:
			result[path] = "modified"
		}
	}
	return result
}
func unmergedPaths(repository *storage.Repository, base, target, source string) (map[string]bool, error) {
	temporary, err := os.MkdirTemp("", "vivarium-conflict-index-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	index := filepath.Join(temporary, "index")
	command := exec.Command("git", "-C", repository.Path(), "read-tree", "-i", "-m", base, target, source)
	command.Env = append(os.Environ(), "GIT_INDEX_FILE="+index)
	if output, commandErr := command.CombinedOutput(); commandErr != nil {
		return nil, fmt.Errorf("calculate conflict index: %w: %s", commandErr, strings.TrimSpace(string(output)))
	}
	command = exec.Command("git", "-C", repository.Path(), "ls-files", "-u", "-z")
	command.Env = append(os.Environ(), "GIT_INDEX_FILE="+index)
	output, commandErr := command.Output()
	if commandErr != nil {
		return nil, commandErr
	}
	result := map[string]bool{}
	for _, record := range strings.Split(string(output), "\x00") {
		if _, path, ok := strings.Cut(record, "\t"); ok {
			result[path] = true
		}
	}
	return result, nil
}
func gitRead(repository *storage.Repository, args ...string) ([]byte, error) {
	command := exec.Command("git", append([]string{"-C", repository.Path()}, args...)...)
	return command.Output()
}

var symbolPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^\s*func\s+\([^)]*\)\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?(?:func|function|class|type|interface|const|var|let|def|message|service|enum)\s+([A-Za-z_][A-Za-z0-9_]*)\b`),
}

var interfaceMethodPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*\([^)]*\)\s*(?:[A-Za-z_*\[({]|$)`)

func declaredSymbol(line string) string {
	for _, pattern := range symbolPatterns {
		if match := pattern.FindStringSubmatch(line); match != nil {
			return match[1]
		}
	}
	return ""
}

func declaredSymbols(text string) map[string]string {
	result := map[string]string{}
	inInterface := false
	pendingInterface := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if symbol := declaredSymbol(line); symbol != "" {
			result[symbol] = trimmed
			if strings.Contains(line, "interface") {
				inInterface = strings.Contains(line, "{")
				pendingInterface = !inInterface
			}
			continue
		}
		if pendingInterface && strings.Contains(line, "{") {
			pendingInterface, inInterface = false, true
			continue
		}
		if inInterface {
			if strings.HasPrefix(trimmed, "}") {
				inInterface = false
				continue
			}
			if match := interfaceMethodPattern.FindStringSubmatch(line); match != nil {
				result[match[1]] = trimmed
			}
		}
	}
	return result
}

func sharedChangedSymbols(repository *storage.Repository, base, source, target snapshotEntry) []string {
	read := func(entry snapshotEntry) string {
		if entry.id == "" {
			return ""
		}
		object, err := repository.ReadObject(entry.id)
		if err != nil || object.Type != storage.BlobObject || len(object.Content) > 1<<20 {
			return ""
		}
		return string(object.Content)
	}
	baseText, sourceText, targetText := read(base), read(source), read(target)
	changed := func(before, after string) map[string]bool {
		result := map[string]bool{}
		prior, current := declaredSymbols(before), declaredSymbols(after)
		for symbol, signature := range current {
			if prior[symbol] != signature {
				result[symbol] = true
			}
		}
		return result
	}
	a, b := changed(baseText, sourceText), changed(baseText, targetText)
	result := []string{}
	for name := range a {
		if b[name] {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}
func schemaOrInterfacePath(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "schema") || strings.Contains(lower, "migration") || strings.HasSuffix(lower, ".proto") || strings.Contains(lower, "openapi") || strings.Contains(lower, "interface")
}
func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func (s *Store) pullEvidence(repositoryID, base, revision string) ([]ConflictPullEvidence, []string) {
	pulls, err := s.List(repositoryID)
	if err != nil {
		return []ConflictPullEvidence{}, []string{"pull request collaboration evidence could not be loaded"}
	}
	result := []ConflictPullEvidence{}
	incomplete := []string{}
	repository, openErr := s.git.Open(repositoryID)
	for _, p := range pulls {
		matches := p.SourceCommitID == revision || (p.MergeCommitID != nil && *p.MergeCommitID == revision)
		if !matches && openErr == nil {
			inRevision, revisionErr := commitReachable(repository, p.SourceCommitID, revision)
			inBase, baseErr := commitReachable(repository, p.SourceCommitID, base)
			matches = revisionErr == nil && baseErr == nil && inRevision && !inBase
		}
		if !matches {
			continue
		}
		evidence := ConflictPullEvidence{ID: p.ID, Title: p.Title, AuthorID: p.AuthorID, DiscussionIDs: []string{}, DecisionIDs: []string{}, AcceptanceCriteria: []string{}}
		if p.TaskID != nil {
			evidence.TaskID = *p.TaskID
		}
		if p.ProposalID != nil {
			evidence.ProposalID = *p.ProposalID
		}
		if p.TaskEvidence != nil && p.TaskEvidence.CompletionCriteria != "" {
			evidence.AcceptanceCriteria = append(evidence.AcceptanceCriteria, p.TaskEvidence.CompletionCriteria)
		}
		if p.ContributionEvidence != nil {
			evidence.AcceptanceCriteria = append(evidence.AcceptanceCriteria, p.ContributionEvidence.AcceptanceCriteria...)
		}
		comments, commentsErr := s.ListComments(repositoryID, p.ID)
		if commentsErr != nil {
			incomplete = append(incomplete, fmt.Sprintf("discussion evidence for pull request %s could not be loaded", p.ID))
		}
		for _, comment := range comments {
			evidence.DiscussionIDs = append(evidence.DiscussionIDs, comment.ID)
		}
		reviews, reviewsErr := s.ListReviews(repositoryID, p.ID)
		if reviewsErr != nil {
			incomplete = append(incomplete, fmt.Sprintf("review decision evidence for pull request %s could not be loaded", p.ID))
		}
		for _, review := range reviews {
			evidence.DecisionIDs = append(evidence.DecisionIDs, review.ID)
		}
		result = append(result, evidence)
	}
	return result, uniqueStrings(incomplete)
}

func evidenceOwners(ownerIDs []string, evidence []ConflictPullEvidence) []string {
	for _, pull := range evidence {
		ownerIDs = append(ownerIDs, pull.AuthorID)
	}
	return uniqueStrings(ownerIDs)
}
