package email

import (
	"context"
	"fmt"
	"strings"
)

// Factory creates provider by name
func NewProvider(name string) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "mailtm", "mail.tm", "mail_tm":
		return NewMailTmProvider(), nil
	case "1secmail", "1sec", "onec":
		return NewOneSecMailProvider(), nil
	case "guerrilla", "guerrillamail", "guerrilla_mail":
		return NewGuerrillaProvider(), nil
	case "custom":
		// Requires --custom-email; pair it with --manual-link/--link unless you
		// host a mail hook (see CustomInbox).
		return &customProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown email provider %q: choose mailtm, 1secmail, guerrilla, custom", name)
	}
}

type customProvider struct{}

func (c *customProvider) Name() string { return "custom" }
func (c *customProvider) CreateInbox(ctx context.Context) (Inbox, error) {
	return nil, fmt.Errorf("custom provider requires existing email - use CreateInboxWithName with your catch-all address local part")
}
func (c *customProvider) CreateInboxWithName(ctx context.Context, localPart string) (Inbox, error) {
	// Expect localPart to be full email
	if strings.Contains(localPart, "@") {
		return NewCustomInbox(localPart), nil
	}
	return nil, fmt.Errorf("custom provider requires full email address")
}

// CreateInboxWithProvider helper for callers that already have provider
func CreateInboxWithProvider(ctx context.Context, providerName, customEmail string) (Inbox, error) {
	if strings.TrimSpace(customEmail) != "" {
		// Use custom email directly - no need to create
		return NewCustomInbox(strings.TrimSpace(customEmail)), nil
	}
	prov, err := NewProvider(providerName)
	if err != nil {
		return nil, err
	}
	return prov.CreateInbox(ctx)
}
