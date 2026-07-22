package nns

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Entry is the committed record of one submitted proposal: what was submitted,
// when, by whom, and what proposal id it became.
type Entry struct {
	Kind          string `json:"kind"`
	ProposalID    uint64 `json:"proposal_id"`
	PayloadSHA256 string `json:"payload_sha256"`
	SubmittedBy   string `json:"submitted_by"` // principal of the signing identity
	Neuron        uint64 `json:"neuron"`
	Host          string `json:"host"`
	SubmittedAt   string `json:"submitted_at"` // RFC3339
}

// State is the single consolidated state file (terraform.tfstate-style): one
// map of proposal name -> Entry. It is the source of truth for "what did we
// actually submit and what did it become".
type State struct {
	Proposals map[string]Entry `json:"proposals"`
}

// DefaultStatePath is the consolidated state file, relative to the module root.
const DefaultStatePath = "state.json"

// LoadState reads the consolidated state file. A missing file is an empty
// state, not an error, so a fresh repo can apply its first proposal.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &State{Proposals: map[string]Entry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", path, err)
	}
	if s.Proposals == nil {
		s.Proposals = map[string]Entry{}
	}
	return &s, nil
}

// SaveState writes the consolidated state with stable, diff-friendly
// formatting (keys sorted by Go's json map encoding).
func SaveState(path string, s *State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// Import records an already-submitted proposal into state (the terraform
// `import` equivalent): it adopts an existing proposal id without submitting
// anything. The payload hash is taken from the spec as currently declared, so a
// later apply can detect drift. It fails if the name is already recorded, to
// avoid silently clobbering a real submission.
func (s *State) Import(spec *Spec, pid uint64, submittedBy, host, at string, neuron uint64) error {
	if pid == 0 {
		return fmt.Errorf("import %s: proposal id must be non-zero", spec.Name)
	}
	if _, ok := s.Proposals[spec.Name]; ok {
		return fmt.Errorf("import %s: already recorded in state", spec.Name)
	}
	hash, err := spec.PayloadSHA256()
	if err != nil {
		return err
	}
	s.Proposals[spec.Name] = Entry{
		Kind:          spec.Kind,
		ProposalID:    pid,
		PayloadSHA256: hash,
		SubmittedBy:   submittedBy,
		Neuron:        neuron,
		Host:          host,
		SubmittedAt:   at,
	}
	return nil
}

// Names returns the recorded proposal names in sorted order.
func (s *State) Names() []string {
	out := make([]string, 0, len(s.Proposals))
	for k := range s.Proposals {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
