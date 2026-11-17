package dependencies

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func cargoMaintenanceClient(t *testing.T, mode string) (*Client, *int) {
	t.Helper()
	calls := 0
	ownerCalls := 0
	c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Host != "crates.io" || r.Header.Get("Authorization") != "" {
			t.Fatal("unsafe public request", r.URL)
		}
		if mode == "transport" {
			return nil, errors.New("unavailable")
		}
		if strings.HasSuffix(r.URL.Path, "/owners") {
			ownerCalls++
			owners := []any{map[string]any{"id": 1, "login": "first", "kind": "user"}, map[string]any{"id": 2, "login": "second", "kind": "user"}}
			switch mode {
			case "owner failure":
				return response(`{}`, 403), nil
			case "missing owners":
				return response(`{}`, 200), nil
			case "null owners":
				return response(`{"users":null}`, 200), nil
			case "one owner":
				owners = owners[:1]
			case "duplicate owner":
				owners[1] = owners[0]
			case "owner id":
				owners[0].(map[string]any)["id"] = 0
			case "owner login":
				owners[0].(map[string]any)["login"] = "../bad"
			case "owner kind":
				owners[0].(map[string]any)["kind"] = "unknown"
			case "ambiguous login":
				owners[1].(map[string]any)["login"] = "FIRST"
			case "team":
				owners[1].(map[string]any)["kind"] = "team"
			case "bot":
				owners[1].(map[string]any)["login"] = "automation[bot]"
			case "owner limit":
				for len(owners) < 1001 {
					owners = append(owners, owners[0])
				}
			case "revoked":
				if ownerCalls > 1 {
					owners = owners[:1]
				}
			case "became team":
				if ownerCalls > 1 {
					owners[1].(map[string]any)["kind"] = "team"
				}
			case "recheck failure":
				if ownerCalls > 1 {
					return response(`{}`, 403), nil
				}
			case "renamed":
				if ownerCalls > 1 {
					owners[1].(map[string]any)["login"] = "new-name"
				}
			}
			b, _ := json.Marshal(map[string]any{"users": owners})
			return response(string(b), 200), nil
		}
		if r.URL.RawQuery != "per_page=100&sort=date" || !strings.HasSuffix(r.URL.Path, "/versions") {
			t.Fatal("unexpected activity request", r.URL)
		}
		pub := func(id int64, version string, owner int64, when time.Time) map[string]any {
			return map[string]any{"id": id, "crate": "pkg", "num": version, "created_at": when, "published_by": map[string]any{"id": owner}}
		}
		rows := []any{pub(10, "1.2.0", 1, checked.Add(-24*time.Hour)), pub(11, "1.1.0", 2, checked.Add(-48*time.Hour))}
		switch mode {
		case "publisher failure":
			return response(`{}`, 404), nil
		case "missing publications":
			return response(`{}`, 200), nil
		case "null publications":
			return response(`{"versions":null}`, 200), nil
		case "one publisher":
			rows[1].(map[string]any)["published_by"] = map[string]any{"id": 1}
		case "unrelated publisher":
			rows[1].(map[string]any)["published_by"] = map[string]any{"id": 99, "login": "second"}
		case "null publisher":
			rows[1].(map[string]any)["published_by"] = nil
		case "publisher id":
			rows[1].(map[string]any)["published_by"] = map[string]any{"id": -1}
		case "boundary":
			rows[1].(map[string]any)["created_at"] = checked.AddDate(0, 0, -180)
		case "old":
			rows[1].(map[string]any)["created_at"] = checked.AddDate(0, 0, -181)
		case "future":
			rows[1].(map[string]any)["created_at"] = checked.Add(time.Second)
		case "missing date":
			delete(rows[1].(map[string]any), "created_at")
		case "wrong crate":
			rows[1].(map[string]any)["crate"] = "unrelated"
		case "bad version":
			rows[1].(map[string]any)["num"] = "garbage"
		case "prerelease":
			rows[1].(map[string]any)["num"] = "1.3.0-rc.1"
		case "duplicate publication":
			rows[1] = rows[0]
		case "duplicate version":
			rows[1].(map[string]any)["num"] = "1.2.0"
		case "publication id":
			rows[1].(map[string]any)["id"] = 0
		case "empty":
			rows = []any{}
		case "publication limit":
			for len(rows) < 101 {
				rows = append(rows, rows[0])
			}
		}
		b, _ := json.Marshal(map[string]any{"versions": rows, "meta": map[string]any{"next_page": "https://evil.test/ignored"}})
		return response(string(b), 200), nil
	})}
	return c, &calls
}

