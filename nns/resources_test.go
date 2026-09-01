package nns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeResources(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resources.hcl")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDuplicateResourceNameRejected(t *testing.T) {
	path := writeResources(t, `
subnet "app" {
  id = "aaaaa-aa"
}
subnet "app" {
  id = "bbbbb-bb"
}
`)
	_, err := loadResources(path)
	if err == nil {
		t.Fatal("duplicate subnet name accepted")
	}
	if !strings.Contains(err.Error(), "app") {
		t.Errorf("error does not name the collision: %v", err)
	}
	// Both declarations are located, so the author can find them.
	if strings.Count(err.Error(), "resources.hcl:") != 2 {
		t.Errorf("error does not locate both declarations: %v", err)
	}
}

func TestDuplicateResourceIDRejected(t *testing.T) {
	path := writeResources(t, `
node "n1" {
  id = "aaaaa-aa"
}
node "n2" {
  id = "aaaaa-aa"
}
`)
	_, err := loadResources(path)
	if err == nil {
		t.Fatal("duplicate node id accepted")
	}
	if !strings.Contains(err.Error(), "aaaaa-aa") {
		t.Errorf("error does not name the collision: %v", err)
	}
}

// Names collide only within a kind: a subnet and a node may share a name, since
// they occupy separate variables in the eval context.
func TestSameNameDifferentKindsAllowed(t *testing.T) {
	path := writeResources(t, `
subnet "shared" {
  id = "aaaaa-aa"
}
node "shared" {
  id = "bbbbb-bb"
}
`)
	if _, err := loadResources(path); err != nil {
		t.Fatalf("same name across kinds rejected: %v", err)
	}
}

// An id may repeat across kinds: a self-operated provider registers the same
// principal as both node_provider and node_operator.
func TestSameIDAcrossKindsAllowed(t *testing.T) {
	path := writeResources(t, `
node_provider "p" {
  id = "aaaaa-aa"
}
node_operator "p" {
  id = "aaaaa-aa"
}
`)
	if _, err := loadResources(path); err != nil {
		t.Fatalf("same id across kinds rejected: %v", err)
	}
}

// resources.hcl and resources/*.hcl are merged into one body, so a collision
// spanning files must be caught too.
func TestDuplicateAcrossFilesRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resources.hcl")
	if err := os.WriteFile(path, []byte("subnet \"app\" {\n  id = \"aaaaa-aa\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, ResourcesDir)
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "extra.hcl"), []byte("subnet \"app\" {\n  id = \"bbbbb-bb\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadResources(path)
	if err == nil {
		t.Fatal("cross-file duplicate accepted")
	}
	// Naming both files is the whole point: the collision is invisible otherwise.
	for _, want := range []string{"resources.hcl:", "extra.hcl:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}

func TestDuplicateProposalNameRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "resources.hcl"), []byte("subnet \"s\" {\n  id = \""+fxSubnet+"\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "proposals.hcl")
	if err := os.WriteFile(path, []byte(`
proposal "upgrade" {
  kind  = "deploy_guestos"
  title = "first"
  deploy_guestos {
    subnet_id          = subnet.s.id
    replica_version_id = "v1"
  }
}
proposal "upgrade" {
  kind  = "deploy_guestos"
  title = "second"
  deploy_guestos {
    subnet_id          = subnet.s.id
    replica_version_id = "v2"
  }
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("duplicate proposal name accepted")
	}
	if !strings.Contains(err.Error(), "upgrade") {
		t.Errorf("error does not name the collision: %v", err)
	}
}

// A guestos_version resource lets many nodes share one declared version, so a
// fleet-wide rollout is a single edit rather than one per node.
func TestGuestosVersionResourceResolves(t *testing.T) {
	const hash = "0c121276f3152d29b5d5b0a25f56ff0a83e64f3e"
	path := writeResources(t, `
guestos_version "latest" {
  id = "`+hash+`"
}
node "n1" {
  id              = "aaaaa-aa"
  guestos_version = guestos_version.latest.id
}
`)
	rs, err := loadResources(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rs.Nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(rs.Nodes))
	}
	if got := rs.Nodes[0].GuestosVersion; got != hash {
		t.Errorf("guestos_version = %q, want %q", got, hash)
	}
}

// The kind participates in collision checking like every other resource.
func TestDuplicateGuestosVersionNameRejected(t *testing.T) {
	path := writeResources(t, `
guestos_version "latest" {
  id = "aaaa"
}
guestos_version "latest" {
  id = "bbbb"
}
`)
	_, err := loadResources(path)
	if err == nil {
		t.Fatal("duplicate guestos_version name accepted")
	}
	if !strings.Contains(err.Error(), "latest") {
		t.Errorf("error does not name the collision: %v", err)
	}
}

// The default is the point of the alias: a node that declares nothing inherits
// it, so a release bump is one edit rather than one per node.
func TestGuestosVersionDefaultInherited(t *testing.T) {
	path := writeResources(t, `
guestos_version "current" {
  id      = "aaaa"
  label   = "release-current"
  default = true
}
guestos_version "pinned" {
  id = "bbbb"
}
node "inherits" {
  id = "aaaaa-aa"
}
node "overrides" {
  id              = "bbbbb-bb"
  guestos_version = guestos_version.pinned.id
}
node "optedout" {
  id              = "ccccc-cc"
  guestos_version = "none"
}
node "gone" {
  id             = "ddddd-dd"
  decommissioned = true
}
`)
	rs, err := loadResources(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := map[string]VersionExpectation{
		"inherits":  {Version: "aaaa", Source: VersionFromDefault},
		"overrides": {Version: "bbbb", Source: VersionFromNode},
	}
	nodes, exp := rs.VersionCheckedNodes()
	if len(nodes) != len(want) {
		t.Fatalf("checked %d nodes, want %d: %+v", len(nodes), len(want), nodes)
	}
	for i, n := range nodes {
		w, ok := want[n.Name]
		if !ok {
			t.Errorf("node %q should not be version-checked", n.Name)
			continue
		}
		if exp[i] != w {
			t.Errorf("node %q: expectation = %+v, want %+v", n.Name, exp[i], w)
		}
	}
}

// Without a default, an undeclared node stays unchecked: the feature must not
// silently start checking every node in an existing config.
func TestNoDefaultLeavesNodesUnchecked(t *testing.T) {
	path := writeResources(t, `
guestos_version "current" {
  id = "aaaa"
}
node "n1" {
  id = "aaaaa-aa"
}
`)
	rs, err := loadResources(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if nodes, _ := rs.VersionCheckedNodes(); len(nodes) != 0 {
		t.Errorf("checked %+v, want none", nodes)
	}
}

// Two defaults would make inheritance depend on block order.
func TestTwoDefaultVersionsRejected(t *testing.T) {
	path := writeResources(t, `
guestos_version "current" {
  id      = "aaaa"
  default = true
}
guestos_version "next" {
  id      = "bbbb"
  default = true
}
`)
	_, err := loadResources(path)
	if err == nil {
		t.Fatal("two defaults accepted")
	}
	for _, want := range []string{"current", "next"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %v", want, err)
		}
	}
}

// An alias points at the same hash as the catalogue entry it tracks, so the
// duplicate-id rule must not apply to versions.
func TestGuestosVersionsMayShareAnID(t *testing.T) {
	path := writeResources(t, `
guestos_version "release_2026_07_23_04_21_base" {
  id = "aaaa"
}
guestos_version "current" {
  id      = "aaaa"
  default = true
}
`)
	if _, err := loadResources(path); err != nil {
		t.Fatalf("an alias sharing the catalogue's hash must load: %v", err)
	}
}
