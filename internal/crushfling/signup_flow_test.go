package crushfling

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLaravelFormSignUpFlow runs the whole sign-up path against a fake Laravel
// site: discovery → form post with _token → magic-link click → session
// confirmation → avatar upload → profile → public profile check. It is the
// offline proof that the new flow is wired end to end.
func TestLaravelFormSignUpFlow(t *testing.T) {
	var gotToken, gotEmail, gotUploadField, gotProfilePath string
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/account", func(w http.ResponseWriter, r *http.Request) {
		signedIn := false
		if c, err := r.Cookie("laravel_session"); err == nil && c.Value != "" {
			signedIn = true
		}
		w.Header().Set("Content-Type", "text/html")
		if signedIn {
			fmt.Fprint(w, `<html><head><meta name="csrf-token" content="tok-2"></head><body>
				<a href="/account/logout">Sign out</a> <a href="/p/alice/">My profile</a></body></html>`)
			return
		}
		fmt.Fprint(w, `<html><head><meta name="csrf-token" content="tok-1"></head><body>
			<form method="post" action="/send">
			  <input type="hidden" name="_token" value="tok-1">
			  <input type="email" name="email">
			  <button type="submit">Continue with Email</button>
			</form>
			<script>fetch('/api/upload');</script></body></html>`)
	})
	mux.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotToken = r.FormValue("_token")
		gotEmail = r.FormValue("email")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>Email sent! We sent a sign-in link to you.</body></html>`)
	})
	mux.HandleFunc("/magic", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "laravel_session", Value: "sess-9", Path: "/"})
		http.Redirect(w, r, "/account", http.StatusFound)
	})
	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			http.Error(w, "bad multipart", 400)
			return
		}
		for name := range r.MultipartForm.File {
			gotUploadField = name
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"url":"https://cdn.example/a.webp"}`)
	})
	mux.HandleFunc("/api/profile", func(w http.ResponseWriter, r *http.Request) {
		gotProfilePath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true}`)
	})
	mux.HandleFunc("/p/alice/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><h1>alice</h1><p>telegram</p></body></html>`)
	})

	ctx := context.Background()
	sess, err := NewSession("", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sess.BaseURL = srv.URL

	reg := &Register{
		cfg:       RegisterConfig{DataDir: t.TempDir()},
		endpoints: EndpointConfig{AccountPath: "/account", UploadPath: "/api/upload", ProfilePaths: []string{"/api/profile"}, PublicProfilePath: "/p/{username}/"},
		logger:    func(string, ...any) {},
	}

	site, err := DiscoverSite(ctx, sess, reg.endpoints)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if site.Style != AuthStyleForm || site.FormAction != "/send" || site.CSRFToken != "tok-1" {
		t.Fatalf("unexpected profile: %+v", site)
	}
	sess.SetCSRF(site.CSRFToken)

	if err := reg.requestMagicLinkWithProfile(ctx, sess, "alice@mail.tm", &CSRFToken{Token: site.CSRFToken}, site); err != nil {
		t.Fatalf("requestMagicLink: %v", err)
	}
	if gotToken != "tok-1" || gotEmail != "alice@mail.tm" {
		t.Fatalf("form fields: token=%q email=%q", gotToken, gotEmail)
	}

	cookies, _, err := sess.ClickMagicLink(ctx, srv.URL+"/magic?token=abc")
	if err != nil {
		t.Fatalf("click: %v", err)
	}
	if !hasSessionCookie(cookies) {
		t.Fatalf("no session cookie: %v", cookies)
	}
	if ok, why := reg.sessionIsAuthenticated(ctx, sess); !ok {
		t.Fatalf("session not confirmed: %s", why)
	}

	pic := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(pic, []byte("\x89PNG\r\n\x1a\nfake"), 0o644); err != nil {
		t.Fatalf("write pic: %v", err)
	}
	uploaded, err := reg.uploadAvatar(ctx, sess, site, pic)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if uploaded != "https://cdn.example/a.webp" || gotUploadField != "file" {
		t.Fatalf("upload = %q via %q", uploaded, gotUploadField)
	}

	prof := ProfileData{Username: "alice", Age: 22, Gender: GenderWoman, LookingFor: LookingForGuys, Country: "US", Platform: PlatformTelegram}
	if err := reg.createProfileAPI(ctx, sess, site, prof, uploaded); err != nil {
		t.Fatalf("profile: %v", err)
	}
	if gotProfilePath != "/api/profile" {
		t.Fatalf("profile path = %q", gotProfilePath)
	}
	if err := reg.verifyProfilePublic(ctx, sess, "alice"); err != nil {
		t.Fatalf("public profile: %v", err)
	}
	// A missing profile must fail, not pass.
	if err := reg.verifyProfilePublic(ctx, sess, "nobody"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 for unknown user, got %v", err)
	}
}
