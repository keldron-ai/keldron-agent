// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package whoami implements the keldron-agent whoami subcommand.
package whoami

import (
	"fmt"

	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/credentials"
)

const defaultEndpoint = "https://api.keldron.ai"

// Run executes the whoami command. Returns exit code.
func Run(args []string) int {
	_ = args

	creds, err := credentials.Load()
	if err != nil || creds == nil {
		fmt.Println("Not logged in.")
		fmt.Println("Run 'keldron-agent login' to connect to Keldron Cloud.")
		return 0
	}

	fmt.Printf("Email:    %s\n", creds.Email)
	fmt.Printf("Account:  %s\n", creds.AccountID)
	fmt.Printf("API Key:  %s\n", config.MaskedCloudAPIKey(creds.APIKey))
	endpoint := creds.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	fmt.Printf("Endpoint: %s\n", endpoint)
	return 0
}
