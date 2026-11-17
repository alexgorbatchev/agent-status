package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

const (
	defaultAgentServerURL = "http://127.0.0.1:4502"
	agentServerTimeout    = 10 * time.Second
)

func fetchServerAgents(ctx context.Context, baseURL, token string) ([]types.IAgentState, error) {
	var agents []types.IAgentState
	err := fetchServerJSON(ctx, baseURL, token, []string{"api", "v1", "agents"}, url.Values{"lifecycle": {string(types.LifecycleRunning)}}, "agent list", &agents)
	return agents, err
}

func agentServerURL() string {
	if endpoint := os.Getenv("AGENT_SERVER_HTTP_URL"); endpoint != "" {
		return endpoint
	}
	return defaultAgentServerURL
}

func fetchServerJSON(ctx context.Context, baseURL, token string, segments []string, query url.Values, label string, dest any) (err error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("server URL must be an HTTP(S) base URL without credentials, query parameters, or fragments")
	}
	if strings.ContainsAny(token, "\r\n") {
		return errors.New("AGENT_STATUS_TOKEN contains invalid header characters")
	}
	u = u.JoinPath(segments...)
	u.RawQuery = query.Encode()
	defer func() {
		if err != nil {
			err = fmt.Errorf("GET %s: %w", u.String(), err)
		}
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return errors.New("could not construct agent-server request; check the server URL")
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{
		Timeout: agentServerTimeout,
		// Require the final API URL so redirects cannot forward the bearer credential.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return serverTransportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("agent-server returned HTTP %d; check AGENT_STATUS_TOKEN matches the server", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent-server returned HTTP %d", resp.StatusCode)
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("invalid %s from agent-server: unexpected JSON response", label)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("invalid %s from agent-server: unexpected trailing data", label)
	}
	return nil
}

func serverTransportError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("requesting agent-server: %w", ctx.Err())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("agent-server request timed out: %w", context.DeadlineExceeded)
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return errors.New("cannot reach agent-server: connection refused; check the server address and whether agent-server is running")
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return errors.New("cannot reach agent-server: DNS lookup failed; check the server hostname and DNS configuration")
	}
	var certificate *tls.CertificateVerificationError
	if errors.As(err, &certificate) {
		return errors.New("agent-server TLS certificate verification failed; check the server hostname and trusted certificate authorities")
	}
	// Do not print raw HTTP errors: malformed redirect URLs and headers can
	// contain server-supplied credentials. The caller adds the validated endpoint.
	return errors.New("agent-server request failed; check connectivity, TLS, and the HTTP response")
}
