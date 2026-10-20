package crushfling

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	emailprov "chitchatbot/internal/crushfling/email"
)

// Site constants
const (
	DefaultBaseURL = "https://crushfling.com"
	APIBaseURL     = "https://crushfling.com/api"
	DefaultUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

// BaseURL / AccountURL are variables, not constants, so the same build can be
// pointed at a local mock server (CLI --base-url) to prove the whole flow
// without touching the live site. They stay at the production values unless
// SetBaseURL is called.
var (
	BaseURL    = DefaultBaseURL
	AccountURL = DefaultBaseURL + "/account"
)

// SetBaseURL overrides the site origin. It rejects a path/query because every
// endpoint in this package is resolved relative to a bare origin.
func SetBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty base url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("base url must start with http:// or https://")
	}
	if u.Host == "" {
		return fmt.Errorf("base url has no host")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("base url must not carry a query or fragment")
	}
	BaseURL = strings.TrimRight(u.Scheme+"://"+u.Host+u.Path, "/")
	AccountURL = BaseURL + "/account"
	// The mail extractor must accept links from this origin too, otherwise a
	// mock/mirror only works with --manual-link.
	emailprov.SetSiteHost(u.Hostname())
	return nil
}

// Gender options as seen on site
const (
	GenderWoman  = "woman"
	GenderMan    = "man"
	GenderTrans  = "trans"
	GenderCouple = "couple"
)

// LookingFor filters
const (
	LookingForAll      = "all"
	LookingForGirls    = "girls"
	LookingForGuys     = "guys"
	LookingForGays     = "gays"
	LookingForLesbians = "lesbians"
	LookingForTrans    = "trans"
)

// Platform options
const (
	PlatformDiscord   = "discord"
	PlatformInstagram = "instagram"
	PlatformKik       = "kik"
	PlatformSnapchat  = "snapchat"
	PlatformTelegram  = "telegram"
	PlatformWhatsapp  = "whatsapp"
)

// RegisterConfig holds all tunables for one registration run
type RegisterConfig struct {
	DataDir         string `json:"data_dir"`
	Count           int    `json:"count"`
	Concurrency     int    `json:"concurrency"`
	Headless        bool   `json:"headless"`
	UseBrowser      bool   `json:"use_browser"` // if true, use Rod fallback for magic-link request
	ProxyFile       string `json:"proxy_file"`
	ProxyEntries    string `json:"proxy_entries"`     // raw proxy lines
	EmailProvider   string `json:"email_provider"`    // mailtm | 1secmail | guerrilla | custom
	CustomEmail     string `json:"custom_email"`      // if using fixed domain (catch-all)
	CustomEmailHook string `json:"custom_email_hook"` // optional HTTP endpoint that returns messages for ?email=
	MaxRetries      int    `json:"max_retries"`
	MailPollSec     int    `json:"mail_poll_sec"`
	MailTimeoutSec  int    `json:"mail_timeout_sec"`
	AvatarDir       string `json:"avatar_dir"`
	MinDelayMs      int    `json:"min_delay_ms"`
	MaxDelayMs      int    `json:"max_delay_ms"`
	DisableCFBypass bool   `json:"disable_cf_bypass"`
	CaptchaAPIKey   string `json:"captcha_api_key"`
	OutputFile      string `json:"output_file"`
	ManualLink      bool   `json:"manual_link"`    // ask the operator to paste the link
	PastedLink      string `json:"pasted_link"`    // one already-received link (single account)
	BrowserVerify   bool   `json:"browser_verify"` // click "Sign In" in Rod if the link sets no cookie

	// Debug enables debug-level lines, per-failure JSON dumps and the HTTP
	// metadata trace. TraceHTTP additionally stores request/response bodies
	// in debug/trace-*.jsonl (local, truncated and redacted).
	Debug     bool `json:"debug"`
	TraceHTTP bool `json:"trace_http"`
}

