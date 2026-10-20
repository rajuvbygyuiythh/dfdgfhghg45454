package crushfling

/*
NETWORK NOTES FOR CRUSHFLING.COM
================================
Last reviewed: 2026-10-20. Everything below is either (a) verified with a real
GET from a browser-rendered fetch this session, or (b) explicitly marked
UNVERIFIED. Nothing here is presented as if it had been observed in DevTools.

A. VERIFIED FACTS
-----------------
1. The sign-in page is https://crushfling.com/account and it is a magic-link
   page:
     - "Enter your email to sign in or create an account."
     - after submitting: "Email sent!", "We sent a sign-in link to.",
       "Open it and tap Sign In.", "Wrong email? Try again",
     - also offers "Continue with Google".
   There is no password field.

2. The site is NOT NextAuth. These return the site's own "Page Not Found"
   template (HTTP 404):
     GET /api/auth/csrf
     GET /api/auth/providers
   The old /signup /register /join /auth /login /create /about paths are 404
   as well, so endpoint guessing is not a viable strategy.

3. Public pages are Laravel-rendered:
     - profiles live at /p/<username>/ (verified from the 404 page's profile
       grid, which links to them),
     - avatars are served from s3.crushfling.com/user/avatar/
       <username>-<platform>-sexting-<gender>-<hash>.webp,
     - the archive shows /livewire/livewire.js?id=... and a Vite bundle at
       /build/assets/app-*.js, i.e. the app uses Livewire (Laravel's
       component framework) plus a bundled script.

4. Only two JSON routes appear in the public archive (Wayback CDX):
     GET /api/groups/1/click        (302)
     GET /api/search/suggestions
   They are not part of registration, but they show /api/* exists.

B. WHAT THAT MEANS FOR THE WORKER
---------------------------------
Laravel + Livewire changes three things compared with the original design:

  1. CSRF: the token is in <meta name="csrf-token">, a hidden _token field and
     an XSRF-TOKEN cookie (URL-encoded). Session.SetCSRF wires all three.
  2. Submit: the email is sent either by a plain <form action="..."> POST or by
     a Livewire component call to /livewire/update with a signed
     wire:snapshot. internal/crushfling/discover.go reads the page and picks
     the right one; the method name comes from wire:submit/wire:click.
  3. Session: a successful click sets a Laravel session cookie
     (laravel_session / <app>_session) and optionally remember_web_*. Cookie
     presence is a hint, so the worker also re-reads /account and treats the
     disappearance of the email form as proof of sign-in.

C. UNVERIFIED (must be confirmed by one live run on Windows)
------------------------------------------------------------
  - the exact magic-link landing path (signed URL or token route),
  - the upload endpoint and its field name (discovery + 5 field names are
    tried, then the avatar is skipped rather than burning the picture),
  - the profile/onboarding payload shape (several paths are tried; a 2xx with
    {"success":false} or {"error":...} counts as a failure),
  - whether profile creation happens through JSON, Livewire, or only the
    browser DOM. If JSON fails, the browser fallback remains available.

D. DIAGNOSTICS
--------------
  --probe   prints the discovered style, CSRF presence, form action, Livewire
            method, and the paths the page references. It performs no
            registration, so it is the first command to run on a new machine.
  --debug   adds per-step timings, debug lines and debug/fail-*.json.
  --trace-http records every request/response (bodies redacted, 32 KiB cap).

E. ERROR CASES TO EXPECT
------------------------
  - 419 from Laravel: CSRF token missing/expired -> re-read the page.
  - 429/Too Many Requests: back off; the worker already waits 10 s on 429.
  - Cloudflare interstitial: detected by content, answered only by the Rod
    browser path (a plain 403 JSON from the auth API is NOT a challenge).
  - Expired magic link: request a new one with a fresh inbox; do not retry the
    same link.

This file is documentation + the small response models used by the client.
*/

const (
	// Endpoint priorities - tried in order
	EndpointMagicLinkPrimary = "/api/auth/signin/email" // NextAuth v4 default
	EndpointMagicLinkAlt1    = "/api/auth/magic-link"
	EndpointMagicLinkAlt2    = "/api/auth/sendMagicLink"
	EndpointMagicLinkAlt3    = "/api/trpc/auth.sendMagicLink"
	EndpointCSRF             = "/api/auth/csrf"
	EndpointVerifyCallback   = "/api/auth/callback/email"
	EndpointOnboarding       = "/onboarding"
	EndpointProfileAPI       = "/api/profile"
	EndpointUploadAPI        = "/api/upload"
	EndpointUserAPI          = "/api/user"
)

// CSRFToken holds extracted token
type CSRFToken struct {
	Token     string
	CookieVal string
}

// OnboardingPayload documents the *intended* field names of the profile
// endpoint. It is reference material, not the wire format: the actual request
// is built by profilePayload, which sends both camelCase and snake_case
// spellings because the site's schema has never been observed.
type OnboardingPayload struct {
	Username         string `json:"username"`
	Age              int    `json:"age"`
	Gender           string `json:"gender"`
	LookingFor       string `json:"lookingFor"`
	Country          string `json:"country"`
	City             string `json:"city,omitempty"`
	Platform         string `json:"platform"`
	PlatformUsername string `json:"platformUsername"`
	Bio              string `json:"bio"`
	AvatarURL        string `json:"avatarUrl,omitempty"`
}
