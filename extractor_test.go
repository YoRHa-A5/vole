package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestHTMLArticle is a sample blog post HTML that readability should extract.
const TestHTMLArticle = `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Understanding Go Concurrency</title>
	<meta name="description" content="A deep dive into Go's concurrency model with goroutines and channels.">
</head>
<body>
	<nav>
		<a href="/">Home</a>
		<a href="/about">About</a>
	</nav>
	<header>
		<h1>Understanding Go Concurrency</h1>
	</header>
	<article>
		<p>Go provides two powerful concurrency primitives: goroutines and channels.
		Goroutines are lightweight threads managed by the Go runtime, while channels
		provide a way for goroutines to communicate and synchronize.</p>
		<h2>Goroutines</h2>
		<p>A goroutine is a function executing concurrently with other goroutines
		in the same address space. They are cheap — the only allocation is for the
		stack, which starts small and grows as needed.</p>
		<h2>Channels</h2>
		<p>Channels are the pipes that connect concurrent goroutines. You can send
		values into channels from one goroutine and receive those values into
		another goroutine.</p>
		<pre><code>ch := make(chan int)
go func() { ch <- 1 }()
&lt;-ch</code></pre>
		<h2>The Go Way</h2>
		<p>Don't communicate by sharing memory; share memory by communicating.
		This principle, articulated by Rob Pike, captures the essence of Go's
		concurrency model. By using channels to pass references to data between
		goroutines, you ensure that only one goroutine has access to the data at
		any given time.</p>
	</article>
	<aside>Sidebar content that should be excluded</aside>
	<footer>Copyright 2024</footer>
</body>
</html>`

// TestHTMLBoilerplate is a page with mostly boilerplate and little content.
const TestHTMLBoilerplate = `<!DOCTYPE html>
<html>
<head><title>Boilerplate Page</title></head>
<body>
	<nav>Nav item 1</nav>
	<nav>Nav item 2</nav>
	<aside>Sidebar</aside>
	<footer>Footer content</footer>
</body>
</html>`

// TestHTMLPlain is a plain text response.
const TestHTMLPlain = "This is plain text content that should be returned as-is."

func newTestClient() *extractClient {
	return newExtractClient(5*time.Second, 10*time.Second, 10*1024*1024, "test/1.0", 500)
}

func TestExtractHTMLArticle(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(TestHTMLArticle))
	}))
	defer ts.Close()

	client := newTestClient()
	result, err := client.extract(ts.URL)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	ex, ok := result.(*ExtractResult)
	if !ok {
		t.Fatalf("expected *ExtractResult, got %T", result)
	}

	if ex.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", ex.StatusCode)
	}

	if ex.ExtractionMethod != "readability" {
		t.Errorf("expected extraction_method 'readability', got '%s'", ex.ExtractionMethod)
	}

	if ex.Title != "Understanding Go Concurrency" {
		t.Errorf("expected title 'Understanding Go Concurrency', got '%s'", ex.Title)
	}

	if ex.Language != "en" {
		t.Errorf("expected language 'en', got '%s'", ex.Language)
	}

	if ex.ContentType != "text/markdown" {
		t.Errorf("expected content_type 'text/markdown', got '%s'", ex.ContentType)
	}

	if len(strings.TrimSpace(ex.Content)) < 50 {
		t.Errorf("expected substantial content, got %d bytes", len(ex.Content))
	}

	// Content should not contain boilerplate
	if strings.Contains(ex.Content, "Nav item") {
		t.Error("content should not contain nav items (boilerplate)")
	}

	if strings.Contains(ex.Content, "Copyright 2024") {
		t.Error("content should not contain footer")
	}
}

func TestExtractHTMLFallback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(TestHTMLBoilerplate))
	}))
	defer ts.Close()

	client := newTestClient()
	result, err := client.extract(ts.URL)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	ex, ok := result.(*ExtractResult)
	if !ok {
		t.Fatalf("expected *ExtractResult, got %T", result)
	}

	// Should fall back to raw since readability finds nothing meaningful
	if ex.ExtractionMethod != "raw" {
		t.Errorf("expected extraction_method 'raw', got '%s'", ex.ExtractionMethod)
	}
}

func TestExtractEmptyContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><head></head><body></body></html>`))
	}))
	defer ts.Close()

	client := newTestClient()
	result, err := client.extract(ts.URL)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	ex, ok := result.(*ExtractResult)
	if !ok {
		t.Fatalf("expected *ExtractResult, got %T", result)
	}

	if ex.Warning != "content_too_short" {
		t.Errorf("expected warning 'content_too_short', got '%s'", ex.Warning)
	}

	if ex.ExtractionMethod != "raw" {
		t.Errorf("expected extraction_method 'raw', got '%s'", ex.ExtractionMethod)
	}
}

func TestExtractPlainPassThrough(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(TestHTMLPlain))
	}))
	defer ts.Close()

	client := newTestClient()
	result, err := client.extract(ts.URL)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	ex, ok := result.(*ExtractResult)
	if !ok {
		t.Fatalf("expected *ExtractResult, got %T", result)
	}

	if ex.ExtractionMethod != "plain_text" {
		t.Errorf("expected extraction_method 'plain_text', got '%s'", ex.ExtractionMethod)
	}

	if ex.ContentType != "text/plain" {
		t.Errorf("expected content_type 'text/plain', got '%s'", ex.ContentType)
	}

	if ex.Content != TestHTMLPlain {
		t.Errorf("expected plain text pass-through, got '%s'", ex.Content)
	}
}

func TestExtractUnsupportedContentType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake pdf content"))
	}))
	defer ts.Close()

	client := newTestClient()
	result, err := client.extract(ts.URL)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	errResp, ok := result.(*ExtractError)
	if !ok {
		t.Fatalf("expected *ExtractError, got %T", result)
	}

	if errResp.Error != "unsupported_content_type" {
		t.Errorf("expected error 'unsupported_content_type', got '%s'", errResp.Error)
	}
}

func TestExtractUpstreamError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client := newTestClient()
	result, err := client.extract(ts.URL)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	errResp, ok := result.(*ExtractError)
	if !ok {
		t.Fatalf("expected *ExtractError, got %T", result)
	}

	if errResp.Error != "upstream_error" {
		t.Errorf("expected error 'upstream_error', got '%s'", errResp.Error)
	}

	if errResp.StatusCode == nil || *errResp.StatusCode != 404 {
		t.Errorf("expected status_code 404, got %v", errResp.StatusCode)
	}
}

func TestExtractUnreachableURL(t *testing.T) {
	client := newTestClient()
	// Use a URL that won't resolve
	result, err := client.extract("http://localhost.invalid:99999/page")
	if err == nil {
		t.Fatal("expected error for unreachable URL")
	}

	if !strings.Contains(err.Error(), "fetch failed") {
		t.Errorf("expected 'fetch failed' error, got: %v", err)
	}

	if result != nil {
		t.Errorf("expected nil result on error, got %T", result)
	}
}

// Handler tests

func TestHandlerPOST(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(TestHTMLArticle))
	}))
	defer ts.Close()

	client := newTestClient()
	h := newHandler(client)

	body := `{"url": "` + ts.URL + `"}`
	req := httptest.NewRequest(http.MethodPost, "/extract", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
		t.Logf("body: %s", rec.Body.String())
	}

	var resp extractResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ExtractionMethod != "readability" {
		t.Errorf("expected extraction_method 'readability', got '%s'", resp.ExtractionMethod)
	}
}

func TestHandlerGET(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(TestHTMLArticle))
	}))
	defer ts.Close()

	client := newTestClient()
	h := newHandler(client)

	req := httptest.NewRequest(http.MethodGet, "/extract?url="+ts.URL, nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
		t.Logf("body: %s", rec.Body.String())
	}

	var resp extractResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Title != "Understanding Go Concurrency" {
		t.Errorf("expected title, got '%s'", resp.Title)
	}
}

func TestHandlerMissingURL(t *testing.T) {
	client := newTestClient()
	h := newHandler(client)

	req := httptest.NewRequest(http.MethodPost, "/extract", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestHandlerUnsupportedMethod(t *testing.T) {
	client := newTestClient()
	h := newHandler(client)

	req := httptest.NewRequest(http.MethodDelete, "/extract", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestHandlerNotFound(t *testing.T) {
	client := newTestClient()
	h := newHandler(client)

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandlerInvalidURLScheme(t *testing.T) {
	client := newTestClient()
	h := newHandler(client)

	req := httptest.NewRequest(http.MethodPost, "/extract", strings.NewReader(`{"url": "file:///etc/passwd"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestHandlerUnsupportedFormat(t *testing.T) {
	client := newTestClient()
	h := newHandler(client)

	req := httptest.NewRequest(http.MethodPost, "/extract", strings.NewReader(`{"url": "http://example.com", "format": "html"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}
