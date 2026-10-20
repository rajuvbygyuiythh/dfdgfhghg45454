package crushfling

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	emailprov "crushfling/internal/crushfling/email"
	"crushfling/internal/proxy"
)

// Register orchestrates full auto-register flow
type Register struct {
	cfg          RegisterConfig
	proxyEntries []proxy.Entry
	captionMgr   *CaptionManager
	pictureMgr   *PictureManager
	accountStore *AccountStore
	endpoints    EndpointConfig
	stats        RegistrationStats
	mu           sync.Mutex
	logger       func(string, ...any)

	// Diagnostics (always available; file output is best effort).
	runLog   *RunLogger
	tracer   *Tracer
	runID    string
	debugDir string

	// Site discovery is shared by all accounts: the page describes its own
	// sign-up flow (Laravel form / Livewire / JSON) and the CSRF token is
	// re-read per session.
	discoverMu sync.Mutex
	site       *SiteProfile
}

func NewRegister(cfg RegisterConfig, logger func(string, ...any)) (*Register, error) {
	if logger == nil {
		logger = func(f string, a ...any) { log.Printf(f, a...) }
	}
	// Load proxies
	var entries []proxy.Entry
	if cfg.ProxyEntries != "" {
		ents, err := proxy.ParseList(cfg.ProxyEntries)
		if err != nil {
			return nil, fmt.Errorf("proxy parse: %w", err)
		}
		entries = ents
	} else if cfg.ProxyFile != "" {
		raw, err := os.ReadFile(cfg.ProxyFile)
		if err != nil {
			return nil, fmt.Errorf("proxy file: %w", err)
		}
		ents, err := proxy.ParseList(string(raw))
		if err != nil {
			return nil, err
		}
		entries = ents
	}
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Join("crushfling", "data")
	}
	if err := EnsureDataLayout(cfg.DataDir); err != nil {
		return nil, fmt.Errorf("data layout: %w", err)
	}
	// Caption manager (creates caption.txt from the template when missing)
	// bio.txt is the operator-facing list; caption.txt still works for data
	// folders created before it existed (see BioPath).
	captionMgr, err := NewCaptionManager(BioPath(cfg.DataDir))
	if err != nil {
		return nil, fmt.Errorf("caption.txt: %w", err)
	}
	// Picture manager
	picDir := cfg.AvatarDir
	if picDir == "" {
		picDir = filepath.Join(cfg.DataDir, "pictures")
	}
	picMgr, err := NewPictureManager(picDir)
	if err != nil {
		return nil, fmt.Errorf("pictures: %w", err)
	}
	// API paths (editable in <data>/api.json)
	endpoints, err := LoadEndpointConfig(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	// Account store
	store := NewAccountStore(cfg.DataDir)

	// Diagnostics: a run log file is always written (credential-free after
	// redaction), debug mode raises the level and enables failure dumps, and
	// --trace-http additionally records request/response bodies.
	runID := NewRunID()
	debugDir := filepath.Join(cfg.DataDir, "debug")
	minLevel := LevelInfo
	if cfg.Debug || cfg.TraceHTTP {
		minLevel = LevelDebug
	}
	runLog, err := NewRunLogger(LogOptions{
		DataDir:  cfg.DataDir,
		ToFile:   true,
		MinLevel: minLevel,
		OnLine: func(_ LogLevel, line string) {
			logger("%s", line)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("run log: %w", err)
	}
	var tracer *Tracer
	if cfg.TraceHTTP {
		tracer, err = NewTracer(TraceOptions{
			Path:       filepath.Join(debugDir, "trace-"+runID+".jsonl"),
			Bodies:     true,
			BodyLimit:  32 << 10,
			MaxRecords: 400,
			OnRecord: func(rec HTTPRecord) {
				runLog.Debugf("http %s %s -> %d (%dms, %dB)", rec.Method, rec.URL, rec.Status, rec.DurationMS, rec.ResponseBytes)
			},
		})
		if err != nil {
			runLog.Warnf("http trace disabled: %v", err)
			tracer = nil
		}
	}

	reg := &Register{
		cfg:          cfg,
		proxyEntries: entries,
		captionMgr:   captionMgr,
		pictureMgr:   picMgr,
		accountStore: store,
		endpoints:    endpoints,
		stats:        RegistrationStats{Total: cfg.Count, ByCode: map[string]int{}},
		logger:       logger,
		runLog:       runLog,
		tracer:       tracer,
		runID:        runID,
		debugDir:     debugDir,
	}
	if minLevel == LevelDebug {
		// Mail polling happens inside the email package; route its
		// diagnostics through this run's logger.
		emailprov.DebugLog = func(format string, args ...any) { reg.log("debug", format, args...) }
	}
	return reg, nil
}

// LogPath returns the run log file, or "" when file logging is unavailable.
func (r *Register) LogPath() string {
	if r == nil || r.runLog == nil {
		return ""
	}
	return r.runLog.Path()
}

// TracePath returns the JSONL trace file, or "" when tracing is off.
func (r *Register) TracePath() string {
	if r == nil || r.tracer == nil {
		return ""
	}
	return r.tracer.Path()
}

// Close flushes the run log and the HTTP trace. It is safe to call twice.
func (r *Register) Close() error {
	if r == nil {
		return nil
	}
	// A finished (or cancelled) run must not leave Chromium windows behind.
	if n := CloseAllBrowsers(); n > 0 {
		r.log("debug", "closed %d leftover browser(s)", n)
	}
	var first error
	if r.tracer != nil {
		if err := r.tracer.Close(); err != nil && first == nil {
			first = err
		}
	}
	if r.runLog != nil {
		if err := r.runLog.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (r *Register) Stats() RegistrationStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stats
}

// RunBulk creates Count accounts with concurrency control.
func (r *Register) RunBulk(ctx context.Context) error {
	r.mu.Lock()
	r.stats.Running = true
	r.stats.Total = r.cfg.Count
	r.stats.Pending = r.cfg.Count
	r.mu.Unlock()
	r.log("info", "run %s: %d account(s), concurrency=%d, provider=%s, log=%s",
		r.runID, r.cfg.Count, r.cfg.Concurrency, r.cfg.EmailProvider, r.LogPath())
	if r.cfg.TraceHTTP {
		r.log("info", "http trace: %s", r.TracePath())
	}
	defer func() {
		r.mu.Lock()
		r.stats.Running = false
		r.stats.Pending = 0
		r.mu.Unlock()
	}()

	return r.runBulk(ctx)
}

// runBulk is the internal scheduling loop.
func (r *Register) runBulk(ctx context.Context) error {
	var firstErr error
	var errMu sync.Mutex

	concurrency := r.cfg.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > r.cfg.Count {
		concurrency = r.cfg.Count
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

loop:
	for i := 0; i < r.cfg.Count; i++ {
		// Waiting for a free slot must also honour cancellation, otherwise
		// StopBulk appears to hang while the queue is full.
		select {
		case <-ctx.Done():
			break loop
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			res := r.RegisterOne(ctx, idx)
			r.mu.Lock()
			if res.Verified {
				r.stats.Success++
			} else {
				r.stats.Failed++
			}
			r.stats.Pending = r.cfg.Count - (r.stats.Success + r.stats.Failed)
			r.mu.Unlock()
			if res.Error != "" {
				errMu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("idx %d: %s", idx, res.Error)
				}
				errMu.Unlock()
			}
			// Only fully verified accounts are persisted; failures stay in the log.
			if res.Verified {
				if err := r.accountStore.Add(*res); err != nil {
					r.log("warn", "[%d] save account: %v", idx, err)
				}
			}
			// Random delay between accounts (cancelable).
			select {
			case <-ctx.Done():
			case <-time.After(RandomDelay(r.cfg.MinDelayMs, r.cfg.MaxDelayMs)):
			}
		}(i)
	}
	wg.Wait()
	return firstErr
}

// RegisterOne creates one account and adds diagnostics: duration, per-step
// timings, stable failure code, counters and (in debug mode) a JSON dump. A
// panic in any step is recovered so one bad account cannot kill the run.
func (r *Register) RegisterOne(ctx context.Context, idx int) (res *AccountResult) {
	start := time.Now()
	attempts := r.cfg.MaxRetries
	if attempts < 0 {
		attempts = 0
	}
	if attempts > 5 {
		// More than five full retries is almost always a broken proxy or a
		// dead provider, and it burns inboxes fast.
		attempts = 5
	}
	for attempt := 0; ; attempt++ {
		res = r.registerOne(ctx, idx)
		if res == nil {
			res = &AccountResult{Error: "internal: registration returned no result", ErrorCode: CodePanic}
		}
		if res.Verified || attempt >= attempts || ctx.Err() != nil || !retryableFailure(res) {
			break
		}
		wait := time.Duration(2*(attempt+1)) * time.Second
		r.log("warn", "[%d] attempt %d failed at %s (%s); retrying in %s",
			idx, attempt+1, res.FailedStep, res.ErrorCode, wait)
		select {
		case <-ctx.Done():
			// Stop pressed: do not start another attempt.
		case <-time.After(wait):
		}
		if ctx.Err() != nil {
			break
		}
	}
	res.DurationMS = time.Since(start).Milliseconds()
	if res.Timings == nil {
		res.Timings = map[string]int64{}
	}
	res.Timings["total"] = res.DurationMS

	r.mu.Lock()
	if r.stats.ByCode == nil {
		r.stats.ByCode = map[string]int{}
	}
	if res.ErrorCode != "" {
		r.stats.ByCode[res.ErrorCode]++
	}
	if res.Error != "" {
		r.stats.LastError = truncate(Redact(res.Error), 300)
	}
	r.mu.Unlock()

	if res.ErrorCode != "" || !res.Verified {
		r.reportFailure(idx, res)
	}
	r.log("info", "[%d] done in %dms verified=%v", idx, res.DurationMS, res.Verified)
	return res
}

// registerOne is the flow itself; RegisterOne wraps it with recovery, timing
// and diagnostics so that every exit path is measured.
func (r *Register) registerOne(ctx context.Context, idx int) (res *AccountResult) {
	defer func() {
		if rec := recover(); rec != nil {
			stack := make([]byte, 16<<10)
			n := runtime.Stack(stack, false)
			res = &AccountResult{
				Error:      fmt.Sprintf("panic: %v", rec),
				ErrorCode:  CodePanic,
				FailedStep: "panic",
			}
			r.log("error", "[%d] panic: %v\n%s", idx, rec, string(stack[:n]))
		}
	}()
	if err := ctx.Err(); err != nil {
		return failResult(ProfileData{}, "start", CodeCanceled, err, "", nil)
	}

	timings := map[string]int64{}
	stepStart := time.Now()
	mark := func(step string) {
		d := time.Since(stepStart)
		timings[step] = d.Milliseconds()
		r.log("debug", "[%d] step %s took %dms", idx, step, d.Milliseconds())
		stepStart = time.Now()
	}

	// Pick proxy
	var proxyEntry *proxy.Entry
	var proxyURL string
	if len(r.proxyEntries) > 0 {
		e := r.proxyEntries[idx%len(r.proxyEntries)]
		if len(r.proxyEntries) > r.cfg.Count {
			e = r.proxyEntries[rand.Intn(len(r.proxyEntries))]
		}
		proxyEntry = &e
		proxyURL = e.TransportURL().String()
		r.log("info", "[%d] proxy: %s", idx, redactProxy(proxyURL))
	}

	// Step 1: Create temp mail inbox
	inbox, err := r.createInbox(ctx, proxyURL)
	if err != nil {
		return failResult(ProfileData{}, "inbox", CodeInbox, err, proxyURL, timings)
	}
	email := inbox.Email()
	r.log("info", "[%d] Temp mail created: %s via %s", idx, Redact(email), inboxType(inbox))
	mark("inbox")

	// Generate the profile from the single source of truth in generator.go:
	// woman, looking for men, one of US/GB/AU/CA/DE. Do not re-implement this
	// here - a second copy is how the persona drifted before.
	profile := GenerateProfile(email)
	// Take one caption as the bio. It is only deleted from caption.txt once the
	// account is verified AND its profile was completed, so a failed attempt
	// does not waste a line.
	captionLine := ""
	captionCommitted := false
	if caption, err := r.captionMgr.Next(); err == nil {
		captionLine = caption
		// Give the reservation back on every failure path; only an account
		// that actually used the caption keeps the reservation consumed.
		defer func() {
			if !captionCommitted {
				r.captionMgr.Release(captionLine)
			}
		}()
		caption = strings.ReplaceAll(caption, "@myhandle", "@"+profile.PlatformUsername)
		caption = strings.ReplaceAll(caption, "{username}", profile.PlatformUsername)
		profile.Bio = caption
		r.log("info", "[%d] Bio line reserved: %q (%d left)", idx, truncate(profile.Bio, 50), r.captionMgr.Count())
	} else {
		r.log("warn", "[%d] bio list empty or missing: %v, using generated bio", idx, err)
	}
	// Pick picture
	var picPath string
	var avatarURL string
	if picPath, err = r.pictureMgr.PickRandom(); err == nil {
		profile.AvatarPath = picPath
		r.log("info", "[%d] Using picture: %s (remaining %d)", idx, filepath.Base(picPath), r.pictureMgr.Count())
	} else {
		r.log("warn", "[%d] no pictures: %v", idx, err)
	}

	// Step 2: Create session (same cookie jar for magic-link click)
	sess, err := NewSession(RandomUserAgent(), proxyURL)
	if err != nil {
		return failResult(profile, "session", CodeConfig, err, proxyURL, timings)
	}
	if r.tracer != nil {
		sess.SetTracer(r.tracer)
	}
	mark("session")

	// Step 3: learn the sign-up flow from the page itself. The live site is a
	// Laravel app: the CSRF token is in a meta tag and the email call is either
	// a plain form post or a Livewire component call. The previously assumed
	// NextAuth routes (/api/auth/csrf, /api/auth/signin/email) 404.
	site := r.discoverSite(ctx, sess)
	if site.CSRFToken != "" {
		sess.SetCSRF(site.CSRFToken)
	}
	mark("discover")

	csrf := &CSRFToken{Token: site.CSRFToken}
	if csrf.Token == "" {
		// Fallback for a JSON site or an operator-supplied api.json path.
		if legacy, ferr := r.fetchCSRF(ctx, sess); ferr == nil {
			csrf = legacy
			sess.SetCSRF(legacy.Token)
		} else {
			r.log("warn", "[%d] no CSRF token found: %v (continuing without)", idx, ferr)
		}
	}
	if csrf.Token != "" {
		// Never print the token itself, not even truncated: it is a valid CSRF
		// secret until the session ends.
		r.log("info", "[%d] CSRF token received (%d chars, cookie names: %s)", idx, len(csrf.Token), cookieNames(sess.Cookies()))
	}
	mark("csrf")

	// A pasted link was obtained elsewhere, so do not send a second sign-in
	// mail; the CSRF/cookies above are enough for the verify request.
	pasted := strings.TrimSpace(r.cfg.PastedLink)
	magicRequested := pasted != ""
	if magicRequested {
		r.log("info", "[%d] Skipping the sign-in request (pasted link supplied)", idx)
	}
	for attempt := 0; !magicRequested && attempt < 3; attempt++ {
		err = r.requestMagicLinkWithProfile(ctx, sess, email, csrf, site)
		if err == nil {
			magicRequested = true
			break
		}
		r.log("warn", "[%d] magic-link attempt %d failed: %v", idx, attempt+1, err)
		wait := 2 * time.Second
		if strings.Contains(err.Error(), "429") {
			wait = 10 * time.Second
		}
		select {
		case <-ctx.Done():
			return failResult(profile, "magic_request", CodeCanceled, ctx.Err(), proxyURL, timings)
		case <-time.After(wait):
		}
		// Fallback to browser mode if API keeps failing
		if attempt == 1 && r.cfg.UseBrowser {
			r.log("info", "[%d] Trying browser fallback for magic link", idx)
			if bcookies, bua, berr := r.requestMagicLinkBrowser(ctx, email, proxyEntry); berr == nil {
				magicRequested = true
				// Reuse the browser's cookies (cf_clearance) and UA so the
				// magic-link click is not treated as a different client.
				sess.SetCookies(bcookies)
				sess.SetUserAgent(bua)
				r.log("info", "[%d] Browser fallback ok (%d cookies copied to session)", idx, len(bcookies))
				break
			} else {
				r.log("warn", "[%d] browser fallback failed: %v", idx, berr)
			}
		}
	}
	if !magicRequested {
		return failResult(profile, "magic_request", CodeMagicRequest, err, proxyURL, timings)
	}
	r.log("info", "[%d] Magic link requested, polling inbox...", idx)
	mark("magic_request")

	// Step 4: obtain the magic link. Three modes, all clicked with the same
	// session in step 5:
	//   a) pasted_link  — a link the operator already received
	//   b) manual_link  — prompt on stdin and use that link
	//   c) default      — poll the temp inbox created above
	var magicLink string
	if pasted := strings.TrimSpace(r.cfg.PastedLink); pasted != "" {
		if err := validateMagicLink(pasted); err != nil {
			return failResult(profile, "pasted_link", CodeBadLink, err, proxyURL, timings)
		}
		magicLink = pasted
		r.log("info", "[%d] Using the pasted magic link", idx)
	} else if r.cfg.ManualLink {
		magicLink, err = readMagicLinkFromStdin(idx)
		if err != nil {
			return failResult(profile, "manual_link", CodeBadLink, err, proxyURL, timings)
		}
	} else {
		pollInterval := time.Duration(r.cfg.MailPollSec) * time.Second
		if pollInterval == 0 {
			pollInterval = 3 * time.Second
		}
		timeout := time.Duration(r.cfg.MailTimeoutSec) * time.Second
		if timeout == 0 {
			timeout = 120 * time.Second
		}
		r.log("debug", "[%d] polling %s every %s, timeout %s", idx, Redact(email), pollInterval, timeout)
		magicLink, err = inbox.WaitForMagicLink(ctx, pollInterval, timeout)
		if err != nil {
			return failResult(profile, "mail_poll", CodeMailPoll, err, proxyURL, timings)
		}
	}
	mark("mail")
	r.log("info", "[%d] Magic link ready: %s", idx, truncate(Redact(magicLink), 100))

	// Step 5: Click magic link IN SAME SESSION (critical)
	cookies, body, clickErr := sess.ClickMagicLink(ctx, magicLink)
	if clickErr != nil {
		r.log("warn", "[%d] magic link click failed: %v", idx, clickErr)
		if !r.cfg.BrowserVerify {
			res := failResult(profile, "magic_click", CodeMagicClick, clickErr, proxyURL, timings)
			res.MagicLink = magicLink
			return res
		}
		// A Cloudflare challenge on the click is recoverable in a browser.
		cookies, body = sess.Cookies(), ""
	}
	mark("magic_click")
	r.log("info", "[%d] Magic link clicked, cookies: %d (%s), body len %d", idx, len(cookies), cookieNames(cookies), len(body))

	// Verified means the verify endpoint handed us a real auth cookie (or the
	// body carried a session token). A landing page that merely contains the
	// word "profile" is not proof, so it is not used here.
	verified := hasSessionCookie(cookies)
	if !verified {
		// Cookies alone are a weak signal (the site may use a different name).
		// Ask the account page: a signed-in visitor no longer sees the email
		// form, so its absence is proof that the click authenticated us.
		if ok, why := r.sessionIsAuthenticated(ctx, sess); ok {
			r.log("info", "[%d] Session confirmed by the account page: %s", idx, why)
			verified = true
		}
	}
	if !verified {
		if extracted := extractSessionFromBody(body); extracted != "" {
			cookies["session-token"] = extracted
			verified = true
		}
	}
	// The /account page says "Open it and tap Sign In": some links land on a
	// page whose button performs the final call. If the plain GET set no
	// session cookie, do that click in a real browser and copy the cookies
	// back into this session.
	if !verified && r.cfg.BrowserVerify {
		r.log("info", "[%d] No session cookie yet - trying the browser verify fallback", idx)
		bcookies, bua, berr := r.verifyMagicLinkBrowser(ctx, magicLink, sess.Cookies(), proxyEntry)
		if len(bcookies) > 0 {
			sess.SetCookies(bcookies)
			cookies = sess.Cookies()
		}
		sess.SetUserAgent(bua)
		verified = hasSessionCookie(cookies)
		if berr != nil {
			r.log("warn", "[%d] browser verify: %v", idx, berr)
		}
		if verified {
			r.log("info", "[%d] Browser verify succeeded (%d cookies)", idx, len(cookies))
		}
	}
	mark("verify")

	// Step 6: Complete profile (upload avatar + set fields)
	profileCompleted := false
	profileVerified := false
	profileErr := ""
	avatarUploaded := false
	if verified {
		if err := ctx.Err(); err != nil {
			return failResult(profile, "upload", CodeCanceled, err, proxyURL, timings)
		}
		// Try avatar upload first if picture exists
		if picPath != "" {
			if uploadedURL, uerr := r.uploadAvatar(ctx, sess, site, picPath); uerr == nil {
				avatarURL = uploadedURL
				profile.AvatarURL = uploadedURL
				avatarUploaded = true
				r.log("info", "[%d] Avatar uploaded: %s", idx, uploadedURL)
			} else {
				r.log("warn", "[%d] avatar upload failed: %v", idx, uerr)
			}
		}
		mark("upload")
		// Profile creation
		if err := r.createProfileAPI(ctx, sess, site, profile, avatarURL); err != nil {
			profileErr = err.Error()
			r.log("warn", "[%d] profile API failed: %v", idx, err)
			// Not fatal for the session, but the account is not usable as a
			// dating profile until the fields exist, so the result keeps the
			// PROFILE code and releases the caption for a later attempt.
		} else {
			profileCompleted = true
			// The picture is only consumed once the profile really exists; a
			// failed profile must not burn an image.
			if avatarUploaded {
				_ = r.pictureMgr.MarkUsed(picPath)
			}
			r.log("info", "[%d] Profile created: %s age %d %s -> %s", idx, profile.Username, profile.Age, profile.Country, profile.Platform)
			// R9 from the roadmap: a 2xx is not proof that the profile is
			// public. Check the public URL and keep the answer in the result.
			if perr := r.verifyProfilePublic(ctx, sess, profile.Username); perr != nil {
				r.log("warn", "[%d] public profile check: %v", idx, perr)
			} else {
				profileVerified = true
				r.log("info", "[%d] Public profile page confirmed", idx)
			}
		}
		mark("profile")
	}

	result := &AccountResult{
		ProfileData:    profile,
		MagicLink:      magicLink,
		SessionCookies: cookies,
		Verified:       verified,
		ProxyUsed:      proxyURL,
		// AvatarPath lives on the embedded ProfileData and is already set
		// there when the picture was picked; Go does not allow assigning a
		// promoted field inside a composite literal.
		ProfileCompleted: profileCompleted,
		ProfileVerified:  profileVerified,
		ProfileError:     profileErr,
		Timings:          timings,
	}
	if verified && profileCompleted && captionLine != "" {
		if err := r.captionMgr.RemoveLine(captionLine); err != nil {
			r.log("warn", "[%d] could not remove the used bio line: %v", idx, err)
		} else {
			captionCommitted = true
			r.log("info", "[%d] Used bio line removed (%d left)", idx, r.captionMgr.Count())
		}
	} else if verified && !profileCompleted && captionLine != "" {
		r.log("info", "[%d] Profile incomplete - bio line released for a later account", idx)
	}
	if !verified {
		result.ErrorCode = CodeVerify
		result.FailedStep = "verify"
		result.Error = newStepError("verify", CodeVerify, fmt.Errorf("no session cookie after the magic link - check token expiry, Cloudflare and api.json paths")).Error()
		result.RawResponse = truncate(Redact(body), 500)
	} else if !profileCompleted {
		result.ErrorCode = CodeProfile
		result.FailedStep = "profile"
		result.Error = newStepError("profile", CodeProfile, fmt.Errorf("%s", profileErr)).Error()
	}
	return result
}

// retryableFailure decides whether a whole account attempt is worth repeating.
//
// Only failures BEFORE the session is proven are retried. A "verify" failure
// is deliberately not retried: the click may have created the account, and a
// second attempt would make a duplicate. Upload/profile failures are not
// retried either - the session is valid and the error is not transient.
func retryableFailure(res *AccountResult) bool {
	if res == nil || res.Verified {
		return false
	}
	switch res.FailedStep {
	case "inbox", "session", "start", "magic_request", "mail_poll", "magic_click":
		return true
	}
	return false
}

func (r *Register) createInbox(ctx context.Context, proxyURL string) (emailprov.Inbox, error) {
	// A fixed catch-all address takes priority: the caller supplies the mail
	// fetcher through the custom provider.
	if strings.TrimSpace(r.cfg.CustomEmail) != "" {
		return emailprov.NewCustomInboxWithHook(
			strings.TrimSpace(r.cfg.CustomEmail),
			strings.TrimSpace(r.cfg.CustomEmailHook),
		), nil
	}
	provName := r.cfg.EmailProvider
	if provName == "" {
		provName = "mailtm"
	}
	prov, err := emailprov.NewProvider(provName)
	if err != nil {
		return nil, fmt.Errorf("email provider %q: %w", provName, err)
	}
	_ = proxyURL // inbox APIs are always reached directly, not via the site proxy
	r.log("debug", "creating inbox via %s", prov.Name())
	inbox, err := prov.CreateInbox(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", prov.Name(), err)
	}
	return inbox, nil
}

func (r *Register) fetchCSRF(ctx context.Context, sess *Session) (*CSRFToken, error) {
	if strings.TrimSpace(r.endpoints.CSRFPath) == "" {
		// Empty means "the token is on the account page", which discovery
		// already tried. Requesting "" would hit the homepage instead.
		return nil, fmt.Errorf("no csrf_path configured (discovery found no token)")
	}
	status, body, _, err := sess.Get(ctx, r.endpoints.CSRFPath)
	if err != nil {
		return nil, err
	}
	if status == 200 {
		var data struct {
			CSRFToken string `json:"csrfToken"`
		}
		if json.Unmarshal([]byte(body), &data) == nil && data.CSRFToken != "" {
			return &CSRFToken{Token: data.CSRFToken}, nil
		}
	}
	// Try extract from HTML
	re := regexp.MustCompile(`name="csrfToken" value="([^"]+)"`)
	m := re.FindStringSubmatch(body)
	if len(m) == 2 {
		return &CSRFToken{Token: m[1]}, nil
	}
	re2 := regexp.MustCompile(`csrfToken["']\s*:\s*["']([^"']+)["']`)
	m2 := re2.FindStringSubmatch(body)
	if len(m2) == 2 {
		return &CSRFToken{Token: m2[1]}, nil
	}
	return nil, fmt.Errorf("csrf not found (status %d)", status)
}

// magicRequest is one candidate endpoint for the "send me a sign-in link" call.
type magicRequest struct {
	path string
	body any
	form bool // send as application/x-www-form-urlencoded instead of JSON
}

func (r *Register) requestMagicLink(ctx context.Context, sess *Session, email string, csrf *CSRFToken) error {
	// Primary: NextAuth v4 email provider. It accepts a form post and, with
	// json=true, answers with a small JSON body instead of a redirect.
	form := url.Values{}
	form.Set("csrfToken", safeToken(csrf))
	form.Set("email", email)
	form.Set("callbackUrl", sess.BaseURL+"/")
	form.Set("json", "true")

	primary := strings.TrimSpace(r.endpoints.MagicLinkPath)
	payloads := []magicRequest{}
	if primary != "" {
		payloads = append(payloads,
			// NextAuth-style form post on the configured path.
			magicRequest{path: primary, body: form.Encode(), form: true},
			// The same path as JSON covers custom magic-link APIs.
			magicRequest{path: primary, body: map[string]string{
				"email": email, "csrfToken": safeToken(csrf), "callbackUrl": sess.BaseURL + "/",
			}},
		)
	}
	for _, alt := range r.endpoints.MagicLinkAltPaths {
		alt = strings.TrimSpace(alt)
		if alt == "" || alt == r.endpoints.MagicLinkPath {
			continue
		}
		payloads = append(payloads, magicRequest{path: alt, body: map[string]string{"email": email}})
	}

	var lastErr error
	var attempts []string
	for _, p := range payloads {
		status, body, err := r.sendMagicRequest(ctx, sess, p)
		if err != nil {
			lastErr = err
			attempts = append(attempts, fmt.Sprintf("%s: %v", p.path, err))
			continue
		}
		if status == 429 {
			return fmt.Errorf("429 rate limited on %s", p.path)
		}
		if status >= 200 && status < 300 {
			// NextAuth returns {"url": "..."} on success and {"url": "/error..."}
			// or an "error" field on failure, so check the body before trusting 2xx.
			var js map[string]any
			if json.Unmarshal([]byte(body), &js) == nil {
				if e, ok := js["error"].(string); ok && e != "" {
					lastErr = fmt.Errorf("%s rejected: %s", p.path, e)
					attempts = append(attempts, lastErr.Error())
					continue
				}
				if u, ok := js["url"].(string); ok && strings.Contains(u, "error") {
					lastErr = fmt.Errorf("%s rejected: %s", p.path, u)
					attempts = append(attempts, lastErr.Error())
					continue
				}
			}
			r.log("debug", "magic request accepted: %s -> %d", p.path, status)
			return nil
		}
		lastErr = fmt.Errorf("%s returned %d: %s", p.path, status, truncate(Redact(body), 200))
		attempts = append(attempts, lastErr.Error())
		if status == 404 || status == 405 || status == 400 || status == 401 {
			continue
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no magic-link endpoint is configured and discovery did not find one - run `crushfling --probe`")
	}
	if len(attempts) > 0 {
		return fmt.Errorf("%w (tried: %s)", lastErr, strings.Join(attempts, " | "))
	}
	return lastErr
}

// refreshCSRF re-reads the account page and installs its token. Laravel
// expires the CSRF token together with the session, so a run that spends two
// minutes polling the mailbox can get a perfectly valid 419 on the next POST.
// The caller retries once with the fresh token instead of giving up.
func (r *Register) refreshCSRF(ctx context.Context, sess *Session) error {
	site, err := DiscoverSite(ctx, sess, r.endpoints)
	if err != nil {
		return fmt.Errorf("csrf refresh: %w", err)
	}
	r.discoverMu.Lock()
	r.site = site
	r.discoverMu.Unlock()
	if strings.TrimSpace(site.CSRFToken) == "" {
		return fmt.Errorf("csrf refresh: the account page has no csrf token")
	}
	sess.SetCSRF(site.CSRFToken)
	r.log("debug", "csrf token refreshed (%d chars)", len(site.CSRFToken))
	return nil
}

// requestMagicLinkWithProfile sends the sign-in mail using the flow the page
// actually uses, then falls back to the static JSON candidates.
func (r *Register) requestMagicLinkWithProfile(ctx context.Context, sess *Session, email string, csrf *CSRFToken, site *SiteProfile) error {
	if site != nil {
		switch site.Style {
		case AuthStyleLivewire:
			if err := r.requestMagicLinkLivewire(ctx, sess, email, site); err == nil {
				return nil
			} else {
				r.log("warn", "livewire request failed: %v (trying the JSON fallbacks)", err)
			}
		case AuthStyleForm:
			if err := r.requestMagicLinkForm(ctx, sess, email, site); err == nil {
				return nil
			} else {
				r.log("warn", "form request failed: %v (trying the JSON fallbacks)", err)
			}
		}
	}
	return r.requestMagicLink(ctx, sess, email, csrf)
}

// requestMagicLinkForm posts the page's own form (Laravel: _token + email).
func (r *Register) requestMagicLinkForm(ctx context.Context, sess *Session, email string, site *SiteProfile) error {
	action := strings.TrimSpace(site.FormAction)
	if action == "" {
		action = "/account"
	}
	form := url.Values{}
	for key, value := range site.ExtraFields {
		if key == "" || value == "" {
			continue
		}
		form.Set(key, value)
	}
	if site.CSRFToken != "" {
		form.Set("_token", site.CSRFToken)
	}
	field := strings.TrimSpace(site.EmailField)
	if field == "" {
		field = "email"
	}
	form.Set(field, email)

	status, body, _, err := sess.PostForm(ctx, action, form)
	if err != nil {
		return err
	}
	if status == 419 {
		// Expired token: re-read the page once and resend with the new one.
		if rerr := r.refreshCSRF(ctx, sess); rerr != nil {
			return fmt.Errorf("%s returned 419 and the token could not be refreshed: %w", action, rerr)
		}
		if site.CSRFToken != "" {
			form.Set("_token", site.CSRFToken)
		}
		status, body, _, err = sess.PostForm(ctx, action, form)
		if err != nil {
			return err
		}
	}
	if status == 429 {
		return fmt.Errorf("429 rate limited on %s", action)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("%s returned %d: %s", action, status, truncate(stripHTML(body), 160))
	}
	if hasRejectionCopy(body) {
		return fmt.Errorf("%s rejected the request: %s", action, truncate(stripHTML(body), 160))
	}
	if hasSuccessCopy(body) {
		r.log("debug", "form post accepted by %s (%d bytes)", action, len(body))
		return nil
	}
	r.log("debug", "form post %s -> %d without confirmation copy; continuing", action, status)
	return nil
}

// requestMagicLinkLivewire drives the Livewire component bound to the email
// form. The snapshot comes from the page; the method name is taken from
// wire:submit/wire:click and a few common names are tried when it is missing.
func (r *Register) requestMagicLinkLivewire(ctx context.Context, sess *Session, email string, site *SiteProfile) error {
	if strings.TrimSpace(site.LivewireSnapshot) == "" {
		return fmt.Errorf("no livewire snapshot on the account page")
	}
	path := strings.TrimSpace(site.LivewirePath)
	if path == "" {
		path = "/livewire/update"
	}
	field := strings.TrimSpace(site.EmailField)
	if field == "" {
		field = "email"
	}
	methods := []string{}
	if site.LivewireMethod != "" {
		methods = append(methods, site.LivewireMethod)
	}
	for _, candidate := range []string{"sendLink", "sendMagicLink", "sendLoginLink", "requestLink", "send", "submit"} {
		dup := false
		for _, existing := range methods {
			if existing == candidate {
				dup = true
			}
		}
		if !dup {
			methods = append(methods, candidate)
		}
	}
	sess.SetHeader("X-Livewire", "true")

	var attempts []string
	for _, method := range methods {
		payload := map[string]any{
			"_token": site.CSRFToken,
			"components": []map[string]any{{
				"snapshot": site.LivewireSnapshot,
				"updates":  map[string]any{field: email},
				"calls":    []map[string]any{{"path": "", "method": method, "params": []any{}}},
			}},
		}
		status, body, _, err := sess.PostJSON(ctx, path, payload)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: %v", method, err))
			continue
		}
		if status < 200 || status >= 300 {
			attempts = append(attempts, fmt.Sprintf("%s: HTTP %d %s", method, status, truncate(stripHTML(body), 100)))
			continue
		}
		var probe struct {
			Errors map[string]any `json:"errors"`
		}
		if json.Unmarshal([]byte(body), &probe) == nil && len(probe.Errors) > 0 {
			attempts = append(attempts, fmt.Sprintf("%s: validation errors %s", method, truncate(stripHTML(body), 100)))
			continue
		}
		if hasRejectionCopy(body) {
			attempts = append(attempts, fmt.Sprintf("%s: %s", method, truncate(stripHTML(body), 100)))
			continue
		}
		r.log("debug", "livewire %s accepted (%d bytes)", method, len(body))
		return nil
	}
	return fmt.Errorf("livewire request failed (tried: %s)", strings.Join(attempts, " | "))
}

// discoverSite parses the account page once and caches the static parts. The
// CSRF token is per session, so a cached profile is returned without it.
func (r *Register) discoverSite(ctx context.Context, sess *Session) *SiteProfile {
	profile, err := DiscoverSite(ctx, sess, r.endpoints)
	if err != nil {
		r.log("warn", "site discovery failed: %v (static api.json paths remain)", err)
	} else {
		r.discoverMu.Lock()
		first := r.site == nil
		r.site = profile
		r.discoverMu.Unlock()
		if first {
			r.log("info", "sign-up flow detected: %s", profile.Describe())
			for _, signal := range profile.Signals {
				r.log("debug", "discovery: %s", signal)
			}
		}
		return profile
	}
	r.discoverMu.Lock()
	defer r.discoverMu.Unlock()
	if r.site != nil {
		cached := *r.site
		cached.CSRFToken = "" // session-bound: never reuse another session's token
		return &cached
	}
	return &SiteProfile{Style: AuthStyleJSON, AccountPath: "/account", EmailField: "email", ExtraFields: map[string]string{}}
}

// sessionIsAuthenticated asks the account page whether the visitor is signed
// in; the email form disappears after a successful magic-link click.
func (r *Register) sessionIsAuthenticated(ctx context.Context, sess *Session) (bool, string) {
	accountPath := strings.TrimSpace(r.endpoints.AccountPath)
	if accountPath == "" {
		accountPath = "/account"
	}
	status, body, _, err := sess.Get(ctx, accountPath)
	if err != nil || status != http.StatusOK {
		return false, ""
	}
	low := strings.ToLower(body)
	for _, marker := range []string{"continue with email", "we sent a sign-in link", "enter your email to sign in"} {
		if strings.Contains(low, marker) {
			return false, ""
		}
	}
	for _, marker := range []string{"sign out", "log out", "logout", "my profile", "account settings"} {
		if strings.Contains(low, marker) {
			return true, "account page shows " + marker
		}
	}
	return false, ""
}

// verifyProfilePublic fetches the public profile URL. A 2xx from the profile
// API is not proof that the profile renders; this is.
func (r *Register) verifyProfilePublic(ctx context.Context, sess *Session, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("empty username")
	}
	template := strings.TrimSpace(r.endpoints.PublicProfilePath)
	if template == "" {
		template = "/p/{username}/"
	}
	path := strings.ReplaceAll(template, "{username}", url.PathEscape(username))
	status, body, _, err := sess.Get(ctx, path)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%s returned 404", path)
	}
	if status != http.StatusOK {
		return fmt.Errorf("%s returned HTTP %d", path, status)
	}
	low := strings.ToLower(body)
	if strings.Contains(low, "page not found") {
		return fmt.Errorf("%s returned the 404 page", path)
	}
	if !strings.Contains(low, strings.ToLower(username)) {
		return fmt.Errorf("%s does not mention %s", path, username)
	}
	return nil
}

// sendMagicRequest performs one candidate request and returns status + body.
func (r *Register) sendMagicRequest(ctx context.Context, sess *Session, p magicRequest) (int, string, error) {
	if !p.form {
		status, body, _, err := sess.PostJSON(ctx, p.path, p.body)
		return status, body, err
	}
	raw, _ := p.body.(string)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sess.BaseURL+p.path, strings.NewReader(raw))
	if err != nil {
		return 0, "", err
	}
	for k, v := range sess.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	resp, err := sess.Client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), nil
}

