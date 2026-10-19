# Dashboard — Architecture & Mapping

> **ডকুমেন্টের উদ্দেশ্য:** ড্যাশবোর্ডের প্রতিটা বাটন/ফিল্ড কোথা থেকে আসে, কোথায় যায়, কোন ফাইলে কী হয় — পুরো map + design।
> **কোনো কোড পরিবর্তন করা হয়নি।** এটা শুধু reference document। (2026-10-19)

সাথে পড়ুন: `docs/PROJECT_MAP.md` (auto-generated inventory), `AGENTS.md` (locked scope), `docs/PROJECT_MEMORY.md` (decisions)।

---

## 1. এক নজরে

| প্রশ্ন | উত্তর |
|---|---|
| UI framework | Wails v2 (Go + Windows WebView2), plain HTML/CSS/JS — React/Vue নেই |
| Frontend ফাইল | `cmd/chitchatui/frontend/dist/{index.html, app.js, styles.css}` (embed করা) |
| UI ↔ Go bridge | `window.go.main.App.<Method>()` (auto-generated binding) |
| Go entry | `main.go` → `NewApp` → `wails.Run` |
| Backend API surface | `app.go`-এর exported `*App` method গুলো (মোট 17টা) |
| settings storage | Badger DB → `data/ui/badger`, key: `settings/{browser,message,proxy,general}`, `logs/*` |
| runtime data | `data/profiles/<profile>/*.json` (login, bot_state, derived), `data/eva_chat/` |
| diagnostics | `logs.txt` (data-র **বাইরে**, exe/project root-এ) |
| বাইরের সেবা | `app.chitchat.gg` (UI), `api.chitchat.gg` (API), `license-api.nilhasancl.workers.dev` (license), optional OpenAI-compatible AI |

---

## 2. স্তর (Layers)

```
┌──────────────────────────────────────────────────────────────────────────┐
│ L1  UI  (WebView2)                                                       │
│  index.html  →  app.js (state machine, 3s/1s poll, event listeners)      │
│  styles.css                                                              │
└───────────────────────────────┬──────────────────────────────────────────┘
                                │ window.go.main.App.Method(args)
                                │ ↑ return value / ↑↑ EventsEmit("ui:*")
┌───────────────────────────────┴──────────────────────────────────────────┐
│ L2  Wails binding layer  —  root  app.go  (type App)                     │
│  settings APIs · bot lifecycle · browser launch · proxy test · AI test   │
│  in-memory: running/runID/botCancel/activeWorkers/chatFeed/license state │
└───────────────────────────────┬──────────────────────────────────────────┘
                                │ plain Go calls
┌───────────────────────────────┴──────────────────────────────────────────┐
│ L3  Core packages  (internal/)                                            │
│  workerplan → কোন profile-এ কোন worker + কোন proxy                        │
│  bot        → HTTP API worker loop (relationships, DM, match, reply)      │
│  browser    → rod / chromedp / DOM chat (Show Mode)                       │
│  proxy      → parse, rotator, relay, test                                │
│  store      → JSON file helpers        uistore → Badger settings/logs     │
│  applog     → logs.txt + redaction     license → hardware key + verify    │
│  runtimeassets → bundled WebView2/Chromium paths                          │
│  api        → chitchat HTTP client     model   → shared structs           │
└──────────────────────────────────────────────────────────────────────────┘
```

**Binding-এর দিক:** UI কখনো `internal/` import করে না — শুধু `App` method আর `model.*` struct (JSON) দেখে। তাই backend বদলালে UI contract ভাঙে না, যদি `model.UISnapshot`/`HomeStats` না বদলায়।

---

## 3. Boot sequence

