package elchi

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

// DNSSnapshot represents the full DNS snapshot response from Elchi backend.
type DNSSnapshot struct {
	Zone        string      `json:"zone"`
	VersionHash string      `json:"version_hash"`
	Records     []DNSRecord `json:"records"`
}

// DNSChangesResponse represents the response from the changes endpoint.
type DNSChangesResponse struct {
	Unchanged   bool        `json:"unchanged"`
	Zone        string      `json:"zone,omitempty"`
	VersionHash string      `json:"version_hash,omitempty"`
	Records     []DNSRecord `json:"records,omitempty"`
}

// DNSRecord represents a single DNS record from Elchi.
type DNSRecord struct {
	Name     string   `json:"name"`               // e.g., "listener1.gslb.elchi"
	Type     string   `json:"type"`               // "A" or "AAAA"
	TTL      uint32   `json:"ttl"`                // TTL in seconds
	IPs      []string `json:"ips"`                // List of IP addresses
	Enabled  bool     `json:"enabled"`            // If false, return CNAME to failover
	Failover string   `json:"failover,omitempty"` // CNAME target when enabled=false
}

// ElchiClient is the HTTP client for the Elchi DNS API.
type ElchiClient struct {
	endpoint   string
	zone       string
	secret     string
	httpClient *http.Client
}

// NewElchiClient creates a new Elchi DNS API client.
func NewElchiClient(endpoint, zone, secret string, timeout time.Duration) *ElchiClient {
	return &ElchiClient{
		endpoint: strings.TrimRight(endpoint, "/"),
		zone:     zone,
		secret:   secret,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
	}
}

// FetchSnapshot fetches the complete DNS snapshot from Elchi backend.
func (c *ElchiClient) FetchSnapshot(ctx context.Context) (*DNSSnapshot, error) {
	// Build request URL
	u, err := url.Parse(fmt.Sprintf("%s/dns/snapshot", c.endpoint))
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}

	q := u.Query()
	q.Set("zone", c.zone)
	u.RawQuery = q.Encode()

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication headers
	c.signRequest(req)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var snapshot DNSSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Validate response
	if snapshot.Zone == "" {
		return nil, fmt.Errorf("invalid response: missing zone")
	}
	if snapshot.VersionHash == "" {
		return nil, fmt.Errorf("invalid response: missing version_hash")
	}

	return &snapshot, nil
}

// CheckChanges checks for DNS changes since the given version hash.
func (c *ElchiClient) CheckChanges(ctx context.Context, sinceHash string) (*DNSChangesResponse, error) {
	// Build request URL
	u, err := url.Parse(fmt.Sprintf("%s/dns/changes", c.endpoint))
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}

	q := u.Query()
	q.Set("zone", c.zone)
	q.Set("since", sinceHash)
	u.RawQuery = q.Encode()

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication headers
	c.signRequest(req)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check status code
	// HTTP 304 Not Modified indicates no changes since the given hash
	if resp.StatusCode == http.StatusNotModified {
		return &DNSChangesResponse{
			Unchanged: true,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var changes DNSChangesResponse
	if err := json.NewDecoder(resp.Body).Decode(&changes); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Validate response
	if !changes.Unchanged {
		if changes.Zone == "" {
			return nil, fmt.Errorf("invalid response: missing zone in changes")
		}
		if changes.VersionHash == "" {
			return nil, fmt.Errorf("invalid response: missing version_hash in changes")
		}
	}

	return &changes, nil
}

// signRequest adds authentication headers to the request.
func (c *ElchiClient) signRequest(req *http.Request) {
	req.Header.Set("X-Elchi-Secret", c.secret)
	req.Header.Set("Accept", "application/json")
}
