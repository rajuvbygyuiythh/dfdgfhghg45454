package crushfling

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

// Session wraps cookie jar + headers to keep same session across magic-link flow
type Session struct {
	Jar       http.CookieJar
	Client    *http.Client
	UserAgent string
	BaseURL   string
	Headers   map[string]string
}

// NewSession creates a session with persistent cookies (critical for magic-link verify)
func NewSession(userAgent, proxyURL string) (*Session, error) {
	if strings.TrimSpace(userAgent) == "" {
		userAgent = DefaultUA
	}
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = false
	transport.MaxIdleConns = 20
	transport.IdleConnTimeout = 90 * time.Second
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	client := &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	return &Session{
		Jar:       jar,
		Client:    client,
		UserAgent: userAgent,
		BaseURL:   BaseURL,
		Headers: map[string]string{
			"User-Agent":      userAgent,
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
			"Accept-Language": "en-US,en;q=0.9",
			// Accept-Encoding is intentionally left to net/http: setting it by
			// hand would disable the transparent gzip decoding we rely on.
			"Sec-Ch-Ua":          `"Chromium";v="131", "Not_A Brand";v="24"`,
			"Sec-Ch-Ua-Mobile":   "?0",
			"Sec-Ch-Ua-Platform": `"Windows"`,
			"Sec-Fetch-Dest":     "document",
			"Sec-Fetch-Mode":     "navigate",
			"Sec-Fetch-Site":     "same-origin",
			"Referer":            BaseURL + "/account",
			"Origin":             BaseURL,
		},
	}, nil
}

