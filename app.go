package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"chitchatbot/internal/applog"
	"chitchatbot/internal/browser"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"chitchatbot/internal/bot"
	"chitchatbot/internal/crushfling"
	"chitchatbot/internal/license"
	"chitchatbot/internal/model"
	"chitchatbot/internal/proxy"
	"chitchatbot/internal/store"
	"chitchatbot/internal/uistore"
	"chitchatbot/internal/workerplan"
)

type App struct {
	ctx             context.Context
	dataDir         string
	uiStore         *uistore.Store
	mu              sync.Mutex
	chatFeed        *homeChatFeed
	showLog         *showLogFeed
	botLog          *botLogFeed
	showModeRunning bool
	running         bool
	runID           uint64
	botCancel       context.CancelFunc
	activeWorkers   map[string]bool
	licenseKey      string
	licenseInvalid  bool
	licenseReason   string
	licenseTimer    *time.Ticker
	licenseDone     chan bool
	crushService    *crushfling.Service
}

func NewApp(dataDir string) (*App, error) {
	uiStore, err := uistore.Open(dataDir)
	if err != nil {
		return nil, err
	}

	// Keep this feature's runtime data inside the app's data folder. A failure
	// here must not stop the chitchat.gg app from starting.
	crushSvc, _ := crushfling.NewService(filepath.Join(dataDir, "crushfling"))

	app := &App{
		dataDir:       dataDir,
		uiStore:       uiStore,
		activeWorkers: map[string]bool{},
		crushService:  crushSvc,
	}

	if err := app.seedDefaults(); err != nil {
		_ = uiStore.Close()
		return nil, err
	}

	key, err := license.GenerateHardwareKey()
	if err != nil {
		app.licenseInvalid = true
		app.licenseReason = "Could not read hardware identifiers."
	} else {
		valid, reason := license.VerifyLicense(key)
		if !valid {
			app.licenseInvalid = true
			app.licenseKey = key
			app.licenseReason = reason
		} else {
			app.licenseKey = key
		}
	}

	return app, nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if a.licenseInvalid {
		runtime.WindowSetSize(a.ctx, 500, 350)
		runtime.WindowCenter(a.ctx)
		return
	}

	a.licenseTimer = time.NewTicker(24 * time.Hour)
	a.licenseDone = make(chan bool)

	go func() {
		for {
			select {
			case <-a.licenseDone:
				return
			case <-a.licenseTimer.C:
				v, r := license.VerifyLicense(a.licenseKey)
				if !v {
					a.mu.Lock()
					a.licenseInvalid = true
					a.licenseReason = r
					a.mu.Unlock()
					a.emit("ui:license_invalid", map[string]string{"key": a.licenseKey, "reason": r})
					runtime.WindowSetSize(a.ctx, 500, 350)
					runtime.WindowCenter(a.ctx)
					return
				}
			}
		}
	}()

	a.log("info", "Wails UI ready.")
}

func (a *App) shutdown(ctx context.Context) {
	_ = a.StopBot()
	if a.licenseTimer != nil {
		a.licenseTimer.Stop()
	}
	if a.licenseDone != nil {
		close(a.licenseDone)
	}
	if a.uiStore != nil {
		_ = a.uiStore.Close()
	}
}

func (a *App) GetInitialState() (model.UISnapshot, error) {
	browserSettings, err := a.uiStore.LoadBrowserSettings()
	if err != nil {
		return model.UISnapshot{}, err
	}

	messageSettings, err := a.uiStore.LoadMessageSettings()
	if err != nil {
		return model.UISnapshot{}, err
	}

	proxySettings, err := a.uiStore.LoadProxySettings()
	if err != nil {
		return model.UISnapshot{}, err
	}

	generalSettings, err := a.uiStore.LoadGeneralSettings()
	if err != nil {
		return model.UISnapshot{}, err
	}

	showSettings, err := a.uiStore.LoadShowModeSettings()
	if err != nil {
		return model.UISnapshot{}, err
	}

	logs, err := a.uiStore.ListLogs(200)
	if err != nil {
		return model.UISnapshot{}, err
	}

	a.mu.Lock()
	running := a.running
	runningAccounts := len(a.activeWorkers)
	isLicenseInvalid := a.licenseInvalid
	licenseKey := a.licenseKey
	licenseReason := a.licenseReason
	showModeRunning := a.showModeRunning
	a.mu.Unlock()

	homeStats, err := a.homeStats(browserSettings, messageSettings, runningAccounts)
	if err != nil {
		return model.UISnapshot{}, err
	}

	return model.UISnapshot{
		Running:         running,
		ShowModeRunning: showModeRunning,
		LicenseInvalid:  isLicenseInvalid,
		LicenseKey:      licenseKey,
		LicenseReason:   licenseReason,
		Home:            homeStats,
		Browser:         browserSettings,
		Message:         messageSettings,
		Proxy:           proxySettings,
		General:         generalSettings,
		ShowMode:        showSettings,
		BrowserOptions:  model.DefaultBrowserOptions(),
		Logs:            logs,
	}, nil
}

func (a *App) homeStats(
	browserSettings model.BrowserSettings,
	messageSettings model.MessageSettings,
	runningAccounts int,
) (model.HomeStats, error) {
	workers, err := workerplan.DiscoverProfileWorkers(a.dataDir, browserSettings.Profile)
	if err != nil {
		return model.HomeStats{}, err
	}

	accountCounts, err := bot.BotAccountStatusCounts(a.dataDir)
	if err != nil {
		return model.HomeStats{}, err
	}

	messagesSent, trafficUsers, snapTgSentCount, skippedAccounts, err := a.countSentMessages()
	if err != nil {
		return model.HomeStats{}, err
	}

	botAccounts := len(workers)
	if runningAccounts+accountCounts.Dead > botAccounts {
		botAccounts = runningAccounts + accountCounts.Dead
	}

	return model.HomeStats{
		BotAccounts:     botAccounts,
		RunningAccounts: runningAccounts,
		UserSendCount:   len(messageSettings.UsernameLines()),
		DeadAccounts:    accountCounts.Dead,
		TrafficUsers:    trafficUsers,
		MessagesSent:    messagesSent,
		SnapTgSentCount: snapTgSentCount,
		SkippedAccounts: skippedAccounts,
	}, nil
}

func (a *App) countSentMessages() (int, int, int, int, error) {
	statePaths, err := filepath.Glob(filepath.Join(a.dataDir, "profiles", "*", "bot_state.json"))
	if err != nil {
		return 0, 0, 0, 0, err
	}

	counts := map[string]int{}
	uniqueUsers := map[string]bool{}
	snapTgSentCount := 0
	skippedUsersCount := 0
	for _, statePath := range statePaths {
		var state model.BotState
		ok, err := store.LoadJSON(statePath, &state)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		if !ok {
			continue
		}

		for accountID, accountState := range state {
			for userID, count := range accountState.SentMessageCounts {
				if count <= 0 {
					continue
				}

				key := accountID + "\x00" + userID
				if count > counts[key] {
					counts[key] = count
				}
				uniqueUsers[userID] = true
			}
			snapTgSentCount += accountState.SnapTgSentCount
			skippedUsersCount += accountState.SkippedUsersCount
		}
	}

	total := 0
	for _, count := range counts {
		total += count
	}
	return total, len(uniqueUsers), snapTgSentCount, skippedUsersCount, nil
}

