// purpose: Establish current maintenance authority using authenticated GitHub role data.
// responsibilities: Require distinct human accounts with current write authority and recent authenticated activity.
// architecture notes: Author fields on Git commits and raw contributor counts are insufficient; unavailable role data stays unknown.
package dependencies

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type githubAccount struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Type  string `json:"type"`
}
type githubCollaborator struct {
	githubAccount
	Permissions map[string]bool `json:"permissions"`
}
type githubPull struct {
	Number    int           `json:"number"`
	User      githubAccount `json:"user"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}
type githubReview struct {
	User        githubAccount `json:"user"`
	State       string        `json:"state"`
	SubmittedAt time.Time     `json:"submitted_at"`
}

// githubArray wraps the list APIs in an object so the common bounded reader can
// continue rejecting null/invalid top-level responses elsewhere.
type githubArray[T any] struct{ Items []T }

func (a *githubArray[T]) UnmarshalJSON(b []byte) error { return decodeGitHubArray(b, &a.Items) }

func (c *Client) githubList(ctx context.Context, path string, out any, sources *[]Source) error {
	return c.read(ctx, "github", "https://api.github.com"+path, nil, out, sources)
}

func (c *Client) maintainerCount(ctx context.Context, repo string, p Policy, now time.Time, sources *[]Source) (*int, error) {
	if Repository(repo) == "" {
		return nil, fmt.Errorf("invalid maintenance repository")
	}
	base := "/repos/" + strings.TrimPrefix(repo, "github.com/")
	authorities := map[int64]string{}
	complete := false
	for page := 1; page <= 5; page++ {
		var collaborators githubArray[githubCollaborator]
		err := c.githubList(ctx, fmt.Sprintf("%s/collaborators?affiliation=all&per_page=100&page=%d", base, page), &collaborators, sources)
		if err != nil {
			return nil, err
		}
		if len(collaborators.Items) > 100 {
			return nil, fmt.Errorf("GitHub role response exceeds page limit")
		}
		for _, user := range collaborators.Items {
			if user.ID <= 0 || user.Login == "" || user.Type == "" || user.Permissions == nil {
				return nil, fmt.Errorf("incomplete GitHub role evidence")
			}
			if user.Type != "User" || strings.HasSuffix(strings.ToLower(user.Login), "[bot]") {
				continue
			}
			if _, ok := user.Permissions["push"]; !ok {
				return nil, fmt.Errorf("incomplete GitHub permissions")
			}
			if user.Permissions["push"] || user.Permissions["maintain"] || user.Permissions["admin"] {
				authorities[user.ID] = user.Login
			}
		}
		if len(collaborators.Items) < 100 {
			complete = true
			break
		}
	}
	// Visibility and activity feeds do not prove absence of other maintainers.
	// Positive evidence can establish multiple active authorities; a partial
	// roster must never be presented as proof of a single-maintainer project.
	if complete && len(authorities) < p.MinMaintainers {
		return nil, fmt.Errorf("insufficient visible GitHub authorities")
	}
	if len(authorities) < p.MinMaintainers {
		return nil, fmt.Errorf("GitHub authority pagination incomplete")
	}
	active := map[int64]bool{}
	now = now.UTC()
	since := now.AddDate(0, 0, -p.ActivityDays)
	var pulls githubArray[githubPull]
	if err := c.githubList(ctx, base+"/pulls?state=all&sort=updated&direction=desc&per_page=10", &pulls, sources); err != nil {
		return nil, err
	}
	if len(pulls.Items) > 10 {
		return nil, fmt.Errorf("GitHub activity response exceeds limit")
	}
	for _, pull := range pulls.Items {
		if pull.Number <= 0 || pull.UpdatedAt.IsZero() || pull.CreatedAt.IsZero() {
			return nil, fmt.Errorf("incomplete GitHub activity evidence")
		}
		if pull.UpdatedAt.Before(since) || pull.UpdatedAt.After(now) {
			continue
		}
		if pull.User.Type == "User" && authorities[pull.User.ID] != "" && !pull.CreatedAt.Before(since) && !pull.CreatedAt.After(now) {
			active[pull.User.ID] = true
		}
		if len(active) >= p.MinMaintainers {
			n := len(active)
			return &n, nil
		}
		for page := 1; page <= 2; page++ {
			var reviews githubArray[githubReview]
			path := fmt.Sprintf("%s/pulls/%d/reviews?per_page=100&page=%d", base, pull.Number, page)
			if err := c.githubList(ctx, path, &reviews, sources); err != nil {
				return nil, err
			}
			if len(reviews.Items) > 100 {
				return nil, fmt.Errorf("GitHub review response exceeds page limit")
			}
			for _, review := range reviews.Items {
				if review.User.Type != "User" || review.User.ID == pull.User.ID || authorities[review.User.ID] == "" {
					continue
				}
				if review.SubmittedAt.IsZero() || review.SubmittedAt.Before(since) || review.SubmittedAt.After(now) {
					continue
				}
				if review.State == "APPROVED" || review.State == "CHANGES_REQUESTED" {
					active[review.User.ID] = true
				}
			}
			if len(active) >= p.MinMaintainers {
				n := len(active)
				return &n, nil
			}
			if len(reviews.Items) < 100 {
				break
			}
		}
	}
	return nil, fmt.Errorf("independent active maintenance is not established")
}

func (c *Client) fillMaintainers(ctx context.Context, e *TrustEvidence, p Policy, now time.Time, r *Report, cache map[string]TrustEvidence) {
	if e.Identity != "verified" || e.Repository == "" {
		return
	}
	prior := cache[e.Repository]
	if !prior.MaintenanceChecked {
		prior.MaintenanceChecked = true
		count, err := c.maintainerCount(ctx, e.Repository, p, now, &r.Details.Sources)
		if err == nil {
			prior.IndependentMaintainers = count
		}
		cache[e.Repository] = prior
	}
	e.IndependentMaintainers = prior.IndependentMaintainers
}

func (c *Client) fillMaintenance(ctx context.Context, system, name string, e *TrustEvidence, p Policy, now time.Time, r *Report, cache map[string]TrustEvidence) {
	c.fillMaintainers(ctx, e, p, now, r, cache)
	if system != "cargo" || p.ID != publicMaintenancePolicy || e.IndependentMaintainers != nil && *e.IndependentMaintainers >= p.MinMaintainers {
		return
	}
	key := "cargo-maintenance:" + name
	prior := cache[key]
	if !prior.cargoMaintenanceChecked {
		prior.cargoMaintenanceChecked = true
		prior.cargoMaintenance = c.cargoMaintainers(ctx, name, p, now, &r.Details.Sources)
		cache[key] = prior
		r.Details.Maintenance = append(r.Details.Maintenance, prior.cargoMaintenance.detail())
	}
	e.cargoMaintenance = prior.cargoMaintenance
}
