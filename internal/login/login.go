// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package login implements the keldron-agent login subcommand.
package login

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/credentials"
	"golang.org/x/term"
)

const defaultEndpoint = "https://api.keldron.ai"

var (
	errInvalidAPIKey      = errors.New("invalid API key")
	errEndpointNeedsHTTPS = errors.New("endpoint must use HTTPS for non-local hosts")
)

// Run executes the login command. Returns exit code.
func Run(args []string) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	endpoint := fs.String("endpoint", defaultEndpoint, "Keldron API endpoint")
	apiKeyFlag := fs.String("api-key", "", "API key (skips interactive prompts)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: keldron-agent login [flags]

Authenticate with Keldron Cloud. Log in with email/password or paste
your API key from app.keldron.ai.

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "login: unexpected argument: %s\n", fs.Arg(0))
		return 1
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-sigChan:
			signal.Stop(sigChan)
			fmt.Println()
			os.Exit(130)
		case <-done:
			return
		}
	}()
	defer close(done)

	client := &http.Client{Timeout: 10 * time.Second}

	keyFromFlag := strings.TrimSpace(*apiKeyFlag)
	if keyFromFlag != "" {
		return apiKeyLogin(client, *endpoint, keyFromFlag)
	}

	reader := bufio.NewReader(os.Stdin)

	existing, _ := credentials.Load()
	if existing != nil {
		if strings.TrimSpace(existing.Email) != "" {
			fmt.Printf("Already logged in as %s\n", existing.Email)
		} else {
			fmt.Println("Already logged in (API key on file)")
		}
		fmt.Print("Log in as a different account? (y/N): ")
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(answer)) != "y" {
			return 0
		}
	}

	fmt.Println()
	fmt.Println("How would you like to log in?")
	fmt.Println("  1. Email and password")
	fmt.Println("  2. Paste API key (from app.keldron.ai)")
	fmt.Println()
	for {
		fmt.Print("Choice (1/2): ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		switch choice {
		case "1":
			return emailPasswordLogin(reader, client, *endpoint)
		case "2":
			fmt.Print("API key: ")
			keyLine, _ := reader.ReadString('\n')
			key := strings.TrimSpace(keyLine)
			if key == "" {
				fmt.Fprintln(os.Stderr, "API key cannot be empty")
				return 1
			}
			return apiKeyLogin(client, *endpoint, key)
		default:
			fmt.Fprintln(os.Stderr, "Please enter 1 or 2.")
		}
	}
}

func normalizeEndpoint(endpoint string) (base string, err error) {
	base = strings.TrimRight(endpoint, "/")
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	host := parsed.Hostname()
	isLocal := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if parsed.Scheme != "https" && !isLocal {
		return "", errEndpointNeedsHTTPS
	}
	return base, nil
}

func validateAPIKey(client *http.Client, base, key string) error {
	req, err := http.NewRequest(http.MethodGet, base+"/v1/fleet/overview", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", key)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Connection failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		_, _ = io.Copy(io.Discard, resp.Body)
		return errInvalidAPIKey
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	msg := strings.TrimSpace(string(body))
	if msg != "" {
		return fmt.Errorf("API key check failed (HTTP %d): %s", resp.StatusCode, msg)
	}
	return fmt.Errorf("API key check failed (HTTP %d)", resp.StatusCode)
}

func apiKeyLogin(client *http.Client, endpoint, key string) int {
	base, err := normalizeEndpoint(endpoint)
	if err != nil {
		if errors.Is(err, errEndpointNeedsHTTPS) {
			fmt.Fprintln(os.Stderr, "Endpoint must use HTTPS for non-local hosts.")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Invalid endpoint URL: %v\n", err)
		return 1
	}

	if err := validateAPIKey(client, base, key); err != nil {
		if errors.Is(err, errInvalidAPIKey) {
			fmt.Fprintln(os.Stderr, "✗ Invalid API key. Check your key at app.keldron.ai and try again.")
			return 1
		}
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	creds := &credentials.Credentials{
		APIKey:    key,
		Email:     "",
		AccountID: "",
		Endpoint:  base,
	}
	if err := credentials.Save(creds); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save credentials: %v\n", err)
		return 1
	}

	fmt.Println("✓ API key verified and saved to ~/.keldron/credentials")
	fmt.Println("  Your agent will now stream telemetry to Keldron Cloud.")
	fmt.Println("  Restart the agent to begin streaming.")
	return 0
}

func emailPasswordLogin(reader *bufio.Reader, client *http.Client, endpoint string) int {
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

	base, err := normalizeEndpoint(endpoint)
	if err != nil {
		if errors.Is(err, errEndpointNeedsHTTPS) {
			fmt.Fprintln(os.Stderr, "Endpoint must use HTTPS for non-local hosts.")
			return 1
		}
		fmt.Fprintf(os.Stderr, "Invalid endpoint URL: %v\n", err)
		return 1
	}

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
	if strings.TrimSpace(result.AccountID) == "" {
		fmt.Fprintln(os.Stderr, "Login response did not include an account ID.")
		return 1
	}

	emailOut := strings.TrimSpace(result.Email)
	if emailOut == "" {
		emailOut = email
	}

	creds := &credentials.Credentials{
		APIKey:    apiKey,
		Email:     emailOut,
		AccountID: strings.TrimSpace(result.AccountID),
		Endpoint:  base,
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