// Get does GET in same session (cookies preserved)
func (s *Session) Get(ctx context.Context, rawURL string) (int, string, http.Header, error) {
	if !strings.HasPrefix(rawURL, "http") {
		rawURL = s.BaseURL + rawURL
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	for k, v := range s.Headers {
		req.Header.Set(k, v)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return 0, "", nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	// Detect the Cloudflare interstitial. A plain 403 is not enough: the site
	// also returns 403 JSON for genuine auth failures, and those must reach
	// the caller as a normal status so the error text stays useful.
	if resp.StatusCode == 403 || resp.StatusCode == 503 {
		lower := strings.ToLower(string(b))
		if strings.Contains(lower, "cf-challenge") ||
			strings.Contains(lower, "just a moment") ||
			strings.Contains(lower, "cf_chl_opt") ||
			(strings.Contains(lower, "cloudflare") && strings.Contains(lower, "challenge")) {
			return resp.StatusCode, string(b), resp.Header, fmt.Errorf("cloudflare challenge %d", resp.StatusCode)
		}
	}
	return resp.StatusCode, string(b), resp.Header, nil
}

// PostJSON does POST JSON in same session
func (s *Session) PostJSON(ctx context.Context, path string, payload any) (int, string, http.Header, error) {
	if !strings.HasPrefix(path, "http") {
		path = s.BaseURL + path
	}
	var bodyReader io.Reader
	if payload != nil {
		switch v := payload.(type) {
		case string:
			bodyReader = strings.NewReader(v)
		default:
			b, _ := json.Marshal(payload)
			bodyReader = strings.NewReader(string(b))
		}
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", path, bodyReader)
	for k, v := range s.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	// Laravel answers JSON (not an HTML redirect/error page) only when it sees
	// an AJAX/JSON request. Without these a 419 or a 422 arrives as HTML and
	// the retry logic cannot tell what happened.
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	resp, err := s.Client.Do(req)
	if err != nil {
		return 0, "", nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header, nil
}

// SetHeader adds/replaces a request header for every later request of this
// session. Used for Laravel's X-CSRF-TOKEN and Livewire's X-Livewire marker.
func (s *Session) SetHeader(name, value string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if s.Headers == nil {
		s.Headers = map[string]string{}
	}
	s.Headers[name] = value
}

// SetCSRF wires a Laravel CSRF token into the session:
//   - X-CSRF-TOKEN for fetch/axios/Livewire requests,
//   - X-XSRF-TOKEN decoded from the XSRF-TOKEN cookie (Laravel URL-encodes it),
//   - the token itself is added to form posts by PostForm.
//
// An empty token is ignored, so callers can pass the discovery result blindly.
func (s *Session) SetCSRF(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	s.SetHeader("X-CSRF-TOKEN", token)
	if raw := s.CookieValue("XSRF-TOKEN"); raw != "" {
		// The cookie value is URL-encoded; Laravel's VerifyCsrfToken decodes
		// it before comparing, so the header must carry the decoded form.
		if decoded, err := url.QueryUnescape(raw); err == nil && decoded != "" {
			s.SetHeader("X-XSRF-TOKEN", decoded)
		}
	}
}

// CookieValue returns one cookie value ("" when absent).
func (s *Session) CookieValue(name string) string {
	if name == "" {
		return ""
	}
	return s.Cookies()[name]
}

// PostForm sends application/x-www-form-urlencoded data in this session and
// follows redirects, so a Laravel flash message ("Email sent!") is visible in
// the returned body.
func (s *Session) PostForm(ctx context.Context, path string, values url.Values) (int, string, http.Header, error) {
	if !strings.HasPrefix(path, "http") {
		path = s.BaseURL + path
	}
	body := ""
	if values != nil {
		body = values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(body))
	if err != nil {
		return 0, "", nil, err
	}
	for k, v := range s.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	resp, err := s.Client.Do(req)
	if err != nil {
		return 0, "", nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header, nil
}

// Cookies returns all cookies for base domain
func (s *Session) Cookies() map[string]string {
	u, _ := url.Parse(s.BaseURL)
	m := map[string]string{}
	for _, c := range s.Jar.Cookies(u) {
		m[c.Name] = c.Value
	}
	return m
}

// SetCookies merges cookies obtained elsewhere (e.g. a real browser that
// solved Cloudflare) into this session's jar.
func (s *Session) SetCookies(cookies map[string]string) {
	if len(cookies) == 0 {
		return
	}
	u, err := url.Parse(s.BaseURL)
	if err != nil {
		return
	}
	list := make([]*http.Cookie, 0, len(cookies))
	for name, value := range cookies {
		if name == "" {
			continue
		}
		list = append(list, &http.Cookie{Name: name, Value: value, Path: "/"})
	}
	s.Jar.SetCookies(u, list)
}

// SetTracer routes every request of this session (including redirect hops)
// through a tracer. It is safe to call twice: the previous tracer is replaced
// but the underlying transport is preserved.
func (s *Session) SetTracer(t *Tracer) {
	if t == nil || s.Client == nil {
		return
	}
	base := s.Client.Transport
	if tr, ok := base.(*tracedTransport); ok {
		base = tr.base
	}
	s.Client.Transport = t.Wrap(base)
}

// SetUserAgent keeps later requests on the same UA as the browser that solved
// the challenge; an empty value keeps the current one.
func (s *Session) SetUserAgent(ua string) {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return
	}
	s.UserAgent = ua
	if s.Headers == nil {
		s.Headers = map[string]string{}
	}
	s.Headers["User-Agent"] = ua
}

// DumpCookies header string
func (s *Session) CookieHeader() string {
	c := s.Cookies()
	parts := make([]string, 0, len(c))
	for k, v := range c {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

// ClickMagicLink follows magic link IN SAME SESSION - this is key step
// The link contains token, server sets session cookie on redirect
func (s *Session) ClickMagicLink(ctx context.Context, magicLink string) (map[string]string, string, error) {
	if !strings.HasPrefix(magicLink, "http") {
		magicLink = s.BaseURL + magicLink
	}
	// Magic link often redirects 302 -> / or /onboarding
	// We follow redirects and capture final cookies
	status, body, hdr, err := s.Get(ctx, magicLink)
	if err != nil {
		return nil, "", err
	}
	// Also check if body contains next onboarding URL
	cookies := s.Cookies()
	// Some flows require extra step: if body has meta refresh or JS redirect, parse it
	if status == 200 {
		if next := extractNextURL(body); next != "" {
			// Follow next URL in same session
			s.Get(ctx, next)
			cookies = s.Cookies()
		}
	}
	// A Location header is only present when the client did not follow it
	// (redirects are followed automatically). Re-requesting it would consume a
	// single-use magic-link token a second time, so it is deliberately ignored.
	_ = hdr
	return cookies, body, nil
}

// nextURLPatterns match the places a post-sign-in redirect can be hidden:
// meta refresh, JS location assignment, or a plain link to onboarding/profile.
var nextURLPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)content=["']\d+\s*;\s*url=([^"']+)["']`),
	regexp.MustCompile(`(?i)(?:window\.)?location(?:\.href)?\s*=\s*["']([^"']+)["']`),
	regexp.MustCompile(`(?i)location\.replace\(\s*["']([^"']+)["']\s*\)`),
	regexp.MustCompile(`(?i)href=["']([^"']*(?:onboarding|welcome|profile)[^"']*)["']`),
}

// extractNextURL returns an absolute next-page URL found in an HTML/JS body.
func extractNextURL(body string) string {
	for _, re := range nextURLPatterns {
		m := re.FindStringSubmatch(body)
		if len(m) != 2 {
			continue
		}
		cand := strings.TrimSpace(m[1])
		cand = strings.ReplaceAll(cand, `\/`, `/`)
		if cand == "" || strings.HasPrefix(cand, "data:") || strings.HasPrefix(cand, "javascript:") {
			continue
		}
		if strings.HasPrefix(cand, "/") {
			return BaseURL + cand
		}
		if strings.HasPrefix(cand, "http://") || strings.HasPrefix(cand, "https://") {
			return cand
		}
	}
	return ""
}