func (r *Register) verifyMagicLinkBrowser(ctx context.Context, link string, seed map[string]string, proxyEntry *proxy.Entry) (map[string]string, string, error) {
	debugLog := func(format string, args ...any) { r.log("debug", format, args...) }
	// Only show a window when the operator already opted into browser mode.
	headless := r.cfg.Headless || !r.cfg.UseBrowser
	dumpDir := ""
	if r.cfg.Debug {
		dumpDir = r.debugDir
	}
	return verifyMagicLinkInBrowser(ctx, link, seed, headless, proxyEntry, dumpDir, debugLog)
}

func (r *Register) requestMagicLinkBrowser(ctx context.Context, email string, proxyEntry *proxy.Entry) (map[string]string, string, error) {
	debugLog := func(format string, args ...any) { r.log("debug", format, args...) }
	dumpDir := ""
	if r.cfg.Debug {
		dumpDir = r.debugDir
	}
	return requestMagicLinkBrowser(ctx, email, r.cfg.Headless, proxyEntry, dumpDir, debugLog)
}

func (r *Register) uploadAvatar(ctx context.Context, sess *Session, site *SiteProfile, picPath string) (string, error) {
	data, err := os.ReadFile(picPath)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(picPath))
	mimeType := "image/jpeg"
	switch ext {
	case ".png":
		mimeType = "image/png"
	case ".webp":
		mimeType = "image/webp"
	case ".gif":
		mimeType = "image/gif"
	}

	// The page's own upload endpoint wins; api.json and the old default are
	// fallbacks. Field names differ between stacks, so each is tried.
	paths := []string{}
	if site != nil {
		paths = append(paths, site.UploadPaths...)
	}
	if configured := strings.TrimSpace(r.endpoints.UploadPath); configured != "" {
		paths = append(paths, configured)
	}
	paths = dedupeStrings(paths)
	if len(paths) == 0 {
		paths = []string{"/api/upload"}
	}

	fields := []string{"file", "image", "avatar", "photo", "picture"}
	var attempts []string
	refreshed := false
	for attempt := 0; attempt < 2; attempt++ {
		saw419 := false
		for _, path := range paths {
			for _, field := range fields {
				uploaded, err := postAvatarMultipart(ctx, sess, path, field, filepath.Base(picPath), mimeType, data)
				if err == nil {
					r.log("debug", "avatar uploaded via %s as %q", path, field)
					return uploaded, nil
				}
				attempts = append(attempts, fmt.Sprintf("%s[%s]: %v", path, field, err))
				if strings.Contains(err.Error(), "419") {
					saw419 = true
				}
				if len(attempts) >= 8 {
					break
				}
			}
			if len(attempts) >= 8 {
				break
			}
		}
		if saw419 && !refreshed {
			if rerr := r.refreshCSRF(ctx, sess); rerr == nil {
				refreshed = true
				attempts = append(attempts, "csrf refreshed after 419")
				continue
			}
		}
		break
	}
	return "", fmt.Errorf("avatar upload failed (tried: %s)", strings.Join(attempts, " | "))
}

