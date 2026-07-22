package pocketic

import (
	"encoding/json"
	"fmt"

	"github.com/aviate-labs/agent-go/candid"
	"github.com/aviate-labs/agent-go/principal"
)

// managementCanister aaaaa-aa is empty raw bytes.
var managementCanister = principal.Principal{Raw: []byte{}}

type subnetSpec struct {
	StateConfig       string   `json:"state_config"`       // "New"
	InstructionConfig string   `json:"instruction_config"` // "Production"
	SubnetAdmins      []string `json:"subnet_admins"`      // null
	CostSchedule      string   `json:"cost_schedule"`      // "Normal"
}

func newSubnet() subnetSpec {
	return subnetSpec{StateConfig: "New", InstructionConfig: "Production", CostSchedule: "Normal"}
}

type extendedSubnetConfigSet struct {
	NNS                 *subnetSpec  `json:"nns"`
	SNS                 *subnetSpec  `json:"sns"`
	II                  *subnetSpec  `json:"ii"`
	Fiduciary           *subnetSpec  `json:"fiduciary"`
	Bitcoin             *subnetSpec  `json:"bitcoin"`
	System              []subnetSpec `json:"system"`
	Application         []subnetSpec `json:"application"`
	CloudEngine         []subnetSpec `json:"cloud_engine"`
	VerifiedApplication []subnetSpec `json:"verified_application"`
}

type instanceConfig struct {
	SubnetConfigSet          extendedSubnetConfigSet `json:"subnet_config_set"`
	HTTPGatewayConfig        *struct{}               `json:"http_gateway_config"`
	StateDir                 *string                 `json:"state_dir"`
	ICPConfig                *struct{}               `json:"icp_config"`
	LogLevel                 *string                 `json:"log_level"`
	BitcoindAddr             *string                 `json:"bitcoind_addr"`
	DogecoindAddr            *string                 `json:"dogecoind_addr"`
	ICPFeatures              *struct{}               `json:"icp_features"`
	IncompleteState          *bool                   `json:"incomplete_state"`
	InitialTime              *uint64                 `json:"initial_time"`
	MainnetNNSSubnetID       *string                 `json:"mainnet_nns_subnet_id"`
	DisableIngressValidation *bool                   `json:"disable_ingress_validation"`
}

type createInstanceResponse struct {
	Created *struct {
		InstanceID int             `json:"instance_id"`
		Topology   json.RawMessage `json:"topology"`
	} `json:"Created"`
	Error *struct {
		Message string `json:"message"`
	} `json:"Error"`
}

// NewInstance creates a PocketIC instance with one NNS subnet and one
// application subnet and returns its id.
func (c *Client) NewInstance() (int, error) {
	nns := newSubnet()
	cfg := instanceConfig{
		SubnetConfigSet: extendedSubnetConfigSet{
			NNS:                 &nns,
			System:              []subnetSpec{},
			Application:         []subnetSpec{newSubnet()},
			CloudEngine:         []subnetSpec{},
			VerifiedApplication: []subnetSpec{},
		},
	}
	raw, err := c.do("POST", "/instances", cfg)
	if err != nil {
		return 0, err
	}
	var resp createInstanceResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, fmt.Errorf("decode CreateInstanceResponse: %w (%s)", err, string(raw))
	}
	if resp.Error != nil {
		return 0, fmt.Errorf("create instance: %s", resp.Error.Message)
	}
	if resp.Created == nil {
		return 0, fmt.Errorf("create instance: unexpected response %s", string(raw))
	}
	return resp.Created.InstanceID, nil
}

// Tick advances the instance by one round. Needed before operations that
// require randomness (e.g. governance neuron-subaccount generation), whose RNG
// is only seeded once the instance makes progress.
func (c *Client) Tick(inst int) error {
	_, err := c.do("POST", fmt.Sprintf("/instances/%d/update/tick", inst), struct{}{})
	return err
}

func (c *Client) SetTime(inst int, nanosSinceEpoch uint64) error {
	body := struct {
		Nanos uint64 `json:"nanos_since_epoch"`
	}{nanosSinceEpoch}
	_, err := c.do("POST", fmt.Sprintf("/instances/%d/update/set_time", inst), body)
	return err
}

// AutoProgress enables background time progression, which lets canister timers
// and heartbeats fire (governance seeds its RNG from a timer).
func (c *Client) AutoProgress(inst int) error {
	body := struct {
		ArtificialDelayMs *uint64 `json:"artificial_delay_ms"`
	}{}
	_, err := c.do("POST", fmt.Sprintf("/instances/%d/auto_progress", inst), body)
	return err
}

// rawEffectivePrincipal is the externally-tagged enum: "None" | {"CanisterId":b64} | {"SubnetId":b64}.
type rawEffectivePrincipal struct {
	none       bool
	canisterID []byte
}