```
main.go
 ├─ resolveDataDir()            → <exe dir>/data
 ├─ applog setup                → logs.txt (MultiWriter: file + stderr)
 ├─ fs.Sub(embedded "cmd/chitchatui/frontend/dist")
 ├─ NewApp(dataDir)  [app.go:45]
 │    ├─ uistore.Open            → data/ui/badger (Badger)
 │    ├─ seedDefaults()          → missing default settings বানায়
 │    └─ license.GenerateHardwareKey() → VerifyLicense()
 │         ├─ valid   → shell দেখাবে
 │         └─ invalid → license overlay (key readonly, copy only)
 │    └─ 24h ticker → re-verify → `ui:license_invalid` event
 └─ wails.Run(...)  1280x820, min 1080x700, dark bg
      ├─ OnStartup  → a.ctx সেট + log "Wails UI ready."
      ├─ OnShutdown → StopBot + uiStore.Close
      └─ Bind: [app]

app.js  (DOMContentLoaded → bootstrap  [app.js:564])
 ├─ bindTabs/bindForms/bindActions/bindEvents
 ├─ refreshState() — 50 বার পর্যন্ত retry (200ms)
 │    └─ GetInitialState() → fillForms() → renderBrowserOptions() → renderHomeStats()
 ├─ setInterval(autoRefreshStats, 3000)     [app.js:576]
 └─ setInterval(refreshChatTranscript, 1000) [app.js:577]
```

**কেন retry:** Wails binding inject হতে দেরি হয়; 50×200ms = 10s window।

---

## 4. Runtime topology — ৩টা আলাদা path

```
A) BOT WORKERS  (Home "Start Bot")
   UI ──StartBot──► buildBotConfigs() ──► DiscoverProfileWorkers()
                                              │ data/profiles/*/login_info*.json
                                              ▼
                                        AssignProfileWorkers()  (proxy round-robin)
                                              ▼
                                    goroutine per profile → bot.Run(ctx,cfg)
                                              ▼
                        HTTP API loop: relationships → accept → DM → match → reply
                                              ▼
                                  data/profiles/<p>/bot_state.json  (stats)

B) FRESH LOGIN  (Browser tab → 01/02 Run)
   UI ──RunBrowser(settings,count)──► validateBrowserRun()
                                        show_mode → rod + LiveChat=true, headless=false
                                              ▼
                                  temp dir (OS temp, repo-র বাইরে)
                                              ▼
                                   backend.Start → manual login
                                              ▼
                                  captureFreshLogin() → data/profiles/<p>/login_info_<uid>_<ts>_<n>.json
                                              ▼
                                  temp browser data delete (retry 5x)

C) SHOW MODE  (Browser tab → 03 Run)
   B-এর পুরো path + settings.LiveChat=true
                                              ▼
                                   runLiveChat() → DOM-only loop
                                   (কোনো WebSocket/API নয়, শুধু পেজের DOM)
                                              ▼
                                   homeChatFeed → Home transcript
```

---

## 5. UI → Backend mapping (নিরাময় tables)

### 5.1 Home panel

| UI control (id) | JS handler (app.js) | Binding | Go (file:line) | পড়ে | লেখে/ফলে |
|---|---|---|---|---|---|
| `start-bot` | `bindActions` 484 | `StartBot` | `app.go:597` | settings, profiles | worker goroutine, `ui:running`, logs |
| `stop-bot` | 489 | `StopBot` | `app.go:675` | — | ctx cancel, `ui:running(false)` |
| `clean-profiles` | 494 | `CleanProfiles` | `app.go:578` → `removeProfileData` `709` | `running` flag | `data/profiles`, `data/tmp`, `data/fresh-runs`, `bot_registry.json` **মুছে** |
| `refresh-state` | 505 | `GetInitialState` | `app.go:129` | সব settings + stats | UI form refill |
| `clear-logs` | 509 | `ClearLogs` | `app.go:464` | — | Badger `logs/*` clear + `ui:logs_cleared` (transcript ছোঁয় না) |
| `chat-session` (change) | 519 | `GetLiveChatSnapshot` | `app.go:1144` → `home_chat.go:241` | in-memory feed | dropdown render |
| stat capsules | `renderHomeStats` 186 | (timer) `GetInitialState` | `app.go:183 homeStats` | profiles + bot_state | ৩ সেকেন্ডে refresh |
| live chat box | `renderChatTranscript` 54 | (timer/event) `GetLiveChatSnapshot` | `app.go:1144` | `homeChatFeed.snapshot` | ১ সেকেন্ডে refresh |
| `running-pill` | `setRunning` 42 | event `ui:running` | `app.go:1120 emitStatus` | — | pill + Clean disable |

