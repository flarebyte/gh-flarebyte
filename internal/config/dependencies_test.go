package config

import (
	"os"
	"strings"
	"testing"
)

func TestDependencyPolicy(t *testing.T) {
	base, err := os.ReadFile("testdata/valid.cue")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		raw  string
		want int
		bad  bool
	}{
		{"", 14, false}, {"dependencies: {}", 14, false},
		{"dependencies: { minAgeDays: 0 }", 0, false},
		{"dependencies: { minAgeDays: 30 }", 30, false},
		{"dependencies: { /* comment */ minAgeDays: 7, }", 7, false},
		{"// dependencies: { minAgeDays: 0 }", 14, false},
		{"dependencies: { minAgeDays: -1 }", 0, true},
		{"dependencies: { minAgeDays: 1.5 }", 0, true},
		{`dependencies: { minAgeDays: "14" }`, 0, true},
		{"dependencies: { minAgeDays: 14 + 2 }", 0, true},
		{"dependencies: { minAgeDays: 1e3 }", 0, true},
		{"dependencies: { minAgeDays: 0x10 }", 0, true},
		{"dependencies: { minAgeDays: 9999999999999999999999999999999 }", 0, true},
		{"dependencies: { minAgeDays: 1, minAgeDays: 0 }", 0, true},
		{"dependencies: {}\ndependencies: {}", 0, true},
		{"dependencies: { other: 1 }", 0, true},
		{"dependencies: { minAgeDays: 4", 0, true},
		{"dependencies: 0", 0, true},
		{"dependencies: { minAgeDays: 4 } /*", 0, true},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			cfg, err := parseCueConfig(string(base) + "\n" + tt.raw + "\n")
			if tt.bad {
				if err == nil {
					t.Fatal("expected invalid policy")
				}
				return
			}
			if err != nil || cfg.Dependencies.MinAgeDays != tt.want {
				t.Fatalf("got %+v err %v", cfg.Dependencies, err)
			}
		})
	}
	if err := Validate(Config{Dependencies: DependencyPolicyConfig{MinAgeDays: -1}}); err == nil || !strings.Contains(err.Error(), "minAgeDays") {
		t.Fatal(err)
	}
}

func FuzzDependencyPolicy(f *testing.F) {
	for _, s := range []string{"dependencies: { minAgeDays: 0 }", "", `x: "dependencies: {}"`, `dependencies: { minAgeDays: -1 }`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		cfg, err := ParseDependencies(raw)
		if err == nil && cfg.MinAgeDays < 0 {
			t.Fatal("negative policy accepted")
		}
	})
}
