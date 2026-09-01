package nns

import (
	"fmt"

	"github.com/aviate-labs/agent-go/candid"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/governance"
	"github.com/swiss-subnet/alpage/pocketic"
)

const installCycles = 100_000_000_000_000

// NNS holds a brought-up NNS on a PocketIC instance.
type NNS struct {
	c        *pocketic.Client
	inst     int
	Proposer principal.Principal // controller/sender for proposal submission
	Hotkey   principal.Principal // optional hotkey on the proposer neuron (empty if none)
	// Members maps a seeded subnet's textual id to the node principals the seed
	// generated for it (empty unless brought up with subnet seeds).
	Members map[string][]principal.Principal
}

// BringUp installs registry, root, and governance(_test) with the proposer
// neuron controlled by controller.
func BringUp(c *pocketic.Client, inst int, controller principal.Principal) (*NNS, error) {
	return BringUpWithHotkey(c, inst, controller, principal.Principal{})
}

// BringUpWithHotkey is BringUp, additionally registering hotkey on the proposer
// neuron so proposals can be submitted by the hotkey principal rather than the
// controller. Empty principal means no hotkey.
func BringUpWithHotkey(c *pocketic.Client, inst int, controller, hotkey principal.Principal) (*NNS, error) {
	return BringUpWithSubnets(c, inst, controller, hotkey, nil)
}

// BringUpWithSubnets is BringUpWithHotkey with the local registry pre-seeded so
// get_subnet resolves the given subnets (membership + replica version). Use it
// to test the read paths (FetchSubnet*, Preflight) against realistic state.
func BringUpWithSubnets(c *pocketic.Client, inst int, controller, hotkey principal.Principal, seeds []SubnetSeed) (*NNS, error) {
	return BringUpWithSubnetsAndProviders(c, inst, controller, hotkey, seeds, nil)
}

// BringUpWithSubnetsAndProviders is BringUpWithSubnets plus node
// provider/operator/dc records, so get_node_operators_and_dcs_of_node_provider
// resolves for tests.
func BringUpWithSubnetsAndProviders(c *pocketic.Client, inst int, controller, hotkey principal.Principal, seeds []SubnetSeed, providers []ProviderSeed) (*NNS, error) {
	return BringUpWith(c, inst, controller, BringUpOptions{Hotkey: hotkey, Subnets: seeds, Providers: providers})
}

// BringUpOptions is the full set of dry-run state knobs. Mirror carries records
// copied verbatim from a live IC (see mirror.go); the fabricated seeds and the
// mirror coexist, so a mirrored application subnet can rely on a seeded system
// subnet to satisfy the init invariants.
type BringUpOptions struct {
	Hotkey    principal.Principal
	Subnets   []SubnetSeed
	Providers []ProviderSeed
	Mirror    *Mirror
}

// BringUpWith is the full bring-up the other entry points delegate to.
func BringUpWith(c *pocketic.Client, inst int, controller principal.Principal, opts BringUpOptions) (*NNS, error) {
	hotkey, seeds, providers := opts.Hotkey, opts.Subnets, opts.Providers
	w, err := wasmPathsFromEnv()
	if err != nil {
		return nil, err
	}

	type unit struct {
		id   principal.Principal
		wasm string
		arg  []byte
	}
	govArg, err := minimalGovernanceInit(controller, hotkey, providers)
	if err != nil {
		return nil, fmt.Errorf("encode governance init: %w", err)
	}
	regArg, members, err := minimalRegistryInit(seeds, providers, opts.Mirror)
	if err != nil {
		return nil, fmt.Errorf("encode registry init: %w", err)
	}
	// root's candid service init is (): empty args.
	empty, err := candid.Marshal([]any{})
	if err != nil {
		return nil, fmt.Errorf("encode root init: %w", err)
	}

	for _, u := range []unit{
		{RegistryID, w.registry, regArg},
		{RootID, w.root, empty},
		{GovernanceID, w.governance, govArg},
	} {
		if err := c.CreateCanisterWithID(inst, u.id, installCycles, controller); err != nil {
			return nil, fmt.Errorf("create %s: %w", u.id.Encode(), err)
		}
		wasm, err := readWasm(u.wasm)
		if err != nil {
			return nil, err
		}
		if err := c.InstallCode(inst, u.id, wasm, u.arg, controller); err != nil {
			return nil, fmt.Errorf("install %s: %w", u.id.Encode(), err)
		}
	}
	return &NNS{c: c, inst: inst, Proposer: controller, Hotkey: hotkey, Members: members}, nil
}

