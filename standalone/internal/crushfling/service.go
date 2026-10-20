package crushfling

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"crushfling/internal/proxy"
)

// Service exposes crushfling auto-register to Wails frontend (and CLI)
type Service struct {
	dataDir  string
	register *Register
	mu       sync.Mutex
	cancel   context.CancelFunc
	running  bool
	stats    RegistrationStats
	logs     []string

	// Diagnostics of the most recent run.
	logPath   string
	tracePath string
	lastCfg   RegisterConfig
}

func NewService(dataDir string) (*Service, error) {
	if dataDir == "" {
		wd, _ := os.Getwd()
		dataDir = filepath.Join(wd, "crushfling", "data")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	if err := EnsureDataLayout(dataDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "debug"), 0o700); err != nil {
		return nil, err
	}
	return &Service{dataDir: dataDir}, nil
}

// DataDir returns the runtime folder this service owns.
func (s *Service) DataDir() string { return s.dataDir }

// GetConfig returns current config template
func (s *Service) GetConfig() RegisterConfig {
	return DefaultRegisterConfig(s.dataDir)
}

// GetStats returns live registration stats. While a bulk run is active the
// counters come from the Register instance, so the dashboard sees progress
// instead of only the final numbers.
func (s *Service) GetStats() RegistrationStats {
	s.mu.Lock()
	reg := s.register
	stats := s.stats
	running := s.running
	s.mu.Unlock()
	if running && reg != nil {
		stats = reg.Stats()
		stats.Running = true
	}
	return stats
}

// GetLogs returns last logs
func (s *Service) GetLogs(limit int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.logs) {
		limit = len(s.logs)
	}
	start := len(s.logs) - limit
	if start < 0 {
		start = 0
	}
	out := make([]string, limit)
	copy(out, s.logs[start:])
	return out
}

func (s *Service) appendLog(msg string) {
	s.mu.Lock()
	s.logs = append(s.logs, fmt.Sprintf("[%s] %s", timeNow(), msg))
	if len(s.logs) > 500 {
		s.logs = s.logs[len(s.logs)-500:]
	}
	s.mu.Unlock()
}

func timeNow() string {
	return time.Now().Format("15:04:05")
}

