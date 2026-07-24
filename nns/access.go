package nns

import (
	"fmt"
	"net/url"

	"github.com/aviate-labs/agent-go"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/governance"
)

type NeuronAccess int

const (
	NeuronAccessNone NeuronAccess = iota
	NeuronAccessController
	NeuronAccessHotkey
)

func (a NeuronAccess) String() string {
	switch a {
	case NeuronAccessController:
		return "controller"
	case NeuronAccessHotkey:
		return "hotkey"
	default:
		return "none"
	}
}

// CheckNeuronAccess probes whether id may submit from neuronID. get_full_neuron
// signed as id succeeds only for the neuron's controller or a hotkey, so it
// doubles as the authorization check.
func CheckNeuronAccess(id *Identity, host string, fetchRootKey bool, neuronID uint64, opts ...FetchOption) (NeuronAccess, error) {
	if host == "" {
		host = MainnetHost
	}
	u, err := url.Parse(host)
	if err != nil {
		return NeuronAccessNone, fmt.Errorf("parse host %q: %w", host, err)
	}
	var o fetchOpts
	for _, opt := range opts {
		opt(&o)
	}
	a, err := governance.NewGovernanceAgent(GovernanceID, agent.Config{
		Identity:                       id.id,
		ClientConfig:                   clientOptions(u),
		FetchRootKey:                   fetchRootKey,
		DisableSignedQueryVerification: o.disableQueryVerify,
	})
	if err != nil {
		return NeuronAccessNone, fmt.Errorf("new governance agent: %w", err)
	}
	res, err := a.GetFullNeuron(neuronID)
	if err != nil {
		return NeuronAccessNone, fmt.Errorf("get_full_neuron(%d): %w", neuronID, err)
	}
	if res.Err != nil {
		return NeuronAccessNone, fmt.Errorf("identity %s cannot access neuron %d: %s",
			id.Principal().Encode(), neuronID, res.Err.ErrorMessage)
	}
	if res.Ok == nil {
		return NeuronAccessNone, fmt.Errorf("get_full_neuron(%d): empty response", neuronID)
	}
	access := classifyAccess(id.Principal(), res.Ok)
	if access == NeuronAccessNone {
		// get_full_neuron succeeded yet the caller is neither controller nor
		// hotkey: an unexpected shape. Since this doubles as the authorization
		// check, refuse rather than assume submit rights.
		return NeuronAccessNone, fmt.Errorf("neuron %d readable by %s but it is neither controller nor hotkey",
			neuronID, id.Principal().Encode())
	}
	return access, nil
}

func classifyAccess(caller principal.Principal, n *governance.Neuron) NeuronAccess {
	if n.Controller != nil && n.Controller.Encode() == caller.Encode() {
		return NeuronAccessController
	}
	for _, hk := range n.HotKeys {
		if hk.Encode() == caller.Encode() {
			return NeuronAccessHotkey
		}
	}
	return NeuronAccessNone
}
