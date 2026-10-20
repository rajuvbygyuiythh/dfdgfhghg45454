package crushfling

// Debugging / observability layer.
//
// Everything here is additive: the registration flow keeps its existing
// signature, but every run now writes a log file, every failure can produce a
// redacted JSON dump (step, error code, HTTP trace, log tail) and the HTTP
// client can be wrapped with a tracer that records status codes and, when
// explicitly enabled, bodies.
//
// Secrets never leave the machine in clear text: Redact() masks magic-link
// tokens, csrf/session tokens, cookies and the local part of email addresses
// before anything is written to a log, a trace file or a dump.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Levels
// ---------------------------------------------------------------------------

// LogLevel is the severity of one log line.
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

func (l LogLevel) prefix() string { return strings.ToLower(l.String()) }

// ParseLogLevel maps the loose level names used across the package ("info",
// "warn", "warning", "error", "debug") to a LogLevel. Unknown names are info.
func ParseLogLevel(name string) LogLevel {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug", "trace":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error", "fatal":
		return LevelError
	default:
		return LevelInfo
	}
}

// ---------------------------------------------------------------------------
// Redaction
// ---------------------------------------------------------------------------

var (
	reURLSecret    = regexp.MustCompile(`(?i)([?&](?:token|code|key|secret|sig|signature|state|session|auth|expires|password)=)([^&\s"']+)`)
	reJSONSecret   = regexp.MustCompile(`(?i)("?(?:csrfToken|csrf_token|sessionToken|session_token|access_token|refresh_token|id_token|magicToken|token|password|api_key|apiKey|captchaToken)"?\s*[:=]\s*"?)([^"',\s}&]+)`)
	reHeaderSecret = regexp.MustCompile(`(?i)((?:cookie|set-cookie|authorization|x-api-key)\s*:\s*)([^\r\n]+)`)
	reBearer       = regexp.MustCompile(`(?i)(bearer\s+)([A-Za-z0-9._~+/=-]{8,})`)
	reEmail        = regexp.MustCompile(`([A-Za-z0-9._%+\-])[A-Za-z0-9._%+\-]*@([A-Za-z0-9.\-]+\.[A-Za-z]{2,})`)
)

// Redact masks credentials, session tokens and email local parts. It is safe
// to call on free text, JSON, URLs and header dumps, and it is applied before
// every log line, trace record and failure dump is written.
func Redact(s string) string {
	if s == "" {
		return s
	}
	s = reURLSecret.ReplaceAllString(s, "$1***")
	s = reJSONSecret.ReplaceAllString(s, "$1***")
	s = reHeaderSecret.ReplaceAllString(s, "$1***")
	s = reBearer.ReplaceAllString(s, "$1***")
	s = reEmail.ReplaceAllString(s, "$1***@$2")
	return s
}

func redactURL(raw string) string { return Redact(raw) }

// redactProxy hides the password of a proxy URL (host:port:user:pass is
// impossible to redact safely, so only the URL form is supported here).
func redactProxy(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Redact(raw)
	}
	if u.User != nil {
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(u.User.Username(), "redacted")
		}
	}
	return u.String()
}

// ---------------------------------------------------------------------------
// Run logger
// ---------------------------------------------------------------------------

// LogOptions configures a RunLogger.
type LogOptions struct {
	DataDir   string
	MinLevel  LogLevel
	ToFile    bool
	FileName  string // relative to DataDir, or absolute; default logs/run-<id>.log
	Console   io.Writer
	OnLine    func(level LogLevel, line string) // e.g. the dashboard log ring
	KeepLines int                               // in-memory tail kept for dumps
}

// RunLogger writes the same line to an optional file, an optional console
// writer and an optional callback (the dashboard/service log). It also keeps a
// small in-memory tail that failure dumps embed.
type RunLogger struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	min     LogLevel
	console io.Writer
	onLine  func(LogLevel, string)
	ring    []string
	keep    int
	counts  map[LogLevel]int
	closed  bool
}

