package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

const mailTmBase = "https://api.mail.tm"

// MailTmProvider uses mail.tm API (free, no key needed)
type MailTmProvider struct {
	client *http.Client
	domain string
	token  string
}

func NewMailTmProvider() *MailTmProvider {
	return &MailTmProvider{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *MailTmProvider) Name() string { return "mail.tm" }

func (p *MailTmProvider) fetchDomains(ctx context.Context) (string, error) {
	if p.domain != "" {
		return p.domain, nil
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", mailTmBase+"/domains", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		HydraMember []struct {
			Domain string `json:"domain"`
		} `json:"hydra:member"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}
	if len(data.HydraMember) == 0 {
		return "", fmt.Errorf("no mail.tm domains available")
	}
	// Pick random domain
	idx := rand.Intn(len(data.HydraMember))
	p.domain = data.HydraMember[idx].Domain
	debugf("mail.tm: using domain %s (%d available)", p.domain, len(data.HydraMember))
	return p.domain, nil
}

func (p *MailTmProvider) CreateInbox(ctx context.Context) (Inbox, error) {
	domain, err := p.fetchDomains(ctx)
	if err != nil {
		return nil, err
	}
	local := randomLocalPart()
	email := local + "@" + domain
	password := randomPassword()
	return p.createAccount(ctx, email, password)
}

func (p *MailTmProvider) CreateInboxWithName(ctx context.Context, localPart string) (Inbox, error) {
	domain, err := p.fetchDomains(ctx)
	if err != nil {
		return nil, err
	}
	email := localPart + "@" + domain
	password := randomPassword()
	return p.createAccount(ctx, email, password)
}

func (p *MailTmProvider) createAccount(ctx context.Context, email, password string) (Inbox, error) {
	// Create account
	payload := map[string]string{"address": email, "password": password}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", mailTmBase+"/accounts", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		return nil, fmt.Errorf("mail.tm create failed %d: %s", resp.StatusCode, string(body))
	}
	// Get token
	tokenPayload := map[string]string{"address": email, "password": password}
	td, _ := json.Marshal(tokenPayload)
	req2, _ := http.NewRequestWithContext(ctx, "POST", mailTmBase+"/token", bytes.NewReader(td))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := p.client.Do(req2)
	if err != nil {
		return nil, err
	}
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		return nil, fmt.Errorf("mail.tm token failed %d: %s", resp2.StatusCode, string(b2))
	}
	var tokResp struct {
		Token string `json:"token"`
		ID    string `json:"id"`
	}
	json.Unmarshal(b2, &tokResp)
	if tokResp.Token == "" {
		return nil, fmt.Errorf("mail.tm no token returned")
	}
	return &mailTmInbox{
		email:    email,
		token:    tokResp.Token,
		client:   p.client,
		provider: p,
	}, nil
}

type mailTmInbox struct {
	email    string
	token    string
	client   *http.Client
	provider *MailTmProvider
}

func (m *mailTmInbox) Email() string { return m.email }

func (m *mailTmInbox) GetMessages(ctx context.Context) ([]Message, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", mailTmBase+"/messages?page=1", nil)
	req.Header.Set("Authorization", "Bearer "+m.token)
	req.Header.Set("Accept", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("mail.tm messages %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var data struct {
		HydraMember []struct {
			ID   string `json:"id"`
			From struct {
				Address string `json:"address"`
			} `json:"from"`
			Subject   string `json:"subject"`
			Intro     string `json:"intro"`
			Seen      bool   `json:"seen"`
			CreatedAt string `json:"createdAt"`
		} `json:"hydra:member"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	result := make([]Message, 0, len(data.HydraMember))
	for _, member := range data.HydraMember {
		// Fetch full message for body
		fullBody := member.Intro
		// Try to get full content
		req2, _ := http.NewRequestWithContext(ctx, "GET", mailTmBase+"/messages/"+member.ID, nil)
		req2.Header.Set("Authorization", "Bearer "+m.token)
		resp2, err := m.client.Do(req2)
		if err == nil {
			b2, _ := io.ReadAll(resp2.Body)
			resp2.Body.Close()
			var full struct {
				HTML string `json:"html"`
				Text string `json:"text"`
				Body string `json:"body"`
			}
			if json.Unmarshal(b2, &full) == nil {
				if full.HTML != "" {
					fullBody = full.HTML
				} else if full.Text != "" {
					fullBody = full.Text
				} else if full.Body != "" {
					fullBody = full.Body
				}
			}
		}
		date, _ := time.Parse(time.RFC3339, member.CreatedAt)
		result = append(result, Message{
			ID:      member.ID,
			From:    member.From.Address,
			Subject: member.Subject,
			Body:    fullBody,
			Date:    date,
		})
	}
	return result, nil
}

func (m *mailTmInbox) WaitForMagicLink(ctx context.Context, pollInterval, timeout time.Duration) (string, error) {
	return WaitHelper(ctx, m, pollInterval, timeout)
}

func randomLocalPart() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 10)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
func randomPassword() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#"
	b := make([]byte, 12)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
