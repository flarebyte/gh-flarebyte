// purpose: Model bounded dependency assessments and versioned machine-readable reports.
// responsibilities: Define policy, evidence, outcomes and provider-neutral records.
// architecture notes: Unknown evidence is explicit and never converted into permission to install.
package dependencies

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode"
)

const (
	Allow   = "allow"
	Deny    = "deny"
	Unknown = "unknown"
)

type Policy struct {
	ID                    string           `json:"id"`
	MinAgeDays            int              `json:"min_age_days"`
	MinStars              int              `json:"min_stars"`
	MinMaintainers        int              `json:"min_maintainers"`
	ActivityDays          int              `json:"activity_days"`
	MaxScorecardAgeDays   int              `json:"max_scorecard_age_days"`
	ScorecardChecks       []CheckThreshold `json:"scorecard_checks"`
	NameReferenceRevision string           `json:"name_reference_revision"`
	NameReferenceDigest   string           `json:"name_reference_digest"`
}

type CheckThreshold struct {
	Name     string `json:"name"`
	MinScore int    `json:"min_score"`
}

func DefaultPolicy(days int) Policy {
	references, _ := json.Marshal(nameReferences)
	hash := sha256.Sum256(references)
	return Policy{ID: publicMaintenancePolicy, MinAgeDays: days, MinStars: 100, MinMaintainers: 2, ActivityDays: 180, MaxScorecardAgeDays: 30,
		NameReferenceRevision: nameReferenceRevision, NameReferenceDigest: "sha256:" + hex.EncodeToString(hash[:]), ScorecardChecks: []CheckThreshold{{"Branch-Protection", 6}, {"Code-Review", 7}, {"Dangerous-Workflow", 10}, {"Maintained", 5}, {"Token-Permissions", 8}}}
}