// NewRunLogger creates a logger. A nil/disabled logger is not needed: pass
// ToFile=false and leave Console/OnLine nil to get a silent logger.
func NewRunLogger(opts LogOptions) (*RunLogger, error) {
	if opts.KeepLines <= 0 {
		opts.KeepLines = 400
	}
	l := &RunLogger{
		min:     opts.MinLevel,
		console: opts.Console,
		onLine:  opts.OnLine,
		keep:    opts.KeepLines,
		counts:  map[LogLevel]int{},
	}
	if !opts.ToFile {
		return l, nil
	}
	dir := opts.DataDir
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join("crushfling", "data")
	}
	name := strings.TrimSpace(opts.FileName)
	if name == "" {
		name = filepath.Join("logs", "run-"+NewRunID()+".log")
	}
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("log folder: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	l.file = f
	l.path = path
	return l, nil
}

// Logf writes one line if the level passes the configured minimum.
func (l *RunLogger) Logf(level LogLevel, format string, args ...any) {
	if l == nil {
		return
	}
	msg := Redact(fmt.Sprintf(format, args...))
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || level < l.min {
		return
	}
	stamp := time.Now().Format("2006-01-02 15:04:05.000")
	line := fmt.Sprintf("%s [%s] %s", stamp, level.String(), msg)
	if l.file != nil {
		_, _ = l.file.WriteString(line + "\n")
	}
	if l.console != nil {
		_, _ = io.WriteString(l.console, line+"\n")
	}
	plain := level.prefix() + ": " + msg
	l.counts[level]++
	l.ring = append(l.ring, plain)
	if len(l.ring) > l.keep {
		l.ring = l.ring[len(l.ring)-l.keep:]
	}
	if l.onLine != nil {
		l.onLine(level, plain)
	}
}

func (l *RunLogger) Debugf(format string, args ...any) { l.Logf(LevelDebug, format, args...) }
func (l *RunLogger) Infof(format string, args ...any)  { l.Logf(LevelInfo, format, args...) }
func (l *RunLogger) Warnf(format string, args ...any)  { l.Logf(LevelWarn, format, args...) }
func (l *RunLogger) Errorf(format string, args ...any) { l.Logf(LevelError, format, args...) }

// Enabled reports whether a level would be written (ignoring the closed flag).
func (l *RunLogger) Enabled(level LogLevel) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.closed && level >= l.min
}

// Path returns the log file path, or "" when file logging is disabled.
func (l *RunLogger) Path() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// Tail returns at most n buffered lines, oldest first.
func (l *RunLogger) Tail(n int) []string {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if n <= 0 || n > len(l.ring) {
		n = len(l.ring)
	}
	out := make([]string, n)
	copy(out, l.ring[len(l.ring)-n:])
	return out
}

