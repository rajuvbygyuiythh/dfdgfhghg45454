package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"crushfling/internal/crushfling"
	"crushfling/internal/proxy"
)

func main() {
	var (
		count         = flag.Int("count", 1, "how many accounts to create")
		concurrency   = flag.Int("concurrency", 1, "parallel workers")
		headless      = flag.Bool("headless", false, "run browser headless")
		useBrowser    = flag.Bool("browser", false, "use browser fallback for magic link")
		browserVerify = flag.Bool("browser-verify", true, "click Sign In in a browser if the link sets no session cookie")
		proxyFile     = flag.String("proxy", "", "path to proxy file (host:port:user:pass per line)")
		proxyList     = flag.String("proxy-list", "", "raw proxy lines")
		emailProvider = flag.String("email", "mailtm", "email provider: mailtm|1secmail|guerrilla|custom")
		customEmail   = flag.String("custom-email", "", "custom catch-all email (for custom provider)")
		mailHook      = flag.String("mail-hook", "", "HTTP endpoint returning messages for ?email=... (custom provider)")
		dataDir       = flag.String("data", "crushfling/data", "data directory (pictures/caption.txt/accounts.json)")
		avatarDir     = flag.String("avatars", "", "avatar pictures directory (default crushfling/data/pictures)")
		output        = flag.String("output", "", "output json file (default dataDir/accounts.json)")
		captchaKey    = flag.String("captcha", "", "2captcha API key for Turnstile")
		manualLink    = flag.Bool("manual-link", false, "after requesting, paste the magic link in this terminal (same session)")
		pastedLink    = flag.String("link", "", "use this already-received magic link (implies count=1)")
		verbose       = flag.Bool("v", false, "verbose logging (same as --debug)")
		checkOnly     = flag.Bool("check", false, "validate the data folder (caption.txt, pictures, api.json) and exit")
		probeOnly     = flag.Bool("probe", false, "inspect the live account page and print the detected sign-up flow (no registrations)")
		debugMode     = flag.Bool("debug", false, "debug logs + data/debug/fail-*.json dumps")
		traceHTTP     = flag.Bool("trace-http", false, "record HTTP bodies in data/debug/trace-*.jsonl (implies --debug)")
		baseURL       = flag.String("base-url", "", "site origin to use (default https://crushfling.com); for testing against a mock")
		bioFile       = flag.String("bio", "", "import this text file as <data>/bio.txt (one bio per line) before the run")
		showProfile   = flag.Bool("preview", false, "print a generated profile (name, bio, country, age) and exit")
		showHelp      = flag.Bool("help", false, "show help")
	)
	flag.Parse()

	if strings.TrimSpace(*baseURL) != "" {
		if err := crushfling.SetBaseURL(*baseURL); err != nil {
			log.Fatalf("--base-url: %v", err)
		}
	}

	if *showHelp {
		printHelp()
		return
	}

	if *avatarDir == "" {
		*avatarDir = filepath.Join(*dataDir, "pictures")
	}
	if *output == "" {
		*output = filepath.Join(*dataDir, "accounts.json")
	}

	if *probeOnly {
		if err := runProbe(*dataDir, *proxyFile, *proxyList); err != nil {
			log.Fatalf("probe failed: %v", err)
		}
		return
	}

	// Importing a bio list is a cheap offline step and must happen before
	// --check/--preview, which read that list.
	if strings.TrimSpace(*bioFile) != "" {
		info, err := crushfling.ImportBioFile(*dataDir, *bioFile)
		if err != nil {
			log.Fatalf("--bio: %v", err)
		}
		fmt.Printf("Bio list loaded: %s (%d usable line(s))\n", info.Path, info.Total)
	}

	if *showProfile {
		profile := crushfling.GenerateProfile("preview@example.test")
		if info, err := crushfling.ReadBioInfo(*dataDir); err == nil && len(info.Preview) > 0 {
			line := info.Preview[0]
			line = strings.ReplaceAll(line, "@myhandle", "@"+profile.PlatformUsername)
			line = strings.ReplaceAll(line, "{username}", profile.PlatformUsername)
			profile.Bio = line
		}
		out, _ := json.MarshalIndent(profile, "", "  ")
		fmt.Println(string(out))
		return
	}

	if *checkOnly {
		if err := runCheck(*dataDir); err != nil {
			log.Fatalf("check failed: %v", err)
		}
		return
	}

	fmt.Print(Banner)
	fmt.Printf("CrushFling Auto Register - %d accounts | site=%s | provider=%s | proxy=%s | concurrency=%d\n",
		*count, crushfling.BaseURL, *emailProvider, firstNonEmpty(*proxyFile, *proxyList, "none"), *concurrency)

	cfg := crushfling.DefaultRegisterConfig(*dataDir)
	cfg.Count = *count
	cfg.Concurrency = *concurrency
	cfg.Headless = *headless
	cfg.UseBrowser = *useBrowser
	cfg.BrowserVerify = *browserVerify
	cfg.ProxyFile = *proxyFile
	cfg.ProxyEntries = *proxyList
	cfg.EmailProvider = *emailProvider
	cfg.CustomEmail = *customEmail
	cfg.CustomEmailHook = *mailHook
	cfg.AvatarDir = *avatarDir
	cfg.CaptchaAPIKey = *captchaKey
	cfg.OutputFile = *output
	cfg.ManualLink = *manualLink
	cfg.PastedLink = *pastedLink
	cfg.Debug = *debugMode || *verbose || *traceHTTP
	cfg.TraceHTTP = *traceHTTP
	if cfg.ManualLink || cfg.PastedLink != "" {
		// One link = one account, and stdin prompts cannot be parallelised.
		cfg.Count = 1
		cfg.Concurrency = 1
		fmt.Println("Manual/pasted link mode: count=1, concurrency=1")
	}

	// Ensure the data layout (caption.txt template, pictures/used/, README).
	if err := crushfling.EnsureDataLayout(*dataDir); err != nil {
		log.Fatalf("prepare data folder: %v", err)
	}
	fmt.Printf("Data folder: %s\n", *dataDir)

	reg, err := crushfling.NewRegister(cfg, func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if *verbose {
			log.Print(msg)
		} else {
			fmt.Println(msg)
		}
	})
	if err != nil {
		log.Fatalf("init failed: %v", err)
	}
	defer reg.Close()
	if path := reg.LogPath(); path != "" {
		fmt.Printf("Run log: %s\n", path)
	}
	if path := reg.TracePath(); path != "" {
		fmt.Printf("HTTP trace: %s\n", path)
	}
	if cfg.Debug {
		fmt.Printf("Debug mode: failure dumps go to %s\n", filepath.Join(*dataDir, "debug"))
	}

	// Ctrl+C must also close Chromium: a cancelled context alone leaves the
	// windows on screen until their own timeouts fire.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*count)*3*time.Minute+10*time.Minute)
	defer cancel()
	stopSignals := make(chan os.Signal, 1)
	signal.Notify(stopSignals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopSignals
		fmt.Println("\nStopping: closing browsers…")
		crushfling.CloseAllBrowsers()
		cancel()
	}()

	start := time.Now()
	if err := reg.RunBulk(ctx); err != nil {
		log.Printf("bulk error: %v", err)
	}
	elapsed := time.Since(start)
	stats := reg.Stats()
	fmt.Printf("\n=== Done in %s ===\nSuccess: %d / Failed: %d / Total: %d\nAccounts saved to: %s\n",
		elapsed.Round(time.Second), stats.Success, stats.Failed, cfg.Count, cfg.OutputFile)
	if len(stats.ByCode) > 0 {
		keys := make([]string, 0, len(stats.ByCode))
		for code := range stats.ByCode {
			keys = append(keys, code)
		}
		sort.Strings(keys)
		fmt.Print("Failure codes: ")
		for i, code := range keys {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%s=%d", code, stats.ByCode[code])
		}
		fmt.Println()
	}
	if stats.LastError != "" {
		fmt.Printf("Last error: %s\n", stats.LastError)
	}
	if path := reg.LogPath(); path != "" {
		fmt.Printf("Run log: %s\n", path)
	}

	// Exit non-zero when nothing succeeded: a script or a CI job must be able
	// to tell a working run from a failed one without parsing the log.
	if stats.Success == 0 && stats.Failed > 0 {
		os.Exit(1)
	}

	// Export CSV too
	if store := crushfling.NewAccountStore(*dataDir); store.Count() > 0 {
		csvPath := filepath.Join(*dataDir, "accounts.csv")
		if err := store.ExportCSV(csvPath); err != nil {
			log.Printf("csv export: %v", err)
		} else {
			fmt.Printf("CSV exported to: %s\n", csvPath)
		}
	}
}

