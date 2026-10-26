package nns

import (
	"fmt"
	"strings"

	"github.com/aviate-labs/agent-go/principal"
	governancepb "github.com/swiss-subnet/alpage/nns/pb/governance"
)

// A proposal kind is declared once, here. Adding one used to mean editing five
// places: the decode switch, the schema block list, and the two render maps.
// Missing the render registration is the one that hurts, because nothing fails
// until someone runs `alp status` on such a proposal and gets a bare function
// number instead of a decoded payload.
type kind struct {
	// Name is the config's kind value and the name of the nested block that
	// carries its fields.
	Name string
	// Function is the canister method the payload targets, as the NNS UI names
	// it. Empty for a native governance action, which carries no function
	// number.
	Function string
	// Doc and Since document the nested block in docs/config.md.
	Doc   string
	Since string
	// Body is the zero value of the HCL body struct, reflected by schemadoc for
	// the field table.
	Body any
	// Action is the zero value of the action this kind decodes to. It supplies
	// the NNS function number and the payload renderer, so both come from the
	// same declaration the decoder does.
	Action Action
	Decode func(s *Spec) (Action, error)
}

var kinds = []kind{
	{
		Name:     "membership",
		Function: "change_subnet_membership",
		Doc:      "Nested in a proposal of kind \"membership\": change_subnet_membership. Holds add/remove node blocks. Renamed from resize in v0.3.0. A config written for v0.2.0 or earlier must rename the block and its kind. A SEV-enabled subnet admits only nodes whose registry record carries a chip_id, and execution traps on one that does not, so preflight checks every incoming node.",
		Since:    "v0.3.0",
		Body:     membershipBody{},
		Action:   MembershipProposal{},
		Decode:   decodeMembershipProposal,
	},
	{
		Name:     "deploy_guestos",
		Function: "deploy_guestos_to_all_subnet_nodes",
		Doc:      "Nested in a proposal of kind \"deploy_guestos\": deploy_guestos_to_all_subnet_nodes. A SEV-enabled subnet runs only a version whose registry record carries guest launch measurements, and execution traps on one that does not, so preflight checks the target.",
		Since:    "v0.1.0",
		Body:     deployGuestosBody{},
		Action:   DeployGuestosAction{},
		Decode:   decodeDeployGuestos,
	},
	{
		Name:     "update_subnet",
		Function: "update_subnet",
		Doc:      "Nested in a proposal of kind \"update_subnet\": update_subnet (UPDATE_CONFIG_OF_SUBNET). Changes a subnet's configuration record. Every field is optional and an omitted one is left untouched on-chain, so a proposal changes exactly what it declares. Preflight diffs every declared field against the current record. Two payload fields are not ordinary settings here: subnet_type, which the registry asserts is unchanged, and sev_enabled, which the registry refuses to turn off.",
		Since:    "v0.5.0",
		Body:     updateSubnetBody{},
		Action:   UpdateSubnetAction{},
		Decode:   decodeUpdateSubnet,
	},
	{
		Name:     "remove_nodes",
		Function: "remove_nodes",
		Doc:      "Nested in a proposal of kind \"remove_nodes\": deregisters nodes, freeing their operators' allowance. Holds one node block per node. The registry rejects a node that is still a subnet member, so remove it from its subnet first; preflight checks this before the proposal is cut.",
		Since:    "v0.5.0",
		Body:     removeNodesBody{},
		Action:   RemoveNodesAction{},
		Decode:   decodeRemoveNodes,
	},
	{
		Name:     "remove_node_operators",
		Function: "remove_node_operators",
		Doc:      "Nested in a proposal of kind \"remove_node_operators\": deletes node operator records. The registry silently skips an operator that still owns nodes, so deregister its nodes first; preflight checks this.",
		Since:    "v0.5.0",
		Body:     removeNodeOperatorsBody{},
		Action:   RemoveNodeOperatorsAction{},
		Decode:   decodeRemoveNodeOperators,
	},
	{
		// No Function: this one is a native governance action rather than an
		// ExecuteNnsFunction, so there is no function number to dispatch on.
		Name:   "remove_node_provider",
		Doc:    "Nested in a proposal of kind \"remove_node_provider\": removes a node provider. Unlike the other kinds this is a native governance action rather than an ExecuteNnsFunction. Governance rejects a provider it does not know, and removing one that still has operators orphans them; preflight checks both.",
		Since:  "v0.5.0",
		Body:   removeNodeProviderBody{},
		Action: RemoveNodeProviderAction{},
		Decode: decodeRemoveNodeProvider,
	},
	{
		Name:     "add_data_centers",
		Function: "add_or_remove_data_centers",
		Doc:      "Nested in a proposal of kind \"add_data_centers\": registers data center records, one dc block each. Only adds: the payload can also remove, but removal has its own preconditions and is not exposed here. The canister traps on a dc_id it already holds, and it drops a record whose id, region, or owner exceeds 255 bytes while still reporting the proposal as executed, so preflight checks both before the proposal is cut. Ids are lowercased on the way in, which alpage does up front so the payload matches the record that lands.",
		Since:    "v0.5.0",
		Body:     addDataCentersBody{},
		Action:   AddDataCentersAction{},
		Decode:   decodeAddDataCenters,
	},
	{
		// Native governance action, like its removal twin: no function number.
		Name:   "add_node_provider",
		Doc:    "Nested in a proposal of kind \"add_node_provider\": registers a node provider. A native governance action rather than an ExecuteNnsFunction. Governance rejects a principal that is already a provider; preflight checks that. The self-declaration is not part of the payload, which carries only the principal and an optional reward account, so link it from the proposal's url and summarise it in summary.",
		Since:  "v0.5.0",
		Body:   addNodeProviderBody{},
		Action: AddNodeProviderAction{},
		Decode: decodeAddNodeProvider,
	},
	{
		Name:     "add_node_operator",
		Function: "add_node_operator",
		Doc:      "Nested in a proposal of kind \"add_node_operator\": registers a node operator under a provider, in a data center, with an allowance of nodes it may register. Submitted as ASSIGN_NOID, governance's name for the registry's add_node_operator. Both referents must already exist on-chain, which makes this the last of the three onboarding proposals. The data center and the provider do not depend on each other, so those two can go in either order or at once. Preflight checks the data center, the provider, and that the operator principal is still free.",
		Since:    "v0.5.0",
		Body:     addNodeOperatorBody{},
		Action:   AddNodeOperatorAction{},
		Decode:   decodeAddNodeOperator,
	},
}

