package nns

import (
	"fmt"
	"os"

	"github.com/aviate-labs/agent-go/identity"
	"github.com/aviate-labs/agent-go/principal"
)

// Identity wraps an ed25519 identity used to derive the proposer/hotkey
// principals. On PocketIC the server-native endpoints do not verify signatures,
// so only the derived principal (Sender) is used here; the keypair still makes
// the setup a real self-authenticating identity rather than a bare text id.
type Identity struct {
	id *identity.Ed25519Identity
}

// NewIdentity generates a fresh random ed25519 identity.
func NewIdentity() (*Identity, error) {
	id, err := identity.NewRandomEd25519Identity()
	if err != nil {
		return nil, err
	}
	return &Identity{id: id}, nil
}

// LoadIdentity loads an ed25519 identity from a PEM file (never committed).
func LoadIdentity(path string) (*Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	id, err := identity.NewEd25519IdentityFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse identity PEM %s: %w", path, err)
	}
	return &Identity{id: id}, nil
}

// Principal returns the self-authenticating principal derived from the key.
func (i *Identity) Principal() principal.Principal { return i.id.Sender() }

// PEM returns the PEM encoding of the identity (for saving/reuse).
func (i *Identity) PEM() ([]byte, error) { return i.id.ToPEM() }
