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

func NewIdentity() (*Identity, error) {
	id, err := identity.NewRandomEd25519Identity()
	if err != nil {
		return nil, err
	}
	return &Identity{id: id}, nil
}

// LoadIdentity loads an ed25519 identity from a PEM file (a secret, never committed).
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

func (i *Identity) Principal() principal.Principal { return i.id.Sender() }

func (i *Identity) PEM() ([]byte, error) { return i.id.ToPEM() }
