package pocketic

import (
	"fmt"
	"net/http"
	"time"

	"github.com/aviate-labs/agent-go/candid/idl"
	"github.com/aviate-labs/agent-go/principal"
)

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second}
}

type canisterSettings struct {
	Controllers *[]principal.Principal `ic:"controllers,omitempty" json:"controllers,omitempty"`
}

type provisionalCreateArgs struct {
	Amount                *int                 `ic:"amount,omitempty" json:"amount,omitempty"`
	Settings              *canisterSettings    `ic:"settings,omitempty" json:"settings,omitempty"`
	SpecifiedID           *principal.Principal `ic:"specified_id,omitempty" json:"specified_id,omitempty"`
	SenderCanisterVersion *uint64              `ic:"sender_canister_version,omitempty" json:"sender_canister_version,omitempty"`
}

type canisterIDRecord struct {
	CanisterID principal.Principal `ic:"canister_id" json:"canister_id"`
}

// installMode is the variant { install; reinstall; upgrade: opt record{...} }.
// Arms are unit variants (null payload), so the active arm is idl.Null.
type installMode struct {
	Install   *idl.Null `ic:"install,variant" json:"install,omitempty"`
	Reinstall *idl.Null `ic:"reinstall,variant" json:"reinstall,omitempty"`
}

type installCodeArgs struct {
	Mode                  installMode         `ic:"mode" json:"mode"`
	CanisterID            principal.Principal `ic:"canister_id" json:"canister_id"`
	WasmModule            []byte              `ic:"wasm_module" json:"wasm_module"`
	Arg                   []byte              `ic:"arg" json:"arg"`
	SenderCanisterVersion *uint64             `ic:"sender_canister_version,omitempty" json:"sender_canister_version,omitempty"`
}

// CreateCanisterWithID provisionally creates a canister at the given id with
// the given cycles, controlled by controller (anonymous if empty).
func (c *Client) CreateCanisterWithID(inst int, id principal.Principal, cycles int, controller principal.Principal) error {
	sender := controller
	if len(sender.Raw) == 0 {
		sender = principal.AnonymousID
	}
	args := provisionalCreateArgs{Amount: &cycles, SpecifiedID: &id}
	if len(controller.Raw) > 0 {
		args.Settings = &canisterSettings{Controllers: &[]principal.Principal{controller}}
	}
	var out canisterIDRecord
	if err := c.UpdateEff(inst, sender, managementCanister, id, "provisional_create_canister_with_cycles",
		[]any{args}, []any{&out}); err != nil {
		return fmt.Errorf("provisional_create_canister_with_cycles(%s): %w", id.Encode(), err)
	}
	if out.CanisterID.Encode() != id.Encode() {
		return fmt.Errorf("created %s, expected %s", out.CanisterID.Encode(), id.Encode())
	}
	return nil
}

// InstallCode installs wasm (raw bytes; gzipped modules are accepted by the
// replica) into the canister with the given candid-encoded init arg.
func (c *Client) InstallCode(inst int, id principal.Principal, wasm, arg []byte, controller principal.Principal) error {
	sender := controller
	if len(sender.Raw) == 0 {
		sender = principal.AnonymousID
	}
	args := installCodeArgs{
		Mode:       installMode{Install: &idl.Null{}},
		CanisterID: id,
		WasmModule: wasm,
		Arg:        arg,
	}
	if err := c.UpdateEff(inst, sender, managementCanister, id, "install_code", []any{args}, nil); err != nil {
		return fmt.Errorf("install_code(%s): %w", id.Encode(), err)
	}
	return nil
}
