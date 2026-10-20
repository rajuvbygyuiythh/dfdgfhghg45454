package crushfling

import (
	"context"
	"fmt"
)

// CaptchaSolver interface for Turnstile / hCaptcha
type CaptchaSolver interface {
	SolveTurnstile(ctx context.Context, siteKey, pageURL string) (string, error)
}

type NoopSolver struct{}

func (n *NoopSolver) SolveTurnstile(ctx context.Context, siteKey, pageURL string) (string, error) {
	return "", fmt.Errorf("captcha required but no solver configured - set captcha_api_key or use browser mode")
}

// TwoCaptchaSolver uses 2captcha API for Cloudflare Turnstile
type TwoCaptchaSolver struct {
	APIKey string
}

func (t *TwoCaptchaSolver) SolveTurnstile(ctx context.Context, siteKey, pageURL string) (string, error) {
	// Placeholder: integrate https://2captcha.com/api
	// POST http://2captcha.com/in.php?method=turnstile&sitekey=xxx&pageurl=xxx
	// Poll http://2captcha.com/res.php?key=xxx&action=get&id=yyy
	return "", fmt.Errorf("2captcha solver not yet wired - implement via internal/captcha package")
}

func NewCaptchaSolver(apiKey string) CaptchaSolver {
	if apiKey == "" {
		return &NoopSolver{}
	}
	return &TwoCaptchaSolver{APIKey: apiKey}
}
