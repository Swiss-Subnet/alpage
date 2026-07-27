package nns

// ApplyPhase is one gate `alp apply` runs before submitting.
type ApplyPhase string

const (
	// PhaseDryRun executes the payload on a local PocketIC NNS. Local: no
	// mainnet, no identity permissions, no network beyond fetching the pinned
	// artifacts once.
	PhaseDryRun ApplyPhase = "dry-run"
	// PhaseNeuronAccess checks the identity may propose with the neuron.
	PhaseNeuronAccess ApplyPhase = "neuron-access"
	// PhasePlan checks the payload is not a no-op against live on-chain state.
	PhasePlan ApplyPhase = "plan"
)

// NeedsNetwork reports whether the phase queries mainnet.
func (p ApplyPhase) NeedsNetwork() bool { return p != PhaseDryRun }

// ApplyPhases is the order apply runs its gates in. The local dry-run comes
// first so it is reachable without mainnet or neuron permissions: verifying
// that a payload encodes and executes is useful on its own, and it is the step
// a reviewer runs before anyone holds a usable key. The two network gates keep
// their fail-fast ordering relative to each other.
func ApplyPhases() []ApplyPhase {
	return []ApplyPhase{PhaseDryRun, PhaseNeuronAccess, PhasePlan}
}