// StartBulk starts bulk registration in background
func (s *Service) StartBulk(cfg RegisterConfig) error {
	s.mu.Lock()
	if s.running || s.cancel != nil {
		s.mu.Unlock()
		return fmt.Errorf("already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	// Ensure data dirs
	if cfg.DataDir == "" {
		cfg.DataDir = s.dataDir
	}
	// The dashboard has no stdin: a manual paste is a UI text field instead.
	cfg.ManualLink = false
	if cfg.PastedLink != "" {
		cfg.Count = 1
		cfg.Concurrency = 1
	}
	os.MkdirAll(cfg.DataDir, 0755)

	reg, err := NewRegister(cfg, func(level string, args ...any) {
		msg := fmt.Sprintf(level, args...)
		s.appendLog(msg)
	})
	if err != nil {
		s.mu.Lock()
		s.cancel = nil
		s.running = false
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.register = reg
	s.running = true
	s.stats = RegistrationStats{Total: cfg.Count, Running: true}
	s.logPath = reg.LogPath()
	s.tracePath = reg.TracePath()
	s.lastCfg = cfg
	s.mu.Unlock()
	s.appendLog(fmt.Sprintf("info: run log file: %s", reg.LogPath()))

	go func() {
		defer reg.Close()
		err := reg.RunBulk(ctx)
		s.mu.Lock()
		s.stats = reg.Stats()
		s.stats.Running = false
		s.cancel = nil
		s.running = false
		s.mu.Unlock()
		if err != nil {
			s.appendLog(fmt.Sprintf("bulk finished with error: %v", err))
		} else {
			s.appendLog("bulk finished")
		}
	}()
	return nil
}

// StopBulk cancels the running bulk AND closes every browser it opened.
//
// The lock is released before any logging: appendLog takes the same mutex, so
// logging under it would deadlock the UI thread.
func (s *Service) StopBulk() error {
	s.mu.Lock()
	cancel := s.cancel
	if cancel == nil {
		s.mu.Unlock()
		return fmt.Errorf("not running")
	}
	cancel()
	s.cancel = nil
	// s.running stays true until the worker goroutine exits, so a second
	// StartBulk cannot overlap with the one that is still shutting down.
	s.mu.Unlock()

	s.appendLog("info: stop requested")
	// Cancelling stops new work; killing the browsers is what actually closes
	// the windows the operator can see.
	if closed := CloseAllBrowsers(); closed > 0 {
		s.appendLog(fmt.Sprintf("info: closed %d browser window(s)", closed))
	}
	return nil
}

// RegisterSingle creates one account synchronously (for UI single-test)
func (s *Service) RegisterSingle(cfg RegisterConfig) (*AccountResult, error) {
	if cfg.DataDir == "" {
		cfg.DataDir = s.dataDir
	}
	reg, err := NewRegister(cfg, func(l string, a ...any) {
		s.appendLog(fmt.Sprintf(l, a...))
	})
	if err != nil {
		return nil, err
	}
	defer reg.Close()
	s.mu.Lock()
	s.logPath = reg.LogPath()
	s.tracePath = reg.TracePath()
	s.lastCfg = cfg
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	res := reg.RegisterOne(ctx, 0)
	return res, nil
}

// ListAccounts returns saved accounts
func (s *Service) ListAccounts() ([]AccountResult, error) {
	store := NewAccountStore(s.dataDir)
	return store.List(), nil
}

// GetCaptionCount returns the remaining bio lines (kept for the existing
// dashboard binding; bio.txt is the preferred file name).
func (s *Service) GetCaptionCount() (int, error) {
	return s.GetBioCount()
}

// GetBioCount returns how many unused bio lines are left.
func (s *Service) GetBioCount() (int, error) {
	info, err := ReadBioInfo(s.dataDir)
	if err != nil {
		return 0, err
	}
	return info.Remaining, nil
}

// GetBioInfo reports the active bio file, how many lines are left and a short
// preview, so the dashboard can show what will be used next.
func (s *Service) GetBioInfo() (BioInfo, error) {
	return ReadBioInfo(s.dataDir)
}

// ImportBioFile replaces <data>/bio.txt with the selected file.
func (s *Service) ImportBioFile(srcPath string) (BioInfo, error) {
	info, err := ImportBioFile(s.dataDir, srcPath)
	if err != nil {
		return BioInfo{}, err
	}
	s.appendLog(fmt.Sprintf("info: bio list loaded from %s (%d line(s))", srcPath, info.Total))
	return info, nil
}

// GetPictureCount returns remaining pictures
func (s *Service) GetPictureCount() (int, error) {
	pm, err := NewPictureManager(filepath.Join(s.dataDir, "pictures"))
	if err != nil {
		return 0, err
	}
	return pm.Count(), nil
}

// ProxyCheck is the dashboard's offline proxy validation result.
type ProxyCheck struct {
	Total  int      `json:"total"`
	Valid  int      `json:"valid"`
	Errors []string `json:"errors"`
}

// ValidateProxies parses a proxy list (file or raw lines) the same way a run
// does. It reports every bad line instead of stopping at the first one, and
// never opens a connection - this is a syntax/format check, not a ping.
func (s *Service) ValidateProxies(filePath, rawList string) ProxyCheck {
	result := ProxyCheck{Errors: []string{}}
	raw := rawList
	if strings.TrimSpace(raw) == "" && strings.TrimSpace(filePath) != "" {
		b, err := os.ReadFile(strings.TrimSpace(filePath))
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("read %s: %v", filePath, err))
			return result
		}
		raw = string(b)
	}
	lines := strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		result.Total++
		if _, err := proxy.ParseLine(line); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: %v", i+1, err))
			continue
		}
		result.Valid++
	}
	return result
}

// AddCaption appends one bio line to the active list.
func (s *Service) AddCaption(caption string) error {
	cm, err := NewCaptionManager(BioPath(s.dataDir))
	if err != nil {
		return err
	}
	return cm.Add(caption)
}

// PreviewProfile generates a preview without registering. It uses the real
// generator and a real bio line (the file is only read, not consumed), so the
// dashboard shows exactly what the next account would look like.
func (s *Service) PreviewProfile() ProfileData {
	profile := GenerateProfile(GenerateEmail("mail.tm"))
	if cm, err := NewCaptionManager(BioPath(s.dataDir)); err == nil {
		if line, err := cm.Peek(); err == nil {
			line = strings.ReplaceAll(line, "@myhandle", "@"+profile.PlatformUsername)
			line = strings.ReplaceAll(line, "{username}", profile.PlatformUsername)
			profile.Bio = line
		}
	}
	if strings.TrimSpace(profile.Bio) == "" {
		profile.Bio = "Preview bio"
	}
	return profile
}

