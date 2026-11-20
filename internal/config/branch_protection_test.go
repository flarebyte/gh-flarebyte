package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBranchProtectionConfig(t *testing.T) {
	cases := []struct {
		name, block, wantErr string
		want                 *BranchProtectionConfig
	}{
		{name: "omitted"},
		{name: "default bypass", block: `branchProtection: { enabled: true }`, want: &BranchProtectionConfig{true, true}},
		{name: "no bypass", block: `branchProtection: { enabled: true, allowAdminCIBypass: false }`, want: &BranchProtectionConfig{true, false}},
		{name: "disabled ignores bypass", block: `branchProtection: { enabled: false, allowAdminCIBypass: true }`, want: &BranchProtectionConfig{false, true}},
		{name: "comments", block: "branchProtection: {\n// enabled: false\nenabled: true\n}", want: &BranchProtectionConfig{true, true}},
		{name: "empty", block: `branchProtection: {}`, wantErr: "repository.branchProtection.enabled"},
		{name: "missing enabled", block: `branchProtection: { allowAdminCIBypass: true }`, wantErr: "repository.branchProtection.enabled"},
		{name: "object type", block: `branchProtection: true`, wantErr: "repository.branchProtection"},
		{name: "enabled type", block: `branchProtection: { enabled: "true" }`, wantErr: "repository.branchProtection.enabled"},
		{name: "bypass type", block: `branchProtection: { enabled: true, allowAdminCIBypass: 1 }`, wantErr: "repository.branchProtection.allowAdminCIBypass"},
		{name: "expression", block: `branchProtection: { enabled: true || false }`, wantErr: "repository.branchProtection"},
		{name: "duplicate field", block: `branchProtection: { enabled: true, enabled: false }`, wantErr: "duplicate repository.branchProtection.enabled"},
		{name: "duplicate object", block: "branchProtection: {enabled: true}\nbranchProtection: {enabled: false}", wantErr: "duplicate repository.branchProtection"},
		{name: "unknown", block: `branchProtection: { enabled: true, requiredChecks: [] }`, wantErr: "repository.branchProtection"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := strings.Replace(configTestHeader, "repository: {", "repository: {\n"+tc.block, 1) + configTestBinaryBuild + configTestRelease
			path := filepath.Join(t.TempDir(), "config.cue")
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := cfg.Repository.BranchProtection
			if tc.want == nil {
				if got != nil {
					t.Fatalf("omitted policy must be nil: %+v", got)
				}
			} else if got == nil || *got != *tc.want {
				t.Fatalf("want %+v, got %+v", tc.want, got)
			}
		})
	}
}

func TestBranchProtectionScopedParsing(t *testing.T) {
	for _, raw := range []string{
		`description: "branchProtection: { enabled: true }"`,
		"// branchProtection: { enabled: true }\nfeatures: { enabled: true }",
		`features: { branchProtection: { enabled: true } }`,
	} {
		cfg, err := parseBranchProtection(raw)
		if err != nil || cfg != nil {
			t.Fatalf("unexpected policy from %q: %+v %v", raw, cfg, err)
		}
	}
	if _, err := parseBranchProtection("branchProtection: { enabled: true"); err == nil {
		t.Fatal("unclosed policy accepted")
	}
}
