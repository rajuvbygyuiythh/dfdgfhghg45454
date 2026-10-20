package crushfling

// Guard tests for the "discovery found nothing" case.
//
// The sign-up plan says a broken run has to fail loudly: with no csrf_path and
// no magic_link_path the old code would GET the site root and POST "" - i.e.
// hit the wrong URLs and report a confusing 404 page. These tests pin the
// behaviour we actually want: stop immediately, name the missing piece and
// tell the operator to run --probe, without issuing a single request.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func newGuardRegister(t *testing.T) (*Register, *Session) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	r, err := NewRegister(RegisterConfig{DataDir: t.TempDir()}, func(string, ...any) {})
	if err != nil {
		t.Fatalf("NewRegister: %v", err)
	}
	// Windows cannot delete the log file while the logger still holds it, so
	// close the register before t.TempDir's cleanup runs (cleanups are LIFO).
	t.Cleanup(func() { _ = r.Close() })
	r.endpoints = EndpointConfig{} // exactly what an empty api.json + no discovery yields

	sess, err := NewSession("", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sess.BaseURL = srv.URL
	t.Cleanup(func() {
		if n := hits.Load(); n != 0 {
			t.Errorf("guard made %d request(s); it must fail before touching the network", n)
		}
	})
	return r, sess
}

func TestRequestMagicLinkFailsLoudlyWithoutEndpoint(t *testing.T) {
	r, sess := newGuardRegister(t)

	err := r.requestMagicLink(context.Background(), sess, "x@example.test", &CSRFToken{})
	if err == nil {
		t.Fatal("want an error when no magic-link endpoint is known")
	}
	msg := err.Error()
	for _, want := range []string{"no magic-link endpoint", "--probe"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

func TestFetchCSRFFailsLoudlyWithoutPath(t *testing.T) {
	r, sess := newGuardRegister(t)

	_, err := r.fetchCSRF(context.Background(), sess)
	if err == nil {
		t.Fatal("want an error when csrf_path is empty")
	}
	if !strings.Contains(err.Error(), "csrf_path") {
		t.Errorf("error %q does not name csrf_path", err)
	}
}

// TestProfileRetriesAfterCSRFExpiry reproduces the most common real-world
// failure of a slow run: the mail poll takes minutes, Laravel rotates the
// token, and the profile POST comes back 419. The retry must re-read the
// account page, install the new token and succeed - not fail the account.
func TestProfileRetriesAfterCSRFExpiry(t *testing.T) {
	const fresh = "fresh-token"
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account":
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, `<html><head><meta name="csrf-token" content="`+fresh+`"></head><body>`+
				`<form action="/account" method="POST"><input type="email" name="email"></form></body></html>`)
		case "/api/profile":
			posts.Add(1)
			if r.Header.Get("X-CSRF-TOKEN") != fresh {
				w.WriteHeader(419) // Laravel's "page expired"
				return
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"success":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg, err := NewRegister(RegisterConfig{DataDir: t.TempDir()}, func(string, ...any) {})
	if err != nil {
		t.Fatalf("NewRegister: %v", err)
	}
	t.Cleanup(func() { _ = reg.Close() }) // close the run log before TempDir cleanup
	reg.endpoints = EndpointConfig{AccountPath: "/account", ProfilePaths: []string{"/api/profile"}}

	sess, err := NewSession("", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sess.BaseURL = srv.URL
	sess.SetCSRF("stale-token")

	profile := ProfileData{
		Username: "alice", Age: 24, Gender: GenderWoman, LookingFor: LookingForGuys,
		Country: "GB", Platform: PlatformTelegram, PlatformUsername: "alice", Bio: "hi",
	}
	if err := reg.createProfileAPI(context.Background(), sess, nil, profile, ""); err != nil {
		t.Fatalf("createProfileAPI did not recover from 419: %v", err)
	}
	if n := posts.Load(); n != 2 {
		t.Fatalf("profile endpoint hit %d times, want 2 (one 419 + one retry)", n)
	}
	if got := sess.Headers["X-CSRF-TOKEN"]; got != fresh {
		t.Fatalf("session token = %q, want the refreshed value", got)
	}
}

// MaxRetries is a dashboard setting; it must actually do something, and it
// must never retry a failure that could have created the account already.
func TestRetryableFailurePolicy(t *testing.T) {
	retry := []string{"inbox", "session", "start", "magic_request", "mail_poll", "magic_click"}
	for _, step := range retry {
		if !retryableFailure(&AccountResult{FailedStep: step}) {
			t.Errorf("step %q should be retryable", step)
		}
	}
	noRetry := []string{"verify", "upload", "profile", "pasted_link", "manual_link", ""}
	for _, step := range noRetry {
		if retryableFailure(&AccountResult{FailedStep: step}) {
			t.Errorf("step %q must not be retried", step)
		}
	}
	if retryableFailure(nil) {
		t.Error("nil result must not be retried")
	}
	if retryableFailure(&AccountResult{FailedStep: "inbox", Verified: true}) {
		t.Error("a verified account must never be retried")
	}
}
