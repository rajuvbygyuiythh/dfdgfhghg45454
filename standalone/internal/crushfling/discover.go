package crushfling

// Site discovery: the sign-up page is the only reliable description of the
// sign-up API.
//
// The live site is a Laravel application (Laravel CSRF token, XSRF-TOKEN
// cookie, /livewire/update) and NOT a NextAuth app: /api/auth/csrf and
// /api/auth/providers both return the site's 404 page. Hard-coding guessed
// endpoints therefore fails before the first request. Instead, this file
// parses https://crushfling.com/account and answers four questions:
//
//  1. How does the page submit the email (plain form / Livewire / JSON fetch)?
//  2. What is the CSRF token and which hidden fields must travel with it?
//  3. Which endpoint paths does the page itself reference?
//  4. Which Livewire component/method is bound to the submit button?
//
// Discovery never invents data: when a signal is absent the caller falls back
// to a static path from api.json or to the browser DOM path, and the reason is
// recorded in Signals for the run log and --probe output.

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// AuthStyle describes how the page asks the server to send the link.
type AuthStyle string

const (
	AuthStyleForm     AuthStyle = "form"     // plain HTML form post (Laravel)
	AuthStyleLivewire AuthStyle = "livewire" // Livewire component call
	AuthStyleJSON     AuthStyle = "json"     // fetch/axios JSON endpoint
	AuthStyleUnknown  AuthStyle = "unknown"
)

// SiteProfile is what the sign-up page says about its own flow.
type SiteProfile struct {
	Style       AuthStyle
	AccountPath string

	CSRFToken   string            // meta[name=csrf-token] or hidden _token
	FormAction  string            // <form action="...">
	FormMethod  string            // POST/GET
	EmailField  string            // input name that receives the address
	ExtraFields map[string]string // hidden inputs (e.g. _token, honeypot)

	LivewirePath     string // usually /livewire/update
	LivewireSnapshot string // wire:snapshot (HTML-unescaped JSON text)
	LivewireMethod   string // wire:submit / wire:click method name

	UploadPaths  []string // page-referenced upload endpoints
	ProfilePaths []string // page-referenced profile endpoints
	Paths        []string // every endpoint path the page referenced
	Signals      []string // human-readable evidence for --probe and the log
}

