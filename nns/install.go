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
	w, err := wasmPathsFromEnv()
	if err != nil {
		return nil, err
	}

	type unit struct {
		id   principal.Principal
		wasm string
		arg  []byte
	}
	govArg, err := minimalGovernanceInit(controller, hotkey)
	if err != nil {
		return nil, fmt.Errorf("encode governance init: %w", err)
	}
	regArg, err := minimalRegistryInit()
	if err != nil {
		return nil, fmt.Errorf("encode registry init: %w", err)
	}
	// root's candid service init is (): empty args.
	empty, _ := candid.Marshal([]any{})

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
	return &NNS{c: c, inst: inst, Proposer: controller, Hotkey: hotkey}, nil
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

func minimalRegistryInit() ([]byte, error) {
	return candid.Marshal([]any{registryInitPayload{Mutations: []registryMutateRequest{}}})
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
func minimalGovernanceInit(controller, hotkey principal.Principal) ([]byte, error) {
	votingPeriod := uint64(30)
	minDD := sixMonthsSeconds
	// Push voting-power decay far out so the neuron keeps full power in tests.
	reduceAfter := uint64(100 * 365 * 24 * 3600)
	g := governance.Governance{
		WaitForQuietThresholdSeconds:        60,
		ShortVotingPeriodSeconds:            30,
		NeuronManagementVotingPeriodSeconds: &votingPeriod,
		GenesisTimestampSeconds:             GenesisTimestampSeconds,
		NodeProviders:                       []governance.NodeProvider{},
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
