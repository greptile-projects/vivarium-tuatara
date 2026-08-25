package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancegraphs"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancepolicies"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type provenanceGraphInput struct {
	RequestID     string                  `json:"request_id"`
	Revision      string                  `json:"revision"`
	PolicyID      string                  `json:"policy_id,omitempty"`
	PolicyVersion int                     `json:"policy_version,omitempty"`
	Nodes         []provenancegraphs.Node `json:"nodes"`
	Edges         []provenancegraphs.Edge `json:"edges"`
}

func registerProvenanceGraphRoutes(mux *http.ServeMux, gitStore *storage.Store, repos *repositories.Store, credentials *auth.Store, graphs *provenancegraphs.Store, policies *provenancepolicies.Store) {
	project := func(g provenancegraphs.Graph, actor string) provenancegraphs.Graph {
		return projectProvenanceGraph(currentProvenanceGraph(g, gitStore, repos), actor)
	}
	mux.HandleFunc("GET /repositories/{id}/provenance-graphs", func(w http.ResponseWriter, r *http.Request) {
		a, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, e := graphs.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "provenance_graphs_unavailable", "provenance graphs could not be read")
			return
		}
		for i := range values {
			values[i] = project(values[i], a.UserID)
		}
		writeJSON(w, 200, map[string]any{"graphs": values})
	})
	mux.HandleFunc("GET /repositories/{id}/provenance-graphs/{graph_id}", func(w http.ResponseWriter, r *http.Request) {
		a, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		g, e := graphs.Get(r.PathValue("graph_id"))
		if e != nil || g.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "provenance_graph_not_found", "provenance graph not found")
			return
		}
		writeJSON(w, 200, project(g, a.UserID))
	})
	mux.HandleFunc("POST /repositories/{id}/provenance-graphs", func(w http.ResponseWriter, r *http.Request) {
		a, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in provenanceGraphInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete revision-exact provenance declaration is required")
			return
		}
		g := provenancegraphs.Graph{RequestID: in.RequestID, RepositoryID: r.PathValue("id"), Revision: in.Revision, PolicyID: in.PolicyID, PolicyVersion: in.PolicyVersion, Nodes: in.Nodes, Edges: in.Edges, CreatedBy: a.UserID}
		if g.PolicyID != "" {
			policy, x := policies.Get(g.PolicyID)
			if x != nil || policy.ScopeKind != "repository" || policy.ScopeID != g.RepositoryID || g.PolicyVersion < 1 || g.PolicyVersion > len(policy.Revisions) || policy.Revisions[g.PolicyVersion-1].Version != g.PolicyVersion {
				writeAPIError(w, 422, "provenance_policy_invalid", "the declared provenance policy revision does not resolve in this repository")
				return
			}
		} else if g.PolicyVersion != 0 {
			writeAPIError(w, 422, "provenance_policy_invalid", "a policy version requires an exact repository policy")
			return
		}
		if e := analyzeProvenanceGraph(&g, gitStore); e != nil {
			writeAPIError(w, 422, "provenance_evidence_invalid", e.Error())
			return
		}
		out, e := graphs.Create(g)
		switch {
		case e == nil:
			writeJSON(w, 201, project(out, a.UserID))
		case errors.Is(e, provenancegraphs.ErrRequestConflict):
			writeAPIError(w, 409, "provenance_request_conflict", "request_id was already used for different exact evidence")
		case errors.Is(e, provenancegraphs.ErrInvalid):
			writeAPIError(w, 400, "invalid_provenance_graph", "nodes, transformations, citations, confidence, and audiences must be complete and consistent")
		default:
			writeAPIError(w, 500, "provenance_graphs_unavailable", "provenance graph could not be persisted")
		}
	})
}

