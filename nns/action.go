package nns

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/aviate-labs/agent-go/candid"
	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/governance"
	"github.com/swiss-subnet/alpage/gen/registry"
)

// NNS function numbers from the governance proto, one per ExecuteNnsFunction
// action we support.
const (
	nnsFunctionChangeSubnetMembership        int32 = 31
	nnsFunctionDeployGuestosToAllSubnetNodes int32 = 58
)

// Action is one kind of NNS proposal. The framework (config, state, dry-run,
// submit, drift detection) stays kind-agnostic: it only needs the metadata, the
// NNS function number, and the candid payload blob. Rendering is kind-specific.
// To add a proposal type, implement this interface and register a decoder in
// spec.go.
type Action interface {
	Kind() string
	NnsFunction() int32
	Metadata() Meta
	// PayloadBlob is the exact candid-encoded blob nested inside the
	// ExecuteNnsFunction action. Its SHA-256 is what state pins.
	PayloadBlob() ([]byte, error)
	RenderPayload(b *strings.Builder, blob []byte)
	// Preflight reconciles the action against live network state, kind-
	// specifically. apply and plan call it without knowing the kind.
	Preflight(host string, fetchRootKey bool, opts ...FetchOption) (Preflight, error)
}

// Meta is the common proposal metadata every kind carries. Concrete actions
// embed it, so it supplies both the fields and the Metadata accessor.
type Meta struct {
	Title   string
	Summary string
	URL     string
}

func (m Meta) Metadata() Meta { return m }

// makeProposalRequest builds the manage_neuron MakeProposal request for any
// action. It is the single source of truth for the outer request, shared by the
// local (PocketIC) and mainnet submit paths so the dry-run verifies exactly what
// the real submission sends.
func makeProposalRequest(neuron governance.NeuronId, a Action) (governance.ManageNeuronRequest, error) {
	blob, err := a.PayloadBlob()
	if err != nil {
		return governance.ManageNeuronRequest{}, err
	}
	m := a.Metadata()
	title := m.Title
	return governance.ManageNeuronRequest{
		Id: &neuron,
		Command: &governance.ManageNeuronCommandRequest{
			MakeProposal: &governance.MakeProposalRequest{
				Title:   &title,
				Summary: m.Summary,
				Url:     m.URL,
				Action: &governance.ProposalActionRequest{
					ExecuteNnsFunction: &governance.ExecuteNnsFunction{
						NnsFunction: a.NnsFunction(),
						Payload:     blob,
					},
				},
			},
		},
	}, nil
}

// payloadSHA256 hashes an action's candid payload blob.
func payloadSHA256(a Action) (string, error) {
	blob, err := a.PayloadBlob()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(blob)
	return fmt.Sprintf("%x", sum), nil
}

// --- resize: change_subnet_membership (add/remove nodes) ---

// ResizeProposal is the "resize subnet" action (e.g. mainnet 141235, 142931).
func (r ResizeProposal) Kind() string       { return "resize" }
func (r ResizeProposal) NnsFunction() int32 { return nnsFunctionChangeSubnetMembership }

func (r ResizeProposal) PayloadBlob() ([]byte, error) {
	payload := registry.ChangeSubnetMembershipPayload{
		NodeIdsAdd:    r.NodeIDsAdd,
		SubnetId:      r.SubnetID,
		NodeIdsRemove: r.NodeIDsRemove,
	}
	blob, err := candid.Marshal([]any{payload})
	if err != nil {
		return nil, fmt.Errorf("encode ChangeSubnetMembershipPayload: %w", err)
	}
	return blob, nil
}

// Preflight diffs the resize against the subnet's current on-chain membership.
func (r ResizeProposal) Preflight(host string, fetchRootKey bool, opts ...FetchOption) (Preflight, error) {
	current, err := FetchSubnetMembership(host, fetchRootKey, r.SubnetID, opts...)
	if err != nil {
		return Preflight{}, fmt.Errorf("fetch membership: %w", err)
	}
	return resizePreflight(PlanResize(r, current)), nil
}