**HomeStats কোথা থেকে (app.go:183):**

| Field | Formula | Source |
|---|---|---|
| `bot_accounts` | `max(worker_count, running+dead)` | `workerplan.DiscoverProfileWorkers`, `bot.BotAccountStatusCounts` |
| `running_accounts` | `len(activeWorkers)` | in-memory map |
| `user_send_count` | username line count | message settings |
| `dead_accounts` | account status count | `BotAccountStatusCounts` |
| `traffic_users` | unique userID across all `bot_state.json` | `countSentMessages` `220` |
| `messages_sent` | প্রতিটা `account+user` key-এর **max** count-এর যোগ | একই |
| `snap_tg_sent_count` | `AccountState.SnapTgSentCount` যোগ | একই |
| `skipped_accounts` | `AccountState.SkippedUsersCount` যোগ | একই |

### 5.2 Proxy panel

| Control | JS | Binding | Go | নোট |
|---|---|---|---|---|
| `proxy-enabled`, `proxy-entries` | form submit 451 | `SaveProxySettings` | `app.go:426` | enabled কিন্তু list খালি → error |
| `proxy-default-scheme` | `proxyInputLines` 316 | — (client-side) | — | plain line-এ `socks5://` prefix যোগ করে |
| `test-proxy` | `testProxyEntries` 325 | `TestProxyEntries` | `app.go:416` | `[]proxy.TestResult`; max 20 line, 4 concurrent, 12s/proxy |
| result rows | 325-359 | — | `internal/proxy/testproxy.go:22` | `ok/tunnel_ok/status/latency_ms/detail/hint/provider_message` |

### 5.3 Message panel

| Control | JS | Binding | Go | প্রভাব |
|---|---|---|---|---|
| mode radio | `setChatMode` 207, listeners 369 | — | — | section show/hide + `use_loaded_sequences` |
| `message-form` submit | 437 | `SaveMessageSettings` | `app.go:384` | sequence, usernames, mode, AI config |
| `load-folder-btn` | 383 | `LoadMessageFolder` | `app.go:486` | folder dialog → সব `.txt` পড়ে → mode=folder |
| `clean-loaded-btn` | 398 | `ClearLoadedMessages` | `app.go:473` | loaded sequences clear |
| `open-evachat-folder-btn` | 427 | `OpenEVAChatDataFolder` | `app.go:537` | `bot.EnsureEVAChatData` → validate → `explorer.exe` |
| `evachat-ai-test-btn` | 411 | `TestEVAChatAI` | `app.go:556` | unsaved values দিয়ে 40s timeout test |
| AI fields | `evaAISettings` 198 | — | — | `ai_enabled/base_url/model/api_key` |

**Mode → engine mapping:**

| Mode | Engine | কোথায় |
|---|---|---|
| `single` | `MessageSequence` line-by-line | `bot/logic.go:976 executeMatchChatSequence` |
| `folder` | random sequence per conversation | একই + `AvailableSequences` |
| `eva_chat_flow` | stateful TXT engine (+ optional AI text) | `bot/logic.go:1146 executeMatchEVAChat`, `internal/bot/eva_chat.go` |
| `keyword` | legacy, UI-তে নেই | `bot/logic.go:1328` (শুধু migration) |

### 5.4 Browser panel

| Control | JS | Binding | Go | নোট |
|---|---|---|---|---|
| `browser-form` submit | 364 | — (`preventDefault`) | — | Enter-এ যেন ভুল backend চালু না হয় |
| `01 Rod` / `02 ChromeDP` / `03 Show Mode` Run | `runBrowserOption` 137 | `RunBrowser(settings,count)` | `app.go:305` → `338 runBrowser` | per-option, count 1–5 |
| `browser-profile` | 137 | একই | `safeProfileName` 858 | `data/profiles/<name>` |
| `fresh-browser-count` | 137 | একই | `validateBrowserRun` 311 | 1–5 clamp, invalid → UI error |
| `browser-headless` | 137 | একই | — | Show Mode-এ জোর করে false |
| status line | `browserStatus` 123 | event `ui:log` (regex `[profile fresh n/m]`) | `app.go:1131 log` | শুধু fresh-browser log দেখায় |