func kindByName(name string) (kind, bool) {
	for _, k := range kinds {
		if k.Name == name {
			return k, true
		}
	}
	return kind{}, false
}

// kindList names the known kinds, for the error a config with an unknown one
// gets.
func kindList() string {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = k.Name
	}
	return strings.Join(names, ", ")
}

// kindSchemaBlocks documents every kind's nested block, so a kind added to the
// table documents itself.
func kindSchemaBlocks() []SchemaBlock {
	out := make([]SchemaBlock, len(kinds))
	for i, k := range kinds {
		out[i] = SchemaBlock{
			Name: k.Name, File: "proposals.hcl",
			Doc: k.Doc, Type: k.Body, Since: k.Since,
		}
	}
	return out
}

// renderTables builds the function-number lookups Render uses. A kind whose
// action is native carries no number and is skipped.
func renderTables() (map[int32]string, map[int32]Action) {
	names := make(map[int32]string, len(kinds))
	actions := make(map[int32]Action, len(kinds))
	for _, k := range kinds {
		fn := k.Action.NnsFunction()
		if fn == governancepb.NnsFunction_NNS_FUNCTION_UNSPECIFIED {
			continue
		}
		names[int32(fn)] = k.Function
		actions[int32(fn)] = k.Action
	}
	return names, actions
}