func (r ResizeProposal) RenderPayload(b *strings.Builder, blob []byte) {
	var p registry.ChangeSubnetMembershipPayload
	if err := candid.Unmarshal(blob, []any{&p}); err != nil {
		fmt.Fprintf(b, "    <undecodable payload: %v>\n", err)
		return
	}
	fmt.Fprintf(b, "    subnet_id: %s\n", p.SubnetId.Encode())
	fmt.Fprintf(b, "    nodes added (%d):\n", len(p.NodeIdsAdd))
	for _, n := range p.NodeIdsAdd {
		fmt.Fprintf(b, "      + %s\n", n.Encode())
	}
	fmt.Fprintf(b, "    nodes removed (%d):\n", len(p.NodeIdsRemove))
	for _, n := range p.NodeIdsRemove {
		fmt.Fprintf(b, "      - %s\n", n.Encode())
	}
}

// --- deploy_guestos: deploy_guestos_to_all_subnet_nodes (upgrade replica) ---

// DeployGuestosAction upgrades every node in a subnet to a replica version.
type DeployGuestosAction struct {
	Meta
	SubnetID         principal.Principal
	ReplicaVersionID string
}

func (d DeployGuestosAction) Kind() string       { return "deploy_guestos" }
func (d DeployGuestosAction) NnsFunction() int32 { return nnsFunctionDeployGuestosToAllSubnetNodes }

func (d DeployGuestosAction) PayloadBlob() ([]byte, error) {
	payload := registry.DeployGuestosToAllSubnetNodesPayload{
		SubnetId:         d.SubnetID,
		ReplicaVersionId: d.ReplicaVersionID,
	}
	blob, err := candid.Marshal([]any{payload})
	if err != nil {
		return nil, fmt.Errorf("encode DeployGuestosToAllSubnetNodesPayload: %w", err)
	}
	return blob, nil
}

// Preflight checks the target replica version against the subnet's current
// version in the registry: deploying the version it already runs is a no-op. It
// also verifies the target is elected, and resolves its release for display.
//
// The current version is a trustless registry query. The elected set is not
// (see ElectedVersions), and an unreadable elected set is a hard failure:
// without it a deploy cannot be verified as executable. AllowUnverifiedElection
// downgrades that to a warning for the case where the source is down and the
// deploy has to go ahead anyway. The release lookup is display-only, so it
// always degrades to a note rather than blocking an otherwise valid deploy.
func (d DeployGuestosAction) Preflight(host string, fetchRootKey bool, opts ...FetchOption) (Preflight, error) {
	current, err := FetchSubnetReplicaVersion(host, fetchRootKey, d.SubnetID, opts...)
	if err != nil {
		return Preflight{}, fmt.Errorf("fetch subnet replica version: %w", err)
	}
	var o fetchOpts
	for _, opt := range opts {
		opt(&o)
	}
	elected, err := FetchElectedVersions(o.explorer(), d.ReplicaVersionID)
	if err != nil {
		if !o.allowUnverified {
			return Preflight{}, fmt.Errorf("fetch elected versions: %w", err)
		}
		elected = ElectedVersions{Unverified: true}
	}
	rel, relErr := FetchRelease(o.dashboard(), d.ReplicaVersionID)
	pf := planDeployGuestos(d.ReplicaVersionID, current, elected, rel)
	if relErr != nil {
		pf.Report += fmt.Sprintf("  release lookup unavailable: %v\n", relErr)
	}
	return pf, nil
}

func (d DeployGuestosAction) RenderPayload(b *strings.Builder, blob []byte) {
	var p registry.DeployGuestosToAllSubnetNodesPayload
	if err := candid.Unmarshal(blob, []any{&p}); err != nil {
		fmt.Fprintf(b, "    <undecodable payload: %v>\n", err)
		return
	}
	fmt.Fprintf(b, "    subnet_id: %s\n", p.SubnetId.Encode())
	fmt.Fprintf(b, "    replica_version_id: %s\n", p.ReplicaVersionId)
}