func (p Policy) Digest() string {
	b, _ := json.Marshal(p)
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

type Candidate struct {
	Version       string `json:"version"`
	AgeDays       int64  `json:"age_days"`
	Compatibility string `json:"compatibility"`
}

type Evidence struct {
	Identity    string               `json:"identity"`
	Stars       *int                 `json:"stars"`
	Maintainers string               `json:"maintainers"`
	Advisories  []string             `json:"advisories"`
	Missing     []string             `json:"missing"`
	Maintenance *MaintenanceEvidence `json:"maintenance,omitempty"`
}

type MaintenanceEvidence struct {
	Basis                   string `json:"basis"`
	AuthorityAccounts       *int   `json:"authority_accounts"`
	ActiveAuthorityAccounts *int   `json:"active_authority_accounts"`
}

type MaintenanceAuthority struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type MaintenanceActivity struct {
	OwnerID     int64     `json:"owner_id"`
	Version     string    `json:"version"`
	PublishedAt time.Time `json:"published_at"`
}

type MaintenanceDetail struct {
	Package       string                 `json:"package"`
	Evidence      MaintenanceEvidence    `json:"evidence"`
	Authorities   []MaintenanceAuthority `json:"authorities"`
	Activities    []MaintenanceActivity  `json:"activities"`
	Missing       []string               `json:"missing"`
	RegistryRoles []RegistryRole         `json:"registry_roles,omitempty"`
	Organization  string                 `json:"organization,omitempty"`
}

type RegistryRole struct {
	Account string `json:"account"`
	Role    string `json:"role"`
}

type Report struct {
	Schema         int           `json:"schema"`
	Package        string        `json:"package"`
	Description    *string       `json:"description"`
	Requested      *string       `json:"requested"`
	Scope          string        `json:"scope"`
	Decision       string        `json:"decision"`
	Reasons        []string      `json:"reasons"`
	Recommendation *Candidate    `json:"recommendation"`
	Alternatives   []Candidate   `json:"alternatives"`
	Evidence       Evidence      `json:"evidence"`
	Policy         string        `json:"policy"`
	PolicyDigest   string        `json:"policy_digest"`
	CheckedAt      time.Time     `json:"checked_at"`
	Details        *Details      `json:"details,omitempty"`
	Error          *ReportError  `json:"error,omitempty"`
	Installation   *Installation `json:"installation,omitempty"`
}

type Installation struct {
	Version             string   `json:"version"`
	Files               []string `json:"files"`
	AssessmentReference string   `json:"assessment_reference"`
}

type ReportError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Source struct {
	Package     string    `json:"package,omitempty"`
	Version     string    `json:"version,omitempty"`
	Provider    string    `json:"provider"`
	URL         string    `json:"url"`
	RetrievedAt time.Time `json:"retrieved_at"`
	Status      string    `json:"status"`
	Attempts    int       `json:"attempts"`
}
type Finding struct {
	cargoMaintenance *cargoMaintenanceProof
	Version          string                `json:"version"`
	Reasons          []string              `json:"reasons"`
	Missing          []string              `json:"missing"`
	Advisories       []string              `json:"advisories"`
	Decision         string                `json:"decision"`
	Evidence         *Evidence             `json:"evidence,omitempty"`
	AdvisoryGroups   []AdvisoryGroup       `json:"advisory_groups,omitempty"`
	Resolution       string                `json:"resolution,omitempty"`
	Runtime          *RuntimeCompatibility `json:"runtime,omitempty"`
}

// RuntimeCompatibility compares declared requirements; it does not attest to an
// installed compiler, successful compilation or feature activation.
type RuntimeCompatibility struct {
	Runtime  string `json:"runtime"`
	Basis    string `json:"basis"`
	Baseline string `json:"baseline,omitempty"`
	Minimum  string `json:"minimum,omitempty"`
	Status   string `json:"status"`
}

type FeatureResolution struct {
	Basis        string              `json:"basis"`
	Status       string              `json:"status"`
	Enabled      []FeatureActivation `json:"enabled"`
	Missing      []string            `json:"missing"`
	MinimumCargo string              `json:"minimum_cargo,omitempty"`
}
type FeatureActivation struct {
	Path     string   `json:"path"`
	Features []string `json:"features"`
}
type Details struct {
	Maintenance           []MaintenanceDetail `json:"maintenance,omitempty"`
	Policy                Policy              `json:"policy"`
	Sources               []Source            `json:"sources"`
	Candidates            []Finding           `json:"candidates"`
	Scorecards            []Scorecard         `json:"scorecards"`
	ConfigPath            string              `json:"config_path"`
	LimitReached          bool                `json:"limit_reached"`
	Project               *ProjectSnapshot    `json:"project,omitempty"`
	RecommendationMissing []string            `json:"recommendation_missing,omitempty"`
}

func NewReport(system, name string, requested *string, p Policy, now time.Time) Report {
	return Report{Schema: 1, Package: system + ":" + name, Requested: requested,
		Scope: "package", Decision: Unknown, Reasons: []string{}, Alternatives: []Candidate{},
		Evidence: Evidence{Identity: "unknown", Maintainers: "unknown", Advisories: []string{}, Missing: []string{}},
		Policy:   p.ID, PolicyDigest: p.Digest(), CheckedAt: now.UTC(),
		Details: &Details{Policy: p, Sources: []Source{}, Candidates: []Finding{}, Scorecards: []Scorecard{}}}
}

// Description treats upstream text as data, removing terminal controls before
// rendering. Limit by Unicode code points, including the truncation marker.
func Description(raw string) *string {
	raw = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, raw)
	raw = strings.Join(strings.Fields(raw), " ")
	if raw == "" {
		return nil
	}
	r := []rune(raw)
	if len(r) > 200 {
		raw = string(r[:199]) + "…"
	}
	return &raw
}

func daysOld(now, published time.Time) int64 {
	seconds := now.Unix() - published.Unix()
	if now.Nanosecond() < published.Nanosecond() {
		seconds--
	}
	return seconds / 86400
}