func TestCargoPublicMaintenance(t *testing.T) {
	for _, mode := range []string{"normal", "renamed", "prerelease", "boundary", "one owner", "team", "bot", "owner failure", "missing owners", "null owners", "duplicate owner", "owner id", "owner login", "owner kind", "ambiguous login", "owner limit", "publisher failure", "missing publications", "null publications", "one publisher", "unrelated publisher", "null publisher", "publisher id", "old", "future", "missing date", "wrong crate", "bad version", "duplicate publication", "duplicate version", "publication id", "empty", "publication limit", "revoked", "became team", "recheck failure", "transport"} {
		t.Run(mode, func(t *testing.T) {
			c, calls := cargoMaintenanceClient(t, mode)
			sources := []Source{}
			p := DefaultPolicy(14)
			proof := c.cargoMaintainers(context.Background(), "pkg", p, checked, &sources)
			e, ok := proof.evidence(p, checked)
			want := mode == "normal" || mode == "renamed" || mode == "prerelease" || mode == "boundary"
			if ok != want {
				t.Fatal(mode, ok, proof.detail())
			}
			if e == nil || e.Basis != cargoMaintenanceBasis || *calls > 3 {
				t.Fatal("bad evidence or unbounded requests", e, *calls)
			}
			if want && (e.ActiveAuthorityAccounts == nil || *e.ActiveAuthorityAccounts != 2) {
				t.Fatal(e)
			}
			if !want && len(proof.missing) == 0 {
				t.Fatal("incomplete evidence unexplained")
			}
		})
	}
}

func TestCargoMaintenancePolicyAndScope(t *testing.T) {
	c, _ := cargoMaintenanceClient(t, "normal")
	sources := []Source{}
	p := DefaultPolicy(14)
	proof := c.cargoMaintainers(context.Background(), "pkg", p, checked, &sources)
	e := completeTrust()
	e.IndependentMaintainers = nil
	e.cargoMaintenance = proof
	v := Version{Key: VersionKey{System: "CARGO", Name: "pkg", Version: "1.0.0"}, PublishedAt: checked.AddDate(-1, 0, 0)}
	f := evaluateRelease(v, nil, nil, e, p, checked)
	if f.Decision != Allow || f.cargoMaintenance == nil || f.Evidence.Maintenance == nil || f.Evidence.Maintainers != "multiple" {
		t.Fatal(f)
	}
	for _, mode := range []string{"v1", "policy change", "expired", "future proof", "other crate", "npm", "missing identity"} {
		t.Run(mode, func(t *testing.T) {
			q, when, release, trust := p, checked, v, e
			switch mode {
			case "v1":
				q.ID = "conservative-v1"
			case "policy change":
				q.MinAgeDays++
			case "expired":
				when = checked.Add(25 * time.Hour)
			case "future proof":
				when = checked.Add(-time.Second)
			case "other crate":
				release.Key.Name = "other"
			case "npm":
				release.Key.System = "NPM"
			case "missing identity":
				trust.Identity = "unverified"
			}
			f := evaluateRelease(release, nil, nil, trust, q, when)
			if f.Decision == Allow {
				t.Fatal("unbound evidence allowed", f)
			}
		})
	}
	old := p
	old.ID = "conservative-v1"
	if old.Digest() == p.Digest() {
		t.Fatal("policy revision reused digest")
	}
	var nilProof *cargoMaintenanceProof
	if _, ok := nilProof.evidence(p, checked); ok {
		t.Fatal("nil proof accepted")
	}
	c, _ = cargoMaintenanceClient(t, "normal")
	sources = nil
	if _, ok := c.cargoMaintainers(context.Background(), "../bad", p, checked, &sources).evidence(p, checked); ok || len(sources) != 0 {
		t.Fatal("unsafe coordinate fetched")
	}
}

