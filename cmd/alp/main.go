// Command alp is the terraform-like tool for managing IC nodes via NNS
// proposals. Proposals are declared in proposals.hcl; results are recorded in a
// single state.json keyed by proposal name.
//
// Subcommands:
//
//	apply  <name>   verify on PocketIC, then (with --yes) submit and record state
//	import <name> <proposal_id>   adopt an already-submitted proposal into state
//	list            show declared proposals and their recorded state
//
// apply always dry-runs first. If state already records a proposal id for the
// name with a matching payload hash, apply refuses to resubmit unless --force.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aviate-labs/agent-go/principal"
	"github.com/swiss-subnet/alpage/gen/governance"
	"github.com/swiss-subnet/alpage/nns"
	"github.com/swiss-subnet/alpage/pocketic"
)

// ProposerNeuronID is the Swiss Subnet's proposer neuron on mainnet.
const ProposerNeuronID uint64 = 12838523358913392196

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return usage()
	}
	switch os.Args[1] {
	case "apply":
		return apply(os.Args[2:])
	case "import":
		return importCmd(os.Args[2:])
	case "list":
		return list(os.Args[2:])
	case "registry":
		return registryCmd(os.Args[2:])
	case "-h", "--help", "help":
		return usage()
	default:
		return fmt.Errorf("unknown subcommand %q (try: apply, import, list, registry)", os.Args[1])
	}
}

func usage() error {
	fmt.Println(`usage:
  alp apply  <name> --identity <key.pem> [--neuron id] [--host url] [--yes] [--force]
  alp import <name> <proposal_id> --identity <key.pem> [--neuron id] [--host url] [--at RFC3339]
  alp list
  alp registry subnet <subnet_id> [--host url]`)
	return nil
}

func apply(argv []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	identityPath := fs.String("identity", "", "path to the ed25519 identity PEM (controller or hotkey of the neuron)")
	config := fs.String("config", nns.DefaultConfigPath, "path to the proposals HCL config")
	statePath := fs.String("state", nns.DefaultStatePath, "path to the consolidated state file")
	neuronID := fs.Uint64("neuron", 0, "proposer neuron id (overrides provider block)")
	host := fs.String("host", "", "IC host to submit to (overrides provider block)")
	yes := fs.Bool("yes", false, "actually submit to the live network after the dry-run")
	force := fs.Bool("force", false, "submit even if state already records a proposal id")
	pos, rest := splitArgs(argv, 1)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("apply: proposal name is required")
	}
	name := pos[0]
	if *identityPath == "" {
		return fmt.Errorf("--identity is required")
	}
	id, err := nns.LoadIdentity(*identityPath)
	if err != nil {
		return err
	}
	cfg, err := nns.LoadConfig(*config)
	if err != nil {
		return err
	}
	spec, err := findSpec(cfg, name)
	if err != nil {
		return err
	}
	action, err := spec.Action()
	if err != nil {
		return err
	}
	hash, err := spec.PayloadSHA256()
	if err != nil {
		return err
	}
	effHost, effNeuron := resolveHost(cfg.Provider, *host), resolveNeuron(cfg.Provider, *neuronID)
	fetchRootKey := cfg.Provider.ShouldFetchRootKey(effHost)

	st, err := nns.LoadState(*statePath)
	if err != nil {
		return err
	}
	if prev, ok := st.Proposals[name]; ok && prev.ProposalID != 0 && !*force {
		same := prev.PayloadSHA256 == hash
		fmt.Printf("already submitted as %d on %s (payload %s)\n", prev.ProposalID, prev.SubmittedAt, matchWord(same))
		if same {
			fmt.Println("nothing to do (pass --force to submit anyway).")
			return nil
		}
		return fmt.Errorf("state records proposal %d but the payload hash changed; refusing without --force", prev.ProposalID)
	}

	neuron := governance.NeuronId{Id: effNeuron}

	fmt.Printf("== Dry run on PocketIC (%s) ==\n", name)
	if err := dryRun(action); err != nil {
		return fmt.Errorf("dry run failed, not submitting: %w", err)
	}

	fmt.Printf("\nProposal:        %s\n", name)
	fmt.Printf("Payload sha256:  %s\n", hash)
	fmt.Printf("Submitting principal: %s\n", id.Principal().Encode())
	fmt.Printf("Target host:     %s (fetch_root_key=%t)\n", effHost, fetchRootKey)
	fmt.Printf("Proposer neuron: %d\n", effNeuron)

	if !*yes {
		fmt.Println("\nDry run only (pass --yes to submit to the live network).")
		return nil
	}
	if !confirm(fmt.Sprintf("Submit this proposal to %s? type 'submit' to proceed: ", effHost)) {
		fmt.Println("Aborted.")
		return nil
	}

	pid, err := nns.SubmitMainnet(id, effHost, fetchRootKey, neuron, action)
	if err != nil {
		return err
	}
	fmt.Printf("Submitted. Proposal id: %d\n", pid)

	st.Proposals[name] = nns.Entry{
		Kind:          spec.Kind,
		ProposalID:    pid,
		PayloadSHA256: hash,
		SubmittedBy:   id.Principal().Encode(),
		Neuron:        effNeuron,
		Host:          effHost,
		SubmittedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := nns.SaveState(*statePath, st); err != nil {
		return fmt.Errorf("submitted as %d but failed to write state %s: %w", pid, *statePath, err)
	}
	fmt.Printf("Recorded state: %s [%s]\n", *statePath, name)
	return nil
}

