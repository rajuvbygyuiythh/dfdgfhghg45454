package crushfling

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chitchatbot/internal/proxy"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// requestMagicLinkBrowser is the fallback used when the pure-HTTP magic-link
// request is rejected (Cloudflare / Turnstile / changed endpoint). It drives a
// real Chromium through Rod so the page's own JS makes the request.
//
// The returned page stays open only for the duration of the request; the
// session cookie is picked up by the HTTP session afterwards, because the
// magic link is clicked over HTTP with the same cookie jar.
func requestMagicLinkBrowser(ctx context.Context, email string, headless bool, proxyEntry *proxy.Entry, dumpDir string, debugLog func(string, ...any)) (map[string]string, string, error) {
	if debugLog == nil {
		debugLog = func(string, ...any) {}
	}

	l := launcher.New().Headless(headless).Leakless(false)
	if proxyEntry != nil {
		u := proxyEntry.TransportURL()
		if u != nil {
			l = l.Proxy(u.String())
		}
	}
	controlURL, err := l.Launch()
	if err != nil {
		return nil, "", fmt.Errorf("launch chromium: %w", err)
	}
	defer l.Cleanup()
	defer trackBrowser(l)()

	br := rod.New().ControlURL(controlURL)
	if err := br.Connect(); err != nil {
		return nil, "", fmt.Errorf("connect chromium: %w", err)
	}
	defer br.Close()

	page, err := br.Page(proto.TargetCreateTarget{URL: AccountURL})
	if err != nil {
		return nil, "", fmt.Errorf("open account page: %w", err)
	}
	page = page.Context(ctx)
	_ = page.WaitLoad()

	// Fill the email field.
	var input *rod.Element
	for _, sel := range []string{`input[type="email"]`, `input[name="email"]`, `input[placeholder*="mail"]`} {
		if el, err := page.Timeout(6 * time.Second).Element(sel); err == nil {
			input = el
			break
		}
	}
	if input == nil {
		if err := page.Timeout(6 * time.Second).WaitLoad(); err != nil {
			debugLog("account page did not load: %v", err)
		}
		savePageArtifacts(page, dumpDir, "request", debugLog)
		return nil, "", fmt.Errorf("email input not found on %s", AccountURL)
	}
	if err := input.SelectAllText(); err != nil {
		debugLog("select email text: %v", err)
	}
	if err := input.Input(email); err != nil {
		return nil, "", fmt.Errorf("type email: %w", err)
	}
	time.Sleep(350 * time.Millisecond)

	// Click "Continue with Email" (fall back to the page's own React handler).
	clicked := false
	for _, sel := range []string{`button[type="submit"]`, `//button[contains(., "Continue")]`} {
		var el *rod.Element
		var err error
		if strings.HasPrefix(sel, "//") {
			el, err = page.Timeout(3 * time.Second).ElementX(sel)
		} else {
			el, err = page.Timeout(3 * time.Second).Element(sel)
		}
		if err == nil && el != nil {
			if err := el.Click(proto.InputMouseButtonLeft, 1); err == nil {
				clicked = true
				break
			}
		}
	}
	if !clicked {
		ok, err := page.Eval(`() => {
			const buttons = Array.from(document.querySelectorAll('button'));
			const target = buttons.find((b) => /continue/i.test(b.textContent || ''));
			if (!target) return false;
			target.click();
			return true;
		}`)
		if err != nil || !ok.Value.Bool() {
			savePageArtifacts(page, dumpDir, "request", debugLog)
			return nil, "", fmt.Errorf("continue button not found")
		}
	}

	// Wait for the confirmation copy ("Email sent!", "We sent a sign-in link").
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		for _, text := range []string{"Email sent", "sign-in link", "sent a sign-in"} {
			if ok, _, _ := page.HasX(`//*[contains(., "` + text + `")]`); ok {
				debugLog("browser confirmed magic-link request (%q)", text)
				return browserCookies(page), browserUserAgent(page), nil
			}
		}
		time.Sleep(400 * time.Millisecond)
	}

	// Last resort: look for an explicit error message so the caller can retry.
	if txt, err := page.Eval(`() => document.body ? document.body.innerText : ''`); err == nil {
		if msg := findErrorMessage(txt.Value.Str()); msg != "" {
			savePageArtifacts(page, dumpDir, "request", debugLog)
			return nil, "", fmt.Errorf("site rejected request: %s", msg)
		}
	}
	savePageArtifacts(page, dumpDir, "request", debugLog)
	return nil, "", fmt.Errorf("timed out waiting for the site to confirm the email")
}

// browserCookies exports the browser jar so the HTTP session can reuse the
// Cloudflare clearance and any cookie the site set during the request.
func browserCookies(page *rod.Page) map[string]string {
	cookies := map[string]string{}
	list, err := page.Cookies([]string{BaseURL})
	if err != nil {
		return cookies
	}
	for _, c := range list {
		if c != nil && c.Name != "" {
			cookies[c.Name] = c.Value
		}
	}
	return cookies
}