### 5.5 Setting panel

| Control | JS | Binding | Go | কোথায় consume হয় |
|---|---|---|---|---|
| ১০টা timing field + submit | 465 | `SaveGeneralSettings` | `app.go:450` | `buildBotConfigs` 747 → `bot.Config` (`Poll/Action/ReplyDelay/MatchReplyTimeout/MatchCloseWait`) |

---

## 6. State contract (UI যা এক packet-এ পায়)

`model.UISnapshot` (`internal/model/ui.go:87`):

```jsonc
{
  "running": false,
  "license_invalid": false, "license_key": "...", "license_reason": "",
  "home": { "bot_accounts":0, "running_accounts":0, "user_send_count":0,
            "dead_accounts":0, "traffic_users":0, "messages_sent":0,
            "snap_tg_sent_count":0, "skipped_accounts":0 },
  "browser": { "profile":"default", "backend":"rod", "headless":false, "live_chat":false },
  "message": { "chat_mode":"single", "sequence":"", "usernames":"",
               "loaded_sequences":[], "use_loaded_sequences":false,
               "ai_enabled":false, "ai_base_url":"", "ai_model":"", "ai_api_key":"" },
  "proxy":   { "enabled":false, "entries":"" },
  "general": { "poll_min_seconds":20, ... 10 fields },
  "browser_options": [ {key,label,description} × 3 ],
  "logs": [ {time,level,source,message} × ≤200 ]
}
```

**গুরুত্বপূর্ণ:** `ai_api_key` UI-তে ফেরত যায় (password input-এ দেখায়)। তাই state/log serialization-এ credential ফাঁস এড়াতে `app.js:32` শুধু stack trace পাঠায়, args নয়।

Live chat snapshot (`home_chat.go:29`):

```jsonc
{ "version": 42, "selected": "s1", "evicted": 0,
  "sessions": [ {"id":"s1","label":"...","active":true,"dropped":0} ],
  "messages": [ {"id":"m1","role":"user|bot","text":"...","order":1} ] }
```

সীমা: **৫০০ message/conversation, ১২ conversation**, শুধু memory-তে (restart-এ মুছে যায়)।

---

## 7. Storage map

```
<exe dir>/
├─ logs.txt                      ← applog (redaction সহ), data-র বাইরে
├─ WebView2\  Chromium\          ← optional bundled runtime (runtimeassets)
└─ data/
   ├─ ui/badger/                 ← uistore (settings + UI log history)
   │    settings/browser|message|proxy|general
   │    logs/<seq>
   ├─ profiles/<profile>/
   │    login_info.json               ← manual/persistent login
   │    login_info_<uid>_<ts>_<n>.json← fresh browser capture (worker-ও হয়)
   │    bot_state.json                ← AccountState: SentMessageCounts,
   │                                     SnapTgSentCount, SkippedUsersCount,
   │                                     SeenMessages, sequence index …
   │    relationships_ids.json        ← derived (pending friends)
   │    dm_user_ids.json              ← derived
   │    dm_conversations.json         ← derived
   │    all_user_ids.json             ← derived
   ├─ eva_chat/
   │    input/*.txt  input/middle_chat/*.txt    ← triggers (user-editable)
   │    output/*.txt output/middle_chat/*.txt   ← reply pools
   │    persona.txt, countries.txt, README.txt
   ├─ bot_registry.json           ← কোন userID কোন worker/profile ধরেছে
   ├─ tmp/ , fresh-runs/          ← disposable (Clean-এ মুছে যায়)
   └─ (temp browser data OS temp-এ, repo-র ভিতরে নয়)
```

**কেন OS temp:** Wails dev file-watcher repo-র ভিতরে নতুন dir দেখলে app restart করে — তাই browser temp বাইরে।

**Clean যা মুছে:** `profiles/`, `tmp/`, `fresh-runs/`, `bot_registry.json`। **Clean যা রাখে:** Badger settings/logs, `eva_chat/` (user-এর লেখা), `logs.txt`।

