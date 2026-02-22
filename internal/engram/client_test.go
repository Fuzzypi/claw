package engram

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func fakeEngramServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/mem/save", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 42, "created": true})
	})
	mux.HandleFunc("POST /api/mem/search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]SearchResult{
			{ID: 1, Title: "Prior Run", Type: "discovery", Snippet: "found something", Rank: -1.5},
		})
	})
	mux.HandleFunc("POST /api/session/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-test-123"})
	})
	mux.HandleFunc("POST /api/session/end", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"ended": true})
	})
	return httptest.NewServer(mux)
}

func TestNewClient_HealthSuccess(t *testing.T) {
	ts := fakeEngramServer(t)
	defer ts.Close()

	c := NewClient(Config{URL: ts.URL})
	if c == nil {
		t.Fatal("client is nil")
	}
	if !c.Available() {
		t.Error("client should be available after successful health check")
	}
}

func TestNewClient_HealthFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(Config{URL: ts.URL})
	if c == nil {
		t.Fatal("client should not be nil (just unavailable)")
	}
	if c.Available() {
		t.Error("client should not be available after failed health check")
	}
}

func TestNewClient_EmptyURL(t *testing.T) {
	c := NewClient(Config{URL: ""})
	if c != nil {
		t.Error("expected nil client for empty URL")
	}
	// Nil receiver safety
	if c.Available() {
		t.Error("nil client should not be available")
	}
}

func TestClient_Save(t *testing.T) {
	ts := fakeEngramServer(t)
	defer ts.Close()

	c := NewClient(Config{URL: ts.URL})
	id, err := c.Save("Test Title", "discovery", "content", "topic-1", "project", "my-proj", "sess-1")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if id != 42 {
		t.Errorf("id = %d, want 42", id)
	}
}

func TestClient_Save_Unavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(Config{URL: ts.URL})
	// Client is unavailable after health check failure
	id, err := c.Save("Title", "discovery", "content", "", "project", "proj", "")
	if err != nil {
		t.Fatalf("Save should return nil error when unavailable: %v", err)
	}
	if id != 0 {
		t.Errorf("id = %d, want 0 when unavailable", id)
	}
}

func TestClient_Search(t *testing.T) {
	ts := fakeEngramServer(t)
	defer ts.Close()

	c := NewClient(Config{URL: ts.URL})
	results, err := c.Search("query", "my-proj", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].Title != "Prior Run" {
		t.Errorf("title = %q, want 'Prior Run'", results[0].Title)
	}
}

func TestClient_Search_WithProject(t *testing.T) {
	var gotProject string
	ts := httptest.NewServer(http.NewServeMux())
	ts.Close() // close immediately, we'll make a new one

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/mem/search", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if p, ok := body["project"].(string); ok {
			gotProject = p
		}
		json.NewEncoder(w).Encode([]SearchResult{})
	})
	ts = httptest.NewServer(mux)
	defer ts.Close()

	c := NewClient(Config{URL: ts.URL})
	c.Search("query", "filtered-proj", 5)

	if gotProject != "filtered-proj" {
		t.Errorf("project sent = %q, want 'filtered-proj'", gotProject)
	}
}

func TestClient_SessionStartEnd(t *testing.T) {
	ts := fakeEngramServer(t)
	defer ts.Close()

	c := NewClient(Config{URL: ts.URL})

	sid := c.SessionStart("my-proj")
	if sid != "sess-test-123" {
		t.Errorf("session_id = %q, want 'sess-test-123'", sid)
	}

	// SessionEnd should not panic or error
	c.SessionEnd(sid, "all done")
}

func TestClient_SessionStart_Unavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(Config{URL: ts.URL})
	sid := c.SessionStart("proj")
	if sid != "" {
		t.Errorf("session_id = %q, want empty when unavailable", sid)
	}
}

func TestClient_GracefulDegradation(t *testing.T) {
	var callCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/mem/save", func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n >= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 1, "created": true})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := NewClient(Config{URL: ts.URL})
	if !c.Available() {
		t.Fatal("should be available initially")
	}

	// First call succeeds
	id, err := c.Save("First", "discovery", "ok", "", "project", "proj", "")
	if err != nil || id != 1 {
		t.Fatalf("first call: id=%d err=%v", id, err)
	}

	// Second call fails — server returns 500, client marks unavailable
	id, err = c.Save("Second", "discovery", "fail", "", "project", "proj", "")
	if err != nil {
		t.Fatalf("second call should return nil error: %v", err)
	}
	if id != 0 {
		t.Errorf("second call id = %d, want 0", id)
	}
	if c.Available() {
		t.Error("client should be unavailable after failure")
	}

	// Third call is a no-op (no HTTP call made)
	id, err = c.Save("Third", "discovery", "noop", "", "project", "proj", "")
	if err != nil || id != 0 {
		t.Errorf("third call: id=%d err=%v, want 0/nil", id, err)
	}

	// Only 2 HTTP calls should have been made (not 3)
	if got := callCount.Load(); got != 2 {
		t.Errorf("HTTP call count = %d, want 2", got)
	}
}
