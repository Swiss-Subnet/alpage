package nns

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempState(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	return path
}

// A verified chip is immutable: silicon does not stop being genuine, so a
// cached verdict is reused and AMD is never asked twice.
func TestChipCacheServesVerified(t *testing.T) {
	s := &State{Chips: map[string]ChipEntry{
		testChipA: {Verified: true, Product: "Milan-B0", VerifiedAt: "2026-08-07T12:00:00Z"},
	}}
	asked := 0
	got := VerifyChipsCached(s, []string{testChipA}, func(chip string) ChipVerification {
		asked++
		return ChipVerification{ChipID: chip, Verified: true}
	})
	if asked != 0 {
		t.Errorf("AMD was asked %d times for a cached chip, want 0", asked)
	}
	if v := got[testChipA]; !v.Verified || v.Product != "Milan-B0" {
		t.Errorf("cached verdict not served, got %+v", v)
	}
}

// An unknown chip is looked up and the verdict recorded, so the next run is
// offline for it.
func TestChipCacheRecordsNewVerification(t *testing.T) {
	s := &State{Chips: map[string]ChipEntry{}}
	got := VerifyChipsCached(s, []string{testChipA}, func(chip string) ChipVerification {
		return ChipVerification{ChipID: chip, Verified: true, Product: "Milan-B0"}
	})
	if !got[testChipA].Verified {
		t.Fatalf("expected verified, got %+v", got[testChipA])
	}
	e, ok := s.Chips[testChipA]
	if !ok {
		t.Fatal("verdict was not recorded in state")
	}
	if !e.Verified || e.Product != "Milan-B0" {
		t.Errorf("recorded %+v, want verified Milan-B0", e)
	}
	if e.VerifiedAt == "" {
		t.Error("recorded entry carries no timestamp")
	}
}

// Only a positive verdict is durable. A refusal or a rate limit must not be
// cached: the first is worth re-checking, the second says nothing at all.
func TestChipCacheDoesNotPersistNegatives(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    ChipVerification
	}{
		{"refusal", ChipVerification{Err: "AMD KDS does not know this chip"}},
		{"rate limit", ChipVerification{Inconclusive: true, Err: "rate-limited"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &State{Chips: map[string]ChipEntry{}}
			VerifyChipsCached(s, []string{testChipA}, func(string) ChipVerification { return tc.v })
			if len(s.Chips) != 0 {
				t.Errorf("negative verdict was cached: %+v", s.Chips)
			}
		})
	}
}

// A chip is asked about once even when several nodes carry it.
func TestChipCacheDedupes(t *testing.T) {
	s := &State{Chips: map[string]ChipEntry{}}
	asked := 0
	VerifyChipsCached(s, []string{testChipA, testChipA, testChipB}, func(chip string) ChipVerification {
		asked++
		return ChipVerification{ChipID: chip, Verified: true}
	})
	if asked != 2 {
		t.Errorf("asked %d times, want 2 (one per distinct chip)", asked)
	}
}

// Refresh re-asks AMD even for a chip already recorded, so a verdict recorded
// wrongly can be corrected without hand-editing state.json.
func TestChipCacheRefreshReasks(t *testing.T) {
	s := &State{Chips: map[string]ChipEntry{
		testChipA: {Verified: true, Product: "stale", VerifiedAt: "2026-01-01T00:00:00Z"},
	}}
	asked := 0
	got := RefreshChips(s, []string{testChipA}, func(chip string) ChipVerification {
		asked++
		return ChipVerification{ChipID: chip, Verified: true, Product: "Milan-B0"}
	})
	if asked != 1 {
		t.Errorf("AMD was asked %d times, want 1", asked)
	}
	if got[testChipA].Product != "Milan-B0" {
		t.Errorf("got %+v, want the fresh verdict", got[testChipA])
	}
	if s.Chips[testChipA].Product != "Milan-B0" {
		t.Errorf("state kept the stale verdict: %+v", s.Chips[testChipA])
	}
}

// A refresh that comes back negative drops the cached verdict rather than
// leaving a chip marked verified that AMD no longer vouches for.
func TestChipCacheRefreshDropsOnRefusal(t *testing.T) {
	s := &State{Chips: map[string]ChipEntry{
		testChipA: {Verified: true, Product: "Milan-B0", VerifiedAt: "2026-01-01T00:00:00Z"},
	}}
	RefreshChips(s, []string{testChipA}, func(string) ChipVerification {
		return ChipVerification{Err: "AMD KDS does not know this chip"}
	})
	if _, ok := s.Chips[testChipA]; ok {
		t.Error("a refused chip should not stay cached as verified")
	}
}

// An inconclusive refresh keeps what we already knew: a rate limit is not
// evidence against a chip AMD previously vouched for.
func TestChipCacheRefreshKeepsOnInconclusive(t *testing.T) {
	s := &State{Chips: map[string]ChipEntry{
		testChipA: {Verified: true, Product: "Milan-B0", VerifiedAt: "2026-01-01T00:00:00Z"},
	}}
	RefreshChips(s, []string{testChipA}, func(string) ChipVerification {
		return ChipVerification{Inconclusive: true, Err: "rate-limited"}
	})
	e, ok := s.Chips[testChipA]
	if !ok || !e.Verified {
		t.Error("an inconclusive refresh must not drop a known-good verdict")
	}
	if e.VerifiedAt != "2026-01-01T00:00:00Z" {
		t.Error("an inconclusive refresh must not restamp the entry")
	}
}

// State written before chip caching existed loads with no chips rather than
// failing.
func TestStateWithoutChipsLoads(t *testing.T) {
	path := writeTempState(t, `{"proposals":{}}`)
	s, err := LoadState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Chips == nil {
		t.Error("Chips should be initialized, not nil")
	}
}