// Endpoints returns every path discovery found, for diagnostics.
func (p *SiteProfile) Endpoints() []string {
	if p == nil {
		return nil
	}
	seen := map[string]bool{}
	if len(p.Paths) > 0 {
		out := make([]string, 0, len(p.Paths))
		for _, path := range p.Paths {
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
		sort.Strings(out)
		return out
	}
	out := make([]string, 0, len(p.UploadPaths)+len(p.ProfilePaths))
	for _, group := range [][]string{p.UploadPaths, p.ProfilePaths} {
		for _, path := range group {
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// Describe renders a short, secret-free summary for logs.
func (p *SiteProfile) Describe() string {
	if p == nil {
		return "no profile"
	}
	parts := []string{
		fmt.Sprintf("style=%s", p.Style),
		fmt.Sprintf("account=%s", p.AccountPath),
	}
	if p.FormAction != "" {
		parts = append(parts, "action="+p.FormAction)
	}
	if p.LivewireMethod != "" {
		parts = append(parts, "livewire="+p.LivewireMethod)
	}
	parts = append(parts, fmt.Sprintf("csrf=%t", p.CSRFToken != ""))
	if eps := p.Endpoints(); len(eps) > 0 {
		parts = append(parts, fmt.Sprintf("endpoints=%d", len(eps)))
	}
	return strings.Join(parts, " ")
}

var (
	reMetaCSRFName    = regexp.MustCompile(`(?is)<meta[^>]+name=["']csrf-token["'][^>]*content=["']([^"']+)["']`)
	reMetaCSRFContent = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']+)["'][^>]*name=["']csrf-token["']`)
	reFormBlock       = regexp.MustCompile(`(?is)<form\b([^>]*)>(.*?)</form>`)
	reInputTag        = regexp.MustCompile(`(?is)<input\b([^>]*)>`)
	reAttrPair        = regexp.MustCompile(`(?is)([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	reWireSnapshot    = regexp.MustCompile(`(?is)wire:snapshot\s*=\s*"([^"]+)"`)
	reWireSubmit      = regexp.MustCompile(`(?is)wire:submit(?:\.[a-zA-Z]+)*\s*=\s*"([^"]+)"`)
	reWireClick       = regexp.MustCompile(`(?is)wire:click\s*=\s*"([^"]+)"`)
	reFetchCall       = regexp.MustCompile("(?is)(?:fetch|axios(?:\\.(?:post|get|put))?|\\$\\.(?:post|get|ajax))\\s*\\(\\s*[\"'`]([^\"'`\\s]+)[\"'`]")
	rePathLiteral     = regexp.MustCompile(`["'](/(?:api|auth|magic|livewire|onboarding|profile|user)[A-Za-z0-9_\-/]*)["']`)
	reHTMLTag         = regexp.MustCompile(`(?is)<[^>]*>`)
	reSpaces          = regexp.MustCompile(`\s+`)
)

// DiscoverSite loads the sign-up page through sess and describes its flow.
func DiscoverSite(ctx context.Context, sess *Session, endpoints EndpointConfig) (*SiteProfile, error) {
	if sess == nil {
		return nil, fmt.Errorf("discovery needs a session")
	}
	accountPath := strings.TrimSpace(endpoints.AccountPath)
	if accountPath == "" {
		accountPath = "/account"
	}
	status, body, _, err := sess.Get(ctx, accountPath)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", accountPath, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d (Cloudflare interstitial or the page moved)", accountPath, status)
	}
	lower := strings.ToLower(body)
	if strings.Contains(lower, "page not found") && !strings.Contains(lower, "sign-in link") {
		return nil, fmt.Errorf("GET %s returned the site's 404 page", accountPath)
	}

	p := &SiteProfile{
		Style:        AuthStyleUnknown,
		AccountPath:  accountPath,
		EmailField:   "email",
		ExtraFields:  map[string]string{},
		LivewirePath: firstNonEmptyPath(endpoints.LivewirePath, "/livewire/update"),
	}
	p.Signals = append(p.Signals, fmt.Sprintf("account page: HTTP %d, %d bytes", status, len(body)))

	// 1) CSRF: Laravel writes it into <meta name="csrf-token"> and as a hidden
	//    _token field. The cookie (XSRF-TOKEN) is URL-encoded and handled by
	//    Session.SetCSRF.
	if m := reMetaCSRFName.FindStringSubmatch(body); m != nil {
		p.CSRFToken = html.UnescapeString(strings.TrimSpace(m[1]))
	} else if m := reMetaCSRFContent.FindStringSubmatch(body); m != nil {
		p.CSRFToken = html.UnescapeString(strings.TrimSpace(m[1]))
	}
	if p.CSRFToken != "" {
		p.Signals = append(p.Signals, "csrf meta tag: present")
	}

	// 2) Livewire marker. The snapshot is a JSON document in an attribute, so
	//    it must be HTML-unescaped before it can be sent back.
	if m := reWireSnapshot.FindStringSubmatch(body); m != nil {
		p.LivewireSnapshot = html.UnescapeString(strings.TrimSpace(m[1]))
		if p.LivewireSnapshot != "" {
			p.Style = AuthStyleLivewire
			p.Signals = append(p.Signals, fmt.Sprintf("livewire snapshot: %d bytes", len(p.LivewireSnapshot)))
		}
	}
	if m := reWireSubmit.FindStringSubmatch(body); m != nil {
		p.LivewireMethod = strings.TrimSpace(m[1])
	} else if m := reWireClick.FindStringSubmatch(body); m != nil {
		p.LivewireMethod = strings.TrimSpace(m[1])
	}
	if p.LivewireMethod != "" {
		p.Signals = append(p.Signals, "livewire method: "+p.LivewireMethod)
	}

	// 3) Forms: find the one that carries an email input.
	forms := 0
	for _, fm := range reFormBlock.FindAllStringSubmatch(body, -1) {
		tagAttrs := parseHTMLAttrs(fm[1])
		inner := fm[2]
		inputs := parseHTMLInputs(inner)
		emailField := ""
		for _, in := range inputs {
			kind := strings.ToLower(in["type"])
			name := in["name"]
			if name == "" {
				continue
			}
			if kind == "email" || strings.Contains(strings.ToLower(name), "email") {
				if emailField == "" {
					emailField = name
				}
			}
			if kind == "hidden" && in["value"] != "" {
				p.ExtraFields[name] = in["value"]
			}
		}
		if emailField == "" {
			continue
		}
		forms++
		if p.EmailField == "" || p.EmailField == "email" {
			p.EmailField = emailField
		}
		if p.FormAction == "" {
			p.FormAction = strings.TrimSpace(tagAttrs["action"])
		}
		if p.FormMethod == "" {
			p.FormMethod = strings.ToUpper(strings.TrimSpace(tagAttrs["method"]))
		}
		break
	}
	if forms > 0 {
		p.Signals = append(p.Signals, fmt.Sprintf("forms with an email input: %d", forms))
	}
	if p.FormAction == "" {
		p.FormAction = accountPath
	}
	if p.FormMethod == "" {
		p.FormMethod = "POST"
	}
	if p.Style != AuthStyleLivewire {
		if forms > 0 {
			p.Style = AuthStyleForm
		} else if len(p.ExtraFields) > 0 || strings.Contains(lower, "continue with email") {
			// The page has an email control but no <form>: the browser posts it
			// with JavaScript, or a Livewire component renders later.
			p.Style = AuthStyleJSON
		}
	}

	// 4) Endpoints referenced by the page or its inline scripts.
	paths := collectPaths(body)
	uploads := []string{}
	profiles := []string{}
	for _, path := range paths {
		low := strings.ToLower(path)
		switch {
		case strings.Contains(low, "upload"), strings.Contains(low, "media"), strings.Contains(low, "avatar"):
			uploads = append(uploads, path)
		case strings.Contains(low, "profile"), strings.Contains(low, "onboarding"), strings.Contains(low, "user/"):
			profiles = append(profiles, path)
		}
	}
	if len(uploads) == 0 && strings.TrimSpace(endpoints.UploadPath) != "" {
		uploads = append(uploads, strings.TrimSpace(endpoints.UploadPath))
	}
	if len(profiles) == 0 {
		for _, path := range endpoints.ProfilePaths {
			if strings.TrimSpace(path) != "" {
				profiles = append(profiles, strings.TrimSpace(path))
			}
		}
	}
	p.UploadPaths = dedupeStrings(uploads)
	p.ProfilePaths = dedupeStrings(profiles)
	p.Paths = dedupeStrings(paths)
	if len(paths) > 0 {
		p.Signals = append(p.Signals, fmt.Sprintf("paths referenced by the page: %d", len(paths)))
	}

	if p.Style == AuthStyleUnknown {
		p.Signals = append(p.Signals, "no form/livewire signal: falling back to the JSON candidates in api.json")
	}
	return p, nil
}

// collectPaths pulls same-site endpoint paths out of the HTML and inline JS.
func collectPaths(body string) []string {
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "javascript:") {
			return
		}
		if strings.HasPrefix(raw, "http") {
			u, err := url.Parse(raw)
			if err != nil {
				return
			}
			host := strings.ToLower(u.Hostname())
			if host != "crushfling.com" && !strings.HasSuffix(host, ".crushfling.com") {
				return
			}
			raw = u.Path
		}
		if !strings.HasPrefix(raw, "/") || strings.Contains(raw, "${") || strings.Contains(raw, "{{") {
			return
		}
		// Drop cache-busting query strings, keep the path only.
		if i := strings.IndexAny(raw, "?#"); i >= 0 {
			raw = raw[:i]
		}
		if len(raw) < 4 || raw == "/api" {
			return
		}
		seen[raw] = true
	}
	for _, m := range reFetchCall.FindAllStringSubmatch(body, -1) {
		add(m[1])
	}
	for _, m := range rePathLiteral.FindAllStringSubmatch(body, -1) {
		add(m[1])
	}
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func parseHTMLInputs(inner string) []map[string]string {
	out := []map[string]string{}
	for _, m := range reInputTag.FindAllStringSubmatch(inner, -1) {
		out = append(out, parseHTMLAttrs(m[1]))
	}
	return out
}

