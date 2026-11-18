package dependencies

import (
	"strings"
	"testing"
)

func TestPythonMarkerRange(t *testing.T) {
	for _, tt := range []struct {
		marker string
		want   pythonMarkerState
	}{
		{`python_version >= '3.8'`, markerTrue},
		{`python_version > '3.8'`, markerTrue},
		{`python_version < '3.9'`, markerFalse},
		{`python_version <= '3.8'`, markerFalse},
		{`python_version == '3.8'`, markerFalse},
		{`python_version != '3.8'`, markerTrue},
		{`python_version >= '3.10'`, markerUnknown},
		{`python_version == '3.9'`, markerUnknown},
		{`python_version != '3.9'`, markerUnknown},
		{`python_version <= '3.9'`, markerUnknown},
		{`python_version > '3.9'`, markerUnknown},
		{`'3.8' < python_version`, markerTrue},
		{`'3.9' > python_version`, markerFalse},
		{`'3.8' <= python_version`, markerTrue},
		{`'3.8' >= python_version`, markerFalse},
		{`python_full_version >= '3.9.1'`, markerTrue},
		{`python_full_version < '3.9.1'`, markerFalse},
		{`python_full_version == '3.9.0'`, markerFalse},
		{`sys_platform == 'win32'`, markerUnknown},
		{`extra == 'security'`, markerUnknown},
		{`sys_platform == 'win32' and python_version < '3.8'`, markerFalse},
		{`sys_platform == 'win32' or python_version >= '3.8'`, markerTrue},
		{`python_version < '3.8' or sys_platform == 'win32'`, markerUnknown},
		{`python_version >= '3.8' and sys_platform == 'win32'`, markerUnknown},
		{`python_version < '3.8' or python_version < '3.7'`, markerFalse},
		{`python_version >= '3.8' and python_version >= '3.9'`, markerTrue},
		{`(python_version < '3.8' or python_version >= '3.8') and python_version >= '3.9'`, markerTrue},
		{`python_version >= '3.8' or (sys_platform == 'win32' and python_version < '3.8')`, markerTrue},
	} {
		t.Run(tt.marker, func(t *testing.T) {
			if got := pythonMarkerRange(tt.marker, ">=3.9.1"); got != tt.want {
				t.Fatal(got, tt.want)
			}
		})
	}
	for _, marker := range []string{`python_version >= '3.8'`, `python_full_version >= '3.9.1'`} {
		if got := pythonMarkerRange(marker, ">=3.8,<4"); got != markerUnknown {
			t.Fatal("complex baseline guessed", got)
		}
	}
}

func TestPythonMarkerMalformedAndBounds(t *testing.T) {
	for _, raw := range []string{
		"", `python_version >= '3.8' or`, `python_version >= '3.8' and ()`,
		`python_version >= '3.8' or (sys_platform == 'win32'`,
		`python_version >= '3.8')`, `python_version >= '3.8' or unknown == 'x'`,
		`python_version >= '3.8' or python_version == 'invalid'`,
		`python_version >= '3.8' or python_version ~= '3.9'`,
		`python_version >= '3.8' or python_version in '3.9'`,
		`python_version >= '3.8' or 'x' == 'x'`,
		`python_version >= '3.8' or python_version == sys_platform`,
		`python_version >= '3.8' or sys_platform == 'unterminated`,
		`python_version >= '3.8' or __import__('os')`,
		`python_version >= '3.8' or sys_platform == 'x\y'`,
		"python_version >= '3.8'\n", strings.Repeat("(", 9) + `python_version >= '3.8'` + strings.Repeat(")", 9),
		strings.Repeat("x", 513),
	} {
		if got := pythonMarkerRange(raw, ">=3.9"); got != markerUnknown {
			t.Fatal(raw, got)
		}
	}
	if _, _, got := pythonMarkedRequirement(`missing[extra]; python_version < '3.8'`, ">=3.9"); got != markerUnknown {
		t.Fatal("unsupported requirement skipped")
	}
	if _, _, got := pythonMarkedRequirement(`missing; python_version < '3.8'`, ">=3.9"); got != markerFalse {
		t.Fatal(got)
	}
}
