package crushfling

import (
	"strings"
	"testing"
)

func TestExtractNextURL(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "meta refresh",
			body: `<meta http-equiv="refresh" content="0; url=/onboarding">`,
			want: BaseURL + "/onboarding",
		},
		{
			name: "js location",
			body: `<script>window.location.href = "\/onboarding\/step-2";</script>`,
			want: BaseURL + "/onboarding/step-2",
		},
		{
			name: "absolute profile link",
			body: `<a href="https://crushfling.com/welcome">go</a>`,
			want: BaseURL + "/welcome",
		},
		{
			name: "nothing useful",
			body: `<html><body>Welcome</body></html>`,
			want: "",
		},
		{
			name: "javascript pseudo url ignored",
			body: `<a href="javascript:void(0)">x</a>`,
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractNextURL(tc.body); got != tc.want {
				t.Fatalf("extractNextURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHasSessionCookieIgnoresCSRFAndCloudflare(t *testing.T) {
	if hasSessionCookie(map[string]string{"cf_clearance": "x", "__Host-next-auth.csrf-token": "y"}) {
		t.Fatal("csrf/cloudflare cookies must not count as a session")
	}
	if !hasSessionCookie(map[string]string{"__Secure-next-auth.session-token": "jwt"}) {
		t.Fatal("next-auth session cookie should count as a session")
	}
	if !hasSessionCookie(map[string]string{"authjs.session-token": "jwt"}) {
		t.Fatal("authjs session cookie should count as a session")
	}
}

func TestExtractSessionFromBody(t *testing.T) {
	if got := extractSessionFromBody(`{"sessionToken":"abc123"}`); got != "abc123" {
		t.Fatalf("sessionToken = %q", got)
	}
	if got := extractSessionFromBody(`{"token":"zzz"}`); got != "zzz" {
		t.Fatalf("token = %q", got)
	}
	if got := extractSessionFromBody(`<html>no token</html>`); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFindErrorMessage(t *testing.T) {
	body := "Continue to crushfling\nToo many attempts. Try again later.\n"
	if got := findErrorMessage(body); !strings.Contains(got, "Too many attempts") {
		t.Fatalf("findErrorMessage() = %q", got)
	}
	if got := findErrorMessage("Email sent! Check your inbox."); got != "" {
		t.Fatalf("successful page should not report an error, got %q", got)
	}
}

func TestValidateMagicLink(t *testing.T) {
	good := "https://crushfling.com/api/auth/callback/email?token=abc&email=x%40mail.tm"
	if err := validateMagicLink(good); err != nil {
		t.Fatalf("good link rejected: %v", err)
	}
	if err := validateMagicLink("http://crushfling.com/verify?token=1"); err != nil {
		t.Fatalf("http link rejected: %v", err)
	}
	if err := validateMagicLink("crushfling.com/verify?token=1"); err == nil {
		t.Fatal("scheme-less link must be rejected")
	}
	if err := validateMagicLink("https://evil.example/crushfling.com"); err == nil {
		t.Fatal("foreign host must be rejected")
	}
}