---

## 8. Event map (Go → UI)

| Event | Emitter | Payload | UI consumer |
|---|---|---|---|
| `ui:running` | `emitStatus` `app.go:1120` | `bool` | `setRunning` 42 → pill + Clean disable |
| `ui:log` | `log()` `app.go:1131` | `model.LogEntry` | `bindEvents` 526 → Browser status (regex filter), badges |
| `ui:chat_changed` | `home_chat.go` observer | `{version}` | `refreshChatTranscript()` (coalesced) |
| `ui:license_invalid` | 24h ticker | `{key,reason}` | overlay দেখায়, shell hide |
| `ui:logs_cleared` | `ClearLogs` | `true` | (log area reset) |

**Pull vs push:** stats/transcript-এর source of truth pull (3s/1s poll) — event শুধু latency কমায়, missed event replay হয়।

---

## 9. Bot engine dataflow (core)

```
bot.Run(ctx,cfg)  [internal/bot/run.go:66]
 ├─ validate backend (rod|chromedp)
 ├─ profilePaths()  [run.go:404] → login/state/derived file paths
 ├─ loadSavedLogin(); token নেই → captureLoginViaBrowser()
 ├─ currentUserID detect (নাহলে error: "log in again")
 ├─ registerBotUser()  → bot_registry.json
 ├─ startRealtimeListener()  → internal/bot/realtime.go (WS events)
 └─ loop:
      runCycle()  [run.go:156]
       ├─ fetchRelationships        (401/403 → errLoginExpired → worker stop)
       ├─ pending incoming accept   (claim + acceptFriend)
       ├─ fetchDMConversations
       ├─ extractDMTargets + claim (multi-worker একই user-এ কাজ না করে)
       ├─ saveDerivedFiles
       └─ processDMAutoResponses  [logic.go:307]
             └─ per user: sentCount → mode অনুযায়ী
                  single/folder → executeMatchChatSequence  [976]
                  eva           → executeMatchEVAChat          [1146]
                  keyword       → executeMatchKeywordChatSequence [1328]
                send → bot_state.json update (count, seen, snap/tg)
      workDone==false && MatchWhenIdle → matchAndChat() [858]
                → new partner → wait match → first message → sequence
      waitForNextCycle()  PollMin..PollMax
```

**Timing:** `Poll*` = cycle wait, `Action*` = action-এর মাঝে wait, `ReplyDelay*` = reply-এর আগে wait (Show Mode-ও), `MatchReplyTimeout*` = partner reply কতক্ষণ অপেক্ষা, `MatchCloseWait*` = chat closed হলে wait।

---

## 10. Show Mode / Home transcript dataflow

```
runLiveChat()  [live_chat.go:190]
 ├─ newLiveResponder()  [125]  → single/folder/eva (AI optional)
 ├─ browser DOM controls (DOMControls) — শুধু visible UI element
 ├─ startHomeChatObserver()  [home_chat.go:297]
 │     └─ homeObservedDOM wrapper: ReadDOMSnapshot + SendDOMMessage hook
 ├─ liveRunner.run()  [249]
 │     loop: step() [279]
 │        ├─ DOM snapshot পড়ে (ID, author, text, order)
 │        ├─ নতুন incoming? → chooseShowModeReplyDelay() [236]
 │        ├─ responder.Reply() → sendLines() [419]
 │        ├─ Skip / START button click (real click)
 │        └─ rotate() [445] — page বদলালে/new match
 └─ browser বন্ধ → ctx end → transcript session "ended"
```

**সীমা (design decision, docs/HOME_CHAT.md):** DOM-only, কোনো protocol/API অনুমান নয়; ২০০০-row reader capacity-তে fail-closed; unknown author guess করে না; transcript read-only।

---

## 11. Proxy subsystem map

