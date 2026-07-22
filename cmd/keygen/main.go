// Command keygen generates a new ed25519 identity, writes its PEM to a file,
// and prints the derived principal to add as a neuron hotkey.
//
// Usage:
//
//	go run ./cmd/keygen --out hotkey.pem
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/swiss-subnet/alpage/nns"
)

func main() {
	out := flag.String("out", "", "path to write a NEW PEM identity to")
	show := flag.String("show", "", "print the principal of an EXISTING PEM identity (does not generate)")
	force := flag.Bool("force", false, "overwrite the out file if it exists")
	flag.Parse()

	if *show != "" {
		id, err := nns.LoadIdentity(*show)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Println(id.Principal().Encode())
		return
	}

	if *out == "" {
		fmt.Fprintln(os.Stderr, "error: --out is required (or use --show <file> to read an existing key)")
		os.Exit(1)
	}
	if _, err := os.Stat(*out); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "error: %s already exists (use --force to overwrite)\n", *out)
		os.Exit(1)
	}

	id, err := nns.NewIdentity()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	pem, err := id.PEM()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, pem, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	fmt.Printf("wrote identity to %s (keep this private, mode 0600)\n", *out)
	fmt.Printf("principal (add this as the neuron hotkey): %s\n", id.Principal().Encode())
}
