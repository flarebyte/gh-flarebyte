package config

import (
	"os"
	"testing"
)

func TestCCConfiguration(t *testing.T) {
	base, err := os.ReadFile("testdata/valid.cue")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		extra string
		want  int
	}{
		{"", 10}, {"cc: {}", 10}, {"cc: { minLines: 15 }", 15},
		{`cc: "bad"`, 0}, {"cc: { minLines: 0 }", 0}, {"cc: { minLines: -2 }", 0},
		{"cc: { minLines: 1.5 }", 0}, {`cc: { minLines: "10" }`, 0},
		{"cc: { minLines: 10, minLines: 20 }", 0}, {"cc: { other: 10 }", 0},
		{"cc: { minLines: 10 + 2 }", 0},
	} {
		t.Run(tc.extra, func(t *testing.T) {
			cfg, err := parseCueConfig(string(base) + "\n" + tc.extra + "\n")
			if tc.want == 0 {
				if err == nil {
					t.Fatal("expected invalid config")
				}
				return
			}
			if err != nil || cfg.CC.MinLines != tc.want {
				t.Fatalf("config=%+v error=%v", cfg.CC, err)
			}
		})
	}
}
