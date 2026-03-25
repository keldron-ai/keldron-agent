// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package logout implements the keldron-agent logout subcommand.
package logout

import (
	"flag"
	"fmt"
	"os"

	"github.com/keldron-ai/keldron-agent/internal/credentials"
)

// Run executes the logout command. Returns exit code.
func Run(args []string) int {
	fs := flag.NewFlagSet("logout", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "logout: unexpected argument: %s\n", fs.Arg(0))
		return 1
	}

	existing, err := credentials.Load()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Not logged in.")
			return 0
		}
		fmt.Fprintf(os.Stderr, "Failed to read credentials: %v\n", err)
		return 1
	}
	if existing == nil {
		fmt.Println("Not logged in.")
		return 0
	}

	email := existing.Email

	if err := credentials.Delete(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to remove credentials: %v\n", err)
		return 1
	}

	fmt.Printf("✓ Logged out (%s)\n", email)
	fmt.Println("  Credentials removed from ~/.keldron/credentials")
	fmt.Println("  Agent may revert to local-only mode on next restart unless cloud.api_key or KELDRON_CLOUD_API_KEY is set.")
	return 0
}