```
settings.entries (text)
   │ proxy.ParseList
   ▼
[]proxy.Entry ──┬─► AssignProfileWorkers()  → প্রতি worker-এ ১টা (index%n)
                ├─► runBrowser() fresh run   → প্রতি browser-এ ১টা (rotator.Next)
                ├─► api.Client (HTTP)        → Rotator
                └─► TestProxyEntries()       → TestResult[] (UI table)

proxy.NewRotator(entries).Next()  → round-robin
proxy/connect.go, relay.go        → CONNECT / SOCKS tunnel + auth
```
`TestProxyEntries`: প্রতি proxy-তে `https://app.chitchat.gg` request; provider-এর নিজের message sanitize করে `provider_message` + পরের ধাপ `hint`।

---

## 12. License subsystem map

```
GenerateHardwareKey()  [internal/license/license.go:27]
   WMI: Win32_ComputerSystemProduct.UUID+IdentifyingNumber
        Win32_BIOS.SerialNumber
        Win32_DiskDrive.SerialNumber
   → MD5 → hex UPPERCASE  (Windows-only)
        ▼
VerifyLicense(key) → POST https://license-api.nilhasancl.workers.dev/check
        ▼
 {valid, reason, expiry}
        ▼
 app: invalid → overlay; 24h ticker re-check; invalid হলে live overlay
```

**Design boundary:** license check UI-এর আগে (`NewApp`), কিন্তু **UI-তে key enter করার কোনো path নেই** — শুধু copy।

---

## 13. Extension guide — "কোথায় কী touch করলে কী হয়"

| চাই | Touch points (এই order-এ) |
|---|---|
| **নতুন Setting field** | `internal/model/ui.go` struct → `uistore` load/save → `app.go` Go method → `index.html` input → `app.js fillForms` + submit handler → `buildBotConfigs`-এ consume → Go test |
| **নতুন stat capsule** | `model.HomeStats` → `app.go homeStats` → `index.html` capsule → `app.js renderHomeStats` |
| **নতুন tab** | `index.html` nav button + section → `app.js` `bindTabs`/`activateTab` (auto by data-attrs) → নতুন binding + `app.go` method → `App.d.ts` regen (`npm run verify` নয়, `wails generate module`) |
| **নতুন browser backend** | `internal/browser/backend.go` interface impl → `browser.New` switch → `model.DefaultBrowserOptions()` → `uistore` backend validation allowlist → tests |
| **নতুন chat mode** | `model` const + `EffectiveChatMode` → `app.js setChatMode`/HTML radio → `bot/logic.go` execute path → `live_chat.go newLiveResponder` → mapping doc |
| **নতুন event** | `app.emit("ui:x", payload)` → `app.js bindEvents EventsOn` → docs (এই table) |
| **নতুন dashboard API** | `app.go` exported method → frontend `invoke("X")` → Wails auto JS binding (`cmd/chitchatui/frontend/wailsjs/`) → test |

---

## 14. Design invariants (ভাঙা যাবে না)

1. **Credentials log-এ যাবে না** — `applog.Redact` + `app.js` শুধু error stack পাঠায়, invocation args নয়।
2. **`innerHTML` দিয়ে message render নয়** — `textContent` + `pre-wrap` (`app.js:54`)।
3. **Clean শুধু data-এর ভিতরে** — `pathInside` guard (`app.go:738`)।
4. **Show Mode = DOM-only** — কোনো WS frame/undocumented API নয় (`AGENTS.md`)।
5. **Bot running থাকলে Clean blocked**।
6. **Settings refresh unsaved form মুছবে না** — `browserFormLoaded` guard।
7. **Worker-প্রতি proxy assignment deterministic** (round-robin), র‍্যান্ডম নয়।
8. **Temp browser data repo-র বাইরে** (dev watcher) এবং সব path-এ cleanup defer।
9. **Stats শুধু `bot_state.json` থেকে** — UI নিজে কোনো count বানায় না।

---

## 15. Known architecture gaps (কোড এখনো বদলানো হয়নি)

