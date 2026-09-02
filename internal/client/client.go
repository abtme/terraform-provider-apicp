// Package client is a thin REST/JSON client for apicp's own control-plane
// API (github.com/abtme/apicp) — bearer-token auth, one endpoint per
// resource type. No account-scoping header exists yet: apicp's own
// auth.Token has no account concept in its current MVP, so unlike this
// author's other Terraform providers there is deliberately no
// APICP_ACCOUNT env var here — adding one would silently do nothing
// against the real API today. Revisit once apicp's own account model
// exists.
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ErrNotFound is returned when apicpd reports a 404 for the requested resource.
var ErrNotFound = errors.New("apicp: resource not found")

// Client talks to one apicpd instance.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// New creates a new apicp API client.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// errorResponse matches api.httpError's {"error": "..."} shape.
type errorResponse struct {
	Error string `json:"error"`
}

// debugHTTP reports whether APICP_DEBUG_HTTP is set, enabling verbose
// request/response logging to stderr — same env var name pattern as
// every sibling provider's *_DEBUG_HTTP flag. The bearer token is
// redacted regardless.
func debugHTTP() bool {
	return os.Getenv("APICP_DEBUG_HTTP") != ""
}

func (c *Client) redact(s string) string {
	if c.Token != "" {
		s = strings.ReplaceAll(s, c.Token, "[redacted]")
	}
	return s
}

// request performs an HTTP call against apicpd and decodes a successful
// JSON response into target (if non-nil). A nil body sends no request
// body at all (matters for endpoints like the certificate issuer, which
// take no input).
func (c *Client) request(method, path string, query url.Values, body, target any) error {
	reqURL := c.BaseURL + "/" + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	var bodyReader io.Reader
	var bodyForLog string
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
		bodyForLog = string(b)
	}

	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if debugHTTP() {
		fmt.Fprintf(os.Stderr, "[apicp] --> %s %s\n", method, c.redact(reqURL))
		if bodyForLog != "" {
			fmt.Fprintf(os.Stderr, "[apicp]     body: %s\n", c.redact(bodyForLog))
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if debugHTTP() {
		fmt.Fprintf(os.Stderr, "[apicp] <-- %d\n", resp.StatusCode)
		if len(respBody) > 0 {
			fmt.Fprintf(os.Stderr, "[apicp]     body: %s\n", c.redact(string(respBody)))
		}
	}

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e errorResponse
		if json.Unmarshal(respBody, &e) == nil && e.Error != "" {
			return fmt.Errorf("apicp API error (%d): %s", resp.StatusCode, e.Error)
		}
		return fmt.Errorf("apicp API error (%d): %s", resp.StatusCode, string(respBody))
	}

	if target != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, target); err != nil {
			return fmt.Errorf("decoding response: %w (body: %s)", err, respBody)
		}
	}
	return nil
}

// Get performs a GET and decodes into target.
func (c *Client) Get(path string, target any) error {
	return c.request(http.MethodGet, path, nil, nil, target)
}

// Post performs a POST with a JSON body and decodes the response into
// target (nil body if body is nil — used by endpoints that take no input,
// like certificate issuance).
func (c *Client) Post(path string, body, target any) error {
	return c.request(http.MethodPost, path, nil, body, target)
}

// Patch performs a PATCH with a JSON body and decodes the response into target.
func (c *Client) Patch(path string, body, target any) error {
	return c.request(http.MethodPatch, path, nil, body, target)
}

// Delete performs a DELETE. apicp's delete endpoints return 204 with no body.
func (c *Client) Delete(path string) error {
	return c.request(http.MethodDelete, path, nil, nil, nil)
}
