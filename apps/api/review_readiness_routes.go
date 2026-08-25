package main

import (
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/reviewplans"
)

func reviewReadinessFor(p pullrequests.PullRequest, pulls *pullrequests.Store, plans *reviewplans.Store) (reviewplans.ReviewReadiness, bool, error) {
	history, err := plans.List(p.RepositoryID, p.ID, p.SourceCommitID, p.TargetCommitID)
	if err != nil {
		return reviewplans.ReviewReadiness{}, false, err
	}
	if len(history) == 0 {
		return reviewplans.ProjectReadiness(nil, nil, nil, nil, p.SourceCommitID, p.TargetCommitID, nil), false, nil
	}
	plan := history[len(history)-1]
	assignments, err := plans.ListAssignments(p.RepositoryID, p.ID)
	if err != nil {
		return reviewplans.ReviewReadiness{}, true, err
	}
	work, err := plans.ListWork(p.RepositoryID, p.ID)
	if err != nil {
		return reviewplans.ReviewReadiness{}, true, err
	}
	resolutions, err := plans.ListFindingResolutions(p.RepositoryID, p.ID)
	if err != nil {
		return reviewplans.ReviewReadiness{}, true, err
	}
	reviews, err := pulls.ListReviews(p.RepositoryID, p.ID)
	if err != nil {
		return reviewplans.ReviewReadiness{}, true, err
	}
	stale := []reviewplans.StaleApproval{}
	for _, review := range reviews {
		if review.Decision == pullrequests.Approved && review.Stale {
			stale = append(stale, reviewplans.StaleApproval{ReviewerID: review.ReviewerID, Revision: review.ReviewedCommitID, Decision: string(review.Decision), CreatedAt: review.UpdatedAt})
		}
	}
	return reviewplans.ProjectReadiness(&plan, assignments, work, resolutions, p.SourceCommitID, p.TargetCommitID, stale), true, nil
}

func configureReviewReadiness(pulls *pullrequests.Store, plans *reviewplans.Store) {
	pulls.ConfigureReviewReadiness(func(p pullrequests.PullRequest) (any, []pullrequests.ReadinessBlocker, error) {
		matrix, required, err := reviewReadinessFor(p, pulls, plans)
		if err != nil || !required || matrix.Complete {
			return matrix, nil, err
		}
		return matrix, []pullrequests.ReadinessBlocker{{Code: "review_coverage_incomplete", Message: "Every required review-plan area must have current accountable coverage."}}, nil
	})
}

func registerReviewReadinessRoute(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, plans *reviewplans.Store) {
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/review-readiness", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		p, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if err != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		matrix, _, err := reviewReadinessFor(p, pulls, plans)
		if err != nil {
			writeAPIError(w, 500, "review_readiness_unavailable", "review readiness could not be derived")
			return
		}
		writeJSON(w, 200, matrix)
	})
}
