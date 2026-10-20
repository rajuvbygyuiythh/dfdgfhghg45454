package crushfling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactHidesCredentials(t *testing.T) {
	in := `GET https://crushfling.com/api/auth/callback/email?token=secret123&email=alice%40mail.tm
To: alice@mail.tm
Cookie: next-auth.session-token=abc; csrfToken=zzz
{"csrfToken":"tok","email":"bob@example.com"}
Authorization: Bearer abcdefghij`
	out := Redact(in)

	for _, secret := range []string{"secret123", "next-auth.session-token=abc", `"tok"`, "abcdefghij"} {
		if strings.Contains(out, secret) {
			t.Fatalf("redaction left %q in:\n%s", secret, out)
		}
	}
	if !strings.Contains(out, "a***@mail.tm") {
		t.Fatalf("email local part not masked: %s", out)
	}
	if !strings.Contains(out, "b***@example.com") {
		t.Fatalf("JSON email not masked: %s", out)
	}
}

func TestRunLoggerLevelsAndFile(t *testing.T) {
	dir := t.TempDir()
	var console bytes.Buffer
	logger, err := NewRunLogger(LogOptions{
		DataDir:  dir,
		ToFile:   true,
		MinLevel: LevelInfo,
		Console:  &console,
	})
	if err != nil {
		t.Fatalf("NewRunLogger: %v", err)
	}
	defer logger.Close()

	logger.Debugf("hidden debug line")
	logger.Infof("hello %s", "world")
	logger.Warnf("link token=secret123")

	if strings.Contains(console.String(), "hidden debug line") {
		t.Fatalf("debug line leaked at info level: %s", console.String())
	}
	if !strings.Contains(console.String(), "hello world") {
		t.Fatalf("console missing info line: %s", console.String())
	}
	if strings.Contains(console.String(), "secret123") {
		t.Fatalf("console leaked a token: %s", console.String())
	}

	path := logger.Path()
	if path == "" {
		t.Fatal("log file path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "WARN") || !strings.Contains(text, "hello world") {
		t.Fatalf("log file misses lines:\n%s", text)
	}
	if strings.Contains(text, "secret123") {
		t.Fatalf("log file leaked a token:\n%s", text)
	}
	if got := logger.Counts()["WARN"]; got != 1 {
		t.Fatalf("WARN count = %d, want 1", got)
	}
	if tail := logger.Tail(2); len(tail) != 2 || !strings.Contains(tail[len(tail)-1], "warn:") {
		t.Fatalf("tail = %#v", tail)
	}
	if !logger.Enabled(LevelWarn) || logger.Enabled(LevelDebug) {
		t.Fatal("Enabled() disagrees with MinLevel")
	}
}

func TestTracerRecordsStatusAndRedactsURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "next-auth.session-token", Value: "sess", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tracer, err := NewTracer(TraceOptions{Bodies: true, BodyLimit: 4096})
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}
	defer tracer.Close()

	sess, err := NewSession("test-agent", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sess.BaseURL = srv.URL
	sess.SetTracer(tracer)

	status, body, _, err := sess.Get(context.Background(), "/api/auth/csrf?token=super-secret")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if status != 200 || !strings.Contains(body, "ok") {
		t.Fatalf("status=%d body=%q", status, body)
	}

	records := tracer.Records()
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	rec := records[0]
	if rec.Status != 200 {
		t.Fatalf("record status = %d", rec.Status)
	}
	if strings.Contains(rec.URL, "super-secret") {
		t.Fatalf("tracer leaked the token: %s", rec.URL)
	}
	if len(rec.SetCookies) != 1 || rec.SetCookies[0] != "next-auth.session-token" {
		t.Fatalf("set-cookie names = %#v", rec.SetCookies)
	}
	if !strings.Contains(rec.ResponseBody, "ok") {
		t.Fatalf("body not captured: %q", rec.ResponseBody)
	}
	if tracker := tracer.Summary(); !strings.Contains(tracker, "1 requests") {
		t.Fatalf("summary = %q", tracker)
	}
}

func TestFailureDumpIsRedactedAndValidJSON(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewRunLogger(LogOptions{DataDir: dir, MinLevel: LevelDebug})
	if err != nil {
		t.Fatalf("NewRunLogger: %v", err)
	}
	defer logger.Close()

	reg := &Register{
		cfg:       RegisterConfig{DataDir: dir, Debug: true},
		endpoints: DefaultEndpointConfig(),
		logger:    func(string, ...any) {},
		runLog:    logger,
		runID:     "test-run",
		debugDir:  filepath.Join(dir, "debug"),
	}
	res := failResult(
		ProfileData{Email: "alice@mail.tm", Bio: "hello"},
		"verify", CodeVerify, fmt.Errorf("boom"),
		"http://user:pass@127.0.0.1:8080", map[string]int64{"total": 12},
	)
	res.MagicLink = "https://crushfling.com/api/auth/callback/email?token=secret-token&email=alice%40mail.tm"

	path, err := reg.WriteFailureDump(1, res)
	if err != nil {
		t.Fatalf("WriteFailureDump: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	text := string(raw)
	for _, secret := range []string{"secret-token", "alice@mail.tm", "user:pass"} {
		if strings.Contains(text, secret) {
			t.Fatalf("dump leaked %q:\n%s", secret, text)
		}
	}
	var dump FailureDump
	if err := json.Unmarshal(raw, &dump); err != nil {
		t.Fatalf("dump is not valid JSON: %v", err)
	}
	if dump.Code != CodeVerify || dump.Step != "verify" {
		t.Fatalf("dump classification = %q/%q", dump.Step, dump.Code)
	}
	if dump.Timings["total"] != 12 {
		t.Fatalf("timings lost: %#v", dump.Timings)
	}
	if dump.ProxyUsed == "" || strings.Contains(dump.ProxyUsed, "pass") {
		t.Fatalf("proxy not redacted: %q", dump.ProxyUsed)
	}
}
