package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubBranchAPI(t *testing.T, fn func(string, string, []byte) ([]byte, error)) {
	t.Helper()
	old := branchAPI
	t.Cleanup(func() { branchAPI = old })
	branchAPI = fn
}

func branchPayload(t *testing.T, r branchRuleset) []byte {
	t.Helper()
	data, err := json.Marshal(branchRulesetPayload{branchRulesetDefinition: r.Definition, ID: r.ID, Source: r.Source, SourceType: r.SourceType})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestBranchProtectionAPIRead(t *testing.T) {
	remote := testBranchRulesets(t)
	calls := 0
	stubBranchAPI(t, func(method, path string, body []byte) ([]byte, error) {
		calls++
		if method != "GET" || body != nil {
			t.Fatal("read issued a write")
		}
		switch path {
		case "repos/owner/repo/rulesets?includes_parents=false&per_page=100&page=1":
			// Return 100 summaries to require a second page. Unmanaged entries
			// must not cause individual fetches or be selected for modification.
			items := []map[string]any{{"id": 1, "name": branchProtectionRulesetName, "source": "owner/repo", "source_type": "Repository"}}
			for i := 1; i < 100; i++ {
				items = append(items, map[string]any{"id": i + 10, "name": fmt.Sprintf("manual-%d", i)})
			}
			return json.Marshal(items)
		case "repos/owner/repo/rulesets?includes_parents=false&per_page=100&page=2":
			return []byte("[]"), nil
		case "repos/owner/repo/rulesets/1":
			return branchPayload(t, remote[0]), nil
		default:
			t.Fatalf("unexpected path %s", path)
			return nil, nil
		}
	})
	got, err := ghReadBranchRulesets("owner/repo")
	if err != nil || len(got) != 1 || got[0].ID != 1 || calls != 3 {
		t.Fatalf("read: %+v %v calls=%d", got, err, calls)
	}
	equal, err := equalBranchRulesets(got[0].Definition, remote[0].Definition)
	if !equal || err != nil {
		t.Fatalf("definition lost: %+v %v", got, err)
	}
}

func TestBranchProtectionAPIWrite(t *testing.T) {
	def := testBranchRulesets(t)[1].Definition
	for _, op := range []string{"create", "update", "delete"} {
		t.Run(op, func(t *testing.T) {
			calls := 0
			stubBranchAPI(t, func(method, path string, body []byte) ([]byte, error) {
				calls++
				wantMethod, wantPath := "POST", "repos/owner/repo/rulesets"
				if op == "update" {
					wantMethod, wantPath = "PUT", wantPath+"/42"
				}
				if op == "delete" {
					wantMethod, wantPath = "DELETE", wantPath+"/42"
				}
				if method != wantMethod || path != wantPath {
					t.Fatalf("wrong request %s %s", method, path)
				}
				if op == "delete" {
					if body != nil {
						t.Fatal("DELETE must not send a body")
					}
				} else {
					var got branchRulesetDefinition
					if err := json.Unmarshal(body, &got); err != nil {
						t.Fatal(err)
					}
					equal, err := equalBranchRulesets(got, def)
					if err != nil || !equal {
						t.Fatalf("wrong payload %s", body)
					}
					for _, key := range []string{`"id":`, `"source":`, `"source_type":`} {
						if strings.Contains(string(body), key) {
							t.Fatalf("read-only metadata in payload: %s", body)
						}
					}
				}
				return nil, nil
			})
			if err := ghApplyBranchRulesetChange("owner/repo", branchRulesetChange{Operation: op, ID: 42, Definition: def}); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

func TestBranchProtectionAPIErrors(t *testing.T) {
	for _, response := range []string{"null", "{}", "invalid"} {
		t.Run(response, func(t *testing.T) {
			stubBranchAPI(t, func(string, string, []byte) ([]byte, error) { return []byte(response), nil })
			if _, err := ghReadBranchRulesets("owner/repo"); err == nil {
				t.Fatal("invalid list accepted")
			}
			if _, err := ghReadActionsAppID(); err == nil {
				t.Fatal("invalid app accepted")
			}
		})
	}
	stubBranchAPI(t, func(string, string, []byte) ([]byte, error) { return nil, errors.New("403 forbidden") })
	if _, err := ghReadBranchRulesets("owner/repo"); err == nil {
		t.Fatal("read error ignored")
	}
	if err := ghApplyBranchRulesetChange("owner/repo", branchRulesetChange{Operation: "delete", ID: 1}); err == nil {
		t.Fatal("write error ignored")
	}
	for _, change := range []branchRulesetChange{{Operation: "delete"}, {Operation: "update"}, {Operation: "unknown"}} {
		if err := ghApplyBranchRulesetChange("owner/repo", change); err == nil {
			t.Fatal("invalid change accepted")
		}
	}
	if _, err := ghReadBranchRulesets("owner/repo?query=1"); err == nil {
		t.Fatal("invalid repo accepted")
	}
}

func TestBranchProtectionAPIIncompleteOrChanged(t *testing.T) {
	for _, scenario := range []string{"no bypass visibility", "wrong identity", "foreign source", "missing rules"} {
		t.Run(scenario, func(t *testing.T) {
			rule := testBranchRulesets(t)[0]
			summary := branchPayload(t, rule)
			switch scenario {
			case "no bypass visibility":
				rule.Definition.BypassActors = nil
			case "wrong identity":
				rule.ID = 999
			case "foreign source":
				rule.Source = "another/repo"
			case "missing rules":
				rule.Definition.Rules = nil
			}
			stubBranchAPI(t, func(_, path string, _ []byte) ([]byte, error) {
				if strings.Contains(path, "?") {
					return append(append([]byte{'['}, summary...), ']'), nil
				}
				return branchPayload(t, rule), nil
			})
			if _, err := ghReadBranchRulesets("owner/repo"); err == nil {
				t.Fatal("unsafe remote definition accepted")
			}
		})
	}
}

func TestBranchProtectionAPIAppIdentity(t *testing.T) {
	stubBranchAPI(t, func(method, path string, body []byte) ([]byte, error) {
		if method != "GET" || path != "apps/github-actions" || body != nil {
			t.Fatalf("wrong app request %s %s", method, path)
		}
		return []byte(`{"id":123}`), nil
	})
	id, err := ghReadActionsAppID()
	if err != nil || id != 123 {
		t.Fatalf("app: %d %v", id, err)
	}
}

func TestBranchProtectionAPITransport(t *testing.T) {
	root := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$BRANCH_TEST_ARGS\"\ncat > \"$BRANCH_TEST_BODY\"\nprintf '{\"ok\":true}'\n"
	if err := os.WriteFile(filepath.Join(root, "gh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BRANCH_TEST_ARGS", filepath.Join(root, "args"))
	t.Setenv("BRANCH_TEST_BODY", filepath.Join(root, "body"))
	t.Setenv("GH_FLAREBYTE_FAKE_READONLY", "")
	data, err := ghBranchAPI("POST", "repos/owner/repo/rulesets", []byte(`{"name":"example"}`))
	if err != nil || string(data) != `{"ok":true}` {
		t.Fatalf("transport: %s %v", data, err)
	}
	args, err := os.ReadFile(filepath.Join(root, "args"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--input\n-\n") || !strings.Contains(string(args), "--method\nPOST\n") {
		t.Fatalf("wrong gh args: %s", args)
	}
	body, err := os.ReadFile(filepath.Join(root, "body"))
	if err != nil || string(body) != `{"name":"example"}` {
		t.Fatalf("stdin: %s %v", body, err)
	}
	if err := os.WriteFile(filepath.Join(root, "gh"), []byte("#!/bin/sh\necho 'HTTP 403 forbidden' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ghBranchAPI("GET", "repos/owner/repo/rulesets", nil); err == nil || !strings.Contains(err.Error(), "403 forbidden") || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("missing error context: %v", err)
	}
	t.Setenv("GH_FLAREBYTE_FAKE_READONLY", "1")
	if _, err := ghBranchAPI("DELETE", "repos/owner/repo/rulesets/1", nil); err == nil {
		t.Fatal("fake readonly attempted branch API")
	}
}
