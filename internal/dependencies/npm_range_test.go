package dependencies

import (
	"strings"
	"testing"
)

func TestNPMPartialAndAlternativeRanges(t *testing.T) {
	for _, test := range []struct {
		rangeText, version string
		match, known       bool
	}{
		{"1", "1.9.9", true, true},
		{"1", "2.0.0", false, true},
		{"1.2", "1.2.9", true, true},
		{"1.2", "1.3.0", false, true},
		{"1.2.x", "1.2.0+build", true, true},
		{"1.X.*", "1.9.0", true, true},
		{"x", "999.0.0", true, true},
		{"*.*.*", "0.0.0", true, true},
		{"1.x || >=2.5.0 <3.0.0", "2.6.0", true, true},
		{"1.x || >=2.5.0 <3.0.0", "2.4.0", false, true},
		{"1.0.0 || latest", "1.0.0", false, false},
		{"latest || 1.0.0", "1.0.0", false, false},
		{"1.0.0 ||", "1.0.0", false, false},
		{"|| 1.0.0", "1.0.0", false, false},
		{"1.x.3", "1.2.3", false, false},
		{"1..x", "1.2.3", false, false},
		{"01.x", "1.2.3", false, false},
		{"1.2.3.4", "1.2.3", false, false},
		{"18446744073709551616.x", "1.2.3", false, false},
		{">=1.x", "1.2.3", false, false},
		{"~1.2", "1.2.3", false, false},
		{"1.x", "1.2.3-beta", false, false},
		{strings.Repeat("1", 4097), "1.0.0", false, false},
		{strings.Repeat("1 || ", 32) + "1", "1.0.0", false, false},
	} {
		t.Run(test.rangeText, func(t *testing.T) {
			match, known := npmSatisfies(test.rangeText, test.version)
			if match != test.match || known != test.known {
				t.Fatalf("got (%v, %v), want (%v, %v)", match, known, test.match, test.known)
			}
		})
	}
}
