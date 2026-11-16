// purpose: Group explicit advisory aliases without inventing independent security votes.
// responsibilities: Preserve source records, withdrawal conflicts, malware signals and available fix bounds.
// architecture notes: Only explicit alias relationships join records; related entries remain separate.
package dependencies

import (
	"regexp"
	"sort"
	"strings"
)

var advisoryID = regexp.MustCompile(`^[A-Za-z0-9:_.-]{1,256}$`)

type AdvisoryGroup struct {
	ID      string     `json:"id"`
	IDs     []string   `json:"ids"`
	Malware bool       `json:"malware"`
	Records []Advisory `json:"records"`
}

func groupAdvisories(items []Advisory, depIDs []string) ([]AdvisoryGroup, []string, []string) {
	parent := map[string]string{}
	var find func(string) string
	find = func(id string) string {
		p, ok := parent[id]
		if !ok {
			parent[id] = id
			return id
		}
		if p != id {
			parent[id] = find(p)
		}
		return parent[id]
	}
	join := func(a, b string) {
		aa, bb := find(a), find(b)
		if aa < bb {
			parent[bb] = aa
		} else {
			parent[aa] = bb
		}
	}
	missing := []string{}
	active := map[string]bool{}
	withdrawn := map[string]bool{}
	for _, a := range items {
		if !advisoryID.MatchString(a.ID) {
			missing = append(missing, "ADVISORY_SCHEMA")
			continue
		}
		find(a.ID)
		for _, alias := range a.Aliases {
			if advisoryID.MatchString(alias) {
				join(a.ID, alias)
			} else if alias != "" {
				missing = append(missing, "ADVISORY_SCHEMA")
			}
		}
		if a.Withdrawn == nil {
			active[a.ID] = true
		} else {
			withdrawn[a.ID] = true
		}
	}
	for _, id := range depIDs {
		if !advisoryID.MatchString(id) {
			missing = append(missing, "ADVISORY_SCHEMA")
			continue
		}
		find(id)
	}
	groups := map[string]*AdvisoryGroup{}
	for id := range parent {
		root := find(id)
		if groups[root] == nil {
			groups[root] = &AdvisoryGroup{IDs: []string{}, Records: []Advisory{}}
		}
		groups[root].IDs = append(groups[root].IDs, id)
	}
	for _, a := range items {
		if advisoryID.MatchString(a.ID) {
			g := groups[find(a.ID)]
			g.Records = append(g.Records, a)
		}
	}
	withdrawnGroup := map[string]bool{}
	for root, g := range groups {
		if len(g.Records) == 0 {
			continue
		}
		withdrawnGroup[root] = true
		for _, record := range g.Records {
			if record.Withdrawn == nil {
				withdrawnGroup[root] = false
			}
		}
	}
	for _, id := range depIDs {
		if !advisoryID.MatchString(id) {
			continue
		}
		if withdrawn[id] || withdrawnGroup[find(id)] {
			missing = append(missing, "ADVISORY_WITHDRAWAL_CONFLICT")
		} else {
			active[id] = true
		}
	}
	out := []AdvisoryGroup{}
	reasons := []string{}
	for _, g := range groups {
		ids := []string{}
		for _, id := range g.IDs {
			if active[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			continue
		}
		sort.Strings(ids)
		g.ID = ids[0]
		g.IDs = unique(g.IDs)
		for _, id := range g.IDs {
			g.Malware = g.Malware || strings.HasPrefix(id, "MAL-")
		}
		sort.SliceStable(g.Records, func(i, j int) bool { return g.Records[i].ID < g.Records[j].ID })
		if g.Malware {
			reasons = append(reasons, "KNOWN_MALWARE")
		} else {
			reasons = append(reasons, "KNOWN_VULNERABILITY")
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, unique(reasons), unique(missing)
}
