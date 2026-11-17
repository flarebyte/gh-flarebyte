package dependencies

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestPythonCatalogLive(t *testing.T) {
	if os.Getenv("GH_FLAREBYTE_DEP_LIVE") != "1" {
		t.Skip("opt-in read-only Python catalog classification")
	}
	for _, name := range []string{"requests", "attrs"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			sources := []Source{}
			ctx = withBudget(ctx)
			client := NewClient()
			p, err := client.Package(ctx, "pypi", name, &sources)
			if err != nil {
				t.Fatal(err)
			}
			stable, prerelease, unsupported := []string{}, []string{}, []string{}
			for _, v := range p.Versions {
				if _, ok := stableVersion("pypi", v.Key.Version); ok {
					stable = append(stable, v.Key.Version)
				} else if knownPrerelease("pypi", v.Key.Version) {
					prerelease = append(prerelease, v.Key.Version)
				} else if !v.Deprecated {
					unsupported = append(unsupported, v.Key.Version)
				}
			}
			availability := []any{}
			for i, version := range unsupported {
				if i >= maxPythonCatalogProbes {
					break
				}
				e, _, _, reasons := client.releaseEvidence(ctx, "pypi", name, version, &sources)
				availability = append(availability, map[string]any{"version": version, "lifecycle_known": e.LifecycleKnown, "withdrawn": e.Withdrawn, "reasons": reasons})
			}
			data, _ := json.Marshal(struct {
				Package      string   `json:"package"`
				Stable       []string `json:"stable"`
				Prerelease   []string `json:"prerelease"`
				Unsupported  []string `json:"unsupported"`
				Sources      []Source `json:"sources"`
				Availability []any    `json:"availability"`
			}{name, unique(stable), unique(prerelease), unique(unsupported), sources, availability})
			t.Logf("LIVE_PYTHON_CATALOG %s", data)
		})
	}
}
