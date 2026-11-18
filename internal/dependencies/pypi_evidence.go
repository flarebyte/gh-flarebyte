// purpose: Corroborate each audited PyPI wheel with its public registry artifact record.
// responsibilities: Retain hash/source/lifecycle conflicts while preserving unresolved distribution metadata and runtime gaps.
package dependencies

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

func (c *Client) checkPyPINode(ctx context.Context, project *ProjectSnapshot, node ProjectNode, f *Finding, sources *[]Source) {
	f.Missing = unique(append(f.Missing, "PYPI_DISTRIBUTION_METADATA", "PYTHON_ENVIRONMENT_CONTEXT"))
	var body struct {
		Info struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"info"`
		URLs *[]struct {
			Filename       string  `json:"filename"`
			URL            string  `json:"url"`
			Size           *int64  `json:"size"`
			Yanked         *bool   `json:"yanked"`
			RequiresPython *string `json:"requires_python"`
			Digests        struct {
				SHA256 string `json:"sha256"`
			} `json:"digests"`
		} `json:"urls"`
	}
	err := c.read(ctx, "pypi-artifact", "https://pypi.org/pypi/"+url.PathEscape(node.Name)+"/"+url.PathEscape(node.Version)+"/json", nil, &body, sources)
	if err != nil {
		var lookup *LookupError
		if errors.As(err, &lookup) && lookup.Status == http.StatusNotFound {
			f.Reasons = unique(append(f.Reasons, "WITHDRAWN_RELEASE"))
		} else {
			f.Missing = unique(append(f.Missing, "PYPI_ARTIFACT_LOOKUP"))
		}
		return
	}
	if body.Info.Name == "" || body.Info.Version == "" {
		f.Missing = unique(append(f.Missing, "PYPI_ARTIFACT_SCHEMA"))
		return
	}
	if !equivalentName("pypi", body.Info.Name, node.Name) || body.Info.Version != node.Version {
		f.Reasons = unique(append(f.Reasons, "IDENTITY_CONFLICT"))
		return
	}
	if body.URLs == nil {
		f.Missing = unique(append(f.Missing, "PYPI_ARTIFACT_SCHEMA"))
		return
	}
	expected := project.pypi[node.Path]
	matches, index := 0, 0
	for i, file := range *body.URLs {
		if file.URL == node.Resolved {
			matches++
			index = i
		}
	}
	if matches == 0 {
		f.Reasons = unique(append(f.Reasons, "ARTIFACT_MISMATCH"))
		return
	}
	if matches > 1 {
		f.Missing = unique(append(f.Missing, "PYPI_ARTIFACT_SCHEMA"))
		return
	}
	file := (*body.URLs)[index]
	checkPythonCompatibility(expected.ManifestPython, expected.LockPython, file.RequiresPython, f)
	if file.Filename == "" || !cargoChecksum.MatchString(file.Digests.SHA256) || expected.Size != 0 && (file.Size == nil || *file.Size <= 0) {
		f.Missing = unique(append(f.Missing, "PYPI_ARTIFACT_SCHEMA"))
		return
	}
	if file.Filename != expected.Filename || "sha256-"+file.Digests.SHA256 != node.Integrity || expected.Size != 0 && *file.Size != expected.Size {
		f.Reasons = unique(append(f.Reasons, "ARTIFACT_MISMATCH"))
		return
	}
	if file.Yanked == nil {
		f.Missing = unique(append(f.Missing, "RELEASE_LIFECYCLE"))
	} else if *file.Yanked {
		f.Reasons = unique(append(f.Reasons, "WITHDRAWN_RELEASE"))
	}
	c.checkPyPIMetadata(ctx, project, node, f, sources)
}

func mergePythonInventory(r *Report, project *ProjectSnapshot) {
	f := Finding{Reasons: []string{}, Missing: append([]string{}, r.Evidence.Missing...), Advisories: append([]string{}, r.Evidence.Advisories...)}
	if r.Decision == Deny {
		f.Reasons = append(f.Reasons, r.Reasons...)
	}
	mergeProjectFindings(&f, project, "")
	r.Recommendation = nil
	r.Alternatives = []Candidate{}
	r.Scope = "package"
	r.Evidence.Missing = unique(append(append(f.Missing, project.Missing...), "PROJECT_RESOLUTION"))
	r.Evidence.Advisories = f.Advisories
	if len(f.Reasons) > 0 {
		r.Decision = Deny
		r.Reasons = f.Reasons
		for _, node := range project.Findings {
			if len(node.Finding.Reasons) > 0 {
				r.Reasons = unique(append(r.Reasons, "LOCKED_INVENTORY_VIOLATION"))
				break
			}
		}
	} else if r.Decision == Allow {
		r.Decision = Unknown
		r.Reasons = []string{"INSUFFICIENT_EVIDENCE"}
	}
}