// Counts returns how many lines were written per level.
func (l *RunLogger) Counts() map[string]int {
	if l == nil {
		return map[string]int{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[string]int{}
	for level, n := range l.counts {
		out[level.String()] = n
	}
	return out
}

// Close flushes and closes the log file; it is safe to call twice.
func (l *RunLogger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

// NewRunID returns a filename-safe identifier for one run.
func NewRunID() string {
	now := time.Now()
	return fmt.Sprintf("%s-%03d", now.Format("20060102-150405"), now.Nanosecond()/1e6)
}

// ---------------------------------------------------------------------------
// Error codes
// ---------------------------------------------------------------------------

// Stable error codes make failures filterable in the log, the dashboard and
// the JSON dumps. Keep them short and machine friendly.
const (
	CodeCanceled     = "CANCELED"
	CodePanic        = "PANIC"
	CodeConfig       = "CONFIG"
	CodeInbox        = "INBOX"
	CodeCSRF         = "CSRF"
	CodeMagicRequest = "MAGIC_REQUEST"
	CodeMailPoll     = "MAIL_POLL"
	CodeBadLink      = "BAD_LINK"
	CodeMagicClick   = "MAGIC_CLICK"
	CodeVerify       = "VERIFY"
	CodeUpload       = "UPLOAD"
	CodeProfile      = "PROFILE"
	CodeSave         = "SAVE"
)

// StepError attaches the step name and a stable code to an underlying error.
type StepError struct {
	Step string
	Code string
	Err  error
}

func (e *StepError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return fmt.Sprintf("%s (%s)", e.Step, e.Code)
	}
	return fmt.Sprintf("%s (%s): %v", e.Step, e.Code, e.Err)
}

func (e *StepError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newStepError(step, code string, err error) *StepError {
	return &StepError{Step: step, Code: code, Err: err}
}

// AsStepError extracts step/code from an error chain.
func AsStepError(err error) (step, code string, ok bool) {
	var se *StepError
	if errors.As(err, &se) {
		return se.Step, se.Code, true
	}
	return "", "", false
}

// failResult builds an AccountResult with a step/code-tagged error. Keeping
// this in one place is what makes the log and the dashboard counters agree.
func failResult(profile ProfileData, step, code string, err error, proxyURL string, timings map[string]int64) *AccountResult {
	return &AccountResult{
		ProfileData: profile,
		Error:       newStepError(step, code, err).Error(),
		FailedStep:  step,
		ErrorCode:   code,
		ProxyUsed:   proxyURL,
		Timings:     timings,
	}
}

// ---------------------------------------------------------------------------
// Failure dump
// ---------------------------------------------------------------------------

// FailureDump is written to <data>/debug/fail-*.json when cfg.Debug is on.
// It deliberately excludes cookies and localStorage: those authenticate the
// account and must never land in a support file.
type FailureDump struct {
	Time       string           `json:"time"`
	RunID      string           `json:"run_id,omitempty"`
	Index      int              `json:"index"`
	Email      string           `json:"email,omitempty"`
	Step       string           `json:"step,omitempty"`
	Code       string           `json:"code,omitempty"`
	Error      string           `json:"error,omitempty"`
	Verified   bool             `json:"verified"`
	DurationMS int64            `json:"duration_ms,omitempty"`
	Timings    map[string]int64 `json:"timings,omitempty"`
	Profile    ProfileData      `json:"profile"`
	MagicLink  string           `json:"magic_link,omitempty"`
	ProxyUsed  string           `json:"proxy_used,omitempty"`
	Endpoints  EndpointConfig   `json:"endpoints"`
	HTTP       []HTTPRecord     `json:"http,omitempty"`
	LogTail    []string         `json:"log_tail,omitempty"`
}

// WriteFailureDump serialises one failure to a redacted JSON file and returns
// its path. The caller decides whether the run config enables dumping.
func (r *Register) WriteFailureDump(idx int, res *AccountResult) (string, error) {
	if r == nil || res == nil {
		return "", fmt.Errorf("nothing to dump")
	}
	dir := r.debugDir
	if dir == "" {
		dir = filepath.Join(r.cfg.DataDir, "debug")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dump := FailureDump{
		Time:       time.Now().UTC().Format(time.RFC3339),
		RunID:      r.runID,
		Index:      idx,
		Email:      res.Email,
		Step:       res.FailedStep,
		Code:       res.ErrorCode,
		Error:      res.Error,
		Verified:   res.Verified,
		DurationMS: res.DurationMS,
		Timings:    res.Timings,
		Profile:    res.ProfileData,
		MagicLink:  res.MagicLink,
		ProxyUsed:  redactProxy(res.ProxyUsed),
		Endpoints:  r.endpoints,
	}
	if r.tracer != nil {
		dump.HTTP = r.tracer.Records()
	}
	if r.runLog != nil {
		dump.LogTail = r.runLog.Tail(120)
	}
	b, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("fail-%s-%02d.json", time.Now().Format("20060102-150405"), idx))
	if err := os.WriteFile(path, []byte(Redact(string(b))), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// reportFailure logs the classification and (in debug mode) writes a dump.
func (r *Register) reportFailure(idx int, res *AccountResult) {
	if r == nil || res == nil {
		return
	}
	step, code := res.FailedStep, res.ErrorCode
	if code == "" {
		code = "UNKNOWN"
	}
	if step == "" {
		step = "unknown"
	}
	if res.Verified {
		// Authenticated but the profile step failed: the account is saved, so
		// calling it FAILED would contradict the "success" counter.
		r.log("warn", "[%d] INCOMPLETE step=%s code=%s error=%s (session valid, account saved)", idx, step, code, res.Error)
	} else {
		r.log("warn", "[%d] FAILED step=%s code=%s error=%s", idx, step, code, res.Error)
	}
	if r.tracer != nil {
		records := r.tracer.Records()
		failed := 0
		for _, rec := range records {
			if rec.Status == 0 || rec.Status >= 400 {
				failed++
			}
		}
		r.log("debug", "[%d] http trace: %d requests, %d failed", idx, len(records), failed)
	}
	if !r.cfg.Debug {
		return
	}
	path, err := r.WriteFailureDump(idx, res)
	if err != nil {
		r.log("warn", "[%d] debug dump failed: %v", idx, err)
		return
	}
	r.log("info", "[%d] debug dump: %s", idx, path)
}
