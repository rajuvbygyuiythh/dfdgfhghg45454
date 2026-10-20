package crushfling

// HTTP tracing: every request that goes through a Session can be recorded with
// method, URL (token-redacted), status, duration, redirect target and the names
// of the cookies the server set. Bodies are only captured when the operator
// enables --trace-http, and even then they stay local, truncated and redacted.
//
// This is the main debugging tool for the parts of crushfling.com that cannot
// be verified from this sandbox: once the Go worker runs on Windows, the trace
// shows exactly which endpoint answered, with what status and what error body.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// HTTPRecord is one request/response pair (one redirect hop per record).
type HTTPRecord struct {
	Time          string   `json:"time"`
	Method        string   `json:"method"`
	URL           string   `json:"url"`
	Status        int      `json:"status"`
	DurationMS    int64    `json:"duration_ms"`
	RequestBytes  int64    `json:"request_bytes,omitempty"`
	ResponseBytes int64    `json:"response_bytes,omitempty"`
	Location      string   `json:"location,omitempty"`
	SetCookies    []string `json:"set_cookies,omitempty"`
	Error         string   `json:"error,omitempty"`
	RequestBody   string   `json:"request_body,omitempty"`
	ResponseBody  string   `json:"response_body,omitempty"`
	Truncated     bool     `json:"truncated,omitempty"`
}

// TraceOptions configures a Tracer.
type TraceOptions struct {
	Path       string // JSONL file; empty = memory only
	Bodies     bool   // capture request/response bodies (--trace-http)
	BodyLimit  int    // bytes kept per body, default 16 KiB
	MaxRecords int    // in-memory records kept for dumps, default 300
	OnRecord   func(HTTPRecord)
}

// Tracer is a RoundTripper wrapper plus an in-memory record ring.
type Tracer struct {
	mu        sync.Mutex
	file      *os.File
	path      string
	bodies    bool
	bodyLimit int
	max       int
	records   []HTTPRecord
	onRecord  func(HTTPRecord)
	closed    bool
}

// NewTracer creates a tracer. A path is optional; without it records stay in
// memory so failure dumps can still embed them.
func NewTracer(opts TraceOptions) (*Tracer, error) {
	t := &Tracer{
		bodies:    opts.Bodies,
		bodyLimit: opts.BodyLimit,
		max:       opts.MaxRecords,
		onRecord:  opts.OnRecord,
	}
	if t.bodyLimit <= 0 {
		t.bodyLimit = 16 << 10
	}
	if t.max <= 0 {
		t.max = 300
	}
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		return t, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	t.file = f
	t.path = path
	return t, nil
}

// Wrap returns a RoundTripper that records through this tracer.
func (t *Tracer) Wrap(base http.RoundTripper) http.RoundTripper {
	if t == nil {
		return base
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return &tracedTransport{tracer: t, base: base}
}

type tracedTransport struct {
	tracer *Tracer
	base   http.RoundTripper
}

func (tr *tracedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return tr.tracer.roundTrip(tr.base, req)
}

func (t *Tracer) captureLimit() int {
	hard := t.bodyLimit * 8
	if hard < 64<<10 {
		hard = 64 << 10
	}
	return hard
}

func (t *Tracer) roundTrip(base http.RoundTripper, req *http.Request) (*http.Response, error) {
	start := time.Now()
	rec := HTTPRecord{
		Time:   start.UTC().Format(time.RFC3339Nano),
		Method: req.Method,
		URL:    redactURL(req.URL.String()),
	}

	if t.bodies && req.Body != nil && req.Body != http.NoBody && textualContentType(req.Header.Get("Content-Type")) {
		orig := req.Body
		body, _ := readCapped(orig, t.captureLimit())
		req.Body = &multiReadCloser{Reader: io.MultiReader(bytes.NewReader(body), orig), Closer: orig}
		rec.RequestBytes = int64(len(body))
		rec.RequestBody, rec.Truncated = previewBody(body, t.bodyLimit)
	}

	resp, err := base.RoundTrip(req)
	rec.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		rec.Error = Redact(err.Error())
		t.record(rec)
		return nil, err
	}

	rec.Status = resp.StatusCode
	if loc := resp.Header.Get("Location"); loc != "" {
		rec.Location = redactURL(loc)
	}
	for _, raw := range resp.Header.Values("Set-Cookie") {
		if name := cookieName(raw); name != "" {
			rec.SetCookies = append(rec.SetCookies, name)
		}
	}

	if t.bodies && resp.Body != nil && resp.Body != http.NoBody {
		orig := resp.Body
		body, rerr := readCapped(orig, t.captureLimit())
		resp.Body = &multiReadCloser{Reader: io.MultiReader(bytes.NewReader(body), orig), Closer: orig}
		rec.ResponseBytes = int64(len(body))
		if textualContentType(resp.Header.Get("Content-Type")) {
			rec.ResponseBody, rec.Truncated = previewBody(body, t.bodyLimit)
		} else {
			rec.ResponseBody = fmt.Sprintf("[binary response, %d bytes captured]", len(body))
			rec.Truncated = true
		}
		if rerr != nil {
			rec.Error = Redact(rerr.Error())
		}
	}

	t.record(rec)
	return resp, nil
}

func (t *Tracer) record(rec HTTPRecord) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.records = append(t.records, rec)
	if len(t.records) > t.max {
		t.records = t.records[len(t.records)-t.max:]
	}
	if t.file != nil {
		if line, err := json.Marshal(rec); err == nil {
			_, _ = t.file.Write(append(line, '\n'))
		}
	}
	cb := t.onRecord
	t.mu.Unlock()
	if cb != nil {
		cb(rec)
	}
}

// Records returns a copy of the in-memory trace, oldest first.
func (t *Tracer) Records() []HTTPRecord {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]HTTPRecord, len(t.records))
	copy(out, t.records)
	return out
}

// Count returns how many requests were recorded.
func (t *Tracer) Count() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.records)
}

// Summary is a one-line health summary for the run log.
func (t *Tracer) Summary() string {
	records := t.Records()
	failed := 0
	for _, rec := range records {
		if rec.Status == 0 || rec.Status >= 400 {
			failed++
		}
	}
	return fmt.Sprintf("%d requests, %d failed", len(records), failed)
}

// Path returns the JSONL file path, or "" for a memory-only tracer.
func (t *Tracer) Path() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.path
}

// Close closes the JSONL file. Safe to call twice.
func (t *Tracer) Close() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	if t.file != nil {
		err := t.file.Close()
		t.file = nil
		return err
	}
	return nil
}

type multiReadCloser struct {
	io.Reader
	io.Closer
}

func readCapped(r io.Reader, limit int) ([]byte, error) {
	if limit <= 0 {
		limit = 64 << 10
	}
	return io.ReadAll(io.LimitReader(r, int64(limit)+1))
}

func previewBody(b []byte, limit int) (string, bool) {
	truncated := false
	if len(b) > limit {
		b = b[:limit]
		truncated = true
	}
	return Redact(string(b)), truncated
}

func textualContentType(ct string) bool {
	ct = strings.ToLower(ct)
	if ct == "" {
		return true
	}
	for _, marker := range []string{"json", "text/", "xml", "javascript", "x-www-form-urlencoded", "graphql"} {
		if strings.Contains(ct, marker) {
			return true
		}
	}
	return false
}

func cookieName(setCookie string) string {
	part := setCookie
	if i := strings.Index(part, ";"); i >= 0 {
		part = part[:i]
	}
	if i := strings.Index(part, "="); i > 0 {
		return strings.TrimSpace(part[:i])
	}
	return ""
}
