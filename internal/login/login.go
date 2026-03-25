// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package login implements the keldron-agent login subcommand.
package login

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/credentials"
	"golang.org/x/term"
)

const defaultEndpoint = "https://api.keldron.ai"

// Run executes the login command. Returns exit code.
func Run(args []string) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	endpoint := fs.String("endpoint", defaultEndpoint, "Keldron API endpoint")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}

	reader := bufio.NewReader(os.Stdin)

	// Check if already logged in
	existing, _ := credentials.Load()
	if existing != nil && (existing.APIKey != "" || existing.Email != "") {
		fmt.Printf("Already logged in as %s\n", existing.Email)
		fmt.Print("Log in as a different account? (y/N): ")
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(answer)) != "y" {
			return 0
		}
	}

	fmt.Print("Email: ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)
	if email == "" {
		fmt.Fprintln(os.Stderr, "Email cannot be empty")
		return 1
	}

	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
		return 1
	}
	password := string(passwordBytes)

	body, err := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode request: %v\n", err)
		return 1
	}

	base := strings.TrimRight(*endpoint, "/")
	parsed, err := url.Parse(base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid endpoint URL: %v\n", err)
		return 1
	}
	host := parsed.Hostname()
	isLocal := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if parsed.Scheme != "https" && !isLocal {
		fmt.Fprintln(os.Stderr, "Endpoint must use HTTPS for non-local hosts.")
		return 1
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(
		base+"/v1/auth/login",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		fmt.Fprintln(os.Stderr, "Invalid email or password.")
		return 1
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Login failed (HTTP %d)\n", resp.StatusCode)
		return 1
	}

	var result struct {
		Token     string `json:"token"`
		APIKey    string `json:"api_key"`
		Email     string `json:"email"`
		AccountID string `json:"account_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse response: %v\n", err)
		return 1
	}

	apiKey := strings.TrimSpace(result.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(result.Token)
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Login response did not include an API key or token.")
		return 1
	}

	emailOut := result.Email
	if emailOut == "" {
		emailOut = email
	}

	creds := &credentials.Credentials{
		APIKey:    apiKey,
		Email:     emailOut,
		AccountID: result.AccountID,
		Endpoint:  *endpoint,
	}
	if err := credentials.Save(creds); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save credentials: %v\n", err)
		return 1
	}

	fmt.Println()
	fmt.Printf("✓ Logged in as %s\n", emailOut)
	fmt.Printf("  API key saved to ~/.keldron/credentials\n")
	fmt.Println()
	fmt.Println("Your agent will now stream telemetry to Keldron Cloud.")
	fmt.Println("Restart the agent to begin streaming, or run:")
	fmt.Println("  KELDRON_CLOUD_API_KEY=<your-api-key> keldron-agent")
	return 0
}