func (e rawEffectivePrincipal) MarshalJSON() ([]byte, error) {
	if e.none {
		return []byte(`"None"`), nil
	}
	return json.Marshal(map[string]string{"CanisterId": b64.EncodeToString(e.canisterID)})
}

type rawCanisterCall struct {
	Sender             string                `json:"sender"`
	CanisterID         string                `json:"canister_id"`
	EffectivePrincipal rawEffectivePrincipal `json:"effective_principal"`
	Method             string                `json:"method"`
	Payload            string                `json:"payload"`
}

// rawMessageID mirrors the server's RawMessageId. effective_principal is kept
// as raw JSON so it round-trips verbatim from submit into await (the value is
// marshal-asymmetric otherwise).
type rawMessageID struct {
	EffectivePrincipal json.RawMessage `json:"effective_principal"`
	MessageID          string          `json:"message_id"`
}

type rawCanisterResult struct {
	Ok  *string          `json:"Ok"`
	Err *json.RawMessage `json:"Err"`
}

func (c *Client) call(inst int, sender, canister principal.Principal, method string, payload []byte, update bool) ([]byte, error) {
	return c.callEff(inst, sender, canister, canister, method, payload, update)
}

// callEff is like call but with an explicit effective principal, which decides
// the routing subnet. For management-canister calls (canister = aaaaa-aa, empty
// bytes) the effective principal must be the TARGET canister id, not the empty
// management id, or the request routes to the default (application) subnet.
func (c *Client) callEff(inst int, sender, canister, effective principal.Principal, method string, payload []byte, update bool) ([]byte, error) {
	body := rawCanisterCall{
		Sender:             b64.EncodeToString(sender.Raw),
		CanisterID:         b64.EncodeToString(canister.Raw),
		EffectivePrincipal: rawEffectivePrincipal{canisterID: effective.Raw},
		Method:             method,
		Payload:            b64.EncodeToString(payload),
	}
	if !update {
		raw, err := c.do("POST", fmt.Sprintf("/instances/%d/read/query", inst), body)
		if err != nil {
			return nil, err
		}
		return unwrapResult(raw)
	}
	raw, err := c.do("POST", fmt.Sprintf("/instances/%d/update/submit_ingress_message", inst), body)
	if err != nil {
		return nil, err
	}
	// submit_ingress_message returns {"Ok": RawMessageId} | {"Err": ...}.
	var submit struct {
		Ok  *rawMessageID    `json:"Ok"`
		Err *json.RawMessage `json:"Err"`
	}
	if err := json.Unmarshal(raw, &submit); err != nil {
		return nil, fmt.Errorf("decode submit response: %w (%s)", err, string(raw))
	}
	if submit.Err != nil {
		return nil, fmt.Errorf("submit rejected: %s", string(*submit.Err))
	}
	if submit.Ok == nil {
		return nil, fmt.Errorf("submit: unexpected response %s", string(raw))
	}
	raw, err = c.do("POST", fmt.Sprintf("/instances/%d/update/await_ingress_message", inst), submit.Ok)
	if err != nil {
		return nil, err
	}
	return unwrapResult(raw)
}

func unwrapResult(raw []byte) ([]byte, error) {
	var res rawCanisterResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decode RawCanisterResult: %w (%s)", err, string(raw))
	}
	if res.Err != nil {
		return nil, fmt.Errorf("canister rejected: %s", string(*res.Err))
	}
	if res.Ok == nil {
		return nil, fmt.Errorf("unexpected result: %s", string(raw))
	}
	return b64.DecodeString(*res.Ok)
}

func (c *Client) Query(inst int, sender, canister principal.Principal, method string, args, out []any) error {
	return c.callCandid(inst, sender, canister, method, args, out, false)
}

func (c *Client) Update(inst int, sender, canister principal.Principal, method string, args, out []any) error {
	return c.callCandid(inst, sender, canister, method, args, out, true)
}

func (c *Client) callCandid(inst int, sender, canister principal.Principal, method string, args, out []any, update bool) error {
	return c.callCandidEff(inst, sender, canister, canister, method, args, out, update)
}

// UpdateEff is Update with an explicit effective principal (routing subnet).
func (c *Client) UpdateEff(inst int, sender, canister, effective principal.Principal, method string, args, out []any) error {
	return c.callCandidEff(inst, sender, canister, effective, method, args, out, true)
}

func (c *Client) callCandidEff(inst int, sender, canister, effective principal.Principal, method string, args, out []any, update bool) error {
	payload, err := candid.Marshal(args)
	if err != nil {
		return fmt.Errorf("encode args for %s: %w", method, err)
	}
	reply, err := c.callEff(inst, sender, canister, effective, method, payload, update)
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return nil
	}
	return candid.Unmarshal(reply, out)
}