| # | Issue | Severity | কোথায় fix হবে |
|---|---|---|---|
| A1 | `cmd/chitchatui/app.go` + `main.go` পুরনো duplicate (license/live-chat/`TestProxyEntries` নেই, আলাদা embed)। root build ঠিক, কিন্তু ওই dir build করলে ভাঙা dashboard | মাঝারি | `cmd/chitchatui/` সরানো বা root-এর thin wrapper বানানো; `install.bat`/docs আপডেট |
| A2 | License key UI-তে enter করা যায় না (input `readonly`) — invalid হলে user নিজে fix করতে পারে না | মাঝারি | `index.html` + `app.js` + নতুন `App` method (save/re-check) |
| A3 | `SaveBrowserSettings` `live_chat` flag-ও persist করে — Show Mode-এর পরে stale flag থেকে যায় | কম | `uistore`-এ ephemeral field বাদ দেওয়া |
| A4 | প্রতি ৩s-এ **পুরো** `UISnapshot` (settings + ২০০ log) টানা হয় — শুধু stats দরকার | কম | আলাদা `GetHomeStats()` binding |
| A5 | `RunFreshBrowsers` binding আছে কিন্তু UI ব্যবহার করে না (dead API) | কম | সরানো বা ব্যবহার |
| A6 | Stats-এ `bot_accounts = max(workers, running+dead)` — dead account worker count-এ ধরা হয় না, তাই সংখ্যা বিভ্রান্তিকর হতে পারে | কম | `homeStats` formula + docs |
| A7 | `homeStats`/`countSentMessages` প্রতি poll-এ সব `bot_state.json` পড়ে — profile বেশি হলে CPU | কম | cache/incremental |
| A8 | License verify remote single point (`workers.dev`) — offline হলে পুরো app overlay-এ আটকে যায় | মাঝারি | grace period / offline cache design সিদ্ধান্ত |

---

## 16. Verification map

| Gate | কী ঢাকে |
|---|---|
| `npm test` (65 tests) | browser UI contract, DOM show-mode, package/source integrity, verification tooling |
| `npm run verify` → `frontend-syntax` | `app.js` syntax |
| `… ui-contract-package-tests` | `tests/*.cjs` (contract + package) |
| `… source-unchanged-during-verification` | verification চলাকালে source বদলায়নি কিনা |
| Go gates (বর্তমানে BLOCKED) | `go vet`, `go test`, `windows-cross-build`, wails production build |

**Go toolchain ছাড়া যা প্রমাণ করা যায় না:** worker loop, license WMI, rod/chromedp, actual build। এগুলো `TEST_REPORT.md` + `verification-evidence/`-এ রাখা আছে (আগের রান)।

---

## 17. Appendix — সম্পূর্ণ binding table

| Binding | Signature | File:line | UI ব্যবহারকারী |
|---|---|---|---|
| `GetInitialState` | `→ UISnapshot` | app.go:129 | bootstrap, refresh, 3s timer |
| `GetLiveChatSnapshot` | `(sessionID) → snapshot` | app.go:1144 | Home chat, 1s timer |
| `SaveBrowserSettings` | `(BrowserSettings) → error` | app.go:283 | (UI এখন সরাসরি নয়) |
| `RunBrowser` | `(BrowserSettings,int) → error` | app.go:305 | Browser Run buttons |
| `RunFreshBrowsers` | `(int) → error` | app.go:295 | unused |
| `SaveMessageSettings` | `(MessageSettings) → error` | app.go:384 | Message save |
| `TestEVAChatAI` | `(MessageSettings) → string` | app.go:556 | Test AI |
| `LoadMessageFolder` | `→ []string` | app.go:486 | Load Folder |
| `ClearLoadedMessages` | `→ error` | app.go:473 | Clean Loaded |
| `OpenEVAChatDataFolder` | `→ string` | app.go:537 | Open EVAChat folder |
| `SaveProxySettings` | `(ProxySettings) → error` | app.go:426 | Proxy save |
| `TestProxyEntries` | `(raw) → []TestResult` | app.go:416 | Test Proxy |
| `SaveGeneralSettings` | `(GeneralSettings) → error` | app.go:450 | Settings save |
| `ClearLogs` | `→ error` | app.go:464 | Clear Logs |
| `CleanProfiles` | `→ error` | app.go:578 | Clean |
| `StartBot` | `→ error` | app.go:597 | Start Bot |
| `StopBot` | `→ error` | app.go:675 | Stop |