// runProbe performs one GET of the sign-in page and prints how that page asks
// the server to send the magic link. It never creates an account, so it is the
// safe first command on a new machine: it replaces guessing endpoint paths
// with evidence from the page itself.
func runProbe(dataDir, proxyFile, proxyList string) error {
	fmt.Print(Banner)
	fmt.Printf("Probing %s (no registration is performed)\n", crushfling.AccountURL)

	proxyURL := ""
	var entries []proxy.Entry
	if strings.TrimSpace(proxyList) != "" {
		parsed, err := proxy.ParseList(proxyList)
		if err != nil {
			return fmt.Errorf("proxy list: %w", err)
		}
		entries = parsed
	} else if strings.TrimSpace(proxyFile) != "" {
		raw, err := os.ReadFile(proxyFile)
		if err != nil {
			return fmt.Errorf("proxy file: %w", err)
		}
		parsed, err := proxy.ParseList(string(raw))
		if err != nil {
			return fmt.Errorf("proxy file: %w", err)
		}
		entries = parsed
	}
	if len(entries) > 0 {
		proxyURL = entries[0].TransportURL().String()
		fmt.Printf("Proxy: %s\n", crushfling.Redact(proxyURL))
	}

	sess, err := crushfling.NewSession("", proxyURL)
	if err != nil {
		return err
	}
	endpoints, err := crushfling.LoadEndpointConfig(dataDir)
	if err != nil {
		return fmt.Errorf("api.json: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	profile, err := crushfling.DiscoverSite(ctx, sess, endpoints)
	if err != nil {
		fmt.Println("FAILED:", err)
		fmt.Println("A Cloudflare challenge cannot be answered over plain HTTP; rerun the")
		fmt.Println("worker with --browser (Rod) or use the dashboard's browser fallback.")
		return err
	}

	fmt.Printf("\nStyle         : %s\n", profile.Style)
	fmt.Printf("Account page  : %s\n", profile.AccountPath)
	fmt.Printf("CSRF token    : %s\n", tokenState(profile.CSRFToken))
	fmt.Printf("Form action   : %s (%s)\n", firstNonEmpty(profile.FormAction, "none"), firstNonEmpty(profile.FormMethod, "-"))
	fmt.Printf("Email field   : %s\n", firstNonEmpty(profile.EmailField, "email"))
	if len(profile.ExtraFields) > 0 {
		names := make([]string, 0, len(profile.ExtraFields))
		for name := range profile.ExtraFields {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Printf("Hidden fields : %s\n", strings.Join(names, ", "))
	}
	fmt.Printf("Livewire      : path=%s method=%s snapshot=%d bytes\n",
		firstNonEmpty(profile.LivewirePath, "-"), firstNonEmpty(profile.LivewireMethod, "-"), len(profile.LivewireSnapshot))
	fmt.Printf("Upload paths  : %s\n", firstNonEmpty(strings.Join(profile.UploadPaths, ", "), "none"))
	fmt.Printf("Profile paths : %s\n", firstNonEmpty(strings.Join(profile.ProfilePaths, ", "), "none"))
	fmt.Printf("Cookies seen  : %s\n", firstNonEmpty(cookieNameList(sess.Cookies()), "none"))

	fmt.Println("\nEvidence:")
	for _, signal := range profile.Signals {
		fmt.Println("  -", signal)
	}

	switch profile.Style {
	case crushfling.AuthStyleLivewire:
		fmt.Printf("\nThe worker will submit the Livewire component (%s on %s).\n",
			firstNonEmpty(profile.LivewireMethod, "auto"), firstNonEmpty(profile.LivewirePath, "/livewire/update"))
	case crushfling.AuthStyleForm:
		fmt.Printf("\nThe worker will post the page's form to %s.\n", firstNonEmpty(profile.FormAction, profile.AccountPath))
	default:
		fmt.Println("\nNo form or Livewire component was found; the worker will try the JSON")
		fmt.Println("candidates from api.json and then the browser (Rod) fallback.")
	}
	fmt.Println("\nNext: crushfling --count 1 --email mailtm --debug --trace-http")
	return nil
}

// orAuto labels an empty path as "auto", because empty means the path is
// discovered from the account page instead of being configured.
func orAuto(value string) string {
	if strings.TrimSpace(value) == "" {
		return "auto"
	}
	return value
}

func tokenState(token string) string {
	if strings.TrimSpace(token) == "" {
		return "missing"
	}
	return fmt.Sprintf("present (%d chars)", len(token))
}

func cookieNameList(cookies map[string]string) string {
	names := make([]string, 0, len(cookies))
	for name := range cookies {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// runCheck validates the local data folder without any network call. It is the
// first thing to run on a new machine: most "it does not work" reports are a
// missing picture folder, an empty caption.txt or a broken api.json.
func runCheck(dataDir string) error {
	fmt.Print(Banner)
	fmt.Printf("Checking data folder: %s\n", dataDir)
	if err := crushfling.EnsureDataLayout(dataDir); err != nil {
		return fmt.Errorf("data layout: %w", err)
	}
	endpoints, err := crushfling.LoadEndpointConfig(dataDir)
	if err != nil {
		return fmt.Errorf("api.json: %w", err)
	}
	bioPath := crushfling.BioPath(dataDir)
	captions, err := crushfling.NewCaptionManager(bioPath)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(bioPath), err)
	}
	pictures, err := crushfling.NewPictureManager(filepath.Join(dataDir, "pictures"))
	if err != nil {
		return fmt.Errorf("pictures: %w", err)
	}
	store := crushfling.NewAccountStore(dataDir)
	logsDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return fmt.Errorf("logs folder: %w", err)
	}

	fmt.Println("OK  data layout")
	fmt.Printf("OK  api.json       csrf=%s magic=%s upload=%s\n",
		orAuto(endpoints.CSRFPath), orAuto(endpoints.MagicLinkPath), orAuto(endpoints.UploadPath))
	fmt.Printf("OK  profile paths  %s\n", strings.Join(endpoints.ProfilePaths, ", "))
	fmt.Printf("OK  bio list       %s: %d usable line(s)\n", filepath.Base(bioPath), captions.Count())
	fmt.Printf("OK  pictures       %d file(s) in %s\n", pictures.Count(), filepath.Join(dataDir, "pictures"))
	fmt.Printf("OK  accounts       %d saved (%d verified)\n", store.Count(), store.SuccessCount())
	fmt.Printf("OK  logs           %s\n", logsDir)

	accountsPath := filepath.Join(dataDir, "accounts.json")
	if raw, err := os.ReadFile(accountsPath); err == nil {
		var probe []json.RawMessage
		if err := json.Unmarshal(raw, &probe); err != nil {
			return fmt.Errorf("%s is not valid JSON: %w", accountsPath, err)
		}
	}
	legacy := crushfling.LegacyNextAuthPaths()
	for _, path := range append([]string{endpoints.CSRFPath, endpoints.MagicLinkPath}, endpoints.MagicLinkAltPaths...) {
		for _, old := range legacy {
			if strings.TrimSpace(path) == old {
				fmt.Printf("WARN api.json still contains the NextAuth path %s - the live\n", old)
				fmt.Println("     site answers that with its 404 page. Delete api.json (it is")
				fmt.Println("     recreated) or run --probe to see the real flow.")
			}
		}
	}
	if strings.TrimSpace(endpoints.MagicLinkPath) == "" {
		fmt.Println("NOTE magic-link path is empty: it is discovered from the account page.")
		fmt.Println("     Run `crushfling --probe` on a machine that can reach the site.")
	}

	if captions.Count() == 0 {
		return fmt.Errorf("caption.txt has no usable lines - add one caption per line")
	}
	if pictures.Count() == 0 {
		fmt.Println("WARN no pictures - accounts will be created without an avatar")
	}
	fmt.Println("\nReady. Next: crushfling --count 1 --email mailtm --debug --trace-http")
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return "none"
}

func printHelp() {
	fmt.Print(Banner)
	fmt.Println(`Usage: crushfling [flags]

Examples:
  # Create 5 accounts with mail.tm (default)
  crushfling --count 5 --email mailtm

  # Bulk with proxies and concurrency
  crushfling --count 20 --proxy proxies.txt --concurrency 3 --email 1secmail

  # Use custom catch-all domain, then paste the link that arrives there
  crushfling --email custom --custom-email "test@mydomain.com" --manual-link

  # Custom domain with your own mail API (?email=<address> -> JSON/text)
  crushfling --email custom --custom-email me@mydomain.com --mail-hook http://127.0.0.1:8081/inbox

  # Use a link you already received (clicked in the same session)
  crushfling --email mailtm --link "https://crushfling.com/...?token=..."

  # Smoke-test the whole flow against a local mock instead of the live site
  crushfling --count 1 --base-url http://127.0.0.1:8099 --manual-link

  # First command on a new machine: what does the sign-in page actually do?
  crushfling --probe

  # Load a bio list (one line per bio, used line is deleted automatically)
  crushfling --bio ./my_bios.txt --count 10

  # What would the next profile look like? (no network, no registration)
  crushfling --preview

  # With pictures and bios
  crushfling --count 10 --avatars ./my_pics --data ./crushfling/data --bio ./bios.txt

  # Offline sanity check (no network): folders, api.json, captions, pictures
  crushfling --check

Flags:`)
	flag.PrintDefaults()
	fmt.Println(`
Data layout:
  crushfling/data/
    pictures/        <- put jpg/png/webp here (one per account, auto moves to used/)
    pictures/used/   <- used pictures moved here
    bio.txt          <- one bio per line, used line auto-deleted
    caption.txt      <- older name, still read when bio.txt is absent
    accounts.json    <- saved accounts (cookies + session)
    accounts.csv     <- CSV export
    logs/            <- one run-*.log per run (redacted)
    debug/           <- fail-*.json dumps, trace-*.jsonl, report-*.txt

Full flow per account:
  1. Create temp inbox (mail.tm/1secmail) -> email
  2. Generate profile (female, looking for male, age 19-24, random name,
     USA + European countries)
  3. Pick caption.txt line (reserved, deleted only after the profile works)
     and a random picture (moved to used/ only after the profile works)
  4. Create HTTP session with cookie jar (same session for the whole flow)
  5. Read /account: CSRF meta token, form action / Livewire component, paths
  6. Submit the email exactly the way the page does (form or Livewire),
     falling back to JSON paths from api.json, then to the Rod browser
  7. Poll inbox for the magic link (poll 3s, timeout 120s)
  8. Click the link IN THE SAME SESSION; confirm by cookie and by re-reading
     /account (the email form disappears when signed in)
  9. Upload the avatar (page paths + 5 field names), create the profile,
     then check the public /p/<username>/ page
 10. Save only verified accounts (cookies, username, bio, proxy)

If the site changes:
  crushfling --probe   prints what the account page really uses today
                       (style, CSRF, form action, Livewire method, paths).
                       Nothing is registered by this command.

Debugging:
  --debug       debug log lines + a redacted JSON dump per failed account
  --trace-http  also record request/response bodies (bodies are truncated
                and token-redacted) in data/debug/trace-*.jsonl
  Every run writes data/logs/run-*.log; the path is printed at startup.
  Open data/debug/ for fail-*.json, trace-*.jsonl and the report.

See docs/CRUSHFLING_AUTO_REGISTER.md for deep API analysis
and docs/CRUSHFLING_DEBUGGING.md for the debugging workflow.`)
}

const Banner = `
  ____                _     _____ _ _             
 / ___|_ __ _   _ ___| |__ |  ___| (_) _ __   __ _ 
| |   | '__| | | / __| '_ \| |_  | | | '_ \ / _' |
| |___| |  | |_| \__ \ | | |  _| | | | | | | (_| |
 \____|_|   \__,_|___/_| |_|_|   |_|_|_| |_|\__, |
                                            |___/ 
       Auto Register Bot - crushfling.com
`
