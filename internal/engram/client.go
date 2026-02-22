package engram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Config holds Engram client configuration.
type Config struct {
	URL     string
	Timeout time.Duration
}

// Client communicates with the Engram HTTP API.
// All methods degrade gracefully: if the server is unreachable,
// methods return zero values and nil errors instead of failing.
type Client struct {
	baseURL    string
	httpClient *http.Client
	available  bool
	mu         sync.RWMutex
}

// SearchResult mirrors the Engram store.SearchResult type.
type SearchResult struct {
	ID      int64   `json:"id"`
	Title   string  `json:"title"`
	Type    string  `json:"type"`
	Snippet string  `json:"snippet"`
	Rank    float64 `json:"rank"`
}

// NewClient creates a Client and performs a health check.
// Returns nil if cfg.URL is empty (Engram not configured).
func NewClient(cfg Config) *Client {
	if cfg.URL == "" {
		return nil
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}

	c := &Client{
		baseURL:    cfg.URL,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}

	resp, err := c.httpClient.Get(cfg.URL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		c.available = false
		return c
	}
	resp.Body.Close()
	c.available = true
	return c
}

// Available reports whether Engram is reachable. Safe to call on nil receiver.
func (c *Client) Available() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.available
}

func (c *Client) markUnavailable() {
	c.mu.Lock()
	c.available = false
	c.mu.Unlock()
}

// Save stores an observation in Engram.
// Empty topicKey and sessionID are omitted from the request.
// On failure, marks client unavailable and returns (0, nil).
func (c *Client) Save(title, typ, content, topicKey, scope, project, sessionID string) (int64, error) {
	if !c.Available() {
		return 0, nil
	}

	body := map[string]any{
		"title":   title,
		"type":    typ,
		"content": content,
		"scope":   scope,
		"project": project,
	}
	if topicKey != "" {
		body["topic_key"] = topicKey
	}
	if sessionID != "" {
		body["session_id"] = sessionID
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := c.post("/api/mem/save", body, &result); err != nil {
		c.markUnavailable()
		return 0, nil
	}
	return result.ID, nil
}

// Search queries Engram for relevant memories.
// Empty project is omitted from the request.
// On failure, returns (nil, nil).
func (c *Client) Search(query, project string, limit int) ([]SearchResult, error) {
	if !c.Available() {
		return nil, nil
	}

	body := map[string]any{
		"query": query,
		"limit": limit,
	}
	if project != "" {
		body["project"] = project
	}

	var results []SearchResult
	if err := c.post("/api/mem/search", body, &results); err != nil {
		c.markUnavailable()
		return nil, nil
	}
	return results, nil
}

// SessionStart begins an Engram session. Returns session ID or empty string on failure.
func (c *Client) SessionStart(project string) string {
	if !c.Available() {
		return ""
	}

	var result struct {
		SessionID string `json:"session_id"`
	}
	if err := c.post("/api/session/start", map[string]string{"project": project}, &result); err != nil {
		c.markUnavailable()
		return ""
	}
	return result.SessionID
}

// SessionEnd ends an Engram session. No-op if sessionID is empty. Fails silently.
func (c *Client) SessionEnd(sessionID, summary string) {
	if !c.Available() || sessionID == "" {
		return
	}

	body := map[string]any{"session_id": sessionID}
	if summary != "" {
		body["summary"] = summary
	}
	if err := c.post("/api/session/end", body, nil); err != nil {
		c.markUnavailable()
	}
}

func (c *Client) post(path string, body any, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(c.baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("engram returned status %d", resp.StatusCode)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}