func (a *App) setWorkerRunning(profile string, running bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeWorkers == nil {
		a.activeWorkers = map[string]bool{}
	}
	if running {
		a.activeWorkers[profile] = true
		return
	}
	delete(a.activeWorkers, profile)
}

func (a *App) clearActiveWorkers() {
	a.mu.Lock()
	a.activeWorkers = map[string]bool{}
	a.mu.Unlock()
}

func (a *App) SaveBrowserSettings(settings model.BrowserSettings) error {
	if strings.TrimSpace(settings.Profile) == "" {
		settings.Profile = "default"
	}
	if err := a.uiStore.SaveBrowserSettings(settings); err != nil {
		return err
	}
	a.log("info", fmt.Sprintf("Browser settings saved. Backend=%s Profile=%s", settings.Backend, settings.Profile))
	return nil
}

// RunFreshBrowsers preserves compatibility for callers using saved settings.
func (a *App) RunFreshBrowsers(count int) error {
	settings, err := a.uiStore.LoadBrowserSettings()
	if err != nil {
		return err
	}
	return a.RunBrowser(settings, count)
}

// RunBrowser uses the clicked row and current form values, never a second
// read of shared saved settings. Accepted settings become the next defaults.
func (a *App) RunBrowser(settings model.BrowserSettings, count int) error {
	return a.runBrowser(settings, count, func(dir, profile string, request model.BrowserSettings, entry *proxy.Entry, index, total int) {
		go a.runFreshBrowser(dir, profile, request, entry, index, total)
	})
}

func validateBrowserRun(settings model.BrowserSettings, count int) (model.BrowserSettings, error) {
	if count < 1 || count > 5 {
		return settings, fmt.Errorf("browser count must be between 1 and 5")
	}
	supported := false
	for _, option := range model.DefaultBrowserOptions() {
		if settings.Backend == option.Key {
			supported = true
			break
		}
	}
	if !supported {
		return settings, fmt.Errorf("unsupported browser backend: %q", settings.Backend)
	}
	// Show Mode is a visible Rod launch preset, not a third browser driver.
	// Normalize before persistence/dispatch so saved sessions use a real backend.
	if settings.Backend == "show_mode" {
		settings.Backend = "rod"
		settings.Headless = false
		settings.LiveChat = true
	}
	settings.Profile = safeProfileName(settings.Profile)
	return settings, nil
}

// The dispatch callback keeps request isolation and preflight failures testable
// without opening a real browser or accessing an account.
func (a *App) runBrowser(settings model.BrowserSettings, count int, launch func(string, string, model.BrowserSettings, *proxy.Entry, int, int)) error {
	browserSettings, err := validateBrowserRun(settings, count)
	if err != nil {
		return err
	}

	proxyRotator, proxyCount, err := a.loadProxyRotator()
	if err != nil {
		return err
	}

	profileName := browserSettings.Profile
	profileDir := filepath.Join(a.dataDir, "profiles", profileName)
	if err := store.EnsureDir(profileDir); err != nil {
		return err
	}

	if err := a.uiStore.SaveBrowserSettings(browserSettings); err != nil {
		return err
	}

	a.log("info", fmt.Sprintf(
		"Starting %d fresh %s browser(s) for profile '%s'.",
		count,
		browserSettings.Backend,
		profileName,
	))
	a.log("info", fmt.Sprintf("Fresh login JSON files will be saved under %s", profileDir))
	if proxyCount > 0 {
		a.log("info", fmt.Sprintf("Fresh browsers will use %d proxy line(s) in rotation.", proxyCount))
	}

	for i := 1; i <= count; i++ {
		index := i
		var selectedProxy *proxy.Entry
		if proxyRotator != nil {
			if nextProxy, ok := proxyRotator.Next(); ok {
				selectedProxy = &nextProxy
			}
		}
		launch(profileDir, profileName, browserSettings, selectedProxy, index, count)
	}

	return nil
}

func (a *App) SaveMessageSettings(settings model.MessageSettings) error {
	existing, _ := a.uiStore.LoadMessageSettings()
	if len(settings.LoadedSequences) == 0 && len(existing.LoadedSequences) > 0 {
		settings.LoadedSequences = existing.LoadedSequences
	}
	// Keyword fields are retained in the stored JSON only for backward
	// compatibility. The selectable third mode now uses EVAChat Flow.
	switch settings.ChatMode {
	case model.ChatModeEVAChat, model.ChatModeKeyword:
		settings.ChatMode = model.ChatModeEVAChat
		settings.UseLoadedSequences = false
	case model.ChatModeFolder:
		settings.UseLoadedSequences = true
	case model.ChatModeSingle:
		settings.UseLoadedSequences = false
	default:
		if settings.UseLoadedSequences && len(settings.LoadedSequences) > 0 {
			settings.ChatMode = model.ChatModeFolder
			settings.UseLoadedSequences = true
		} else {
			settings.ChatMode = model.ChatModeSingle
			settings.UseLoadedSequences = false
		}
	}
	if err := a.uiStore.SaveMessageSettings(settings); err != nil {
		return err
	}
	a.log("info", fmt.Sprintf("Message settings saved. Mode=%s", settings.EffectiveChatMode()))
	return nil
}

// TestProxyEntries tests the current form without saving or exposing credentials.
func (a *App) TestProxyEntries(raw string) ([]proxy.TestResult, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 65*time.Second)
	defer cancel()
	return proxy.TestList(ctx, raw, "https://app.chitchat.gg/")
}

func (a *App) SaveProxySettings(settings model.ProxySettings) error {
	if settings.Enabled {
		entries, err := proxy.ParseList(settings.Entries)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return fmt.Errorf("proxy list is enabled but empty")
		}
	}

	if err := a.uiStore.SaveProxySettings(settings); err != nil {
		return err
	}

	if settings.Enabled {
		entries, _ := proxy.ParseList(settings.Entries)
		a.log("info", fmt.Sprintf("Proxy settings saved. %d proxy line(s) enabled for Rod, ChromeDP, and Go request rotation.", len(entries)))
	} else {
		a.log("info", "Proxy settings saved. Proxy use is disabled.")
	}
	return nil
}

func (a *App) SaveShowModeSettings(settings model.ShowModeSettings) error {
	return a.uiStore.SaveShowModeSettings(settings)
}

func (a *App) SaveGeneralSettings(settings model.GeneralSettings) error {
	// Freeze legacy migration before accepting a standard-worker-only update.
	if _, err := a.uiStore.LoadShowModeSettings(); err != nil {
		return err
	}
	for _, pair := range [][2]int{{settings.PollMinSeconds, settings.PollMaxSeconds}, {settings.ActionMinSeconds, settings.ActionMaxSeconds}, {settings.MatchReplyTimeoutMinSeconds, settings.MatchReplyTimeoutMaxSeconds}, {settings.MatchCloseWaitMinSeconds, settings.MatchCloseWaitMaxSeconds}, {settings.ReplyDelayMinSeconds, settings.ReplyDelayMaxSeconds}} {
		if pair[0] < 0 || pair[1] < pair[0] || pair[1] > 86400 {
			return fmt.Errorf("invalid timing range: min must not exceed max (maximum 86400 seconds)")
		}
	}

	if err := a.uiStore.SaveGeneralSettings(settings); err != nil {
		return err
	}
	a.log("info", fmt.Sprintf(
		"Timing settings saved. Poll=%ds-%ds Action=%ds-%ds",
		settings.PollMinSeconds,
		settings.PollMaxSeconds,
		settings.ActionMinSeconds,
		settings.ActionMaxSeconds,
	))
	return nil
}