func decodeMembershipProposal(s *Spec) (Action, error) {
	var body membershipBody
	if err := s.decodeRest(&body); err != nil {
		return nil, err
	}
	subnet, err := principal.Decode(body.SubnetID)
	if err != nil {
		return nil, fmt.Errorf("proposal %q: subnet_id %q: %w", s.Name, body.SubnetID, err)
	}
	add, err := decodeNodes(body.Add)
	if err != nil {
		return nil, fmt.Errorf("proposal %q: %w", s.Name, err)
	}
	remove, err := decodeNodes(body.Remove)
	if err != nil {
		return nil, fmt.Errorf("proposal %q: %w", s.Name, err)
	}
	return MembershipProposal{
		Meta:     s.Meta(),
		SubnetID: subnet, NodeIDsAdd: add, NodeIDsRemove: remove,
	}, nil
}

func decodeDeployGuestos(s *Spec) (Action, error) {
	var body deployGuestosBody
	if err := s.decodeRest(&body); err != nil {
		return nil, err
	}
	subnet, err := principal.Decode(body.SubnetID)
	if err != nil {
		return nil, fmt.Errorf("proposal %q: subnet_id %q: %w", s.Name, body.SubnetID, err)
	}
	if body.ReplicaVersionID == "" {
		return nil, fmt.Errorf("proposal %q: replica_version_id is required", s.Name)
	}
	return DeployGuestosAction{Meta: s.Meta(), SubnetID: subnet, ReplicaVersionID: body.ReplicaVersionID}, nil
}

func decodeUpdateSubnet(s *Spec) (Action, error) {
	var body updateSubnetBody
	if err := s.decodeRest(&body); err != nil {
		return nil, err
	}
	subnet, err := principal.Decode(body.SubnetID)
	if err != nil {
		return nil, fmt.Errorf("proposal %q: subnet_id %q: %w", s.Name, body.SubnetID, err)
	}
	settings, err := body.settings()
	if err != nil {
		return nil, fmt.Errorf("proposal %q: %w", s.Name, err)
	}
	return UpdateSubnetAction{Meta: s.Meta(), SubnetID: subnet, Settings: settings}, nil
}

func decodeRemoveNodes(s *Spec) (Action, error) {
	var body removeNodesBody
	if err := s.decodeRest(&body); err != nil {
		return nil, err
	}
	if len(body.Nodes) == 0 {
		return nil, fmt.Errorf("proposal %q: at least one node block is required", s.Name)
	}
	ids, err := decodeNodes(body.Nodes)
	if err != nil {
		return nil, fmt.Errorf("proposal %q: %w", s.Name, err)
	}
	return RemoveNodesAction{Meta: s.Meta(), NodeIDs: ids}, nil
}

func decodeRemoveNodeOperators(s *Spec) (Action, error) {
	var body removeNodeOperatorsBody
	if err := s.decodeRest(&body); err != nil {
		return nil, err
	}
	if len(body.Operators) == 0 {
		return nil, fmt.Errorf("proposal %q: at least one operator is required", s.Name)
	}
	ids := make([]principal.Principal, len(body.Operators))
	for i, op := range body.Operators {
		p, err := principal.Decode(op)
		if err != nil {
			return nil, fmt.Errorf("proposal %q: operator id %q: %w", s.Name, op, err)
		}
		ids[i] = p
	}
	return RemoveNodeOperatorsAction{Meta: s.Meta(), OperatorIDs: ids}, nil
}