// importCmd adopts an already-submitted proposal into state, the terraform
// `import` equivalent. It records the id plus the payload hash currently
// declared for the name, so a later apply can detect drift.
func importCmd(argv []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	identityPath := fs.String("identity", "", "identity PEM used for the original submission (records submitted_by)")
	config := fs.String("config", nns.DefaultConfigPath, "path to the proposals HCL config")
	statePath := fs.String("state", nns.DefaultStatePath, "path to the consolidated state file")
	neuronID := fs.Uint64("neuron", ProposerNeuronID, "proposer neuron id used for the original submission")
	host := fs.String("host", nns.MainnetHost, "host the original submission went to")
	at := fs.String("at", "", "submission time (RFC3339); defaults to unknown")
	pos, rest := splitArgs(argv, 2)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if len(pos) != 2 {
		return fmt.Errorf("import: usage: import <name> <proposal_id> [flags]")
	}
	name, pidStr := pos[0], pos[1]
	var pid uint64
	if _, err := fmt.Sscan(pidStr, &pid); err != nil || pid == 0 {
		return fmt.Errorf("import: invalid proposal id %q", pidStr)
	}
	spec, err := nns.LoadSpec(*config, name)
	if err != nil {
		return err
	}
	submittedBy := ""
	if *identityPath != "" {
		id, err := nns.LoadIdentity(*identityPath)
		if err != nil {
			return err
		}
		submittedBy = id.Principal().Encode()
	}
	st, err := nns.LoadState(*statePath)
	if err != nil {
		return err
	}
	if err := st.Import(spec, pid, submittedBy, *host, *at, *neuronID); err != nil {
		return err
	}
	if err := nns.SaveState(*statePath, st); err != nil {
		return err
	}
	fmt.Printf("Imported %s as proposal %d into %s\n", name, pid, *statePath)
	return nil
}

// registryCmd groups read-only registry queries.
func registryCmd(argv []string) error {
	if len(argv) < 1 {
		return fmt.Errorf("registry: usage: registry subnet <subnet_id> [--host url]")
	}
	switch argv[0] {
	case "subnet":
		return registrySubnet(argv[1:])
	default:
		return fmt.Errorf("unknown registry query %q (try: subnet)", argv[0])
	}
}

func registrySubnet(argv []string) error {
	fs := flag.NewFlagSet("registry subnet", flag.ContinueOnError)
	host := fs.String("host", nns.MainnetHost, "IC host to query")
	pos, rest := splitArgs(argv, 1)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("registry subnet: subnet id is required")
	}
	subnetID, err := principal.Decode(pos[0])
	if err != nil {
		return fmt.Errorf("invalid subnet id %q: %w", pos[0], err)
	}
	fetchRootKey := *host != nns.MainnetHost
	nodes, err := nns.FetchSubnetMembership(*host, fetchRootKey, subnetID)
	if err != nil {
		return err
	}
	fmt.Print(nns.RenderResourcesHCL(subnetID.Encode(), nodes))
	return nil
}