// UnifiedSettingsPayload is the dashboard's single timing form. The bridge
// fans it out to the two independent stores (settings/general and
// settings/show_mode/v1); neither existing struct nor DB key changes.
type UnifiedSettingsPayload struct {
	FirstMessage         model.FirstMessageSettings `json:"first_message"`
	ReplyDelayMinSeconds int                        `json:"reply_delay_min_seconds"`
	ReplyDelayMaxSeconds int                        `json:"reply_delay_max_seconds"`
	AutoSkip             bool                       `json:"auto_skip"`
	NoReplyMinSeconds    int                        `json:"no_reply_min_seconds"`
	NoReplyMaxSeconds    int                        `json:"no_reply_max_seconds"`
	NextChatMinSeconds   int                        `json:"next_chat_min_seconds"`
	NextChatMaxSeconds   int                        `json:"next_chat_max_seconds"`
	PollMinSeconds       int                        `json:"poll_min_seconds"`
	PollMaxSeconds       int                        `json:"poll_max_seconds"`
	ActionMinSeconds     int                        `json:"action_min_seconds"`
	ActionMaxSeconds     int                        `json:"action_max_seconds"`
}

func (p UnifiedSettingsPayload) validate() error {
	if err := p.FirstMessage.Validate(); err != nil {
		return err
	}
	pairs := []struct {
		name     string
		min, max int
	}{
		{"reply delay", p.ReplyDelayMinSeconds, p.ReplyDelayMaxSeconds},
		{"no-reply timeout", p.NoReplyMinSeconds, p.NoReplyMaxSeconds},
		{"next-chat wait", p.NextChatMinSeconds, p.NextChatMaxSeconds},
		{"poll", p.PollMinSeconds, p.PollMaxSeconds},
		{"action", p.ActionMinSeconds, p.ActionMaxSeconds},
	}
	for _, pair := range pairs {
		if pair.min < 0 || pair.max < pair.min || pair.max > 86400 {
			return fmt.Errorf("%s must satisfy 0 <= min <= max <= 86400 seconds", pair.name)
		}
	}
	if p.PollMinSeconds < 1 || p.ActionMinSeconds < 1 {
		return fmt.Errorf("poll and action minimums must be at least 1 second")
	}
	if p.AutoSkip && p.NoReplyMinSeconds < 1 {
		return fmt.Errorf("auto skip requires a positive no-reply timeout")
	}
	return nil
}

// SaveUnifiedSettings keeps the dashboard simple while preserving the exact
// storage layout: the background worker and Show Mode still read their own
// keys and keep their separate execution paths.
func (a *App) SaveUnifiedSettings(payload UnifiedSettingsPayload) error {
	if err := payload.validate(); err != nil {
		return err
	}

	general := model.GeneralSettings{
		FirstMessage:                payload.FirstMessage,
		PollMinSeconds:              payload.PollMinSeconds,
		PollMaxSeconds:              payload.PollMaxSeconds,
		ActionMinSeconds:            payload.ActionMinSeconds,
		ActionMaxSeconds:            payload.ActionMaxSeconds,
		MatchReplyTimeoutMinSeconds: payload.NoReplyMinSeconds,
		MatchReplyTimeoutMaxSeconds: payload.NoReplyMaxSeconds,
		MatchCloseWaitMinSeconds:    payload.NextChatMinSeconds,
		MatchCloseWaitMaxSeconds:    payload.NextChatMaxSeconds,
		ReplyDelayMinSeconds:        payload.ReplyDelayMinSeconds,
		ReplyDelayMaxSeconds:        payload.ReplyDelayMaxSeconds,
	}
	show := model.ShowModeSettings{
		FirstMessage:         payload.FirstMessage,
		ReplyDelayMinSeconds: payload.ReplyDelayMinSeconds,
		ReplyDelayMaxSeconds: payload.ReplyDelayMaxSeconds,
		AutoSkip:             payload.AutoSkip,
		NoReplyMinSeconds:    payload.NoReplyMinSeconds,
		NoReplyMaxSeconds:    payload.NoReplyMaxSeconds,
		NextChatMinSeconds:   payload.NextChatMinSeconds,
		NextChatMaxSeconds:   payload.NextChatMaxSeconds,
	}

	// Validate the mode-specific shape before the first write, so a bad Show
	// Mode value cannot leave only one key updated.
	if err := uistore.ValidateShowModeSettings(show); err != nil {
		return err
	}
	if err := a.SaveGeneralSettings(general); err != nil {
		return err
	}
	if err := a.SaveShowModeSettings(show); err != nil {
		return err
	}

	a.log("info", fmt.Sprintf(
		"Unified settings saved to general + show_mode (first=%s reply=%ds-%ds auto_skip=%t).",
		payload.FirstMessage.Effective().Mode,
		payload.ReplyDelayMinSeconds,
		payload.ReplyDelayMaxSeconds,
		payload.AutoSkip,
	))
	return nil
}

func (a *App) ClearLogs() error {
	if err := a.uiStore.ClearLogs(); err != nil {
		return err
	}
	a.emit("ui:logs_cleared", true)
	a.log("info", "Logs cleared.")
	return nil
}

func (a *App) ClearLoadedMessages() error {
	settings, err := a.uiStore.LoadMessageSettings()
	if err != nil {
		return err
	}
	settings.LoadedSequences = nil
	settings.UseLoadedSequences = false
	if settings.ChatMode == model.ChatModeFolder {
		settings.ChatMode = model.ChatModeSingle
	}
	return a.uiStore.SaveMessageSettings(settings)
}

func (a *App) LoadMessageFolder() ([]string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Folder with Chat Patterns",
	})
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, nil
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sequences []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".txt") {
			content, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				continue
			}
			seq := strings.TrimSpace(string(content))
			if seq != "" {
				sequences = append(sequences, seq)
			}
		}
	}

	if len(sequences) == 0 {
		return nil, errors.New("no valid .txt files found in the selected folder")
	}

	settings, err := a.uiStore.LoadMessageSettings()
	if err != nil {
		return nil, err
	}

	settings.ChatMode = model.ChatModeFolder
	settings.LoadedSequences = sequences
	settings.UseLoadedSequences = true
	if err := a.uiStore.SaveMessageSettings(settings); err != nil {
		return nil, err
	}

	return sequences, nil
}

// OpenEVAChatDataFolder bootstraps the editable EVAChat data pack, validates
// it, and opens the local folder. It never imports arbitrary keyword files.
func (a *App) OpenEVAChatDataFolder() (string, error) {
	dataRoot, err := bot.EnsureEVAChatData(a.dataDir)
	if err != nil {
		return "", err
	}
	engine, err := bot.LoadEVAChatEngine(dataRoot)
	if err != nil {
		return "", err
	}
	if err := exec.Command("explorer.exe", dataRoot).Start(); err != nil {
		return "", err
	}
	summary := fmt.Sprintf("EVAChat data folder ready: %s (%d trigger categories, %d reply pools)", dataRoot, len(engine.InputCategories()), len(engine.OutputPools()))
	a.log("info", summary)
	return summary, nil
}