// postAvatarMultipart uploads one file and returns the URL/path the server
// reports. The multipart body is built with the standard writer so the
// boundary and headers are always valid.
func postAvatarMultipart(ctx context.Context, sess *Session, path, field, filename, mimeType string, data []byte) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filename))
	header.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sess.BaseURL+path, &buf)
	if err != nil {
		return "", err
	}
	for k, v := range sess.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	resp, err := sess.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d %s", resp.StatusCode, truncate(stripHTML(string(body)), 120))
	}

	var parsed struct {
		URL      string `json:"url"`
		Location string `json:"location"`
		Path     string `json:"path"`
		Data     struct {
			URL  string `json:"url"`
			Path string `json:"path"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &parsed)
	for _, candidate := range []string{parsed.URL, parsed.Location, parsed.Path, parsed.Data.URL, parsed.Data.Path} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate), nil
		}
	}
	// Last resort: the first absolute URL in the reply.
	re := regexp.MustCompile(`https://[^\s"']+`)
	if m := re.FindString(string(body)); m != "" {
		return m, nil
	}
	return "", fmt.Errorf("no URL in the upload response: %s", truncate(stripHTML(string(body)), 120))
}

// profilePayload builds the onboarding body. Laravel APIs in the wild accept
// either camelCase or snake_case, and we have never seen this site's schema;
// sending both spellings in one object is harmless (Laravel ignores extra
// input) and removes a whole class of 422s. Empty optional fields are omitted.
func profilePayload(profile ProfileData, avatarURL string) map[string]any {
	p := map[string]any{
		"username":          profile.Username,
		"age":               profile.Age,
		"gender":            profile.Gender,
		"lookingFor":        profile.LookingFor,
		"looking_for":       profile.LookingFor,
		"country":           profile.Country,
		"platform":          profile.Platform,
		"platformUsername":  profile.PlatformUsername,
		"platform_username": profile.PlatformUsername,
		"bio":               profile.Bio,
	}
	if profile.Name != "" {
		// The profile page can show a display name next to the handle. The
		// exact key was never observed, so send the three spellings Laravel
		// forms use; unknown inputs are ignored by the server.
		p["name"] = profile.Name
		p["display_name"] = profile.Name
		p["displayName"] = profile.Name
	}
	if profile.City != "" {
		p["city"] = profile.City
	}
	if avatarURL != "" {
		p["avatarUrl"] = avatarURL
		p["avatar_url"] = avatarURL
	}
	return p
}

func (r *Register) createProfileAPI(ctx context.Context, sess *Session, site *SiteProfile, profile ProfileData, avatarURL string) error {
	payload := profilePayload(profile, avatarURL)
	// Discovered paths first, then the configured fallbacks.
	paths := []string{}
	if site != nil {
		paths = append(paths, site.ProfilePaths...)
	}
	paths = append(paths, r.endpoints.ProfilePaths...)
	paths = append(paths, "/api/profile/create")
	seen := map[string]bool{}
	var attempts []string
	for _, ep := range paths {
		ep = strings.TrimSpace(ep)
		if ep == "" || seen[ep] {
			continue
		}
		seen[ep] = true
		status, body, _, err := sess.PostJSON(ctx, ep, payload)
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: %v", ep, err))
			continue
		}
		if status == 419 {
			// The CSRF token expired while the mailbox was being polled.
			if rerr := r.refreshCSRF(ctx, sess); rerr != nil {
				attempts = append(attempts, fmt.Sprintf("%s: 419 and the token could not be refreshed: %v", ep, rerr))
				continue
			}
			status, body, _, err = sess.PostJSON(ctx, ep, payload)
			if err != nil {
				attempts = append(attempts, fmt.Sprintf("%s: %v (after csrf refresh)", ep, err))
				continue
			}
			if status == 419 {
				attempts = append(attempts, fmt.Sprintf("%s: 419 again after refreshing the token", ep))
				continue
			}
		}
		if status >= 200 && status < 300 {
			// A 2xx can still be a logical failure; inspect small JSON
			// replies instead of trusting the status code blindly.
			var js map[string]any
			if json.Unmarshal([]byte(body), &js) == nil {
				if ok, exists := js["success"].(bool); exists && !ok {
					attempts = append(attempts, fmt.Sprintf("%s: success=false %s", ep, truncate(Redact(body), 120)))
					continue
				}
				if e, ok := js["error"].(string); ok && e != "" {
					attempts = append(attempts, fmt.Sprintf("%s: %s", ep, e))
					continue
				}
			}
			r.log("debug", "profile accepted: %s -> %d", ep, status)
			return nil
		}
		attempts = append(attempts, fmt.Sprintf("%s: HTTP %d %s", ep, status, truncate(Redact(strings.TrimSpace(body)), 160)))
	}
	return fmt.Errorf("profile endpoints failed (tried: %s)", strings.Join(attempts, " | "))
}