func approvedPublicCargo(t *testing.T) (string, Report, map[string][]byte) {
	t.Helper()
	dir, _, blobs := approvedCrateArtifacts(t, "1.85")
	c := cargoClient(t, dir, "normal")
	base := c.HTTP
	c.GitHubToken = func(context.Context) (string, error) { return "", errors.New("no repository authority") }
	public, _ := cargoMaintenanceClient(t, "normal")
	c.HTTP = doFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "crates.io" && (strings.HasSuffix(req.URL.Path, "/owners") || strings.HasSuffix(req.URL.Path, "/versions")) {
			resp, err := public.HTTP.Do(req)
			if err != nil {
				return nil, err
			}
			b, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, err
			}
			if strings.Contains(req.URL.Path, "/child/") {
				b = []byte(strings.ReplaceAll(string(b), `"crate":"pkg"`, `"crate":"child"`))
			}
			return response(string(b), 200), nil
		}
		resp, err := base.Do(req)
		if err != nil || req.URL.Host != "crates.io" || strings.HasSuffix(req.URL.Path, "/dependencies") {
			return resp, err
		}
		defer resp.Body.Close()
		var data map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			t.Fatal(err)
		}
		version := data["version"].(map[string]any)
		h := sha256.Sum256(blobs[version["crate"].(string)])
		version["checksum"], version["rust_version"], version["edition"] = hex.EncodeToString(h[:]), "1.85", "2021"
		b, _ := json.Marshal(data)
		return response(string(b), 200), nil
	})
	r := c.AssessLocal(context.Background(), dir, "cargo", "pkg", nil, DefaultPolicy(14), checked)
	if r.Decision != Allow {
		b, _ := json.Marshal(r)
		t.Fatal("public authority fixture blocked", string(b))
	}
	return dir, r, blobs
}

func TestCargoMaintenanceAssessmentAndSeal(t *testing.T) {
	dir, r, blobs := approvedPublicCargo(t)
	if len(r.Details.Maintenance) != 2 || r.Evidence.Maintenance == nil || r.Policy != publicMaintenancePolicy {
		t.Fatal(r)
	}
	artifacts, err := PrepareCargoArtifacts(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range artifacts {
		if _, err := VerifyCargoArtifact(blobs[a.Node.Name], a); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"missing proof", "different proof", "changed summary", "changed finding", "changed policy"} {
		t.Run(mode, func(t *testing.T) {
			_, report, _ := approvedPublicCargo(t)
			switch mode {
			case "missing proof":
				report.Details.Project.Findings[0].Finding.cargoMaintenance = nil
			case "different proof":
				report.Details.Project.Findings[0].Finding.cargoMaintenance.name = "other"
			case "changed summary":
				report.Evidence.Maintenance = nil
			case "changed finding":
				report.Details.Project.Findings[0].Finding.Evidence.Maintenance.Basis = "forged"
			case "changed policy":
				report.PolicyDigest = "changed"
			}
			if _, err := PrepareCargoArtifacts(dir, report); err == nil {
				t.Fatal("forged approval accepted")
			}
		})
	}
	before := artifacts[0].expected.maintenance.clone()
	for i := range r.Details.Project.Findings {
		r.Details.Project.Findings[i].Finding.cargoMaintenance.owners[0].ID = 99
	}
	if !reflect.DeepEqual(before, artifacts[0].expected.maintenance) || !artifacts[0].valid() {
		t.Fatal("prepared proof was mutable")
	}
}

func TestCargoMaintenanceCache(t *testing.T) {
	c, calls := cargoMaintenanceClient(t, "normal")
	p := DefaultPolicy(14)
	r := NewReport("cargo", "pkg", nil, p, checked)
	cache := map[string]TrustEvidence{}
	for i := 0; i < 2; i++ {
		e := TrustEvidence{}
		c.fillMaintenance(context.Background(), "cargo", "pkg", &e, p, checked, &r, cache)
		if _, ok := e.cargoMaintenance.evidence(p, checked); !ok {
			t.Fatal("cache lost proof")
		}
	}
	if *calls != 3 || len(r.Details.Maintenance) != 1 {
		t.Fatal("duplicate package fetch", *calls)
	}
}

func TestCargoPublicMaintenanceInstallPlan(t *testing.T) {
	dir, r, blobs := approvedPublicCargo(t)
	installCargoSource(t, dir)
	plan, err := PrepareCargoInstall(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := PrepareCargoStage(dir, privateCargoStageDir(t), r)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.UseInstallPlan(plan); err != nil {
		t.Fatal(err)
	}
	for _, a := range stage.Artifacts() {
		if err := stage.AddArtifact(blobs[a.Node.Name], a); err != nil {
			t.Fatal(err)
		}
	}
	if !plan.Unchanged(dir) {
		t.Fatal("public authority plan changed source inputs")
	}
}