// TestEVAChatAI sends one small request to the configured AI API (unsaved
// dashboard values are used) and returns the model's answer.
func (a *App) TestEVAChatAI(settings model.MessageSettings) (string, error) {
	settings.AIEnabled = true
	dataRoot, err := bot.EnsureEVAChatData(a.dataDir)
	if err != nil {
		return "", err
	}
	ai, err := bot.LoadEVAAI(dataRoot, settings)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	answer, err := ai.Test(ctx)
	if err != nil {
		a.log("error", fmt.Sprintf("EVAChat AI test failed: %v", err))
		return "", err
	}
	summary := fmt.Sprintf("AI OK (%s / %s) -> %q", ai.BaseURL, ai.Model, answer)
	a.log("info", summary)
	return summary, nil
}

func (a *App) CleanProfiles() error {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()

	if running {
		return fmt.Errorf("stop the bot before cleaning profile data")
	}

	if err := a.removeProfileData(); err != nil {
		return err
	}

	a.clearActiveWorkers()

	a.log("info", "Profile data cleaned.")
	return nil
}

func (a *App) StartBot() error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		a.log("info", "Bot is already running.")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.runID++
	runID := a.runID
	a.running = true
	a.botCancel = cancel
	a.mu.Unlock()

	a.emitStatus(true)
	a.log("activity", "bot started wait to process")
	a.botEvent("system", "bot started — preparing workers", "", "", "", true)

	configs, err := a.buildBotConfigs()
	if err != nil {
		a.mu.Lock()
		a.running = false
		a.botCancel = nil
		if a.runID == runID {
			a.runID++
		}
		a.mu.Unlock()
		a.emitStatus(false)
		return err
	}

	a.log("info", fmt.Sprintf("Starting %d worker(s) across all discovered profiles.", len(configs)))
	a.botEvent("system", fmt.Sprintf("starting %d worker(s)", len(configs)), "", "", "", true)

	var wg sync.WaitGroup
	wg.Add(len(configs))

	for _, cfg := range configs {
		workerCfg := cfg
		profileName := workerCfg.Profile
		a.setWorkerRunning(profileName, true)
		go func() {
			defer func() {
				a.setWorkerRunning(profileName, false)
				wg.Done()
			}()

			err := bot.Run(ctx, workerCfg)
			if errors.Is(err, context.Canceled) {
				a.log("info", fmt.Sprintf("[%s] Worker stopped.", profileName))
				a.botEvent("system", "worker stopped", "", "", profileName, true)
				return
			}
			if err != nil {
				a.log("error", fmt.Sprintf("[%s] Worker stopped with error: %v", profileName, err))
				a.botEvent("error", fmt.Sprintf("worker stopped: %v", err), "", "", profileName, false)
				return
			}
			a.log("info", fmt.Sprintf("[%s] Worker finished.", profileName))
			a.botEvent("system", "worker finished", "", "", profileName, true)
		}()
	}

	go func() {
		wg.Wait()

		a.mu.Lock()
		if a.runID == runID {
			a.running = false
			a.botCancel = nil
			a.runID++
			a.mu.Unlock()
			a.log("info", "All bot workers stopped.")
			a.botEvent("system", "all workers stopped", "", "", "", true)
			a.emitStatus(false)
			return
		}
		a.mu.Unlock()
	}()

	return nil
}

func (a *App) StopBot() error {
	a.mu.Lock()
	cancel := a.botCancel
	if cancel == nil {
		a.mu.Unlock()
		return nil
	}
	a.botCancel = nil
	a.running = false
	a.mu.Unlock()

	cancel()
	a.clearActiveWorkers()
	a.log("info", "Stopping all bot workers.")
	a.botEvent("system", "stop requested — workers shutting down", "", "", "", true)
	a.emitStatus(false)
	return nil
}

func (a *App) seedDefaults() error {
	if _, err := a.uiStore.LoadBrowserSettings(); err != nil {
		return err
	}
	if _, err := a.uiStore.LoadMessageSettings(); err != nil {
		return err
	}
	if _, err := a.uiStore.LoadProxySettings(); err != nil {
		return err
	}
	if _, err := a.uiStore.LoadGeneralSettings(); err != nil {
		return err
	}
	return nil
}

func (a *App) removeProfileData() error {
	dataDir, err := filepath.Abs(a.dataDir)
	if err != nil {
		return err
	}

	targets := []string{
		filepath.Join(a.dataDir, "profiles"),
		filepath.Join(a.dataDir, "tmp"),
		filepath.Join(a.dataDir, "fresh-runs"),
		filepath.Join(a.dataDir, "bot_registry.json"),
	}

	for _, target := range targets {
		targetAbs, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if !pathInside(dataDir, targetAbs) {
			return fmt.Errorf("refusing to clean path outside data directory: %s", targetAbs)
		}
		if err := os.RemoveAll(targetAbs); err != nil {
			return err
		}
	}

	return nil
}

func pathInside(parent string, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if parent == child {
		return false
	}
	return strings.HasPrefix(child, parent+string(os.PathSeparator))
}

