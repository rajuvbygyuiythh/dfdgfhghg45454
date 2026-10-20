package email

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// OneSecMailProvider uses 1secmail.com API (no auth, simple)
type OneSecMailProvider struct {
	client *http.Client
}

func NewOneSecMailProvider() *OneSecMailProvider {
	return &OneSecMailProvider{client: &http.Client{Timeout: 15 * time.Second}}
}
func (p *OneSecMailProvider) Name() string { return "1secmail" }

func (p *OneSecMailProvider) CreateInbox(ctx context.Context) (Inbox, error) {
	// 1secmail domain list: 1secmail.com, 1secmail.net, 1secmail.org
	domains := []string{"1secmail.com", "1secmail.net", "1secmail.org"}
	domain := domains[rand.Intn(len(domains))]
	local := randomLocalPartOneSec()
	email := local + "@" + domain
	return &oneSecMailInbox{email: email, login: local, domain: domain, client: p.client}, nil
}
func (p *OneSecMailProvider) CreateInboxWithName(ctx context.Context, localPart string) (Inbox, error) {
	domains := []string{"1secmail.com", "1secmail.net", "1secmail.org"}
	domain := domains[rand.Intn(len(domains))]
	email := localPart + "@" + domain
	return &oneSecMailInbox{email: email, login: localPart, domain: domain, client: p.client}, nil
}

type oneSecMailInbox struct {
	email  string
	login  string
	domain string
	client *http.Client
}

func (m *oneSecMailInbox) Email() string { return m.email }