func parseHTMLAttrs(tag string) map[string]string {
	out := map[string]string{}
	for _, m := range reAttrPair.FindAllStringSubmatch(tag, -1) {
		key := strings.ToLower(strings.TrimSpace(m[1]))
		value := m[2]
		if value == "" {
			value = m[3]
		}
		if key == "" {
			continue
		}
		out[key] = html.UnescapeString(value)
	}
	return out
}

// stripHTML turns a response body into a short single-line excerpt that is
// safe to put in a log line.
func stripHTML(body string) string {
	text := reHTMLTag.ReplaceAllString(body, " ")
	text = html.UnescapeString(text)
	text = reSpaces.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// hasSuccessCopy reports whether a page/JSON reply confirms that the sign-in
// mail was accepted.
func hasSuccessCopy(body string) bool {
	low := strings.ToLower(body)
	for _, marker := range []string{
		"email sent", "sign-in link", "sign in link", "magic link",
		"check your inbox", "check your email", "we sent", "link sent",
	} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

// hasRejectionCopy reports a clear refusal, so the caller can retry instead of
// polling a mailbox that will stay empty for two minutes.
func hasRejectionCopy(body string) bool {
	low := strings.ToLower(body)
	for _, marker := range []string{
		"too many requests", "too many attempts", "rate limit", "throttle",
		"invalid email", "email is invalid", "not a valid email",
		"captcha", "turnstile", "try again later",
	} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func firstNonEmptyPath(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
