package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/bdobrica/ThinkPixelWS/api/openapi"
	"github.com/google/uuid"
)

const (
	defaultServerURL = "http://127.0.0.1:8080"
	defaultTimeout   = 30 * time.Second
)

type environment func(string) string

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "thinkpixelwsctl:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv environment, stdout, stderr io.Writer) error {
	root := flag.NewFlagSet("thinkpixelwsctl", flag.ContinueOnError)
	root.SetOutput(stderr)
	serverURL := root.String("server", valueOrDefault(getenv("THINKPIXELWS_API_URL"), defaultServerURL), "ThinkPixelWS API base URL")
	tokenFile := root.String("token-file", getenv("THINKPIXELWS_TOKEN_FILE"), "file containing an OIDC bearer token")
	root.Usage = func() { printUsage(stderr) }
	if err := root.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	remaining := root.Args()
	if len(remaining) == 0 || remaining[0] == "help" {
		printUsage(stdout)
		return nil
	}
	if remaining[0] != "workspace" {
		return fmt.Errorf("unknown command %q", remaining[0])
	}
	if len(remaining) < 2 {
		return errors.New("workspace requires a subcommand: list or describe")
	}

	client, err := newAPIClient(*serverURL, *tokenFile)
	if err != nil {
		return err
	}
	switch remaining[1] {
	case "list":
		return runWorkspaceList(ctx, client, remaining[2:], stdout, stderr)
	case "describe":
		return runWorkspaceDescribe(ctx, client, remaining[2:], stdout, stderr)
	default:
		return fmt.Errorf("unknown workspace subcommand %q", remaining[1])
	}
}

func runWorkspaceList(ctx context.Context, client openapi.Invoker, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("workspace list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	cursor := flags.String("cursor", "", "opaque cursor returned by a previous request")
	limit := flags.Int("limit", 0, "maximum number of workspaces (1-500)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("workspace list does not accept positional arguments")
	}
	if *limit < 0 || *limit > 500 {
		return errors.New("--limit must be between 1 and 500 when set")
	}

	params := openapi.ListWorkspacesParams{}
	if *cursor != "" {
		params.Cursor.SetTo(*cursor)
	}
	if *limit != 0 {
		params.Limit.SetTo(*limit)
	}
	page, err := client.ListWorkspaces(ctx, params)
	if err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}
	return writeJSON(stdout, page)
}

func runWorkspaceDescribe(ctx context.Context, client openapi.Invoker, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("workspace describe", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: thinkpixelwsctl workspace describe WORKSPACE_ID")
	}
	id, err := uuid.Parse(flags.Arg(0))
	if err != nil {
		return fmt.Errorf("invalid workspace ID: %w", err)
	}
	workspace, err := client.GetWorkspace(ctx, openapi.GetWorkspaceParams{WorkspaceID: openapi.UUID(id)})
	if err != nil {
		return fmt.Errorf("describe workspace: %w", err)
	}
	return writeJSON(stdout, workspace)
}

func newAPIClient(serverURL, tokenFile string) (*openapi.Client, error) {
	return newAPIClientWithTransport(serverURL, tokenFile, http.DefaultTransport)
}

func newAPIClientWithTransport(serverURL, tokenFile string, base http.RoundTripper) (*openapi.Client, error) {
	endpoint, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse API URL: %w", err)
	}
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && isLoopback(endpoint.Hostname())) {
		return nil, errors.New("API URL must use HTTPS; HTTP is allowed only for loopback development endpoints")
	}
	if endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("API URL must be an absolute base URL without user info, query, or fragment")
	}
	if tokenFile == "" {
		return nil, errors.New("OIDC token file is required (--token-file or THINKPIXELWS_TOKEN_FILE)")
	}
	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("read OIDC token file: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return nil, errors.New("OIDC token file must contain one non-empty token")
	}

	httpClient := &http.Client{Timeout: defaultTimeout, Transport: bearerTransport{token: token, base: base}}
	client, err := openapi.NewClient(endpoint.String(), openapi.WithClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create API client: %w", err)
	}
	return client, nil
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(request)
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func valueOrDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  thinkpixelwsctl [global flags] workspace list [--cursor CURSOR] [--limit N]
  thinkpixelwsctl [global flags] workspace describe WORKSPACE_ID

Global flags:
  --server URL       ThinkPixelWS API base URL (THINKPIXELWS_API_URL)
  --token-file PATH  file containing an OIDC bearer token (THINKPIXELWS_TOKEN_FILE)

Responses are written as JSON. The CLI always calls the public API.`)
}
