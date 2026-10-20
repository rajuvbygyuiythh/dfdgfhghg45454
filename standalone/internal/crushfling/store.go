package crushfling

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AccountStore persists verified accounts to <data>/accounts.json.
type AccountStore struct {
	path     string
	mu       sync.Mutex
	accounts []AccountResult
}

func NewAccountStore(dataDir string) *AccountStore {
	if dataDir == "" {
		dataDir = filepath.Join("crushfling", "data")
	}
	s := &AccountStore{path: filepath.Join(dataDir, "accounts.json")}
	s.load()
	return s
}

func (s *AccountStore) load() {
	b, err := os.ReadFile(s.path)
	if err != nil {
		s.accounts = []AccountResult{}
		return
	}
	if err := json.Unmarshal(b, &s.accounts); err != nil || s.accounts == nil {
		s.accounts = []AccountResult{}
	}
}

func (s *AccountStore) Add(a AccountResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	s.accounts = append(s.accounts, a)
	return s.saveLocked()
}

func (s *AccountStore) List() []AccountResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AccountResult, len(s.accounts))
	copy(out, s.accounts)
	return out
}

func (s *AccountStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.accounts)
}

func (s *AccountStore) SuccessCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, a := range s.accounts {
		if a.Verified {
			n++
		}
	}
	return n
}

func (s *AccountStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	// Never write the sign-in link to disk: it is a single-use credential and
	// accounts.json gets copied around. Keep the account, drop the secret.
	sanitized := make([]AccountResult, 0, len(s.accounts))
	for _, a := range s.accounts {
		a.MagicLink = Redact(a.MagicLink)
		sanitized = append(sanitized, a)
	}
	b, err := json.MarshalIndent(sanitized, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ExportCSV writes a spreadsheet-friendly copy next to accounts.json.
func (s *AccountStore) ExportCSV(csvPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var b strings.Builder
	b.WriteString("name,username,email,age,gender,looking_for,country,city,platform,platform_username,bio,avatar,magic_link,cookies,verified,created_at\n")
	for _, a := range s.accounts {
		cookies := make([]string, 0, len(a.SessionCookies))
		for k, v := range a.SessionCookies {
			cookies = append(cookies, k+"="+v)
		}
		row := []string{
			a.Name, a.Username, a.Email, fmt.Sprint(a.Age), a.Gender, a.LookingFor,
			a.Country, a.City, a.Platform, a.PlatformUsername, a.Bio,
			a.AvatarPath, Redact(a.MagicLink), strings.Join(cookies, "; "),
			fmt.Sprint(a.Verified), a.CreatedAt.Format(time.RFC3339),
		}
		for i := range row {
			row[i] = csvCell(row[i])
		}
		b.WriteString(strings.Join(row, ","))
		b.WriteString("\n")
	}
	if err := os.MkdirAll(filepath.Dir(csvPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(csvPath, []byte(b.String()), 0o644)
}

func csvCell(s string) string {
	if s == "" {
		return ""
	}
	if strings.ContainsAny(s, ",\"\n\r") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}
