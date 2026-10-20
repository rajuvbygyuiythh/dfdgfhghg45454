package email

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// DebugLog is an optional hook installed by the crushfling package so poll
// diagnostics (status errors, message counts) reach the run log. It must not
// receive bodies: they contain magic-link tokens.
var DebugLog func(format string, args ...any)

// SiteHost is the host a sign-in link must point at. It defaults to the live
// site and is updated by crushfling.SetBaseURL, so the normal inbox path also
// works against a mock or a mirror (otherwise only --manual-link could).
var SiteHost = "crushfling.com"

// SetSiteHost overrides SiteHost. Empty input restores the default.
func SetSiteHost(host string) {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	if i := strings.IndexAny(host, ":/"); i >= 0 {
		host = host[:i]
	}
	if host == "" {
		SiteHost = "crushfling.com"
		return
	}
	SiteHost = host
}

func debugf(format string, args ...any) {
	if DebugLog != nil {
		DebugLog(format, args...)
	}
}

// Provider interface for disposable email services
type Provider interface {
	Name() string
	// CreateInbox creates a new random inbox and returns the email address
	CreateInbox(ctx context.Context) (Inbox, error)
	// CreateInboxWithName creates inbox with specific local part (for 1secmail etc)
	CreateInboxWithName(ctx context.Context, localPart string) (Inbox, error)
}

type Inbox interface {
	Email() string
	// WaitForMagicLink polls inbox for crushfling magic link
	WaitForMagicLink(ctx context.Context, pollInterval, timeout time.Duration) (string, error)
	// GetMessages returns raw messages for debugging
	GetMessages(ctx context.Context) ([]Message, error)
}

type Message struct {
	ID      string
	From    string
	Subject string
	Body    string
	Date    time.Time
}

// ExtractMagicLink finds the crushfling sign-in link in an email body.
//
// Real HTML mail escapes the query separator as &amp; and can also contain
// unsubscribe/social links, so the body is normalised first and the URLs are
// scored instead of taking the first match.
func ExtractMagicLink(body string) (string, error) {
	body = strings.ReplaceAll(body, "&amp;amp;", "&amp;")
	body = strings.ReplaceAll(body, "&amp;", "&")
	body = strings.ReplaceAll(body, "&#38;", "&")
	body = strings.ReplaceAll(body, `\/`, `/`)

	urls := urlPattern.FindAllString(body, -1)
	best, bestScore := "", 0
	for _, raw := range urls {
		candidate := raw
		for _, stop := range []string{"&nbsp;", "&lt;", "&gt;", "&quot;", "&#"} {
			if i := strings.Index(candidate, stop); i > 0 {
				candidate = candidate[:i]
			}
		}
		candidate = strings.TrimRight(candidate, ".,;:!?)]}>\"'")
		if !isCrushflingURL(candidate) {
			continue
		}
		score := scoreMagicLink(candidate)
		if score > bestScore {
			best, bestScore = candidate, score
		}
	}
	if best != "" {
		return best, nil
	}
	return "", fmt.Errorf("no crushfling sign-in link found in the email body")
}

var urlPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

func isCrushflingURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	base := strings.ToLower(SiteHost)
	return host == base || strings.HasSuffix(host, "."+base)
}

func scoreMagicLink(raw string) int {
	lower := strings.ToLower(raw)
	score := 1 // being a crushfling URL is already a signal
	if strings.Contains(lower, "token=") {
		score += 6
	}
	for _, marker := range []string{"verify", "callback", "magic", "signin", "sign-in", "auth/"} {
		if strings.Contains(lower, marker) {
			score += 3
			break
		}
	}
	for _, noise := range []string{"unsubscribe", "preferences", "privacy", "terms", "help", "support"} {
		if strings.Contains(lower, noise) {
			score -= 10
		}
	}
	return score
}

// WaitHelper polls inbox until magic link found or timeout
func WaitHelper(ctx context.Context, inbox Inbox, pollInterval, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	polls := 0
	messages := 0
	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timeout waiting for magic link after %v (%d polls, %d messages)", timeout, polls, messages)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		polls++
		msgs, err := inbox.GetMessages(ctx)
		if err != nil {
			// transient, continue polling - but say why, otherwise a broken
			// provider looks identical to a slow email delivery.
			debugf("inbox poll error (%d): %v", polls, err)
		} else {
			messages += len(msgs)
			if len(msgs) == 0 {
				debugf("inbox poll %d: no messages yet", polls)
			}
			for _, m := range msgs {
				// Look for crushfling sender
				if strings.Contains(strings.ToLower(m.From), "crushfling") ||
					strings.Contains(strings.ToLower(m.Subject), "crushfling") ||
					strings.Contains(strings.ToLower(m.Subject), "magic") ||
					strings.Contains(strings.ToLower(m.Subject), "sign in") ||
					strings.Contains(strings.ToLower(m.Body), "crushfling") {

					link, err := ExtractMagicLink(m.Body)
					if err == nil {
						debugf("magic link found in message %q", m.Subject)
						return link, nil
					}
					// Also try subject
					link, err = ExtractMagicLink(m.Subject)
					if err == nil {
						debugf("magic link found in the subject %q", m.Subject)
						return link, nil
					}
					debugf("message %q from %q has no magic link: %v", m.Subject, m.From, err)
				} else {
					// Even if sender not matching, try to extract link anyway (some forwarders)
					if link, err := ExtractMagicLink(m.Body); err == nil && strings.Contains(link, "crushfling") {
						return link, nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
