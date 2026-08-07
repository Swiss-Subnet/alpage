package nns

import "time"

// ChipEntry is a recorded AMD verdict for one chip id. Only positive verdicts
// are stored: a chip AMD vouches for stays genuine, so the lookup never needs
// repeating. A refusal is worth re-checking (wrong product guess, KDS outage)
// and a rate limit says nothing at all, so neither is recorded.
type ChipEntry struct {
	Verified   bool   `json:"verified"`
	Product    string `json:"product,omitempty"`
	VerifiedAt string `json:"verified_at"` // RFC3339
}

// VerifyChipsCached returns an AMD verdict per distinct chip, asking verify
// only for chips with no recorded verdict. New positive verdicts are added to
// state; the caller saves it.
func VerifyChipsCached(s *State, chips []string, verify func(string) ChipVerification) map[string]ChipVerification {
	return verifyChips(s, chips, verify, false)
}

// RefreshChips re-asks AMD about every chip, ignoring what state records. It is
// the escape hatch for a verdict cached wrongly: a refusal drops the entry, so
// a chip stops reading as verified once AMD stops vouching for it. An
// inconclusive answer leaves the existing entry untouched, since a rate limit
// is not evidence against a chip.
func RefreshChips(s *State, chips []string, verify func(string) ChipVerification) map[string]ChipVerification {
	return verifyChips(s, chips, verify, true)
}

func verifyChips(s *State, chips []string, verify func(string) ChipVerification, refresh bool) map[string]ChipVerification {
	if s.Chips == nil {
		s.Chips = make(map[string]ChipEntry)
	}
	out := make(map[string]ChipVerification, len(chips))
	for _, chip := range chips {
		if chip == "" {
			continue
		}
		if _, done := out[chip]; done {
			continue
		}
		if e, ok := s.Chips[chip]; ok && e.Verified && !refresh {
			out[chip] = ChipVerification{ChipID: chip, Verified: true, Product: e.Product}
			continue
		}
		v := verify(chip)
		out[chip] = v
		switch {
		case v.Verified:
			s.Chips[chip] = ChipEntry{
				Verified:   true,
				Product:    v.Product,
				VerifiedAt: time.Now().UTC().Format(time.RFC3339),
			}
		case refresh && !v.Inconclusive:
			delete(s.Chips, chip)
		}
	}
	return out
}