func browserUserAgent(page *rod.Page) string {
	res, err := page.Eval(`() => navigator.userAgent`)
	if err != nil || res == nil {
		return ""
	}
	return res.Value.Str()
}

// findErrorMessage extracts the first short error-looking line from page text.
func findErrorMessage(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many") ||
			strings.Contains(lower, "invalid") || strings.Contains(lower, "try again") {
			if len(line) > 160 {
				line = line[:160]
			}
			return line
		}
	}
	return ""
}

// verifyMagicLinkInBrowser is the fallback for the case the /account page
// describes: the email link opens a page where the user must tap "Sign In".
// The HTTP click already happened, so this only runs when no session cookie
// appeared. The HTTP session's cookies (including cf_clearance) are seeded
// into the browser so the site sees the same client.
func verifyMagicLinkInBrowser(ctx context.Context, magicLink string, seed map[string]string, headless bool, proxyEntry *proxy.Entry, dumpDir string, debugLog func(string, ...any)) (map[string]string, string, error) {
	if debugLog == nil {
		debugLog = func(string, ...any) {}
	}

	l := launcher.New().Headless(headless).Leakless(false)
	if proxyEntry != nil {
		if u := proxyEntry.TransportURL(); u != nil {
			l = l.Proxy(u.String())
		}
	}
	controlURL, err := l.Launch()
	if err != nil {
		return nil, "", fmt.Errorf("launch chromium: %w", err)
	}
	defer l.Cleanup()
	defer trackBrowser(l)()

	br := rod.New().ControlURL(controlURL)
	if err := br.Connect(); err != nil {
		return nil, "", fmt.Errorf("connect chromium: %w", err)
	}
	defer br.Close()

	page, err := br.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, "", fmt.Errorf("open blank page: %w", err)
	}
	page = page.Context(ctx)

	if len(seed) > 0 {
		params := make([]*proto.NetworkCookieParam, 0, len(seed))
		for name, value := range seed {
			if name == "" {
				continue
			}
			params = append(params, &proto.NetworkCookieParam{
				Name:  name,
				Value: value,
				URL:   BaseURL + "/",
			})
		}
		if err := page.SetCookies(params); err != nil {
			debugLog("seed cookies: %v", err)
		}
	}

	if err := page.Navigate(magicLink); err != nil {
		savePageArtifacts(page, dumpDir, "verify", debugLog)
		return nil, "", fmt.Errorf("open magic link: %w", err)
	}
	_ = page.WaitLoad()
	time.Sleep(2 * time.Second)

	clickExpr := `() => {
		const wanted = /^(sign in|signin|continue|verify|confirm|log in|login)$/i;
		const nodes = Array.from(document.querySelectorAll('button, a[role="button"], input[type="submit"]'));
		const target = nodes.find((node) => wanted.test((node.textContent || node.value || '').trim()));
		if (!target) return false;
		target.click();
		return true;
	}`

	deadline := time.Now().Add(20 * time.Second)
	clicked := false
	for time.Now().Before(deadline) {
		cookies := browserCookies(page)
		if hasSessionCookie(cookies) {
			return cookies, browserUserAgent(page), nil
		}
		if !clicked {
			if res, err := page.Eval(clickExpr); err == nil && res != nil && res.Value.Bool() {
				clicked = true
				debugLog("clicked the page's sign-in button")
			}
		}
		time.Sleep(700 * time.Millisecond)
	}

	cookies := browserCookies(page)
	savePageArtifacts(page, dumpDir, "verify", debugLog)
	if len(cookies) == 0 {
		return nil, "", fmt.Errorf("browser verify timed out without any cookie")
	}
	return cookies, browserUserAgent(page), fmt.Errorf("browser verify timed out; no session cookie yet")
}

// savePageArtifacts writes a screenshot and the page HTML next to the failure
// dumps when debug mode is on. Seeing the real page is the fastest way to
// recognise a Cloudflare interstitial or a changed onboarding form.
func savePageArtifacts(page *rod.Page, dir, tag string, debugLog func(string, ...any)) {
	if page == nil || strings.TrimSpace(dir) == "" {
		return
	}
	if debugLog == nil {
		debugLog = func(string, ...any) {}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		debugLog("page artifacts: %v", err)
		return
	}
	stamp := time.Now().Format("20060102-150405")
	if shot, err := page.Screenshot(false, nil); err == nil {
		path := filepath.Join(dir, fmt.Sprintf("%s-%s.png", tag, stamp))
		if err := os.WriteFile(path, shot, 0o600); err == nil {
			debugLog("page screenshot: %s", path)
		}
	}
	if html, err := page.HTML(); err == nil && html != "" {
		if len(html) > 512*1024 {
			html = html[:512*1024]
		}
		path := filepath.Join(dir, fmt.Sprintf("%s-%s.html", tag, stamp))
		if err := os.WriteFile(path, []byte(Redact(html)), 0o600); err == nil {
			debugLog("page HTML: %s", path)
		}
	}
}
