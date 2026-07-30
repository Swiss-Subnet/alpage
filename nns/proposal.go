package nns

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/governance"
)

// ProposerNeuron returns the id of the proposer neuron pre-seeded into
// governance at genesis (see proposerNeuron in install.go).
func (n *NNS) ProposerNeuron() governance.NeuronId {
	return governance.NeuronId{Id: ProposerNeuronID}
}

// ResizeProposal describes a subnet-membership change (add/remove nodes).
type ResizeProposal struct {
	Meta
	SubnetID      principal.Principal
	NodeIDsAdd    []principal.Principal
	NodeIDsRemove []principal.Principal
}

func (n *NNS) SubmitResize(neuron governance.NeuronId, p ResizeProposal) (uint64, error) {
	return n.SubmitResizeAs(n.Proposer, neuron, p)
}

// SubmitResizeAs submits an ExecuteNnsFunction(change_subnet_membership)
// proposal as an explicit caller. sender may be the neuron's controller or one
// of its hotkeys; governance authorizes either to make proposals.
func (n *NNS) SubmitResizeAs(sender principal.Principal, neuron governance.NeuronId, p ResizeProposal) (uint64, error) {
	return n.SubmitAs(sender, neuron, p)
}

func (n *NNS) SubmitAs(sender principal.Principal, neuron governance.NeuronId, a Action) (uint64, error) {
	req, err := makeProposalRequest(neuron, a)
	if err != nil {
		return 0, err
	}
	var resp governance.ManageNeuronResponse
	if err := n.c.Update(n.inst, sender, GovernanceID, "manage_neuron",
		[]any{req}, []any{&resp}); err != nil {
		return 0, fmt.Errorf("manage_neuron: %w", err)
	}
	return proposalIDFromResponse(resp)
}

func (n *NNS) GetProposalInfo(id uint64) (*governance.ProposalInfo, error) {
	var out *governance.ProposalInfo
	if err := n.c.Query(n.inst, n.Proposer, GovernanceID, "get_proposal_info",
		[]any{id}, []any{&out}); err != nil {
		return nil, fmt.Errorf("get_proposal_info: %w", err)
	}
	if out == nil {
		return nil, fmt.Errorf("proposal %d not found", id)
	}
	return out, nil
}

// topicName / statusName render the numeric enums the way the NNS UI labels them.
var topicName = map[int32]string{0: "Unspecified", 7: "SubnetManagement", 12: "IcOsVersionDeployment"}
var statusName = map[int32]string{1: "Open", 2: "Rejected", 3: "Adopted", 4: "Executed", 5: "Failed"}

// funcName / renderer map an NNS function number to its label and payload
// decoder, so rendering dispatches on the stored proposal's function without
// caring which Action produced it.
var funcName = map[int32]string{
	nnsFunctionChangeSubnetMembership:        "change_subnet_membership",
	nnsFunctionDeployGuestosToAllSubnetNodes: "deploy_guestos_to_all_subnet_nodes",
}
var renderer = map[int32]Action{
	nnsFunctionChangeSubnetMembership:        ResizeProposal{},
	nnsFunctionDeployGuestosToAllSubnetNodes: DeployGuestosAction{},
}

func label(m map[int32]string, v int32) string {
	if s, ok := m[v]; ok {
		return fmt.Sprintf("%s (%d)", s, v)
	}
	return fmt.Sprintf("%d", v)
}

// Render produces the human-readable view of a stored proposal, decoding the
// nested ExecuteNnsFunction payload the same information the NNS dapp displays.
func Render(pi *governance.ProposalInfo) string { return render(pi, false) }

// RenderVerbose is Render plus the wire-level facts worth checking before a
// real submission: the raw NNS function number and the exact bytes of the
// nested candid payload (length and SHA-256).
func RenderVerbose(pi *governance.ProposalInfo) string { return render(pi, true) }

func render(pi *governance.ProposalInfo, verbose bool) string {
	var b strings.Builder
	id := uint64(0)
	if pi.Id != nil {
		id = pi.Id.Id
	}
	fmt.Fprintf(&b, "Proposal %d\n", id)
	fmt.Fprintf(&b, "  Status: %s\n", label(statusName, pi.Status))
	if t := pi.LatestTally; t != nil {
		fmt.Fprintf(&b, "  Tally:  yes=%d no=%d total=%d\n", t.Yes, t.No, t.Total)
	}
	if fr := pi.FailureReason; fr != nil {
		fmt.Fprintf(&b, "  Failure: %s\n", fr.ErrorMessage)
	}
	fmt.Fprintf(&b, "  Topic:  %s\n", label(topicName, pi.Topic))
	if pi.Proposer != nil {
		fmt.Fprintf(&b, "  Proposer neuron: %d\n", pi.Proposer.Id)
	}
	if pi.Proposal != nil {
		if pi.Proposal.Title != nil {
			fmt.Fprintf(&b, "  Title:   %s\n", *pi.Proposal.Title)
		}
		if s := pi.Proposal.Summary; s != "" {
			fmt.Fprintf(&b, "  Summary: %s\n", firstLine(s))
		}
		if u := pi.Proposal.Url; u != "" {
			fmt.Fprintf(&b, "  URL:     %s\n", u)
		}
		if act := pi.Proposal.Action; act != nil && act.ExecuteNnsFunction != nil {
			ef := act.ExecuteNnsFunction
			fmt.Fprintf(&b, "  Action:  ExecuteNnsFunction #%d", ef.NnsFunction)
			if name, ok := funcName[ef.NnsFunction]; ok {
				fmt.Fprintf(&b, " (%s)\n", name)
			} else {
				b.WriteString("\n")
			}
			if verbose {
				fmt.Fprintf(&b, "    nns_function: %d\n", ef.NnsFunction)
				sum := sha256.Sum256(ef.Payload)
				fmt.Fprintf(&b, "    payload: %d bytes, sha256=%x\n", len(ef.Payload), sum)
			}
			if r, ok := renderer[ef.NnsFunction]; ok {
				r.RenderPayload(&b, ef.Payload)
			}
		}
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