func (a *App) buildBotConfigs() ([]bot.Config, error) {
	browserSettings, err := a.uiStore.LoadBrowserSettings()
	if err != nil {
		return nil, err
	}

	messageSettings, err := a.uiStore.LoadMessageSettings()
	if err != nil {
		return nil, err
	}

	generalSettings, err := a.uiStore.LoadGeneralSettings()
	if err != nil {
		return nil, err
	}

	proxyEntries, err := a.loadProxyEntries()
	if err != nil {
		return nil, err
	}

	var evaChatConfig *bot.EVAChatConfig
	if messageSettings.EffectiveChatMode() == model.ChatModeEVAChat {
		dataRoot, err := bot.EnsureEVAChatData(a.dataDir)
		if err != nil {
			return nil, err
		}
		engine, err := bot.LoadEVAChatEngine(dataRoot)
		if err != nil {
			return nil, err
		}
		ai, err := bot.LoadEVAAI(dataRoot, messageSettings)
		if err != nil {
			return nil, err
		}
		evaChatConfig = &bot.EVAChatConfig{DataRoot: dataRoot, Engine: engine, AI: ai}
		a.log("info", fmt.Sprintf("EVAChat Flow ready: %d trigger categories and %d reply pools loaded from %s. %s", len(engine.InputCategories()), len(engine.OutputPools()), dataRoot, ai.Describe()))
	}

	profiles, err := workerplan.DiscoverProfileWorkers(a.dataDir, browserSettings.Profile)
	if err != nil {
		return nil, err
	}

	assignments := workerplan.AssignProfileWorkers(profiles, proxyEntries)
	configs := make([]bot.Config, 0, len(assignments))

	for _, assignment := range assignments {
		cfg := bot.DefaultConfig(
			a.dataDir,
			assignment.Profile,
			browserSettings.Backend,
			browserSettings.Headless,
		)
		cfg.LoginInfoPath = assignment.LoginInfoPath
		cfg.ChatMode = messageSettings.EffectiveChatMode()
		if cfg.ChatMode == model.ChatModeEVAChat {
			cfg.EVAChat = evaChatConfig
			// EVAChat has its own persisted stage state. Keep MessageSequence
			// empty so sequence code cannot accidentally drive this mode.
			cfg.MessageSequence = nil
			cfg.AvailableSequences = nil
		} else if cfg.ChatMode == model.ChatModeFolder && len(messageSettings.LoadedSequences) > 0 {
			cfg.AvailableSequences = make([][]string, len(messageSettings.LoadedSequences))
			for i, seqStr := range messageSettings.LoadedSequences {
				cfg.AvailableSequences[i] = model.MessageSequenceLines(seqStr)
			}
		} else {
			cfg.AvailableSequences = [][]string{messageSettings.SequenceLines()}
		}
		cfg.MessageUsernames = messageSettings.UsernameLines()
		cfg.FirstMessage = generalSettings.FirstMessage.Effective()
		cfg.MatchWhenIdle = true
		cfg.PollMin = time.Duration(generalSettings.PollMinSeconds) * time.Second
		cfg.PollMax = time.Duration(generalSettings.PollMaxSeconds) * time.Second
		cfg.ActionMin = time.Duration(generalSettings.ActionMinSeconds) * time.Second
		cfg.ActionMax = time.Duration(generalSettings.ActionMaxSeconds) * time.Second
		cfg.MatchReplyTimeoutMin = time.Duration(generalSettings.MatchReplyTimeoutMinSeconds) * time.Second
		cfg.MatchReplyTimeoutMax = time.Duration(generalSettings.MatchReplyTimeoutMaxSeconds) * time.Second
		cfg.MatchCloseWaitMin = time.Duration(generalSettings.MatchCloseWaitMinSeconds) * time.Second
		cfg.MatchCloseWaitMax = time.Duration(generalSettings.MatchCloseWaitMaxSeconds) * time.Second
		cfg.ReplyDelayMin = time.Duration(generalSettings.ReplyDelayMinSeconds) * time.Second
		cfg.ReplyDelayMax = time.Duration(generalSettings.ReplyDelayMaxSeconds) * time.Second

		profileName := assignment.Profile
		proxyLabel := "direct"
		if assignment.HasProxy {
			assignedProxy := assignment.Proxy
			cfg.ProxyRotator = proxy.NewRotator([]proxy.Entry{assignedProxy})
			proxyLabel = assignedProxy.Label()
		}

		cfg.Logf = func(format string, args ...any) {
			message := fmt.Sprintf(strings.TrimSuffix(format, "\n"), args...)
			a.log("info", fmt.Sprintf("[%s] %s", profileName, message))
			kind := "system"
			if strings.HasPrefix(message, "SEND |") {
				kind = "out"
			} else if strings.HasPrefix(message, "RECEIVE |") {
				kind = "in"
			}
			a.botEvent(kind, message, "", "", profileName, true)
		}
		cfg.Activityf = func(format string, args ...any) {
			message := fmt.Sprintf(strings.TrimSuffix(format, "\n"), args...)
			a.log("activity", message)
			kind := "system"
			if strings.HasPrefix(message, "SEND |") {
				kind = "out"
			} else if strings.HasPrefix(message, "RECEIVE |") {
				kind = "in"
			}
			a.botEvent(kind, message, "", "", profileName, true)
		}

		if assignment.LoginInfoPath != "" {
			msg := fmt.Sprintf("Prepared worker for user/profile %s using %s from %s.", profileName, proxyLabel, assignment.LoginInfoPath)
			a.log("info", msg)
			a.botEvent("system", msg, "", "", profileName, true)
		} else {
			msg := fmt.Sprintf("Prepared worker for profile %s using %s.", profileName, proxyLabel)
			a.log("info", msg)
			a.botEvent("system", msg, "", "", profileName, true)
		}
		configs = append(configs, cfg)
	}

	return configs, nil
}

func safeProfileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "..", "_")
	name = strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_").Replace(name)
	name = strings.Map(func(r rune) rune {
		if r < 32 {
			return '_'
		}
		return r
	}, name)
	// Win32 ignores trailing spaces/dots and reserves device names even when
	// they have an extension (for example CON.txt). Keep profiles unambiguous.
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "default"
	}
	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	reserved := stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL"
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
		reserved = true
	}
	if reserved {
		name = "_" + name
	}
	return name
}

func (a *App) runFreshBrowser(profileDir string, profileName string, settings model.BrowserSettings, selectedProxy *proxy.Entry, index int, total int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.runFreshBrowserWithBackend(ctx, profileDir, profileName, settings, selectedProxy, index, total, browser.New)
}

// Production and browser-free tests share the entire capture/save/cleanup path.
// Only backend construction is injected; no global factory or fake production mode.
func (a *App) runFreshBrowserWithBackend(ctx context.Context, profileDir string, profileName string, settings model.BrowserSettings, selectedProxy *proxy.Entry, index int, total int, newBackend func(string) (browser.Backend, error)) {
	proxyLabel := "direct"
	if selectedProxy != nil {
		proxyLabel = selectedProxy.Label()
	}
	if settings.LiveChat {
		a.clearShowLog()
		a.setShowModeRunning(true)
		defer a.setShowModeRunning(false)
		a.showModeEvent("system", "run requested", "", fmt.Sprintf("profile=%s · proxy=%s · browser=%s", profileName, proxyLabel, settings.Backend), false)
	}

	// Keep disposable browser data outside the repo so wails dev file watching
	// doesn't restart the app when a fresh browser run creates a new directory.
	tempPattern := fmt.Sprintf("chitchat-fresh-browser-%s-%02d-", profileName, index)
	browserTempDir, err := os.MkdirTemp("", tempPattern)
	if err != nil {
		// Low-integrity launch tokens and some AV policies deny %TEMP%. The
		// app's own data directory is writable (the settings store lives
		// there), so use it for the disposable browser profile instead.
		fallbackRoot := filepath.Join(a.dataDir, "tmp")
		if mkErr := store.EnsureDir(fallbackRoot); mkErr != nil {
			a.log("error", fmt.Sprintf("[%s fresh %d/%d] Failed to create temp browser dir: %v (fallback: %v)", profileName, index, total, err, mkErr))
			a.showLiveError("could not create a temporary browser profile", mkErr)
			return
		}
		browserTempDir, err = os.MkdirTemp(fallbackRoot, tempPattern)
		if err != nil {
			a.log("error", fmt.Sprintf("[%s fresh %d/%d] Failed to create temp browser dir: %v", profileName, index, total, err))
			a.showLiveError("could not create a temporary browser profile", err)
			return
		}
		a.log("info", fmt.Sprintf("[%s fresh %d/%d] System temp is not writable; using %s", profileName, index, total, browserTempDir))
	}

	// Register cleanup immediately, including failures before backend creation.
	defer a.removeFreshBrowserData(browserTempDir, profileName, index, total)

	userDataDir := filepath.Join(browserTempDir, "user-data")
	if err := store.EnsureDir(userDataDir); err != nil {
		a.log("error", fmt.Sprintf("[%s fresh %d/%d] Failed to create temp browser dir: %v", profileName, index, total, err))
		a.showLiveError("could not prepare the browser profile", err)
		return
	}

	backend, err := newBackend(settings.Backend)
	if err != nil {
		a.log("error", fmt.Sprintf("[%s fresh %d/%d] Failed to create browser backend: %v", profileName, index, total, err))
		a.showLiveError("could not create the browser backend", err)
		return
	}
	// LIFO defers close the browser before removing its files.
	defer func() { _ = backend.Close() }()

	a.log("info", fmt.Sprintf(
		"[%s fresh %d/%d] Opening brand-new %s browser. Old profile data is not used.",
		profileName,
		index,
		total,
		settings.Backend,
	))
	if selectedProxy != nil {
		a.log("info", fmt.Sprintf("[%s fresh %d/%d] Using proxy %s", profileName, index, total, selectedProxy.Label()))
	}

	if err := backend.Start(ctx, browser.Options{
		StartURL:            "https://app.chitchat.gg/text?auth=email",
		UserDataDir:         userDataDir,
		Headless:            settings.Headless,
		DisablePageRecovery: true,
		Proxy:               selectedProxy,
	}); err != nil {
		a.log("error", fmt.Sprintf("[%s fresh %d/%d] Browser start failed: %v", profileName, index, total, err))
		a.showLiveError("browser start failed", err)
		return
	}
	openedDetail := "fresh session; manual login"
	if settings.UseSavedSession {
		openedDetail = "checking the saved profile session"
	}
	a.showModeEvent("system", "browser opened", "", openedDetail, false)

	login, restored := model.LoginInfo{}, false
	if settings.LiveChat && settings.UseSavedSession {
		login, restored = a.restoreProfileSession(ctx, backend, profileDir, profileName, settings.Backend)
	}
	if !restored {
		a.showModeEvent("system", "browser opened", "", "waiting for manual login", false)
		var err error
		login, err = a.captureFreshLogin(ctx, backend, settings.Backend, profileName, index, total)
		if err != nil {
			var banned *browser.BannedError
			if errors.As(err, &banned) {
				a.handleBanned(profileDir, profileName, banned.Report)
				return
			}
			if errors.Is(err, browser.ErrPageUnavailable) || errors.Is(err, context.Canceled) {
				a.log("info", fmt.Sprintf("[%s fresh %d/%d] Fresh browser was closed before login was saved.", profileName, index, total))
				a.showModeEvent("system", "browser closed before login was saved", "", "", false)
				return
			}
			a.log("error", fmt.Sprintf("[%s fresh %d/%d] Fresh login capture failed: %v", profileName, index, total, err))
			a.showLiveError("login capture failed", err)
			return
		}

		login.Profile = profileName
		login.Backend = settings.Backend
		login.UserID = login.CurrentUserID()
		login.SavedAt = time.Now().UTC()
		loginPath := a.freshLoginInfoPath(profileDir, login, index)

		if err := store.SaveJSON(loginPath, login); err != nil {
			a.log("error", fmt.Sprintf("[%s fresh %d/%d] Failed to save login info: %v", profileName, index, total, err))
			a.showLiveError("could not save the login session", err)
			return
		}

		a.log("info", fmt.Sprintf(
			"[%s fresh %d/%d] Login saved to %s. Temporary browser data will be removed after close.",
			profileName,
			index,
			total,
			loginPath,
		))
		a.showModeEvent("system", "login saved", "", loginPath, false)
	}

	// Show Mode: the browser stays open and chats in place. This blocks until
	// the user closes the browser window or the run is cancelled; the deferred
	// backend.Close above then performs the usual cleanup.
	if settings.LiveChat {
		a.showModeEvent("system", "chat watcher starting", "", "", false)
		a.runLiveChat(ctx, backend, login, profileName, index, total, proxyLabel)
	}
}