func (m *oneSecMailInbox) GetMessages(ctx context.Context) ([]Message, error) {
	url := fmt.Sprintf("https://www.1secmail.com/api/v1/?action=getMessages&login=%s&domain=%s", m.login, m.domain)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("1secmail messages %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var msgs []struct {
		ID      int    `json:"id"`
		From    string `json:"from"`
		Subject string `json:"subject"`
		Date    string `json:"date"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal(body, &msgs); err != nil {
		return nil, err
	}
	result := make([]Message, 0, len(msgs))
	for _, mm := range msgs {
		// Fetch full body if needed
		fullBody := mm.Body
		if fullBody == "" {
			fullURL := fmt.Sprintf("https://www.1secmail.com/api/v1/?action=readMessage&login=%s&domain=%s&id=%d", m.login, m.domain, mm.ID)
			req2, _ := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
			if resp2, err := m.client.Do(req2); err == nil {
				b2, _ := io.ReadAll(resp2.Body)
				resp2.Body.Close()
				var full struct {
					Body     string `json:"body"`
					HTMLBody string `json:"htmlBody"`
					TextBody string `json:"textBody"`
				}
				if json.Unmarshal(b2, &full) == nil {
					if full.HTMLBody != "" {
						fullBody = full.HTMLBody
					} else if full.TextBody != "" {
						fullBody = full.TextBody
					} else if full.Body != "" {
						fullBody = full.Body
					}
				}
			}
		}
		date, _ := time.Parse("2006-01-02 15:04:05", mm.Date)
		result = append(result, Message{
			ID:      fmt.Sprint(mm.ID),
			From:    mm.From,
			Subject: mm.Subject,
			Body:    fullBody,
			Date:    date,
		})
	}
	return result, nil
}

func (m *oneSecMailInbox) WaitForMagicLink(ctx context.Context, pollInterval, timeout time.Duration) (string, error) {
	return WaitHelper(ctx, m, pollInterval, timeout)
}

func randomLocalPartOneSec() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 9)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	// Ensure not starting with number for safety
	if b[0] >= '0' && b[0] <= '9' {
		b[0] = letters[rand.Intn(26)]
	}
	return string(b)
}

// GuerrillaProvider uses guerrillamail.com
type GuerrillaProvider struct {
	client *http.Client
}

func NewGuerrillaProvider() *GuerrillaProvider {
	return &GuerrillaProvider{client: &http.Client{Timeout: 15 * time.Second}}
}
func (p *GuerrillaProvider) Name() string { return "guerrillamail" }

func (p *GuerrillaProvider) CreateInbox(ctx context.Context) (Inbox, error) {
	// Guerrilla API: https://api.guerrillamail.com/ajax.php?f=get_email_address
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.guerrillamail.com/ajax.php?f=get_email_address&lang=en", nil)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("guerrillamail create %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var data struct {
		EmailAddr      string `json:"email_addr"`
		EmailTimestamp int64  `json:"email_timestamp"`
		SIDToken       string `json:"sid_token"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if data.EmailAddr == "" {
		// Never invent an address: a made-up inbox silently times out later,
		// which looks like "mail never arrived" instead of "provider broken".
		return nil, fmt.Errorf("guerrillamail returned no address: %s", strings.TrimSpace(string(body)))
	}
	if data.SIDToken == "" {
		return nil, fmt.Errorf("guerrillamail returned no sid_token")
	}
	return &guerrillaInbox{email: data.EmailAddr, sid: data.SIDToken, client: p.client}, nil
}

func (p *GuerrillaProvider) CreateInboxWithName(ctx context.Context, localPart string) (Inbox, error) {
	// Guerrilla doesn't support custom name easily, fallback to random
	return p.CreateInbox(ctx)
}

type guerrillaInbox struct {
	email  string
	sid    string
	client *http.Client
}

func (m *guerrillaInbox) Email() string { return m.email }

func (m *guerrillaInbox) GetMessages(ctx context.Context) ([]Message, error) {
	// Guerrilla wants seq=0 for the newest page.
	url := fmt.Sprintf("https://api.guerrillamail.com/ajax.php?f=check_email&seq=0&sid_token=%s", m.sid)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	// Guerrilla wants IP set via header? Use simple
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("guerrillamail messages %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var data struct {
		List []struct {
			MailID      string `json:"mail_id"`
			MailFrom    string `json:"mail_from"`
			MailSubject string `json:"mail_subject"`
			MailBody    string `json:"mail_body"`
			MailDate    string `json:"mail_date"`
		} `json:"list"`
		Count string `json:"count"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	result := make([]Message, 0, len(data.List))
	for _, it := range data.List {
		date, _ := time.Parse("2006-01-02 15:04:05", it.MailDate)
		result = append(result, Message{
			ID:      it.MailID,
			From:    it.MailFrom,
			Subject: it.MailSubject,
			Body:    it.MailBody,
			Date:    date,
		})
	}
	return result, nil
}

func (m *guerrillaInbox) WaitForMagicLink(ctx context.Context, pollInterval, timeout time.Duration) (string, error) {
	return WaitHelper(ctx, m, pollInterval, timeout)
}

// CustomInbox is for a catch-all / personal address. Two ways to use it:
//  1. --manual-link / --link: the operator reads the mail and pastes the link,
//     so no mail API is needed at all.
//  2. HookURL: a small HTTP endpoint you host that returns the messages for
//     ?email=<address> (JSON []Message or a plain body containing the link).
type CustomInbox struct {
	email   string
	hookURL string // optional webhook where magic link will be POSTed
	client  *http.Client
}

func NewCustomInbox(email string) *CustomInbox {
	return NewCustomInboxWithHook(email, "")
}

// NewCustomInboxWithHook additionally polls hookURL?email=<address>.
func NewCustomInboxWithHook(email, hookURL string) *CustomInbox {
	return &CustomInbox{email: email, hookURL: strings.TrimSpace(hookURL), client: &http.Client{Timeout: 15 * time.Second}}
}
func (c *CustomInbox) Email() string { return c.email }
func (c *CustomInbox) GetMessages(ctx context.Context) ([]Message, error) {
	// If hookURL provided, poll it
	if c.hookURL != "" {
		req, _ := http.NewRequestWithContext(ctx, "GET", c.hookURL+"?email="+c.email, nil)
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var msgs []Message
		if json.Unmarshal(b, &msgs) == nil {
			return msgs, nil
		}
		// Plain text magic link?
		s := strings.TrimSpace(string(b))
		if strings.Contains(strings.ToLower(s), strings.ToLower(SiteHost)) {
			return []Message{{ID: "1", Body: s, From: "crushfling", Subject: "magic"}}, nil
		}
	}
	return nil, fmt.Errorf("custom inbox requires external mail fetcher - configure hookURL")
}
func (c *CustomInbox) WaitForMagicLink(ctx context.Context, pollInterval, timeout time.Duration) (string, error) {
	return WaitHelper(ctx, c, pollInterval, timeout)
}
