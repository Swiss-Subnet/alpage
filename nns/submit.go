package nns

import (
	"fmt"

	"github.com/swiss-subnet/alpage/gen/governance"
)

// Submitter sends an action to the network and returns the new proposal id.
// The production implementation wraps SubmitMainnet; tests inject a fake.
type Submitter interface {
	Submit(neuron governance.NeuronId, a Action) (uint64, error)
}

// MainnetSubmitter is the live Submitter: a signed ingress submission to host.
type MainnetSubmitter struct {
	Identity     *Identity
	Host         string
	FetchRootKey bool
}

func (m MainnetSubmitter) Submit(neuron governance.NeuronId, a Action) (uint64, error) {
	return SubmitMainnet(m.Identity, m.Host, m.FetchRootKey, neuron, a)
}

// RecordArgs is the metadata written to state for a successful submission,
// alongside the resulting proposal id.
type RecordArgs struct {
	Name        string
	Kind        string
	Hash        string
	SubmittedBy string
	Neuron      uint64
	Host        string
	At          string // RFC3339; injected so callers control the clock
}

// SubmitAndRecord submits an action and, only on success, records the outcome
// in state and persists it via save. It is the single ordering guarantee that
// state is never written for a submission that did not happen. If the write
// fails after a successful submit, the returned error names the proposal id so
// the operator can recover a submission that is live but unrecorded; the id is
// still returned in that case.
func SubmitAndRecord(sub Submitter, st *State, neuron governance.NeuronId, a Action, args RecordArgs, save func(*State) error) (uint64, error) {
	pid, err := sub.Submit(neuron, a)
	if err != nil {
		return 0, err
	}
	st.Proposals[args.Name] = Entry{
		Kind:          args.Kind,
		ProposalID:    pid,
		PayloadSHA256: args.Hash,
		SubmittedBy:   args.SubmittedBy,
		Neuron:        args.Neuron,
		Host:          args.Host,
		SubmittedAt:   args.At,
	}
	if err := save(st); err != nil {
		return pid, fmt.Errorf("submitted as %d but failed to write state: %w", pid, err)
	}
	return pid, nil
}