// restoreProfileSession injects the newest saved login for this profile into
// the fresh browser. Failure is not fatal: the caller falls back to manual
// login, so a stale/expired session never blocks a run.
func (a *App) restoreProfileSession(ctx context.Context, backend browser.Backend, profileDir, profileName, backendName string) (model.LoginInfo, bool) {
	login, path, ok := latestProfileLogin(profileDir)
	if !ok {
		return model.LoginInfo{}, false
	}
	seeder, ok := backend.(browser.SessionSeeder)
	if !ok {
		a.log("info", fmt.Sprintf("[%s] Saved session found, but %s does not support session restore; manual login is required.", profileName, backendName))
		return model.LoginInfo{}, false
	}
	a.showModeEvent("system", "saved session found", "", filepath.Base(path)+" · restoring", false)
	if err := seeder.SeedSession(ctx, login); err != nil {
		a.log("error", fmt.Sprintf("[%s] Saved session restore failed: %v; falling back to manual login.", profileName, err))
		a.showModeEvent("system", "session restore failed", "", "manual login required", false)
		return model.LoginInfo{}, false
	}
	ready, err := browser.SessionAuthenticated(ctx, backend, "https://api.chitchat.gg")
	if err != nil || !ready {
		a.log("info", fmt.Sprintf("[%s] Saved session is no longer valid; manual login is required.", profileName))
		a.showModeEvent("system", "saved session expired", "", "manual login required", false)
		return model.LoginInfo{}, false
	}
	login.Profile = profileName
	login.Backend = backendName
	login.UserID = login.CurrentUserID()
	a.log("info", fmt.Sprintf("[%s] Session restored from %s; no manual login needed.", profileName, path))
	a.showModeEvent("system", "session restored", "", "auto-login from saved profile", false)
	return login, true
}

