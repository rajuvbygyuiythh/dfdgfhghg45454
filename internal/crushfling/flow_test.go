package crushfling

// Offline end-to-end test of the magic-link flow against a fake NextAuth
// server. This is the only place where the whole request/verify sequence can
// be exercised without touching crushfling.com, and it is what keeps the
// inferred endpoints honest: if the code stops sending the CSRF form, stops
// following the callback or stops storing the session cookie, this fails.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// nextAuthEndpoints keeps the legacy JSON flow testable: the production
// defaults now discover these paths from the page instead of assuming them.
func nextAuthEndpoints() EndpointConfig {
	return EndpointConfig{
		AccountPath:       "/account",
		CSRFPath:          "/api/auth/csrf",
		MagicLinkPath:     "/api/auth/signin/email",
		MagicLinkAltPaths: []string{"/api/auth/magic-link"},
		LivewirePath:      "/livewire/update",
		UploadPath:        "/api/upload",
		ProfilePaths:      []string{"/api/profile"},
		PublicProfilePath: "/p/{username}/",
	}
}

func newFakeNextAuth(t *testing.T) (*httptest.Server, *sync.Map) {
	t.Helper()
	seen := &sync.Map{}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/csrf":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"csrfToken": "csrf-abc"})
		case "/api/auth/signin/email":
			_ = r.ParseForm()
			seen.Store("email", r.FormValue("email"))
			seen.Store("csrf", r.FormValue("csrfToken"))
			seen.Store("content_type", r.Header.Get("Content-Type"))
			seen.Store("callback", r.FormValue("callbackUrl"))
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"url":%q}`, srv.URL+"/api/auth/verifyRequest")
		case "/api/auth/callback/email":
			http.SetCookie(w, &http.Cookie{Name: "next-auth.session-token", Value: "sess-1", Path: "/", HttpOnly: true})
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body>Welcome <a href="/onboarding">Continue</a></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func TestMagicLinkFlowWithFakeServer(t *testing.T) {
	srv, seen := newFakeNextAuth(t)
	ctx := context.Background()

	reg := &Register{
		cfg:       RegisterConfig{DataDir: t.TempDir(), MailPollSec: 1, MailTimeoutSec: 2},
		endpoints: nextAuthEndpoints(),
		logger:    func(string, ...any) {},
	}

	sess, err := NewSession("", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sess.BaseURL = srv.URL

	csrf, err := reg.fetchCSRF(ctx, sess)
	if err != nil {
		t.Fatalf("fetchCSRF: %v", err)
	}
	if csrf.Token != "csrf-abc" {
		t.Fatalf("csrf token = %q", csrf.Token)
	}

	if err := reg.requestMagicLink(ctx, sess, "alice@mail.tm", csrf); err != nil {
		t.Fatalf("requestMagicLink: %v", err)
	}
	if got, _ := seen.Load("email"); got != "alice@mail.tm" {
		t.Fatalf("posted email = %v", got)
	}
	if got, _ := seen.Load("csrf"); got != "csrf-abc" {
		t.Fatalf("posted csrf = %v", got)
	}
	contentType, _ := seen.Load("content_type")
	if ct, ok := contentType.(string); !ok || !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		t.Fatalf("content type = %v", contentType)
	}
	if got, _ := seen.Load("callback"); got != srv.URL+"/" {
		t.Fatalf("callbackUrl = %v", got)
	}

	cookies, body, err := sess.ClickMagicLink(ctx, srv.URL+"/api/auth/callback/email?token=abc&email=alice%40mail.tm")
	if err != nil {
		t.Fatalf("ClickMagicLink: %v", err)
	}
	if !hasSessionCookie(cookies) {
		t.Fatalf("no session cookie after the callback: %s", cookieNames(cookies))
	}
	if cookies["next-auth.session-token"] != "sess-1" {
		t.Fatalf("session cookie value = %q", cookies["next-auth.session-token"])
	}
	if body == "" {
		t.Fatal("callback body was not read")
	}
}

func TestMagicLinkFallsBackToAltPath(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/api/auth/magic-link" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"success":true}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	reg := &Register{
		cfg:       RegisterConfig{DataDir: t.TempDir()},
		endpoints: nextAuthEndpoints(),
		logger:    func(string, ...any) {},
	}
	sess, err := NewSession("", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sess.BaseURL = srv.URL

	if err := reg.requestMagicLink(context.Background(), sess, "a@b.example", nil); err != nil {
		t.Fatalf("requestMagicLink: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, path := range hits {
		if path == "/api/auth/magic-link" {
			found = true
		}
	}
	if !found {
		t.Fatalf("alt endpoint was not tried, hits = %v", hits)
	}
}

func TestServiceDebugReportRedacts(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.appendLog("info: sign-in link token=secret123 for alice@mail.tm")

	path, err := svc.CreateDebugReport()
	if err != nil {
		t.Fatalf("CreateDebugReport: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, "secret123") || strings.Contains(text, "alice@mail.tm") {
		t.Fatalf("report leaked credentials:\n%s", text)
	}
	if !strings.Contains(text, "CrushFling debug report") || !strings.Contains(text, "error codes") {
		t.Fatalf("report looks incomplete:\n%s", text)
	}
	if info := svc.GetDebugInfo(); info.DataDir == "" {
		t.Fatal("GetDebugInfo returned an empty data dir")
	}
}
