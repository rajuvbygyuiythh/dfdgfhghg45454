package crushfling

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const laravelSignInPage = `<!DOCTYPE html>
<html><head>
<meta name="csrf-token" content="tok-abc123">
<title>Account</title>
</head><body>
<form method="post" action="/signin">
  <input type="hidden" name="_token" value="tok-abc123">
  <input type="email" name="email" placeholder="Email" required>
  <button type="submit">Continue with Email</button>
</form>
<script>fetch('/api/auth/email', {method: 'POST'});</script>
</body></html>`

const livewireSignInPage = `<!DOCTYPE html>
<html><head><meta name="csrf-token" content="tok-lw"></head><body>
<div wire:snapshot="{&quot;data&quot;:{&quot;email&quot;:null},&quot;memo&quot;:{&quot;name&quot;:&quot;auth.login&quot;,&quot;id&quot;:&quot;cmp-1&quot;}}">
  <input type="email" name="email" wire:model="email">
  <button type="button" wire:click="sendLink">Continue with Email</button>
</div>
</body></html>`

func newPageServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/account":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func discoverFrom(t *testing.T, body string) (*SiteProfile, *Session) {
	t.Helper()
	srv := newPageServer(t, body)
	sess, err := NewSession("", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sess.BaseURL = srv.URL
	profile, err := DiscoverSite(context.Background(), sess, DefaultEndpointConfig())
	if err != nil {
		t.Fatalf("DiscoverSite: %v", err)
	}
	return profile, sess
}

func TestDiscoverLaravelForm(t *testing.T) {
	profile, _ := discoverFrom(t, laravelSignInPage)

	if profile.Style != AuthStyleForm {
		t.Fatalf("style = %q, want %q", profile.Style, AuthStyleForm)
	}
	if profile.CSRFToken != "tok-abc123" {
		t.Fatalf("csrf token = %q", profile.CSRFToken)
	}
	if profile.FormAction != "/signin" {
		t.Fatalf("form action = %q", profile.FormAction)
	}
	if profile.FormMethod != "POST" {
		t.Fatalf("form method = %q", profile.FormMethod)
	}
	if profile.EmailField != "email" {
		t.Fatalf("email field = %q", profile.EmailField)
	}
	if profile.ExtraFields["_token"] != "tok-abc123" {
		t.Fatalf("hidden fields = %#v", profile.ExtraFields)
	}
	// The page references a JSON endpoint; it must be visible to the probe.
	if !strings.Contains(strings.Join(profile.Endpoints(), ","), "/api/auth/email") {
		t.Fatalf("endpoints = %#v", profile.Endpoints())
	}
}

func TestDiscoverLivewire(t *testing.T) {
	profile, _ := discoverFrom(t, livewireSignInPage)

	if profile.Style != AuthStyleLivewire {
		t.Fatalf("style = %q, want %q", profile.Style, AuthStyleLivewire)
	}
	if profile.LivewireMethod != "sendLink" {
		t.Fatalf("livewire method = %q", profile.LivewireMethod)
	}
	if profile.LivewirePath != "/livewire/update" {
		t.Fatalf("livewire path = %q", profile.LivewirePath)
	}
	// The snapshot attribute is HTML-escaped JSON and must be unescaped before
	// it can be posted back to Livewire.
	if !strings.Contains(profile.LivewireSnapshot, `"name":"auth.login"`) {
		t.Fatalf("snapshot not unescaped: %q", profile.LivewireSnapshot)
	}
	if strings.Contains(profile.LivewireSnapshot, "&quot;") {
		t.Fatal("snapshot still contains HTML entities")
	}
}

func TestDiscoverRejects404Page(t *testing.T) {
	srv := newPageServer(t, `<html><body><h1>404</h1><h2>Page Not Found</h2></body></html>`)
	sess, err := NewSession("", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sess.BaseURL = srv.URL
	if _, err := DiscoverSite(context.Background(), sess, DefaultEndpointConfig()); err == nil {
		t.Fatal("expected an error for the site's 404 page")
	}
}

func TestCollectPathsKeepsSameSitePathsOnly(t *testing.T) {
	body := `
		fetch("/api/upload", {method: "POST"});
		axios.post('/api/profile', {});
		$.post("https://crushfling.com/api/onboarding");
		fetch("https://evil.example/api/steal");
		fetch("/api/${dynamic}/x");
		window.location = "/livewire/update";`
	paths := collectPaths(body)
	joined := strings.Join(paths, ",")
	for _, want := range []string{"/api/upload", "/api/profile", "/api/onboarding", "/livewire/update"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %v", want, paths)
		}
	}
	if strings.Contains(joined, "evil.example") || strings.Contains(joined, "dynamic") {
		t.Fatalf("unsafe path kept: %v", paths)
	}
}

func TestSuccessAndRejectionCopy(t *testing.T) {
	if !hasSuccessCopy(`<p>Email sent! We sent a sign-in link to you.</p>`) {
		t.Fatal("confirmation copy not detected")
	}
	if hasSuccessCopy(`<p>Enter your email to sign in</p>`) {
		t.Fatal("sign-in form must not look like a confirmation")
	}
	if !hasRejectionCopy(`{"message":"Too Many Requests"}`) {
		t.Fatal("rate-limit refusal not detected")
	}
}

func TestSessionCSRFHeaders(t *testing.T) {
	sess, err := NewSession("", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sess.SetCSRF("tok-1")
	if got := sess.Headers["X-CSRF-TOKEN"]; got != "tok-1" {
		t.Fatalf("X-CSRF-TOKEN = %q", got)
	}
	// Laravel URL-encodes the XSRF-TOKEN cookie; the header must be decoded.
	sess.SetCookies(map[string]string{"XSRF-TOKEN": "abc%3Ddef"})
	sess.SetCSRF("tok-2")
	if got := sess.Headers["X-XSRF-TOKEN"]; got != "abc=def" {
		t.Fatalf("X-XSRF-TOKEN = %q, want the decoded cookie", got)
	}
}

func TestHasSessionCookieRejectsCSRFCookies(t *testing.T) {
	if hasSessionCookie(map[string]string{"XSRF-TOKEN": "x", "laravel_csrf": "y"}) {
		t.Fatal("csrf cookies must not count as a session")
	}
	if !hasSessionCookie(map[string]string{"laravel_session": "abc"}) {
		t.Fatal("laravel_session should count")
	}
	if !hasSessionCookie(map[string]string{"remember_web_59ba36addc2b2f9401580f014c7f58ea4e30989d": "abc"}) {
		t.Fatal("Laravel remember cookie should count")
	}
}
