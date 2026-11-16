package dependencies

import (
	"reflect"
	"slices"
	"testing"
)

func TestAdvisoryAliasGroups(t *testing.T) {
	items := []Advisory{
		{ID: "GHSA-a", Aliases: []string{"CVE-one", ""}},
		{ID: "PYSEC-a", Aliases: []string{"CVE-one"}},
		{ID: "MAL-one", Aliases: []string{"GHSA-b"}},
		{ID: "GHSA-b"},
		{ID: "withdrawn", Withdrawn: &checked},
		{},
	}
	groups, reasons, missing := groupAdvisories(items, []string{"GHSA-a", "CVE-one", "MAL-one", "withdrawn", "deps-only", ""})
	if len(groups) != 3 || !slices.Contains(reasons, "KNOWN_MALWARE") || !slices.Contains(missing, "ADVISORY_SCHEMA") || !slices.Contains(missing, "ADVISORY_WITHDRAWAL_CONFLICT") {
		t.Fatalf("%+v %v %v", groups, reasons, missing)
	}
	var joined *AdvisoryGroup
	for i := range groups {
		if slices.Contains(groups[i].IDs, "PYSEC-a") {
			joined = &groups[i]
		}
	}
	if joined == nil || len(joined.Records) != 2 || !reflect.DeepEqual(joined.IDs, []string{"CVE-one", "GHSA-a", "PYSEC-a"}) {
		t.Fatal(joined)
	}
	groups, reasons, missing = groupAdvisories([]Advisory{{ID: "withdrawn", Withdrawn: &checked}}, nil)
	if len(groups) > 0 || len(reasons) > 0 || len(missing) > 0 {
		t.Fatal(groups, reasons, missing)
	}
	groups, _, _ = groupAdvisories([]Advisory{{ID: "z", Aliases: []string{"a"}}, {ID: "a", Aliases: []string{"z"}}}, nil)
	if len(groups) != 1 {
		t.Fatal("alias cycle", groups)
	}
	groups, reasons, missing = groupAdvisories([]Advisory{{ID: "GHSA-withdrawn", Aliases: []string{"CVE-withdrawn"}, Withdrawn: &checked}}, []string{"CVE-withdrawn"})
	if len(groups) > 0 || len(reasons) > 0 || !slices.Contains(missing, "ADVISORY_WITHDRAWAL_CONFLICT") {
		t.Fatal("withdrawn alias counted as active", groups, reasons, missing)
	}
	_, _, missing = groupAdvisories([]Advisory{{ID: "valid", Aliases: []string{"bad\nidentifier"}}, {ID: "bad\nidentifier"}}, []string{"also bad"})
	if !slices.Contains(missing, "ADVISORY_SCHEMA") {
		t.Fatal("invalid advisory identity accepted")
	}
}
