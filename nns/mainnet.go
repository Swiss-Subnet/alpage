package nns

import (
	"fmt"
	"net/url"

	"github.com/aviate-labs/agent-go"
	"github.com/swiss-subnet/alpage/gen/governance"
)

// MainnetHost is the default IC boundary node used for real submissions.
const MainnetHost = "https://icp-api.io"

// SubmitMainnet submits any Action to a live IC (default mainnet) as a signed
// ingress message, using agent-go's Agent for the request-id, signing, CBOR
// envelope, and read_state polling. id is the identity that must be the
// neuron's controller or a registered hotkey. fetchRootKey must be false for
// real mainnet and true for local/test networks. It returns the new proposal id.
//
// This talks to the real network. Callers are expected to have verified the
// exact payload locally first (see cmd/submit, which dry-runs before calling
// this).
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

// proposalIDFromResponse extracts the new proposal id from a manage_neuron
// response, or returns the governance error / rejection reason it carries.
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