// registryInitPayload mirrors the registry canister's RegistryCanisterInitPayload.
// The candid service declares no init args, but the Rust init still decodes this
// record from the install arg; an empty () traps. All fields default to empty.
type registryInitPayload struct {
	SwappingEnabledSubnets     *[]principal.Principal  `ic:"swapping_enabled_subnets,omitempty" json:"swapping_enabled_subnets,omitempty"`
	IsSwappingFeatureEnabled   *bool                   `ic:"is_swapping_feature_enabled,omitempty" json:"is_swapping_feature_enabled,omitempty"`
	Mutations                  []registryMutateRequest `ic:"mutations" json:"mutations"`
	SwappingWhitelistedCallers *[]principal.Principal  `ic:"swapping_whitelisted_callers,omitempty" json:"swapping_whitelisted_callers,omitempty"`
}

type registryMutateRequest struct {
	Preconditions []registryPrecondition `ic:"preconditions" json:"preconditions"`
	Mutations     []registryMutation     `ic:"mutations" json:"mutations"`
}

type registryPrecondition struct {
	Key             []byte `ic:"key" json:"key"`
	ExpectedVersion uint64 `ic:"expected_version" json:"expected_version"`
}

type registryMutation struct {
	Key          []byte `ic:"key" json:"key"`
	MutationType int32  `ic:"mutation_type" json:"mutation_type"`
	Value        []byte `ic:"value" json:"value"`
}

func minimalRegistryInit(seeds []SubnetSeed, providers []ProviderSeed, mirror *Mirror) ([]byte, map[string][]principal.Principal, error) {
	var mirroredSubnets []principal.Principal
	var mirroredVersions []string
	if mirror != nil {
		mirroredSubnets, mirroredVersions = mirror.subnets, mirror.versions
	}
	muts, members, err := seedMutations(seeds, mirroredSubnets, mirroredVersions)
	if err != nil {
		return nil, nil, err
	}
	provMuts, err := seedProviderMutations(providers)
	if err != nil {
		return nil, nil, err
	}
	muts = append(muts, provMuts...)
	if mirror != nil {
		// Mirror last, and it wins: a real record and a fabricated one for the
		// same key would be two INSERTs, which the registry rejects, and the
		// real one is the point of mirroring.
		muts = append(mirrorPrecedence(muts, mirror.mutations), mirror.mutations...)
	}
	reqs := []registryMutateRequest{}
	if len(muts) > 0 {
		reqs = append(reqs, registryMutateRequest{Preconditions: []registryPrecondition{}, Mutations: muts})
	}
	blob, err := candid.Marshal([]any{registryInitPayload{Mutations: reqs}})
	return blob, members, err
}

// mirrorPrecedence drops the seeded mutations whose key the mirror also
// supplies.
func mirrorPrecedence(seeded, mirrored []registryMutation) []registryMutation {
	real := make(map[string]bool, len(mirrored))
	for _, m := range mirrored {
		real[string(m.Key)] = true
	}
	out := seeded[:0]
	for _, m := range seeded {
		if !real[string(m.Key)] {
			out = append(out, m)
		}
	}
	return out
}

// ProposerNeuronID is the id of the neuron pre-seeded at genesis for submitting
// proposals.
const ProposerNeuronID uint64 = 1

// GenesisTimestampSeconds is the governance genesis time. It is well before the
// runtime clock (SetTime, ~2023-11) so the proposer neuron has accrued age and
// its dissolve delay is measured against a realistic present.
const GenesisTimestampSeconds uint64 = 1_600_000_000 // 2020-09

