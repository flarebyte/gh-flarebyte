package dependencies

import (
	"slices"
	"testing"
)

func TestPythonRuntimeCompatibility(t *testing.T) {
	for _, tt := range []struct {
		name, manifest, lock, wheel, status, reason string
		missing                                     bool
	}{
		{"match", ">=3.9", ">=3.8", ">=3.8,<4.0", "compatible", "", false},
		{"patch", ">=3.9.1", "", ">=3.9.2", "incompatible", "PYTHON_VERSION_INCOMPATIBLE", false},
		{"upper", ">=3.12", "", "<3.12", "incompatible", "PYTHON_VERSION_INCOMPATIBLE", false},
		{"excluded", ">=3.9", "", "!=3.9.0", "incompatible", "PYTHON_VERSION_INCOMPATIBLE", false},
		{"lock", ">=3.9", ">=3.10", ">=3.8", "incompatible", "PYTHON_LOCK_BASELINE_INCOMPATIBLE", false},
		{"unconstrained", ">=3.9", "", "", "compatible", "", false},
		{"no baseline", "", "", ">=3.8", "unknown", "", true},
		{"complex baseline", ">=3.9,<4", "", ">=3.8", "unknown", "", true},
		{"wildcard", ">=3.9", "", "==3.9.*", "unknown", "", true},
		{"compatible release", ">=3.9", "", "~=3.9", "unknown", "", true},
		{"invalid trailing", ">=3.9", "", ">=3.10,bad", "unknown", "", true},
		{"invalid lock", ">=3.9", "bad", ">=3.8", "unknown", "", true},
		{"leading zero", ">=03.9", "", ">=3.8", "unknown", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := Finding{}
			checkPythonCompatibility(tt.manifest, tt.lock, &tt.wheel, &f)
			if f.Runtime == nil || f.Runtime.Status != tt.status || slices.Contains(f.Missing, "RUNTIME_COMPATIBILITY") != tt.missing {
				t.Fatal(f)
			}
			if tt.reason != "" && !slices.Contains(f.Reasons, tt.reason) || tt.reason == "" && len(f.Reasons) > 0 {
				t.Fatal(f)
			}
		})
	}
	f := Finding{}
	checkPythonCompatibility(">=3.9", "", nil, &f)
	if f.Runtime.Status != "unknown" || !slices.Contains(f.Missing, "RUNTIME_COMPATIBILITY") {
		t.Fatal(f)
	}
}

func TestPythonRuntimeVersionAndSpecifiers(t *testing.T) {
	for _, raw := range []string{"", "3", "3.9.0.1", "3.9rc1", "3.09", "3.-1", "3.9+local", "3.9\n", "3.999999999999999999999999999"} {
		if _, ok := pythonRuntimeVersion(raw); ok {
			t.Fatal(raw)
		}
	}
	base, _ := pythonRuntimeVersion("3.10.1")
	for _, spec := range []string{">3.10", "<=3.10.1", "==3.10.1", "!=3.9", ">= 3.8, < 4.0"} {
		if match, ok := pythonRequirementMatches(spec, base); !ok || !match {
			t.Fatal(spec)
		}
	}
	for _, spec := range []string{"", "3.10", "===3.10", ">=3.9,", ">3.10 || <3.8"} {
		if _, ok := pythonRequirementMatches(spec, base); ok {
			t.Fatal(spec)
		}
	}
}