// LogPath returns the run log of the most recent attempt ("" before any run).
func (s *Service) LogPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logPath
}

// TracePath returns the JSONL HTTP trace, or "" when tracing was off.
func (s *Service) TracePath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tracePath
}

// DebugInfo is the small snapshot the dashboard uses to link logs and dumps.
type DebugInfo struct {
	DataDir   string            `json:"data_dir"`
	LogPath   string            `json:"log_path"`
	TracePath string            `json:"trace_path"`
	Debug     bool              `json:"debug"`
	Trace     bool              `json:"trace"`
	Stats     RegistrationStats `json:"stats"`
}

// GetDebugInfo reports where the current run keeps its diagnostics.
func (s *Service) GetDebugInfo() DebugInfo {
	s.mu.Lock()
	info := DebugInfo{
		DataDir:   s.dataDir,
		LogPath:   s.logPath,
		TracePath: s.tracePath,
		Debug:     s.lastCfg.Debug || s.lastCfg.TraceHTTP,
		Trace:     s.lastCfg.TraceHTTP,
		Stats:     s.stats,
	}
	s.mu.Unlock()
	return info
}

// CreateDebugReport writes a redacted, copy-pasteable support report next to
// the failure dumps and returns its path.
func (s *Service) CreateDebugReport() (string, error) {
	s.mu.Lock()
	logs := make([]string, len(s.logs))
	copy(logs, s.logs)
	stats := s.stats
	logPath := s.logPath
	tracePath := s.tracePath
	cfg := s.lastCfg
	s.mu.Unlock()

	endpoints, err := LoadEndpointConfig(s.dataDir)
	if err != nil {
		endpoints = DefaultEndpointConfig()
	}
	captions, _ := s.GetCaptionCount()
	pictures, _ := s.GetPictureCount()
	accounts, _ := s.ListAccounts()
	verified := 0
	for _, account := range accounts {
		if account.Verified {
			verified++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "CrushFling debug report\n")
	fmt.Fprintf(&b, "=======================\n")
	fmt.Fprintf(&b, "generated  : %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "data dir   : %s\n", s.dataDir)
	fmt.Fprintf(&b, "run log    : %s\n", logPath)
	fmt.Fprintf(&b, "http trace : %s\n", tracePath)
	fmt.Fprintf(&b, "stats      : total=%d success=%d failed=%d pending=%d running=%v\n",
		stats.Total, stats.Success, stats.Failed, stats.Pending, stats.Running)
	fmt.Fprintf(&b, "error codes: %s\n", countSummary(stats.ByCode))
	if stats.LastError != "" {
		fmt.Fprintf(&b, "last error : %s\n", stats.LastError)
	}
	fmt.Fprintf(&b, "accounts   : %d saved (%d verified)\n", len(accounts), verified)
	fmt.Fprintf(&b, "bios       : %d left\n", captions)
	fmt.Fprintf(&b, "pictures   : %d left\n", pictures)
	if cfg.EmailProvider != "" || cfg.DataDir != "" {
		fmt.Fprintf(&b, "last run   : provider=%s concurrency=%d browser=%v browser_verify=%v debug=%v trace=%v\n",
			cfg.EmailProvider, cfg.Concurrency, cfg.UseBrowser, cfg.BrowserVerify, cfg.Debug, cfg.TraceHTTP)
	}
	fmt.Fprintf(&b, "endpoints  : csrf=%s magic=%s upload=%s\n",
		endpoints.CSRFPath, endpoints.MagicLinkPath, endpoints.UploadPath)
	fmt.Fprintf(&b, "profile    : %s\n", strings.Join(endpoints.ProfilePaths, ", "))

	b.WriteString("\nrecent log:\n")
	start := len(logs) - 80
	if start < 0 {
		start = 0
	}
	if len(logs) == 0 {
		b.WriteString("  (no log lines yet)\n")
	}
	for _, line := range logs[start:] {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	b.WriteString("\nNote: cookies and session tokens are never written to this report.\n")

	dir := filepath.Join(s.dataDir, "debug")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "report-"+NewRunID()+".txt")
	if err := os.WriteFile(path, []byte(Redact(b.String())), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func countSummary(m map[string]int) string {
	if len(m) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, m[key]))
	}
	return strings.Join(parts, " ")
}