const sixMonthsSeconds uint64 = 15_778_800 // ~183 days

// proposerNeuron builds a non-dissolving neuron with a large stake and a long
// dissolve delay so it has voting power and is eligible to submit proposals.
func proposerNeuron(controller, hotkey principal.Principal) governance.Neuron {
	id := ProposerNeuronID
	dd := uint64(31_557_600) // 1 year, above the min-dissolve-delay-to-vote
	acct := make([]byte, 32)
	acct[31] = byte(id)
	hotkeys := []principal.Principal{}
	if len(hotkey.Raw) > 0 {
		hotkeys = append(hotkeys, hotkey)
	}
	return governance.Neuron{
		Id:                         &governance.NeuronId{Id: id},
		Controller:                 &controller,
		CachedNeuronStakeE8s:       1_000_000_000_000, // 10_000 ICP
		KycVerified:                true,
		AgingSinceTimestampSeconds: GenesisTimestampSeconds,
		Account:                    acct,
		DissolveState:              &governance.DissolveState{DissolveDelaySeconds: &dd},
		HotKeys:                    hotkeys,
		Followees: []struct {
			Field0 int32                `ic:"0,tuple" json:"0"`
			Field1 governance.Followees `ic:"1,tuple" json:"1"`
		}{},
	}
}

// minimalGovernanceInit builds the smallest Governance init the canister will
// accept, with economics set so the pre-seeded neuron actually has voting power
// (a non-nil VotingPowerEconomics with a 6-month minimum dissolve delay to vote).
//
// Node providers live in governance's heap, separate from the registry records
// the same seeds produce; AddOrRemoveNodeProvider validates against this list.
func minimalGovernanceInit(controller, hotkey principal.Principal, providers []ProviderSeed) ([]byte, error) {
	votingPeriod := uint64(30)
	minDD := sixMonthsSeconds
	// Push voting-power decay far out so the neuron keeps full power in tests.
	reduceAfter := uint64(100 * 365 * 24 * 3600)
	nps, err := governanceNodeProviders(providers)
	if err != nil {
		return nil, err
	}
	g := governance.Governance{
		WaitForQuietThresholdSeconds:        60,
		ShortVotingPeriodSeconds:            30,
		NeuronManagementVotingPeriodSeconds: &votingPeriod,
		GenesisTimestampSeconds:             GenesisTimestampSeconds,
		NodeProviders:                       nps,
		ToClaimTransfers:                    []governance.NeuronStakeTransfer{},
		Economics: &governance.NetworkEconomics{
			NeuronMinimumStakeE8s: 100_000_000, // 1 ICP
			VotingPowerEconomics: &governance.VotingPowerEconomics{
				NeuronMinimumDissolveDelayToVoteSeconds: &minDD,
				StartReducingVotingPowerAfterSeconds:    &reduceAfter,
			},
		},
		// Pre-seed the proposer neuron at genesis: create_neuron needs the ICP
		// ledger and update_neuron only mutates an existing neuron, so the
		// proposer must exist from init.
		Neurons: []struct {
			Field0 uint64            `ic:"0,tuple" json:"0"`
			Field1 governance.Neuron `ic:"1,tuple" json:"1"`
		}{
			{Field0: ProposerNeuronID, Field1: proposerNeuron(controller, hotkey)},
		},
	}
	return candid.Marshal([]any{g})
}

// governanceNodeProviders dedupes the seeds by provider id: several operator
// seeds commonly share one provider.
func governanceNodeProviders(seeds []ProviderSeed) ([]governance.NodeProvider, error) {
	out := []governance.NodeProvider{}
	seen := map[string]bool{}
	for _, s := range seeds {
		if s.ProviderID == "" || seen[s.ProviderID] {
			continue
		}
		seen[s.ProviderID] = true
		id, err := principal.Decode(s.ProviderID)
		if err != nil {
			return nil, fmt.Errorf("provider seed %q: %w", s.ProviderID, err)
		}
		out = append(out, governance.NodeProvider{Id: &id})
	}
	return out, nil
}
