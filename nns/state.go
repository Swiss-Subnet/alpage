package nns

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	// Status is the proposal's lifecycle state as last observed on-chain,
	// recorded by `alp status`. Empty means never observed. Once terminal it
	// never changes, which is what lets `list` stay offline and still know that
	// drift against this entry is inert.
	Status ProposalState `json:"status,omitempty"`
	// ResolvedAt is when the proposal reached its terminal state on-chain
	// (RFC3339), taken from the chain's own timestamp rather than when alp
	// happened to look.
	ResolvedAt string `json:"resolved_at,omitempty"`
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
// formatting (keys sorted by Go's json map encoding). It writes to a temp file
// in the same directory and renames it into place so a crash mid-write cannot
// corrupt the record of what was submitted on-chain.
func SaveState(path string, s *State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
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

// ApplyOutcome is the resubmission guard's verdict for one apply.
type ApplyOutcome int

const (
	ApplyProceed     ApplyOutcome = iota // no prior real submission (or --force): submit
	ApplyNothingToDo                     // already submitted with the identical payload: skip
)

// ApplyDecision is the resubmission guard: same payload is a no-op, drifted
// payload errors unless force, a zero proposal id is not a real prior. The
// returned *Entry is the prior that drove the verdict (nil when none) so
// callers report from it rather than recomputing the predicate.
//
// A proposal in a terminal state is refused outright, force included:
// resubmitting under the same name would overwrite the record of what actually
// happened on-chain. Resubmitting means giving the new proposal its own name.
func (s *State) ApplyDecision(name, hash string, force bool) (ApplyOutcome, *Entry, error) {
	prev, ok := s.Proposals[name]
	if !ok || prev.ProposalID == 0 {
		return ApplyProceed, nil, nil
	}
	if prev.Status.Terminal() {
		return ApplyProceed, &prev, fmt.Errorf(
			"proposal %d is %s and cannot be resubmitted under the same name; rename the proposal block to submit a new one",
			prev.ProposalID, prev.Status)
	}
	if force {
		return ApplyProceed, &prev, nil
	}
	if prev.PayloadSHA256 == hash {
		return ApplyNothingToDo, &prev, nil
	}
	return ApplyProceed, &prev, fmt.Errorf("state records proposal %d but the payload hash changed; refusing without --force", prev.ProposalID)
}

// RecordState updates a recorded proposal's observed lifecycle state, and
// reports whether anything changed. Monotonic: a terminal state is never
// overwritten, and an empty observation never clears a recorded one, so a
// governance purge or a transient query cannot erase known history. resolvedAt
// is the chain's own resolution timestamp and is recorded only for a terminal
// state.
func (s *State) RecordState(name string, st ProposalState, resolvedAt string) bool {
	e, ok := s.Proposals[name]
	if !ok || st == "" || e.Status == st || e.Status.Terminal() {
		return false
	}
	e.Status = st
	if st.Terminal() {
		e.ResolvedAt = resolvedAt
	}
	s.Proposals[name] = e
	return true
}

// RecordSubmittedAt corrects a recorded proposal's submission time from the
// chain's own proposal timestamp, and reports whether anything changed. The
// chain is authoritative here: whatever was recorded at submit or import time
// was at best a local clock reading and at worst a hand-written placeholder. An
// empty observation never clears a recorded value.
func (s *State) RecordSubmittedAt(name, at string) bool {
	e, ok := s.Proposals[name]
	if !ok || at == "" || e.SubmittedAt == at {
		return false
	}
	e.SubmittedAt = at
	s.Proposals[name] = e
	return true
}

func (s *State) Names() []string {
	out := make([]string, 0, len(s.Proposals))
	for k := range s.Proposals {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
