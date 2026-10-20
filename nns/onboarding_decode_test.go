package nns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aviate-labs/agent-go/candid"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/registry"
)

// onboardingConfig writes a resources.hcl carrying the onboarding resources
// plus the given proposals.hcl body, and loads the named proposal out of it.
func onboardingConfig(t *testing.T, proposals string) (Action, error) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("resources.hcl", `
node_provider "np" {
  id = "`+fxProvider+`"
}
node_operator "op" {
  id = "`+fxOperator+`"
}
data_center "zh3" {
  id     = "zh3"
  region = "Europe,CH,Zurich"
}
`)
	write("proposals.hcl", proposals)
	spec, err := LoadSpec(filepath.Join(dir, "proposals.hcl"), "p")
	if err != nil {
		return nil, err
	}
	return spec.Action()
}

const (
	fxProvider = "dchi6-uidam-bqgay-dambq-gayda-mbqga-ydamb-qgayd-ambqg-aydam-bqg"
	fxOperator = "hsi7b-rl4wt-lum3m-ophfi-oxgx5-u2q7r-ak7ag-nyiik-c4gam-epo2r-3qe"
)

// The region lives in the data_center resource, so an add_data_centers proposal
// references it rather than restating it where the two can drift apart.
func TestAddDataCentersReferencesResourceRegion(t *testing.T) {
	action, err := onboardingConfig(t, `
proposal "p" {
  kind  = "add_data_centers"
  title = "Add"

  add_data_centers {
    dc {
      id     = data_center.zh3.id
      region = data_center.zh3.region
      owner  = "Cloud Engines AG"
    }
  }
}
`)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	dcs := action.(AddDataCentersAction).DataCenters
	if len(dcs) != 1 || dcs[0].Region != "Europe,CH,Zurich" {
		t.Fatalf("region should resolve from the resource, got %+v", dcs)
	}
}

// The canister lowercases both ids on the way in, so alpage does it up front
// and the submitted payload matches the record that lands.
func TestOnboardingLowercasesDataCenterIDs(t *testing.T) {
	add, err := onboardingConfig(t, `
proposal "p" {
  kind  = "add_data_centers"
  title = "Add"

  add_data_centers {
    dc {
      id     = "ZH3"
      region = "Europe,CH,Zurich"
      owner  = "Cloud Engines AG"
    }
  }
}
`)
	if err != nil {
		t.Fatalf("load add_data_centers: %v", err)
	}
	if got := add.(AddDataCentersAction).DataCenters[0].ID; got != "zh3" {
		t.Errorf("data center id = %q, want zh3", got)
	}

	op, err := onboardingConfig(t, `
proposal "p" {
  kind  = "add_node_operator"
  title = "Add"

  add_node_operator {
    node_operator_id = node_operator.op.id
    node_provider_id = node_provider.np.id
    dc_id            = "ZH3"
  }
}
`)
	if err != nil {
		t.Fatalf("load add_node_operator: %v", err)
	}
	if got := op.(AddNodeOperatorAction).DcID; got != "zh3" {
		t.Errorf("operator dc_id = %q, want zh3", got)
	}
}

// Governance validates the account identifier's checksum at submission, so a
// typo has to fail at load instead of after a round trip to the NNS.
func TestAddNodeProviderRejectsBadRewardAccount(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account string
	}{
		{"bad checksum", strings.Repeat("01", 32)},
		{"too short", strings.Repeat("01", 16)},
		{"not hex", strings.Repeat("zz", 32)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := onboardingConfig(t, `
proposal "p" {
  kind  = "add_node_provider"
  title = "Add"

  add_node_provider {
    node_provider_id = node_provider.np.id
    reward_account   = "`+tc.account+`"
  }
}
`)
			if err == nil {
				t.Fatal("a malformed reward_account must fail at load")
			}
			if !strings.Contains(err.Error(), "reward_account") {
				t.Errorf("error should name the field, got %v", err)
			}
		})
	}
}

// A valid account identifier survives the round trip into the payload.
func TestAddNodeProviderAcceptsDerivedRewardAccount(t *testing.T) {
	want := principal.NewAccountID(mintPrincipal("reward account fixture"), principal.DefaultSubAccount)
	action, err := onboardingConfig(t, `
proposal "p" {
  kind  = "add_node_provider"
  title = "Add"

  add_node_provider {
    node_provider_id = node_provider.np.id
    reward_account   = "`+want.Encode()+`"
  }
}
`)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := action.(AddNodeProviderAction).RewardAccount
	if len(got) != 32 {
		t.Fatalf("reward account should be the 32 byte form, got %d bytes", len(got))
	}
}

