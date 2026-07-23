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
	case "status":
		return status(os.Args[2:])
	case "plan":
		return plan(os.Args[2:])
	case "reconcile":
		return reconcile(os.Args[2:])
	case "registry":
		return registryCmd(os.Args[2:])
	case "-h", "--help", "help":
		return usage()
	default:
		return fmt.Errorf("unknown subcommand %q (try: apply, plan, import, list, status, reconcile, registry)", os.Args[1])
	}
}

func usage() error {
	fmt.Println(`usage:
  alp apply  <name> --identity <key.pem> [--neuron id] [--host url] [--yes] [--force]
  alp plan   <name> [--host url]
  alp import <name> <proposal_id> --identity <key.pem> [--neuron id] [--host url] [--at RFC3339]
  alp list
  alp status [--host url]
  alp reconcile [--host url]
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
	outcome, prev, err := st.ApplyDecision(name, hash, *force)
	if prev != nil {
		fmt.Printf("already submitted as %d on %s (payload %s)\n", prev.ProposalID, prev.SubmittedAt, matchWord(prev.PayloadSHA256 == hash))
	}
	if err != nil {
		return err
	}
	if outcome == nns.ApplyNothingToDo {
		fmt.Println("nothing to do (pass --force to submit anyway).")
		return nil
	}

	neuron := governance.NeuronId{Id: effNeuron}

	fmt.Printf("== Dry run on PocketIC (%s) ==\n", name)
	if err := dryRun(action); err != nil {
		return fmt.Errorf("dry run failed, not submitting: %w", err)
	}

	// Second, independent no-op gate: ApplyDecision above guards against
	// resubmitting a recorded payload; this guards against submitting one that
	// is a no-op against live on-chain state. Either can abort.
	if err := planCheck(action, effHost, fetchRootKey, *force); err != nil {
		return err
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

	sub := nns.MainnetSubmitter{Identity: id, Host: effHost, FetchRootKey: fetchRootKey}
	args := nns.RecordArgs{
		Name:        name,
		Kind:        spec.Kind,
		Hash:        hash,
		SubmittedBy: id.Principal().Encode(),
		Neuron:      effNeuron,
		Host:        effHost,
		At:          time.Now().UTC().Format(time.RFC3339),
	}
	save := func(s *nns.State) error { return nns.SaveState(*statePath, s) }
	pid, err := nns.SubmitAndRecord(sub, st, neuron, action, args, save)
	if err != nil {
		return err
	}
	fmt.Printf("Submitted. Proposal id: %d\n", pid)
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

// planCheck runs the action's preflight against live state before submit. A
// no-op result is refused unless force; warnings print but do not block.
func planCheck(action nns.Action, host string, fetchRootKey, force bool) error {
	pf, err := action.Preflight(host, fetchRootKey)
	if err != nil {
		return fmt.Errorf("preflight: %w", err)
	}
	if pf.Report == "" {
		return nil
	}
	fmt.Print("\n== Preflight vs on-chain state ==\n")
	fmt.Print(pf.Report)
	if pf.Level == nns.PreflightNoOp && !force {
		return fmt.Errorf("proposal is a no-op against current on-chain state; refusing to submit without --force")
	}
	return nil
}

// plan reconciles a proposal against live on-chain state, kind-agnostically via
// Preflight. Read-only counterpart to apply's PocketIC dry-run: the dry-run
// checks the payload encodes and executes, plan checks it against reality.
func plan(argv []string) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	config := fs.String("config", nns.DefaultConfigPath, "path to the proposals HCL config")
	host := fs.String("host", "", "IC host to query (overrides provider block)")
	pos, rest := splitArgs(argv, 1)
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("plan: proposal name is required")
	}
	cfg, err := nns.LoadConfig(*config)
	if err != nil {
		return err
	}
	spec, err := findSpec(cfg, pos[0])
	if err != nil {
		return err
	}
	action, err := spec.Action()
	if err != nil {
		return err
	}
	effHost := resolveHost(cfg.Provider, *host)
	fetchRootKey := cfg.Provider.ShouldFetchRootKey(effHost)
	pf, err := action.Preflight(effHost, fetchRootKey)
	if err != nil {
		return err
	}
	if pf.Level == nns.PreflightClean {
		if pf.Report != "" {
			fmt.Print(pf.Report)
		}
		return nil
	}
	fmt.Print(pf.Report)
	return fmt.Errorf("plan flagged issues; review before applying")
}

// status reads each recorded proposal back from live governance and reports its
// actual on-chain status (open/adopted/rejected/executed/failed), closing the
// loop between what state says we submitted and what became of it. Read-only.
func status(argv []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	config := fs.String("config", nns.DefaultConfigPath, "path to the proposals HCL config")
	statePath := fs.String("state", nns.DefaultStatePath, "path to the consolidated state file")
	host := fs.String("host", "", "IC host to query (overrides provider block)")
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
	effHost := resolveHost(cfg.Provider, *host)
	fetchRootKey := cfg.Provider.ShouldFetchRootKey(effHost)
	for _, s := range cfg.Proposals {
		entry, ok := st.Proposals[s.Name]
		if !ok || entry.ProposalID == 0 {
			fmt.Printf("%-24s  not submitted\n", s.Name)
			continue
		}
		ps, err := nns.FetchProposalStatus(effHost, fetchRootKey, entry.ProposalID)
		if err != nil {
			fmt.Printf("%-24s  proposal %d  status query failed: %v\n", s.Name, entry.ProposalID, err)
			continue
		}
		fmt.Println(nns.StatusLine(s.Name, entry, ps))
	}
	return nil
}

// reconcile diffs the declared resources.hcl against live on-chain registry
// membership, per subnet. Read-only, and the resource-level counterpart to
// status: list/status close the config<->state<->network loop for proposals,
// reconcile closes it for the node/subnet inventory. Exits nonzero on drift so
// it is usable as a CI gate.
func reconcile(argv []string) error {
	fs := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	config := fs.String("config", nns.DefaultConfigPath, "path to the proposals HCL config")
	host := fs.String("host", "", "IC host to query (overrides provider block)")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	cfg, err := nns.LoadConfig(*config)
	if err != nil {
		return err
	}
	nns.Color = nns.DetectColor()
	effHost := resolveHost(cfg.Provider, *host)
	fetchRootKey := cfg.Provider.ShouldFetchRootKey(effHost)
	if len(cfg.Resources.Subnets) == 0 && len(cfg.Resources.Providers) == 0 {
		fmt.Println("no subnet or node_provider resources declared in resources.hcl")
		return nil
	}
	var b strings.Builder
	drift := false
	for _, sn := range cfg.Resources.Subnets {
		subnetID, err := principal.Decode(sn.ID)
		if err != nil {
			return fmt.Errorf("subnet %q: invalid id %q: %w", sn.Name, sn.ID, err)
		}
		live, err := nns.FetchSubnetMembership(effHost, fetchRootKey, subnetID)
		if err != nil {
			return fmt.Errorf("subnet %q: %w", sn.Name, err)
		}
		status, err := nodeStatusForSubnet(cfg.Resources, sn.ID, live)
		if err != nil {
			return fmt.Errorf("subnet %q: %w", sn.Name, err)
		}
		rc := nns.Reconcile(cfg.Resources, sn.ID, live, status)
		rc.Render(&b)
		drift = drift || rc.HasDrift()
	}
	if len(cfg.Resources.Providers) > 0 {
		byProvider, err := providerOperators(cfg.Resources, effHost, fetchRootKey)
		if err != nil {
			return err
		}
		pr := nns.ReconcileProviders(cfg.Resources, byProvider)
		pr.Render(&b)
		drift = drift || pr.HasDrift()

		byOperator, err := operatorNodes(cfg.Resources)
		if err != nil {
			return err
		}
		nr := nns.ReconcileOperatorNodes(cfg.Resources, byOperator)
		nr.Render(&b)
		drift = drift || nr.HasDrift()
	}
	fmt.Print(b.String())
	if drift {
		return fmt.Errorf("reconcile found drift between resources.hcl and on-chain state")
	}
	return nil
}

// providerOperators queries, for each declared node_provider, the operators and
// data centers the registry records it owns. Trustless: a typed registry query,
// no HTTP explorer. Progress is on stderr.
func providerOperators(r *nns.Resources, host string, fetchRootKey bool) (map[string][]nns.ProviderOperator, error) {
	out := make(map[string][]nns.ProviderOperator, len(r.Providers))
	for i, p := range r.Providers {
		fmt.Fprintf(os.Stderr, "checking node provider %d/%d (%s)...\n", i+1, len(r.Providers), p.ID)
		pid, err := principal.Decode(p.ID)
		if err != nil {
			return nil, fmt.Errorf("provider %q: invalid id %q: %w", p.Name, p.ID, err)
		}
		ops, err := nns.FetchProviderOperators(host, fetchRootKey, pid)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", p.Name, err)
		}
		out[p.ID] = ops
	}
	return out, nil
}

// operatorNodes queries, for each declared node_operator, the node ids the
// registry records it owns. See FetchOperatorNodes's HTTP-trust caveat; the
// explorer is used because there is no typed operator->nodes query. Progress is
// on stderr.
func operatorNodes(r *nns.Resources) (map[string][]string, error) {
	out := make(map[string][]string, len(r.Operators))
	for i, op := range r.Operators {
		fmt.Fprintf(os.Stderr, "checking node operator %d/%d (%s)...\n", i+1, len(r.Operators), op.ID)
		nodes, err := nns.FetchOperatorNodes(nns.DefaultRegistryExplorer, op.ID)
		if err != nil {
			return nil, fmt.Errorf("operator %q: %w", op.Name, err)
		}
		out[op.ID] = nodes
	}
	return out, nil
}

// nodeStatusForSubnet fetches registry registration state for each declared
// node on a subnet that is not a current live member: those are the candidates
// for the deregistered-vs-missing distinction. Progress is on stderr.
func nodeStatusForSubnet(r *nns.Resources, subnetID string, live []string) (map[string]nns.NodeStatus, error) {
	member := map[string]bool{}
	for _, id := range live {
		member[id] = true
	}
	var pending []string
	for _, n := range r.Nodes {
		if n.Subnet == subnetID && !member[n.ID] {
			pending = append(pending, n.ID)
		}
	}
	if len(pending) == 0 {
		return nil, nil
	}
	status := make(map[string]nns.NodeStatus, len(pending))
	for i, id := range pending {
		fmt.Fprintf(os.Stderr, "checking node record %d/%d (%s)...\n", i+1, len(pending), id)
		s, err := nns.FetchNodeStatus(nns.DefaultRegistryExplorer, id)
		if err != nil {
			return nil, err
		}
		status[id] = s
	}
	return status, nil
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