func list(argv []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	config := fs.String("config", nns.DefaultConfigPath, "path to the proposals HCL config")
	statePath := fs.String("state", nns.DefaultStatePath, "path to the consolidated state file")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	cfg, err := nns.LoadConfig(*config)
	if err != nil {
		return err
	}
	st, err := nns.LoadState(*statePath)
	if err != nil {
		return err
	}
	for _, s := range cfg.Proposals {
		hash, err := s.PayloadSHA256()
		if err != nil {
			return err
		}
		entry, ok := st.Proposals[s.Name]
		switch {
		case !ok || entry.ProposalID == 0:
			fmt.Printf("%-24s  not submitted\n", s.Name)
		case entry.PayloadSHA256 == hash:
			fmt.Printf("%-24s  proposal %d  (in sync)\n", s.Name, entry.ProposalID)
		default:
			fmt.Printf("%-24s  proposal %d  (DRIFT: config payload changed since submit)\n", s.Name, entry.ProposalID)
		}
	}
	return nil
}

// splitArgs separates the first n leading positional arguments from the rest,
// so callers can write `import <name> <id> --flags` (Go's flag package
// otherwise stops parsing at the first positional). Any positionals appearing
// after flags remain in the flag set's trailing args.
func splitArgs(argv []string, n int) (pos, rest []string) {
	for len(pos) < n && len(argv) > 0 && !strings.HasPrefix(argv[0], "-") {
		pos = append(pos, argv[0])
		argv = argv[1:]
	}
	return pos, argv
}

// findSpec locates a proposal by name in an already-loaded config.
func findSpec(cfg *nns.Config, name string) (*nns.Spec, error) {
	for i := range cfg.Proposals {
		if cfg.Proposals[i].Name == name {
			return &cfg.Proposals[i], nil
		}
	}
	return nil, fmt.Errorf("proposal %q not found", name)
}

// resolveHost applies precedence: CLI flag > provider block > built-in default.
func resolveHost(p nns.Provider, flagHost string) string {
	if flagHost != "" {
		return flagHost
	}
	if p.Host != "" {
		return p.Host
	}
	return nns.MainnetHost
}

// resolveNeuron applies precedence: CLI flag > provider block > built-in default.
func resolveNeuron(p nns.Provider, flagNeuron uint64) uint64 {
	if flagNeuron != 0 {
		return flagNeuron
	}
	if p.Neuron != 0 {
		return p.Neuron
	}
	return ProposerNeuronID
}

func matchWord(same bool) string {
	if same {
		return "unchanged"
	}
	return "CHANGED"
}

// dryRun brings up a local NNS, submits the exact same action, and prints the
// decoded result. Any error here aborts the real submission.
func dryRun(action nns.Action) error {
	c, err := pocketic.Start("")
	if err != nil {
		return err
	}
	defer c.Close()
	inst, err := c.NewInstance()
	if err != nil {
		return err
	}
	local, err := nns.NewIdentity()
	if err != nil {
		return err
	}
	n, err := nns.BringUp(c, inst, local.Principal())
	if err != nil {
		return err
	}
	if err := c.SetTime(inst, 1_700_000_000_000_000_000); err != nil {
		return err
	}
	if err := c.AutoProgress(inst); err != nil {
		return err
	}
	// The local NNS seeds its proposer neuron at a fixed genesis id, so the
	// dry-run always submits as that neuron regardless of the mainnet --neuron.
	pid, err := n.SubmitAs(n.Proposer, n.ProposerNeuron(), action)
	if err != nil {
		return err
	}
	pi, err := n.GetProposalInfo(pid)
	if err != nil {
		return err
	}
	fmt.Print(nns.RenderVerbose(pi))
	return nil
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	s := bufio.NewScanner(os.Stdin)
	if !s.Scan() {
		return false
	}
	return strings.TrimSpace(s.Text()) == "submit"
}