// ProfileData is what we fill after account creation
type ProfileData struct {
	Name             string    `json:"name,omitempty"` // display name ("Emma Rose")
	Username         string    `json:"username"`
	Email            string    `json:"email"`
	Password         string    `json:"password,omitempty"` // not used for magic-link but stored
	Age              int       `json:"age"`
	Gender           string    `json:"gender"`
	LookingFor       string    `json:"looking_for"`
	Country          string    `json:"country"` // ISO2 e.g. US
	City             string    `json:"city,omitempty"`
	Platform         string    `json:"platform"`
	PlatformUsername string    `json:"platform_username"`
	Bio              string    `json:"bio"`
	AvatarPath       string    `json:"avatar_path,omitempty"` // local file that was uploaded
	AvatarURL        string    `json:"avatar_url,omitempty"`  // URL the site returned for it
	Tags             []string  `json:"tags,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// AccountResult stores the saved session after successful registration
type AccountResult struct {
	ProfileData
	MagicLink      string            `json:"magic_link"`
	SessionCookies map[string]string `json:"session_cookies"`
	LocalStorage   map[string]string `json:"local_storage,omitempty"`
	UserID         string            `json:"user_id,omitempty"`
	Verified       bool              `json:"verified"`
	Error          string            `json:"error,omitempty"`
	ProxyUsed      string            `json:"proxy_used,omitempty"`
	RawResponse    string            `json:"raw_response,omitempty"`

	// Diagnostics: which step failed, a stable error code, how long the
	// account took and the per-step timings (milliseconds).
	ErrorCode  string           `json:"error_code,omitempty"`
	FailedStep string           `json:"failed_step,omitempty"`
	DurationMS int64            `json:"duration_ms,omitempty"`
	Timings    map[string]int64 `json:"timings,omitempty"`

	// ProfileCompleted is true only when the onboarding/profile call
	// succeeded; ProfileError keeps its redacted error text otherwise.
	ProfileCompleted bool   `json:"profile_completed"`
	ProfileVerified  bool   `json:"profile_verified,omitempty"`
	ProfileError     string `json:"profile_error,omitempty"`
}

// Registration stats for dashboard
type RegistrationStats struct {
	Total     int            `json:"total"`
	Success   int            `json:"success"`
	Failed    int            `json:"failed"`
	Pending   int            `json:"pending"`
	Running   bool           `json:"running"`
	LastError string         `json:"last_error,omitempty"`
	ByCode    map[string]int `json:"by_code,omitempty"`
}

func DefaultRegisterConfig(dataDir string) RegisterConfig {
	return RegisterConfig{
		DataDir:        dataDir,
		Count:          1,
		Concurrency:    1,
		Headless:       false,
		UseBrowser:     false,
		BrowserVerify:  true,
		EmailProvider:  "mailtm",
		MaxRetries:     3,
		MailPollSec:    3,
		MailTimeoutSec: 120,
		MinDelayMs:     1500,
		MaxDelayMs:     3500,
		OutputFile:     "crushfling_accounts.json",
	}
}

// API response models (reverse engineered from /account page)

// MagicLinkRequest is sent to trigger email
type MagicLinkRequest struct {
	Email        string `json:"email"`
	RedirectURL  string `json:"redirectUrl,omitempty"`
	Locale       string `json:"locale,omitempty"`
	CaptchaToken string `json:"captchaToken,omitempty"`
}

// MagicLinkResponse is returned after requesting magic link
type MagicLinkResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
	Token   string `json:"token,omitempty"`
}

// VerifyResponse after following magic link
type VerifyResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token"`
	UserID  string `json:"userId"`
	Error   string `json:"error"`
}

// EndpointConfig lets the operator recalibrate the API paths without a rebuild.
// crushfling/data/api.json is written with these defaults on first run; edit it
// after recording one manual registration with capture/ (see docs).
type EndpointConfig struct {
	// AccountPath is the sign-in page that is parsed before every account.
	AccountPath string `json:"account_path"`
	// CSRFPath is only used as a fallback when the page carries no token; the
	// token normally comes from <meta name="csrf-token"> on AccountPath.
	CSRFPath          string   `json:"csrf_path"`
	MagicLinkPath     string   `json:"magic_link_path"`
	MagicLinkAltPaths []string `json:"magic_link_alt_paths"`
	// LivewirePath is the endpoint Livewire posts component updates to.
	LivewirePath string `json:"livewire_path"`
	// UploadPath/ProfilePaths are fallbacks: the page's own paths win.
	UploadPath        string   `json:"upload_path"`
	ProfilePaths      []string `json:"profile_paths"`
	PublicProfilePath string   `json:"public_profile_path"`
	Notes             string   `json:"notes,omitempty"`
}

// DefaultEndpointConfig carries only facts that are true for the live site or
// harmless fallbacks. The sign-in page is a Laravel page (meta csrf-token,
// XSRF-TOKEN cookie, /livewire/update); the old NextAuth routes
// (/api/auth/csrf, /api/auth/signin/email) return the site's 404 page and are
// therefore no longer defaults. MagicLinkPath is empty on purpose: the real
// action is discovered from the page (form action or Livewire method).
func DefaultEndpointConfig() EndpointConfig {
	return EndpointConfig{
		AccountPath:       "/account",
		CSRFPath:          "",
		MagicLinkPath:     "",
		MagicLinkAltPaths: nil,
		LivewirePath:      "/livewire/update",
		UploadPath:        "/api/upload",
		ProfilePaths:      []string{"/api/profile", "/api/user/profile", "/api/onboarding"},
		PublicProfilePath: "/p/{username}/",
		Notes:             "Paths are discovered from the account page at run time. Run `crushfling --probe` to print what was detected.",
	}
}

// LegacyNextAuthPaths are the paths that were assumed before the site was
// identified as Laravel. They are kept only so --check can warn about an
// api.json written by an older build.
func LegacyNextAuthPaths() []string {
	return []string{"/api/auth/csrf", "/api/auth/signin/email", "/api/auth/magic-link", "/api/auth/callback/email"}
}