func analyzeProvenanceGraph(g *provenancegraphs.Graph, gitStore *storage.Store) error {
	repo, e := gitStore.Open(g.RepositoryID)
	if e != nil {
		return errors.New("repository Git data is unavailable")
	}
	commit, e := repo.ReadCommit(storage.ObjectID(g.Revision))
	if e != nil {
		return errors.New("the exact analysis revision does not resolve")
	}
	paths, e := repo.WalkTree(commit.Tree)
	if e != nil {
		return errors.New("the exact revision tree cannot be analyzed")
	}
	blobs := map[string]storage.ObjectID{}
	for _, p := range paths {
		if p.Type == storage.BlobObject {
			blobs[p.Path] = p.ID
		}
	}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		n.DeclaredBy = g.CreatedBy
		if n.Kind == "commit" && n.Revision == "" {
			n.Revision = g.Revision
		}
		for j := range n.Citations {
			c := &n.Citations[j]
			if c.Kind == "repository_file" {
				if c.Revision != "" && c.Revision != g.Revision {
					return errors.New("file citations must bind the analyzed revision")
				}
				id, ok := blobs[c.Path]
				if !ok {
					return errors.New("a cited repository file does not exist at the analyzed revision")
				}
				o, x := repo.ReadObject(id)
				if x != nil {
					return errors.New("a cited repository file cannot be verified")
				}
				sum := sha256.Sum256(o.Content)
				c.ResourceID = string(id)
				c.Revision = g.Revision
				c.SHA256 = hex.EncodeToString(sum[:])
			}
		}
	}
	for i := range g.Edges {
		g.Edges[i].DeclaredBy = g.CreatedBy
	}
	g.Diagnostics = deriveProvenanceDiagnostics(*g)
	raw, _ := json.Marshal(struct {
		Revision      string
		PolicyID      string
		PolicyVersion int
		Nodes         []provenancegraphs.Node
		Edges         []provenancegraphs.Edge
	}{g.Revision, g.PolicyID, g.PolicyVersion, g.Nodes, g.Edges})
	sum := sha256.Sum256(raw)
	g.AnalysisDigest = hex.EncodeToString(sum[:])
	return nil
}
func deriveProvenanceDiagnostics(g provenancegraphs.Graph) []provenancegraphs.Diagnostic {
	out := []provenancegraphs.Diagnostic{}
	incoming := map[string]int{}
	licenses := map[string]map[string]bool{}
	for _, e := range g.Edges {
		incoming[e.To]++
		if e.Confidence == "contradicted" {
			out = append(out, provenancegraphs.Diagnostic{Kind: "contradictory_claim", Severity: "blocking", EdgeID: e.ID, Message: "The retained transformation has contradictory evidence.", AttributedTo: e.DeclaredBy})
		}
	}
	for _, n := range g.Nodes {
		if (n.Kind == "file" || n.Kind == "fragment" || n.Kind == "asset" || n.Kind == "artifact") && incoming[n.ID] == 0 {
			out = append(out, provenancegraphs.Diagnostic{Kind: "missing_origin", Severity: "blocking", NodeID: n.ID, Message: "No attributable origin transformation reaches this shipped material.", AttributedTo: n.DeclaredBy})
		}
		if n.Confidence == "unknown" {
			out = append(out, provenancegraphs.Diagnostic{Kind: "unknown_origin", Severity: "blocking", NodeID: n.ID, Message: "The declaration retains unknown origin rather than inferring one.", AttributedTo: n.DeclaredBy})
		}
		if n.License != "" {
			key := strings.ToLower(n.Label)
			if licenses[key] == nil {
				licenses[key] = map[string]bool{}
			}
			licenses[key][strings.ToLower(n.License)] = true
		}
	}
	for label, values := range licenses {
		if len(values) > 1 {
			out = append(out, provenancegraphs.Diagnostic{Kind: "contradictory_license", Severity: "blocking", Message: "Conflicting license claims remain for " + label + "."})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind+out[i].NodeID < out[j].Kind+out[j].NodeID })
	return out
}
func currentProvenanceGraph(g provenancegraphs.Graph, gitStore *storage.Store, repos *repositories.Store) provenancegraphs.Graph {
	r, e := repos.GetByID(g.RepositoryID)
	if e != nil {
		return g
	}
	repo, e := gitStore.Open(g.RepositoryID)
	if e != nil {
		return g
	}
	ref, e := repo.ReadReference("refs/heads/" + r.DefaultBranch)
	if e != nil {
		return g
	}
	g.CurrentRevision = ref.Target
	g.Stale = g.Revision != ref.Target
	reachable := false
	if commits, e := repo.ListCommitAncestry(storage.ObjectID(ref.Target)); e == nil {
		for _, c := range commits {
			if string(c.ID) == g.Revision {
				reachable = true
			}
		}
	}
	if !reachable {
		g.Diagnostics = append(g.Diagnostics, provenancegraphs.Diagnostic{Kind: "rewritten_history", Severity: "blocking", Message: "The analyzed revision is no longer reachable from the current default branch."})
	} else if g.Stale {
		g.Diagnostics = append(g.Diagnostics, provenancegraphs.Diagnostic{Kind: "stale_analysis", Severity: "warning", Message: "The default branch moved after this exact analysis."})
	}
	return g
}
func projectProvenanceGraph(g provenancegraphs.Graph, actor string) provenancegraphs.Graph {
	// Projection must never mutate the retained graph or a later authorized
	// reader could inherit a prior reader's redaction through shared slices.
	raw, _ := json.Marshal(g)
	var projected provenancegraphs.Graph
	_ = json.Unmarshal(raw, &projected)
	g = projected
	visible := map[string]bool{}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		allowed := n.Audience != "restricted"
		for _, id := range n.AudienceIDs {
			if id == actor {
				allowed = true
			}
		}
		if allowed {
			visible[n.ID] = true
			continue
		}
		n.Label = "Restricted provenance source"
		n.Revision = ""
		n.License = ""
		n.Obligations = nil
		n.Citations = nil
		n.AudienceIDs = nil
		n.Restricted = true
	}
	for i := range g.Edges {
		e := &g.Edges[i]
		if !visible[e.From] || !visible[e.To] {
			e.Citation = provenancegraphs.Citation{}
			e.ToolNodeID = ""
		}
	}
	return g
}
