// Package nodeapi is a typed client for the signetd HTTP API.
//
// The platform only ever calls the *unauthenticated* endpoints itself —
// health, info, and debug stats — because those are all it needs to run the
// marketplace's liveness view. Every authenticated call (keygen, sign,
// delegate, admin key listing) is made by the developer's own browser with
// the developer's own credential and merely forwarded by Proxy, so the
// platform never holds anything that could authorize a signature.
package nodeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to one signetd instance.
type Client struct {
	BaseURL string
	http    *http.Client
}

// New builds a client with a short timeout: these calls sit in the request
// path of console page loads, so a hung node must fail fast rather than hold
// a connection open.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 8 * time.Second},
	}
}

// Health is the response of GET /v1/health.
type Health struct {
	Status string `json:"status"`
}

// Info is the response of GET /v1/info — the node's network identity.
type Info struct {
	PeerID          string   `json:"peer_id"`
	EthereumAddress string   `json:"ethereum_address"`
	Addrs           []string `json:"addrs"`
	NodeType        string   `json:"node_type"`
}

// DebugStats is the response of GET /debug/stats.
type DebugStats struct {
	Peers       int `json:"peers"`
	Connections int `json:"connections"`
	Goroutines  int `json:"goroutines"`
	HeapAllocMB int `json:"heap_alloc_mb"`
}

// KeyInfo is one entry of the POST /admin/keys response. The platform caches
// these after the console fetches them with the developer's auth key.
type KeyInfo struct {
	GroupID         string   `json:"group_id"`
	KeyID           string   `json:"key_id"`
	Curve           string   `json:"curve"`
	PublicKey       string   `json:"public_key"`
	EthereumAddress string   `json:"ethereum_address"`
	Threshold       int      `json:"threshold"`
	Parties         []string `json:"parties"`
	Status          string   `json:"status"`
	Scope           string   `json:"scope"`
}

// Health checks node liveness.
func (c *Client) Health(ctx context.Context) (*Health, error) {
	var out Health
	if err := c.get(ctx, "/v1/health", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Info reads the node's identity.
func (c *Client) Info(ctx context.Context) (*Info, error) {
	var out Info
	if err := c.get(ctx, "/v1/info", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Stats reads the node's peer and memory counters.
func (c *Client) Stats(ctx context.Context) (*DebugStats, error) {
	var out DebugStats
	if err := c.get(ctx, "/debug/stats", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Probe measures liveness and round-trip latency in one call, which is what
// the marketplace health sampler records.
func (c *Client) Probe(ctx context.Context) (ok bool, latency time.Duration, peers int, err error) {
	start := time.Now()
	h, err := c.Health(ctx)
	latency = time.Since(start)
	if err != nil {
		return false, latency, 0, err
	}
	if h.Status != "ok" {
		return false, latency, 0, fmt.Errorf("node reports status %q", h.Status)
	}
	// Peer count is a nice-to-have; a node with /debug/stats disabled is still
	// healthy, so a failure here does not demote the probe.
	if s, serr := c.Stats(ctx); serr == nil {
		peers = s.Peers
	}
	return true, latency, peers, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("GET %s: status %d: %s", path, res.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// ProxyRequest is one forwarded call from the console to a node.
type ProxyRequest struct {
	NodeURL string
	Path    string
	Method  string
	Body    []byte
}

// ProxyResponse carries the node's reply back to the console verbatim.
type ProxyResponse struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

// AllowedProxyPaths is the exact set of node paths the platform will forward.
// It is an allowlist rather than a denylist: the proxy is reachable by any
// signed-in developer, so a path that is not deliberately exposed must not be
// reachable through it.
var AllowedProxyPaths = map[string]bool{
	"/v1/health":            true,
	"/v1/info":              true,
	"/v1/auth":              true,
	"/v1/keygen":            true,
	"/v1/sign":              true,
	"/v1/delegate":          true,
	"/v1/keys/disable":      true,
	"/v1/keys/enable":       true,
	"/v1/keys/delete":       true,
	"/admin/keys":           true,
	"/admin/reshare":        true,
	"/admin/reshare/status": true,
	"/debug/stats":          true,
}

// Proxy forwards a console request to a node and returns the raw response.
//
// It exists because signetd sets no CORS headers, so a browser cannot call a
// node directly. The forwarded body is opaque to the platform — the session
// signature inside it was produced by the developer's key and is verified by
// the node, not here.
func Proxy(ctx context.Context, client *http.Client, req ProxyRequest) (*ProxyResponse, error) {
	if !AllowedProxyPaths[req.Path] {
		return nil, fmt.Errorf("path %q is not proxyable", req.Path)
	}
	target, err := url.Parse(strings.TrimRight(req.NodeURL, "/") + req.Path)
	if err != nil {
		return nil, fmt.Errorf("bad node url: %w", err)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf("node url must be http or https")
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if len(req.Body) > 0 {
		body = strings.NewReader(string(req.Body))
	}
	hreq, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	if len(req.Body) > 0 {
		hreq.Header.Set("Content-Type", "application/json")
	}

	res, err := client.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("proxy %s %s: %w", method, req.Path, err)
	}
	defer res.Body.Close()
	// Signing responses are small; the cap stops a misbehaving node from
	// streaming unbounded data through the platform.
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read proxied response: %w", err)
	}
	ct := res.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	return &ProxyResponse{StatusCode: res.StatusCode, ContentType: ct, Body: raw}, nil
}
