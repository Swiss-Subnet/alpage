package nns

import (
	"fmt"
	"net/url"

	"github.com/aviate-labs/agent-go"
	"github.com/swiss-subnet/alpage/gen/governance"
)

const MainnetHost = "https://icp-api.io"

// SubmitMainnet submits an Action to a live IC as a signed ingress message and
// returns the new proposal id. id must be the neuron's controller or a
// registered hotkey. fetchRootKey must be false for real mainnet and true only
// for local/test networks (fetching the root key from an untrusted network
// would defeat response verification).
func SubmitMainnet(id *Identity, host string, fetchRootKey bool, neuron governance.NeuronId, action Action) (uint64, error) {
	if host == "" {
		host = MainnetHost
	}
	u, err := url.Parse(host)
	if err != nil {
		return 0, fmt.Errorf("parse host %q: %w", host, err)
	}
	a, err := agent.New(agent.Config{
		Identity:     id.id,
		ClientConfig: []agent.ClientOption{agent.WithHostURL(u)},
		FetchRootKey: fetchRootKey,
	})
	if err != nil {
		return 0, fmt.Errorf("new agent: %w", err)
	}

	req, err := makeProposalRequest(neuron, action)
	if err != nil {
		return 0, err
	}
	var resp governance.ManageNeuronResponse
	if err := a.Call(GovernanceID, "manage_neuron", []any{req}, []any{&resp}); err != nil {
		return 0, fmt.Errorf("manage_neuron: %w", err)
	}
	return proposalIDFromResponse(resp)
}

func proposalIDFromResponse(resp governance.ManageNeuronResponse) (uint64, error) {
	if resp.Command == nil {
		return 0, fmt.Errorf("manage_neuron: empty command in response")
	}
	if e := resp.Command.Error; e != nil {
		return 0, fmt.Errorf("governance rejected: %s (error_type %d)", e.ErrorMessage, e.ErrorType)
	}
	mp := resp.Command.MakeProposal
	if mp == nil {
		return 0, fmt.Errorf("manage_neuron: unexpected command variant %+v", resp.Command)
	}
	if mp.ProposalId == nil {
		msg := ""
		if mp.Message != nil {
			msg = *mp.Message
		}
		return 0, fmt.Errorf("proposal not created: %s", msg)
	}
	return mp.ProposalId.Id, nil
}
