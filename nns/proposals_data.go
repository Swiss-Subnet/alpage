package nns

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// SwissSubnetWave1 is loaded from the committed proposals.hcl, the single
// source of truth for the payload. It is kept as a package var for the
// reproduce command and the e2e tests; the HCL is authoritative.
var SwissSubnetWave1 = mustLoadSpec("swiss-subnet-wave1")

// mustLoadSpec loads a named proposal from the repo's proposals.hcl, resolved
// relative to this source file so it works from any working directory (tests
// run from nns/, commands from the module root). It panics on failure because
// these are committed, in-tree specs that must always parse.
func mustLoadSpec(name string) ResizeProposal {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot resolve source path for spec loading")
	}
	root := filepath.Dir(filepath.Dir(self)) // nns/ -> repo root
	path := filepath.Join(root, DefaultConfigPath)
	spec, err := LoadSpec(path, name)
	if err != nil {
		panic(fmt.Sprintf("load spec %s: %v", name, err))
	}
	p, err := spec.Proposal()
	if err != nil {
		panic(fmt.Sprintf("decode spec %s: %v", name, err))
	}
	return p
}