// The canister stores coordinates without range-checking them, so a transposed
// pair would be accepted on-chain. Rejected at load instead.
func TestAddDataCentersRejectsOutOfRangeGPS(t *testing.T) {
	_, err := onboardingConfig(t, `
proposal "p" {
  kind  = "add_data_centers"
  title = "Add"

  add_data_centers {
    dc {
      id     = "zh3"
      region = "Europe,CH,Zurich"
      owner  = "Cloud Engines AG"

      gps {
        latitude  = 147.3769
        longitude = 8.5417
      }
    }
  }
}
`)
	if err == nil || !strings.Contains(err.Error(), "latitude") {
		t.Fatalf("an out-of-range latitude must fail at load, got %v", err)
	}
}

// rewardable_nodes is the field the canister calls the old one, so alpage sends
// it empty and exposes only max_rewardable_nodes.
func TestAddNodeOperatorPayloadOmitsLegacyRewardableNodes(t *testing.T) {
	action, err := onboardingConfig(t, `
proposal "p" {
  kind  = "add_node_operator"
  title = "Add"

  add_node_operator {
    node_operator_id     = node_operator.op.id
    node_provider_id     = node_provider.np.id
    dc_id                = data_center.zh3.id
    max_rewardable_nodes = { "type3.1" = 28 }
  }
}
`)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	blob, err := action.PayloadBlob()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var p registry.AddNodeOperatorPayload
	if err := candid.Unmarshal(blob, []any{&p}); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(p.RewardableNodes) != 0 {
		t.Errorf("rewardable_nodes should be sent empty, got %+v", p.RewardableNodes)
	}
	if p.MaxRewardableNodes == nil || len(*p.MaxRewardableNodes) != 1 {
		t.Fatalf("max_rewardable_nodes should carry the declared map, got %+v", p.MaxRewardableNodes)
	}
	if got := (*p.MaxRewardableNodes)[0]; got.Field0 != "type3.1" || got.Field1 != 28 {
		t.Errorf("max_rewardable_nodes = %+v, want type3.1 -> 28", got)
	}
}

// Go map order is randomised, so an unsorted reward map would change the
// payload hash on every reload and state would read it as drift.
func TestAddNodeOperatorPayloadHashIsStable(t *testing.T) {
	action := AddNodeOperatorAction{
		Meta:       Meta{Title: "stable"},
		OperatorID: mintPrincipal("op"),
		ProviderID: mintPrincipal("np"),
		DcID:       "zh3",
		MaxRewardableNodes: map[string]uint32{
			"type3.1": 28, "type1.1": 4, "type2": 7, "type0": 1,
		},
	}
	first, err := payloadSHA256(action)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	for range 20 {
		again, err := payloadSHA256(action)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		if again != first {
			t.Fatalf("payload hash is not stable across encodes: %s then %s", first, again)
		}
	}
}

// Seeding the provider unconditionally would make the dry run pass for a
// provider mainnet does not have, which is the case governance rejects.
func TestAddNodeOperatorSeedsProviderOnlyWhenRegistered(t *testing.T) {
	action := AddNodeOperatorAction{
		OperatorID: mintPrincipal("op"),
		ProviderID: mintPrincipal("np"),
		DcID:       "zh3",
	}
	if seeds := action.DryRunSeeds(nil).Providers; len(seeds) != 0 {
		t.Errorf("an unregistered provider must not be seeded, got %+v", seeds)
	}
	registered := action.DryRunSeeds([]string{action.ProviderID.Encode()}).Providers
	if len(registered) != 1 {
		t.Fatalf("a registered provider must be seeded, got %+v", registered)
	}
	// The data center always comes along, since the invariant needs it either way.
	if dcs := action.DryRunSeeds(nil).DataCenters; len(dcs) != 1 || dcs[0].ID != "zh3" {
		t.Errorf("the data center must always be seeded, got %+v", dcs)
	}
}

// The add is only meaningful if the target is absent, so seeding it is what
// makes the duplicate case fail locally the way it would on mainnet.
func TestAddNodeProviderSeedsOnlyExistingTarget(t *testing.T) {
	action := AddNodeProviderAction{ProviderID: mintPrincipal("np")}
	if seeds := action.DryRunSeeds(nil).Providers; len(seeds) != 0 {
		t.Errorf("a provider mainnet does not have must not be seeded, got %+v", seeds)
	}
	if seeds := action.DryRunSeeds([]string{action.ProviderID.Encode()}).Providers; len(seeds) != 1 {
		t.Errorf("a provider mainnet already has must be seeded, got %+v", seeds)
	}
}