func decodeAddDataCenters(s *Spec) (Action, error) {
	var body addDataCentersBody
	if err := s.decodeRest(&body); err != nil {
		return nil, err
	}
	if len(body.DataCenters) == 0 {
		return nil, fmt.Errorf("proposal %q: at least one data_center block is required", s.Name)
	}
	dcs := make([]DataCenterSpec, len(body.DataCenters))
	for i, dc := range body.DataCenters {
		if dc.ID == "" {
			return nil, fmt.Errorf("proposal %q: data_center id is required", s.Name)
		}
		if dc.Region == "" {
			return nil, fmt.Errorf("proposal %q: data_center %q: region is required", s.Name, dc.ID)
		}
		// The canister lowercases the id on the way in, so the payload states
		// what will actually land rather than what was typed.
		spec := DataCenterSpec{ID: strings.ToLower(dc.ID), Region: dc.Region, Owner: dc.Owner}
		if g := dc.GPS; g != nil {
			// The canister does not range-check coordinates, so a transposed
			// pair would be stored verbatim. Rejected here instead.
			if g.Latitude < -90 || g.Latitude > 90 {
				return nil, fmt.Errorf("proposal %q: data_center %q: latitude %v out of range [-90, 90]", s.Name, dc.ID, g.Latitude)
			}
			if g.Longitude < -180 || g.Longitude > 180 {
				return nil, fmt.Errorf("proposal %q: data_center %q: longitude %v out of range [-180, 180]", s.Name, dc.ID, g.Longitude)
			}
			spec.GPS = &GPSCoordinates{Latitude: float32(g.Latitude), Longitude: float32(g.Longitude)}
		}
		dcs[i] = spec
	}
	return AddDataCentersAction{Meta: s.Meta(), DataCenters: dcs}, nil
}

func decodeAddNodeProvider(s *Spec) (Action, error) {
	var body addNodeProviderBody
	if err := s.decodeRest(&body); err != nil {
		return nil, err
	}
	p, err := principal.Decode(body.NodeProviderID)
	if err != nil {
		return nil, fmt.Errorf("proposal %q: node_provider_id %q: %w", s.Name, body.NodeProviderID, err)
	}
	action := AddNodeProviderAction{Meta: s.Meta(), ProviderID: p}
	if body.RewardAccount != "" {
		// Governance checks the account identifier's embedded CRC32 and
		// rejects a bad one at submission, so it is checked at load instead.
		id, err := principal.DecodeAccountID(body.RewardAccount)
		if err != nil {
			return nil, fmt.Errorf("proposal %q: reward_account %q: %w", s.Name, body.RewardAccount, err)
		}
		action.RewardAccount = id.Bytes()
	}
	return action, nil
}

func decodeAddNodeOperator(s *Spec) (Action, error) {
	var body addNodeOperatorBody
	if err := s.decodeRest(&body); err != nil {
		return nil, err
	}
	operator, err := principal.Decode(body.NodeOperatorID)
	if err != nil {
		return nil, fmt.Errorf("proposal %q: node_operator_id %q: %w", s.Name, body.NodeOperatorID, err)
	}
	provider, err := principal.Decode(body.NodeProviderID)
	if err != nil {
		return nil, fmt.Errorf("proposal %q: node_provider_id %q: %w", s.Name, body.NodeProviderID, err)
	}
	if body.DcID == "" {
		return nil, fmt.Errorf("proposal %q: dc_id is required", s.Name)
	}
	return AddNodeOperatorAction{
		Meta:       s.Meta(),
		OperatorID: operator,
		ProviderID: provider,
		// The canister lowercases dc_id on the way in, so the payload states
		// what will actually land rather than what was typed.
		DcID:               strings.ToLower(body.DcID),
		NodeAllowance:      body.NodeAllowance,
		IPv6:               body.IPv6,
		MaxRewardableNodes: body.MaxRewardableNodes,
	}, nil
}

func decodeRemoveNodeProvider(s *Spec) (Action, error) {
	var body removeNodeProviderBody
	if err := s.decodeRest(&body); err != nil {
		return nil, err
	}
	p, err := principal.Decode(body.NodeProviderID)
	if err != nil {
		return nil, fmt.Errorf("proposal %q: node_provider_id %q: %w", s.Name, body.NodeProviderID, err)
	}
	return RemoveNodeProviderAction{Meta: s.Meta(), ProviderID: p}, nil
}
