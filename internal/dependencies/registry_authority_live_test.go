package dependencies

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestPublicRegistryAuthorityLive(t *testing.T) {
	if os.Getenv("GH_FLAREBYTE_DEP_LIVE") != "1" {
		t.Skip("opt-in anonymous npm/PyPI registry role checks")
	}
	for _, test := range []struct{ system, name string }{{"npm", "lodash"}, {"npm", "is-number"}, {"pypi", "requests"}, {"pypi", "attrs"}} {
		t.Run(test.system+"/"+test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			sources := []Source{}
			o := NewClient().registryAuthorities(withBudget(ctx), test.system, test.name, &sources)
			if !o.known || len(sources) != 1 || sources[0].Status != "ok" {
				t.Fatal("public role contract unavailable", o.detail(), sources)
			}
			if o.evidence().ActiveAuthorityAccounts != nil {
				t.Fatal("unproven activity emitted")
			}
			b, err := json.Marshal(struct {
				Detail  MaintenanceDetail `json:"detail"`
				Sources []Source          `json:"sources"`
			}{o.detail(), sources})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("LIVE_REGISTRY_AUTHORITY %s", b)
		})
	}
}
