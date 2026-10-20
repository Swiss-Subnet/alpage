package nns

import (
	"testing"

	"github.com/aviate-labs/agent-go/principal"
)

// Fixture principals are minted, not invented. A hand-typed principal almost
// always fails the CRC32 checksum principal.Decode enforces, and it fails at
// config load with "invalid checksum" rather than anywhere near the fixture, so
// the cost of guessing is an hour of looking in the wrong place.
//
// mintPrincipal derives a valid self-authenticating principal from a seed
// string. Deterministic, so the literal committed in testdata can be checked
// against it rather than trusted.
func mintPrincipal(seed string) principal.Principal {
	return principal.NewSelfAuthenticating([]byte(seed))
}

// fixturePrincipals maps each minted testdata id to the seed it came from. To
// add one, put the seed here with an empty id and run this test: it reports the
// principal to paste into the HCL.
var fixturePrincipals = map[string]string{
	"golden node_provider np2": "x6osd-5npm5-fpmp4-4d5xj-bphuq-sut6o-yw5us-mjhlj-cjx6u-w5vzw-nqe",
	"golden node_operator op2": "prius-wr533-ucjae-zih7h-cfbne-2a7yf-q5sob-onsfu-ugi45-ihsgq-kqe",
}

// Reward accounts are minted for the same reason principals are. Governance
// validates the account identifier's embedded CRC32 at submission.
func TestFixtureRewardAccount(t *testing.T) {
	const want = "453c3699c3809eaca2ba129333cffc7665b739818b0d227283c2b77b29488c8d"
	got := principal.NewAccountID(mintPrincipal("golden node_provider np2"), principal.DefaultSubAccount)
	if got.Encode() != want {
		t.Errorf("reward account for np2 = %s, testdata has %s", got.Encode(), want)
	}
}

func TestFixturePrincipals(t *testing.T) {
	for seed, want := range fixturePrincipals {
		got := mintPrincipal(seed).Encode()
		if got != want {
			t.Errorf("mintPrincipal(%q) = %s, testdata has %s", seed, got, want)
		}
		if _, err := principal.Decode(want); err != nil {
			t.Errorf("testdata principal %s does not decode: %v", want, err)
		}
	}
}

// TestGoldenSrcPrincipalsDecode is the backstop the minting convention does not
// cover: every principal in the golden fixtures, minted or inherited, has to
// decode. LoadConfig does the decoding, so a bad one anywhere fails here.
func TestGoldenSrcPrincipalsDecode(t *testing.T) {
	if _, err := LoadConfig("testdata/golden_src/proposals.hcl"); err != nil {
		t.Fatalf("load golden fixtures: %v", err)
	}
}
