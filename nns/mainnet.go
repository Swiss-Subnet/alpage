package nns

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/aviate-labs/agent-go"
	"github.com/swiss-subnet/alpage/gen/governance"
)

const MainnetHost = "https://icp-api.io"

// queryTimeout bounds any single agent-go or HTTP call to the IC. Without it a
// stalled icp-api.io connection hangs plan/apply/status forever; matches the
// deadline the explorer fetchers in noderecord.go already set.
const queryTimeout = 15 * time.Second

// clientOptions builds the agent-go client options for host u, always with a
// bounded HTTP client so no query path can hang indefinitely.
func clientOptions(u *url.URL) []agent.ClientOption {
	return []agent.ClientOption{
		agent.WithHostURL(u),
		agent.WithHttpClient(&http.Client{Timeout: queryTimeout}),
	}
}

// FetchNodeProviders returns the node providers governance has registered, as
// textual principals. Read-only query, no identity. This is the list
// AddOrRemoveNodeProvider::ToRemove validates against, and the registry's
// provider queries are NOT a substitute: a provider whose nodes and operators
// are already removed still has a governance record.
func FetchNodeProviders(host string, fetchRootKey bool, opts ...FetchOption) ([]string, error) {
	if host == "" {
		host = MainnetHost
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse host %q: %w", host, err)
	}
	var o fetchOpts
	for _, opt := range opts {
		opt(&o)
	}
	a, err := governance.NewGovernanceAgent(GovernanceID, agent.Config{
		ClientConfig:                   clientOptions(u),
		FetchRootKey:                   fetchRootKey,
		DisableSignedQueryVerification: o.disableQueryVerify,
	})
	if err != nil {
		return nil, fmt.Errorf("new governance agent: %w", err)
	}
	resp, err := a.ListNodeProviders()
	if err != nil {
		return nil, fmt.Errorf("list_node_providers: %w", err)
	}
	out := make([]string, 0, len(resp.NodeProviders))
	for _, np := range resp.NodeProviders {
		if np.Id == nil {
			continue
		}
		out = append(out, np.Id.Encode())
	}
	return out, nil
}

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
		ClientConfig: clientOptions(u),
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
