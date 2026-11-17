package dependencies

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Public GETs only. No GitHub/registry credentials, archives or native tools.
func TestCargoPublicMaintenanceLive(t *testing.T) {
	if os.Getenv("GH_FLAREBYTE_DEP_LIVE") != "1" {
		t.Skip("opt-in anonymous registry maintenance checks")
	}
	for _, name := range []string{"autocfg", "unicode-ident", "libc"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			sources := []Source{}
			p := DefaultPolicy(14)
			now := time.Now().UTC()
			proof := NewClient().cargoMaintainers(withBudget(ctx), name, p, now, &sources)
			if !proof.ownersKnown {
				t.Fatal("live owner contract unavailable", proof.detail(), sources)
			}
			for _, missing := range proof.missing {
				if missing == "CARGO_PUBLISHER_LOOKUP" || missing == "CARGO_PUBLISHER_SCHEMA" || missing == "CARGO_OWNER_RECHECK" {
					t.Fatal("live publisher contract unavailable", proof.detail(), sources)
				}
			}
			_, qualified := proof.evidence(p, now)
			data, err := json.Marshal(struct {
				Policy    string            `json:"policy"`
				Qualified bool              `json:"qualified"`
				Detail    MaintenanceDetail `json:"detail"`
				Sources   []Source          `json:"sources"`
			}{p.ID, qualified, proof.detail(), sources})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("LIVE_CARGO_MAINTENANCE %s", data)
		})
	}
}
