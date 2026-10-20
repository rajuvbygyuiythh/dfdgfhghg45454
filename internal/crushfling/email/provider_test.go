package email

import (
	"strings"
	"testing"
)

func TestExtractMagicLinkPrefersTokenURLAndUnescapesHTML(t *testing.T) {
	body := `<p>Tap to sign in</p>
<a href="https://crushfling.com/api/auth/callback/email?callbackUrl=%2F&token=abc123&amp;email=x%40mail.tm">Sign in</a>
<a href="https://crushfling.com/unsubscribe?email=x">Unsubscribe</a>`

	link, err := ExtractMagicLink(body)
	if err != nil {
		t.Fatalf("ExtractMagicLink: %v", err)
	}
	if !strings.Contains(link, "token=abc123") {
		t.Fatalf("wrong link chosen: %q", link)
	}
	if strings.Contains(link, "&amp;") {
		t.Fatalf("HTML entity was not unescaped: %q", link)
	}
	if !strings.Contains(link, "&email=") {
		t.Fatalf("second query parameter lost: %q", link)
	}
}

func TestExtractMagicLinkHandlesEncodedSeparator(t *testing.T) {
	body := `<a href="https://crushfling.com/verify?token=xyz&#38;email=a%40b.c">Go</a>`
	link, err := ExtractMagicLink(body)
	if err != nil {
		t.Fatalf("ExtractMagicLink: %v", err)
	}
	if !strings.HasPrefix(link, "https://crushfling.com/verify?token=xyz&email=") {
		t.Fatalf("link = %q", link)
	}
}

func TestExtractMagicLinkHandlesJSONEscapedSlashes(t *testing.T) {
	body := `{"url":"https:\/\/crushfling.com\/verify?token=zzz"}`
	link, err := ExtractMagicLink(body)
	if err != nil {
		t.Fatalf("ExtractMagicLink: %v", err)
	}
	if !strings.Contains(link, "token=zzz") {
		t.Fatalf("link = %q", link)
	}
}

func TestExtractMagicLinkRejectsForeignHost(t *testing.T) {
	body := `<a href="https://evil.example/verify?token=abc">x</a>`
	if link, err := ExtractMagicLink(body); err == nil {
		t.Fatalf("expected error, got %q", link)
	}
}

func TestExtractMagicLinkIgnoresNoiseLinks(t *testing.T) {
	body := `<a href="https://crushfling.com/help">Help</a>
<a href="https://crushfling.com/privacy">Privacy</a>`
	if link, err := ExtractMagicLink(body); err == nil {
		t.Fatalf("expected error, got %q", link)
	}
}

func TestScoreMagicLink(t *testing.T) {
	token := scoreMagicLink("https://crushfling.com/api/auth/callback/email?token=abc")
	plain := scoreMagicLink("https://crushfling.com/")
	if token <= plain {
		t.Fatalf("token link (%d) should score above the homepage (%d)", token, plain)
	}
	if scoreMagicLink("https://crushfling.com/unsubscribe?x=1") >= plain {
		t.Fatal("unsubscribe links must score below the homepage link")
	}
}

// TestExtractMagicLinkFollowsSiteHost proves that a mirror/mock origin works
// through the normal inbox path (not only via --manual-link).
func TestExtractMagicLinkFollowsSiteHost(t *testing.T) {
	link := "http://127.0.0.1:8099/verify?token=abc"
	if _, err := ExtractMagicLink(link); err == nil {
		t.Fatal("link from an unrelated host must be rejected by default")
	}
	SetSiteHost("http://127.0.0.1:8099/account")
	t.Cleanup(func() { SetSiteHost("") })
	got, err := ExtractMagicLink("<p>Click <a href=\"" + link + "\">here</a></p>")
	if err != nil {
		t.Fatalf("extract after SetSiteHost: %v", err)
	}
	if got != link {
		t.Fatalf("got %q, want %q", got, link)
	}
	if _, err := ExtractMagicLink("https://crushfling.com/verify?token=zzz"); err == nil {
		t.Fatal("after switching hosts the default host must no longer be accepted")
	}
}