func (r *Register) log(level, format string, args ...any) {
	if r == nil {
		return
	}
	if r.runLog != nil {
		r.runLog.Logf(ParseLogLevel(level), format, args...)
		return
	}
	if r.logger != nil {
		r.logger("%s", level+": "+Redact(fmt.Sprintf(format, args...)))
	}
}

// helpers
func safeToken(c *CSRFToken) string {
	if c == nil {
		return ""
	}
	return c.Token
}

// hasSessionCookie reports whether the jar holds a real auth cookie
// (next-auth.session-token, authjs.session-token, __session, ...).
// Cloudflare's cf_clearance and the csrf-token cookies deliberately do not
// count: both contain "next-auth" in their names, but neither authenticates.
// cookieNames lists cookie names without values for diagnostics.
func cookieNames(cookies map[string]string) string {
	if len(cookies) == 0 {
		return "none"
	}
	names := make([]string, 0, len(cookies))
	for name := range cookies {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func hasSessionCookie(cookies map[string]string) bool {
	for name := range cookies {
		low := strings.ToLower(name)
		// CSRF cookies and Cloudflare cookies mention "session"/"auth" in
		// their names on some stacks but never authenticate.
		if strings.Contains(low, "csrf") || strings.Contains(low, "xsrf") ||
			strings.Contains(low, "cf_clearance") || strings.Contains(low, "cf_chl") {
			continue
		}
		if strings.Contains(low, "session") ||
			strings.HasPrefix(low, "remember_") ||
			low == "remember_token" {
			return true
		}
	}
	return false
}
func extractSessionFromBody(body string) string {
	re := regexp.MustCompile(`"sessionToken"\s*:\s*"([^"]+)"`)
	m := re.FindStringSubmatch(body)
	if len(m) == 2 {
		return m[1]
	}
	re2 := regexp.MustCompile(`token["']\s*:\s*["']([^"']+)["']`)
	m2 := re2.FindStringSubmatch(body)
	if len(m2) == 2 {
		return m2[1]
	}
	return ""
}
func inboxType(inbox emailprov.Inbox) string {
	// crude type detection via email domain
	e := inbox.Email()
	if strings.Contains(e, "mail.tm") {
		return "mail.tm"
	}
	if strings.Contains(e, "1secmail") {
		return "1secmail"
	}
	return "custom"
}

// validateMagicLink rejects anything that cannot be a crushfling sign-in link.
// The host check is exact so "https://evil.example/crushfling.com" fails.
func validateMagicLink(link string) error {
	link = strings.TrimSpace(link)
	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		return fmt.Errorf("link must start with http:// or https://")
	}
	u, err := url.Parse(link)
	if err != nil {
		return fmt.Errorf("link is not a URL: %w", err)
	}
	host := strings.ToLower(u.Hostname())
	base, berr := url.Parse(BaseURL)
	if berr != nil || base.Hostname() == "" {
		return fmt.Errorf("link host %q cannot be checked against %q", host, BaseURL)
	}
	baseHost := strings.ToLower(base.Hostname())
	if host != baseHost && !strings.HasSuffix(host, "."+baseHost) {
		return fmt.Errorf("link host %q is not %s", host, baseHost)
	}
	return nil
}

// readMagicLinkFromStdin lets the operator paste a link that arrived in a
// mailbox the temp-mail API cannot read (custom catch-all, real inbox, ...).
// The link is then clicked with the same session that requested it.
func readMagicLinkFromStdin(idx int) (string, error) {
	fmt.Fprintf(os.Stderr, "[%d] Paste the crushfling sign-in link and press Enter: ", idx)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return "", fmt.Errorf("nothing pasted: %v", err)
	}
	line = strings.TrimSpace(line)
	if err := validateMagicLink(line); err != nil {
		return "", err
	}
	return line, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