func (a *App) captureFreshLogin(
	ctx context.Context,
	backend browser.Backend,
	backendName string,
	profileName string,
	index int,
	total int,
) (model.LoginInfo, error) {
	waitingForTokenLogged := false
	waitingForSessionLogged := false

	for {
		if checker, ok := backend.(browser.BanChecker); ok {
			if report, banErr := checker.CheckBan(ctx); banErr == nil && report.Banned {
				return model.LoginInfo{}, &browser.BannedError{Report: report}
			}
		}
		login, err := backend.CaptureSession(ctx)
		if err != nil {
			return model.LoginInfo{}, err
		}

		token := login.Token()
		if token != "" {
			ready, authErr := browser.SessionAuthenticated(ctx, backend, "https://api.chitchat.gg")
			if authErr == nil && ready {
				login.UserID = login.CurrentUserID()
				if login.UserID == "" {
					return model.LoginInfo{}, fmt.Errorf("login detected, but current bot user ID could not be read from token or browser storage")
				}
				return login, nil
			}
		}

		if token == "" && !waitingForTokenLogged {
			a.log("info", fmt.Sprintf(
				"[%s fresh %d/%d] Waiting for login token in the new %s browser window.",
				profileName,
				index,
				total,
				backendName,
			))
			waitingForTokenLogged = true
		}

		if token != "" && !waitingForSessionLogged {
			a.log("info", fmt.Sprintf(
				"[%s fresh %d/%d] Token detected, but the session is still unauthorized. Finish the full login in the %s browser window.",
				profileName,
				index,
				total,
				backendName,
			))
			waitingForSessionLogged = true
		}

		select {
		case <-ctx.Done():
			return model.LoginInfo{}, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// ProfileOption is safe profile metadata for the UI: never cookies or tokens.
type ProfileOption struct {
	Name       string `json:"name"`
	HasSession bool   `json:"has_session"`
	SavedAt    string `json:"saved_at,omitempty"`
}

// ListProfiles lists the saved browser-profile directories so the UI can show
// a picker. A profile with a saved login is auto-restored by Run Browser.
func (a *App) ListProfiles() ([]ProfileOption, error) {
	profilesDir := filepath.Join(a.dataDir, "profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	options := make([]ProfileOption, 0, len(entries)+1)
	seen := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		seen[name] = true
		option := ProfileOption{Name: name}
		if login, path, ok := latestProfileLogin(filepath.Join(profilesDir, name)); ok {
			option.HasSession = true
			if !login.SavedAt.IsZero() {
				option.SavedAt = login.SavedAt.Local().Format("15:04")
			} else if info, statErr := os.Stat(path); statErr == nil {
				option.SavedAt = info.ModTime().Format("15:04")
			}
		}
		options = append(options, option)
	}
	if !seen["default"] {
		options = append(options, ProfileOption{Name: "default"})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Name == "default" {
			return true
		}
		if options[j].Name == "default" {
			return false
		}
		return options[i].Name < options[j].Name
	})
	return options, nil
}

// latestProfileLogin returns the newest saved login that still has a token.
func latestProfileLogin(profileDir string) (model.LoginInfo, string, bool) {
	paths := loginInfoFiles(profileDir)
	for _, path := range paths {
		var login model.LoginInfo
		ok, err := store.LoadJSON(path, &login)
		if err != nil || !ok || login.Token() == "" {
			continue
		}
		return login, path, true
	}
	return model.LoginInfo{}, "", false
}

func loginInfoFiles(profileDir string) []string {
	matches, _ := filepath.Glob(filepath.Join(profileDir, "login_info*.json"))
	sort.Slice(matches, func(i, j int) bool {
		left, leftErr := os.Stat(matches[i])
		right, rightErr := os.Stat(matches[j])
		if leftErr != nil || rightErr != nil {
			return matches[i] > matches[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	return matches
}

// handleBanned stops the current run, reports the site's ban details and
// deletes the profile's saved session so the banned account is not retried.
func (a *App) handleBanned(profileDir, profileName string, report browser.BanReport) {
	detail := report.Summary()
	a.log("error", fmt.Sprintf("[%s] BANNED | %s", profileName, detail))
	a.showModeEvent("error", "BANNED — account suspended", "", detail, false)
	paths := loginInfoFiles(profileDir)
	targets := paths
	if report.UserID != "" {
		targets = make([]string, 0, 1)
		for _, path := range paths {
			var login model.LoginInfo
			ok, err := store.LoadJSON(path, &login)
			if err != nil || !ok {
				continue
			}
			if strings.EqualFold(login.CurrentUserID(), report.UserID) {
				targets = append(targets, path)
			}
		}
	}
	removed := 0
	for _, path := range targets {
		if err := os.Remove(path); err == nil {
			removed++
		} else {
			a.log("error", fmt.Sprintf("[%s] Could not delete banned session %s: %v", profileName, path, err))
		}
	}
	a.log("warn", fmt.Sprintf("[%s] Removed %d saved session file(s) after ban (user=%s).", profileName, removed, report.UserID))
	a.showModeEvent("system", "saved session deleted", "", fmt.Sprintf("%s · %d file(s) removed", profileName, removed), false)
}

func (a *App) freshLoginInfoPath(loginDir string, login model.LoginInfo, index int) string {
	userID := sanitizeFreshFileSegment(login.CurrentUserID())
	if userID == "" {
		userID = fmt.Sprintf("browser-%02d", index)
	}

	stamp := fmt.Sprintf("%s-%09d", login.SavedAt.UTC().Format("20060102-150405"), login.SavedAt.UTC().Nanosecond())
	filename := fmt.Sprintf("login_info_%s_%s_%02d.json", userID, stamp, index)
	return filepath.Join(loginDir, filename)
}

func sanitizeFreshFileSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	value = strings.ReplaceAll(value, "..", "_")
	value = strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", " ", "_").Replace(value)
	return strings.Trim(value, "._")
}

func (a *App) removeFreshBrowserData(tempDir string, profileName string, index int, total int) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if err := os.RemoveAll(tempDir); err == nil {
			a.log("info", fmt.Sprintf(
				"[%s fresh %d/%d] Removed temporary browser data from %s",
				profileName,
				index,
				total,
				tempDir,
			))
			return
		} else {
			lastErr = err
			time.Sleep(300 * time.Millisecond)
		}
	}

	if lastErr != nil {
		a.log("warn", fmt.Sprintf(
			"[%s fresh %d/%d] Could not remove temporary browser data from %s: %v",
			profileName,
			index,
			total,
			tempDir,
			lastErr,
		))
	}
}

func (a *App) loadProxyRotator() (*proxy.Rotator, int, error) {
	entries, err := a.loadProxyEntries()
	if err != nil {
		return nil, 0, err
	}
	if len(entries) == 0 {
		return nil, 0, nil
	}
	return proxy.NewRotator(entries), len(entries), nil
}

func (a *App) loadProxyEntries() ([]proxy.Entry, error) {
	proxySettings, err := a.uiStore.LoadProxySettings()
	if err != nil {
		return nil, err
	}
	if !proxySettings.Enabled {
		return nil, nil
	}

	entries, err := proxy.ParseList(proxySettings.Entries)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("proxy list is enabled but empty")
	}

	return entries, nil
}

func (a *App) emitStatus(running bool) {
	a.emit("ui:running", running)
}

func (a *App) emit(name string, data any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, name, data)
}

func (a *App) log(level string, message string) {
	// Persist first, independently of Badger/UI events; no Home visibility filter.
	disk := applog.Logger{Path: applog.Path(a.dataDir), Source: "app"}
	disk.Log(level, message)
	entry, err := a.uiStore.AppendLog(level, message)
	if err == nil {
		a.emit("ui:log", entry) // Browser status still consumes this event.
	} else {
		disk.Error(fmt.Sprintf("UI log persistence failed: %v", err))
	}
}

// --- CrushFling Auto Register (delegates to internal/crushfling Service) ---

func (a *App) GetCrushConfig() crushfling.RegisterConfig {
	if a.crushService == nil {
		return crushfling.DefaultRegisterConfig(filepath.Join(a.dataDir, "crushfling"))
	}
	return a.crushService.GetConfig()
}
func (a *App) GetCrushStats() crushfling.RegistrationStats {
	if a.crushService == nil {
		return crushfling.RegistrationStats{}
	}
	return a.crushService.GetStats()
}
func (a *App) StartCrushBulk(cfg crushfling.RegisterConfig) error {
	if a.crushService == nil {
		return fmt.Errorf("crushfling service not initialized")
	}
	return a.crushService.StartBulk(cfg)
}
func (a *App) StopCrushBulk() error {
	if a.crushService == nil {
		return fmt.Errorf("crushfling service not initialized")
	}
	return a.crushService.StopBulk()
}
func (a *App) GetCrushLogs(limit int) []string {
	if a.crushService == nil {
		return nil
	}
	return a.crushService.GetLogs(limit)
}
func (a *App) GetPictureCount() (int, error) {
	if a.crushService == nil {
		return 0, fmt.Errorf("not initialized")
	}
	return a.crushService.GetPictureCount()
}
func (a *App) GetCaptionCount() (int, error) {
	if a.crushService == nil {
		return 0, fmt.Errorf("not initialized")
	}
	return a.crushService.GetCaptionCount()
}
func (a *App) PreviewCrushProfile() crushfling.ProfileData {
	if a.crushService == nil {
		return crushfling.ProfileData{}
	}
	return a.crushService.PreviewProfile()
}
func (a *App) ListCrushAccounts() ([]crushfling.AccountResult, error) {
	if a.crushService == nil {
		return nil, fmt.Errorf("not initialized")
	}
	return a.crushService.ListAccounts()
}
func (a *App) OpenCrushflingDataFolder() (string, error) {
	dir := filepath.Join(a.dataDir, "crushfling")
	if a.crushService != nil {
		dir = a.crushService.DataDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(dir)
	_ = exec.Command("explorer.exe", abs).Start()
	return "CrushFling data folder: " + abs, nil
}

func (a *App) GetCrushDebugInfo() crushfling.DebugInfo {
	if a.crushService == nil {
		return crushfling.DebugInfo{DataDir: filepath.Join(a.dataDir, "crushfling")}
	}
	return a.crushService.GetDebugInfo()
}

func (a *App) GetCrushLogPath() string {
	if a.crushService == nil {
		return ""
	}
	return a.crushService.LogPath()
}

// CreateCrushDebugReport writes a redacted support report next to the failure
// dumps and returns its path.
func (a *App) CreateCrushDebugReport() (string, error) {
	if a.crushService == nil {
		return "", fmt.Errorf("crushfling service not initialized")
	}
	return a.crushService.CreateDebugReport()
}

// OpenCrushLogFolder opens <data>/crushfling/logs in Explorer.
func (a *App) OpenCrushLogFolder() (string, error) {
	dir := filepath.Join(a.dataDir, "crushfling", "logs")
	if a.crushService != nil {
		dir = filepath.Join(a.crushService.DataDir(), "logs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(dir)
	_ = exec.Command("explorer.exe", abs).Start()
	return "CrushFling logs: " + abs, nil
}

// --- CrushFling dashboard helpers (professional panel) ---------------------

// PickCrushPictureFolder lets the operator choose the pictures folder with a
// native dialog. Returns "" when the dialog is cancelled.
func (a *App) PickCrushPictureFolder() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select the folder with profile pictures",
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

// PickCrushBioFile selects a text file and imports it as <data>/bio.txt.
// One non-empty line = one bio; the imported list is de-duplicated.
func (a *App) PickCrushBioFile() (crushfling.BioInfo, error) {
	if a.crushService == nil {
		return crushfling.BioInfo{}, fmt.Errorf("crushfling service not initialized")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select the bio list (one line per bio)",
		Filters: []runtime.FileFilter{
			{DisplayName: "Text files (*.txt)", Pattern: "*.txt"},
		},
	})
	if err != nil {
		return crushfling.BioInfo{}, err
	}
	if strings.TrimSpace(path) == "" {
		return crushfling.BioInfo{}, nil
	}
	return a.crushService.ImportBioFile(path)
}

// GetCrushBioInfo summarises the active bio list (path, remaining, preview).
func (a *App) GetCrushBioInfo() (crushfling.BioInfo, error) {
	if a.crushService == nil {
		return crushfling.BioInfo{}, fmt.Errorf("crushfling service not initialized")
	}
	return a.crushService.GetBioInfo()
}

// ImportCrushBioFile imports a bio file by path (no dialog), used by tests and
// by the CLI-facing UI when the operator pastes a path.
func (a *App) ImportCrushBioFile(path string) (crushfling.BioInfo, error) {
	if a.crushService == nil {
		return crushfling.BioInfo{}, fmt.Errorf("crushfling service not initialized")
	}
	return a.crushService.ImportBioFile(path)
}

// PickCrushProxyFile selects a proxy list file.
func (a *App) PickCrushProxyFile() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select the proxy list",
		Filters: []runtime.FileFilter{
			{DisplayName: "Proxy lists (*.txt)", Pattern: "*.txt"},
		},
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// ValidateCrushProxies parses the proxy list exactly like a run would and
// reports how many entries are usable. It never makes a network request.
func (a *App) ValidateCrushProxies(filePath, rawList string) crushfling.ProxyCheck {
	if a.crushService == nil {
		return crushfling.ProxyCheck{Errors: []string{"crushfling service not initialized"}}
	}
	return a.crushService.ValidateProxies(filePath, rawList)
}

// OpenCrushPicturesFolder opens the picture folder in the OS file manager
// (Explorer on Windows, xdg-open/open elsewhere so the same build is usable in
// development).
func (a *App) OpenCrushPicturesFolder() (string, error) {
	dir := filepath.Join(a.dataDir, "crushfling", "pictures")
	if a.crushService != nil {
		dir = filepath.Join(a.crushService.DataDir(), "pictures")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(dir)
	if err := openPathInOS(abs); err != nil {
		return "CrushFling pictures: " + abs, err
	}
	return "CrushFling pictures: " + abs, nil
}

// openPathInOS opens a folder (or file) with the platform's default handler.
func openPathInOS(path string) error {
	switch stdruntime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

// CloseCrushBrowsers kills every browser the worker opened. The Stop button
// calls StopBulk (which already does this); this binding is for the explicit
// "Close browsers" control.
func (a *App) CloseCrushBrowsers() int {
	return crushfling.CloseAllBrowsers()
}

// GetCrushBrowsersOpen reports how many browsers are registered right now.
func (a *App) GetCrushBrowsersOpen() int {
	return crushfling.OpenBrowserCount()
}

// GetCrushSummary is the single call the dashboard polls for its counters.
type CrushSummary struct {
	Running     bool                         `json:"running"`
	Pictures    int                          `json:"pictures"`
	Bios        int                          `json:"bios"`
	Accounts    int                          `json:"accounts"`
	Verified    int                          `json:"verified"`
	Browsers    int                          `json:"browsers"`
	BioFile     string                       `json:"bio_file"`
	PictureDir  string                       `json:"picture_dir"`
	DataDir     string                       `json:"data_dir"`
	AccountsCSV string                       `json:"accounts_csv"`
	Stats       crushfling.RegistrationStats `json:"stats"`
}

// GetCrushSummary returns everything the dashboard header shows in one call.
func (a *App) GetCrushSummary() CrushSummary {
	summary := CrushSummary{Browsers: crushfling.OpenBrowserCount()}
	if a.crushService == nil {
		return summary
	}
	summary.DataDir = a.crushService.DataDir()
	summary.Stats = a.crushService.GetStats()
	summary.Running = summary.Stats.Running
	if n, err := a.crushService.GetPictureCount(); err == nil {
		summary.Pictures = n
	}
	if info, err := a.crushService.GetBioInfo(); err == nil {
		summary.Bios = info.Remaining
		summary.BioFile = info.Path
	}
	if accounts, err := a.crushService.ListAccounts(); err == nil {
		summary.Accounts = len(accounts)
		for _, account := range accounts {
			if account.Verified {
				summary.Verified++
			}
		}
	}
	summary.PictureDir = filepath.Join(a.crushService.DataDir(), "pictures")
	summary.AccountsCSV = filepath.Join(a.crushService.DataDir(), "accounts.csv")
	return summary
}

// Compatibility aliases used by the dashboard overlay (short names).
func (a *App) GetStats() crushfling.RegistrationStats            { return a.GetCrushStats() }
func (a *App) StartBulk(cfg crushfling.RegisterConfig) error     { return a.StartCrushBulk(cfg) }
func (a *App) StopBulk() error                                   { return a.StopCrushBulk() }
func (a *App) GetLogs(limit int) []string                        { return a.GetCrushLogs(limit) }
func (a *App) ListAccounts() ([]crushfling.AccountResult, error) { return a.ListCrushAccounts() }
func (a *App) PreviewProfile() crushfling.ProfileData            { return a.PreviewCrushProfile() }

// Separate read-only display API; settings/stats/logs contracts are unchanged.
func (a *App) GetLiveChatSnapshot(sessionID string) homeChatSnapshot {
	return a.homeChatFeed().snapshot(sessionID)
}
