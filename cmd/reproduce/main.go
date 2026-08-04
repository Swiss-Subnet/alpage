// Command reproduce brings up a local NNS on PocketIC, submits the synthetic
// membership fixture, and prints the decoded proposal. Run from the repo root.
package main

import (
	"fmt"
	"os"

	"github.com/swiss-subnet/alpage/nns"
	"github.com/swiss-subnet/alpage/pocketic"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	c, err := pocketic.Start("")
	if err != nil {
		return err
	}
	defer c.Close()

	inst, err := c.NewInstance()
	if err != nil {
		return err
	}
	controller, err := nns.NewIdentity()
	if err != nil {
		return err
	}
	n, err := nns.BringUp(c, inst, controller.Principal())
	if err != nil {
		return err
	}
	if err := c.SetTime(inst, 1_700_000_000_000_000_000); err != nil {
		return err
	}
	if err := c.AutoProgress(inst); err != nil {
		return err
	}

	spec, err := nns.LoadSpec("nns/testdata/golden_src/proposals.hcl", "membership-fixture")
	if err != nil {
		return err
	}
	membership, err := spec.Proposal()
	if err != nil {
		return err
	}
	pid, err := n.SubmitMembership(n.ProposerNeuron(), membership)
	if err != nil {
		return err
	}
	pi, err := n.GetProposalInfo(pid)
	if err != nil {
		return err
	}
	fmt.Print(nns.Render(pi))
	return nil
}
