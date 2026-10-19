// ChitChat Bot — two self-contained pages.
//
// Show Mode: Setting + Proxy + Message + Run Browser + Live Logs. One click
// saves the three stores and launches the visible browser; the Go runner emits
// rule-aware events for the live log.
// Background Bot: Login + Proxy + Message + Settings + Live Logs + Start/Stop.
//
// No backend behavior is merged: StartBot and the Show Mode DOM runner stay
// separate Go paths.

const state = {
  snapshot: null,
  tab: "show",
  runningShow: false,
  runningBot: false,
  busy: { show: false, bot: false },
  loadedSequences: { sm: 0, bg: 0 },
  profiles: [],
  activeSub: { show: "show-setting", bot: "bg-login" },
  showLog: { after: 0, entries: [], pending: false },
  botLog: { after: 0, entries: [], pending: false },
};

// Global form store: every field is bound by id, so switching sub-pages never
// loses typed values (the DOM may hide, but the store is the source of truth).
const formStore = { show: {}, bot: {} };

function fieldValue(el) {
  return el.type === "checkbox" ? el.checked : el.value;
}

function setFieldValue(el, stored) {
  if (el.type === "checkbox") el.checked = !!stored;
  else el.value = stored;
}

function captureGlobalState() {
  for (const [slice, panel] of [["show", '[data-panel="show"]'], ["bot", '[data-panel="bot"]']]) {
    document.querySelectorAll(`${panel} input, ${panel} select, ${panel} textarea`).forEach((el) => {
      if (el.id) formStore[slice][el.id] = fieldValue(el);
    });
  }
}

function restoreGlobalState(slice) {
  const panel = slice === "show" ? '[data-panel="show"]' : '[data-panel="bot"]';
  document.querySelectorAll(`${panel} input, ${panel} select, ${panel} textarea`).forEach((el) => {
    if (!el.id || !(el.id in formStore[slice]) || el === document.activeElement) return;
    setFieldValue(el, formStore[slice][el.id]);
  });
}

function bindGlobalState() {
  for (const [slice, panel] of [["show", '[data-panel="show"]'], ["bot", '[data-panel="bot"]']]) {
    document.querySelectorAll(`${panel} input, ${panel} select, ${panel} textarea`).forEach((el) => {
      if (!el.id) return;
      formStore[slice][el.id] = fieldValue(el);
      const save = () => { formStore[slice][el.id] = fieldValue(el); };
      el.addEventListener("input", save);
      el.addEventListener("change", save);
    });
  }
}

function byId(id) { return document.getElementById(id); }
function value(id) { const el = byId(id); return el ? String(el.value || "").trim() : ""; }
function numberValue(id) { return Number(byId(id).value); }

function backend() {
  return window.go && window.go.main && window.go.main.App;
}

async function invoke(method, ...args) {
  try {
    const api = backend();
    if (!api || typeof api[method] !== "function") throw new Error(`Wails binding not ready: ${method}`);
    return await api[method](...args);
  } catch (error) {
    reportFrontendError(`Backend ${method} failed`, error);
    throw error;
  }
}

function runtimeEvents() { return window.runtime || {}; }

function reportFrontendError(context, error) {
  try {
    const runtime = runtimeEvents();
    if (typeof runtime.LogError === "function") {
      const detail = error && error.stack ? error.stack : String(error);
      runtime.LogError(`Frontend | ${context} | ${detail}`);
    }
  } catch (_) { /* logging bridge unavailable */ }
}

function setStatus(id, message, kind) {
  const el = byId(id);
  if (!el) return;
  el.textContent = message;
  el.classList.toggle("status-error", kind === "error");
  el.classList.toggle("status-ok", kind === "ok");
}

// ---------------------------------------------------------------------------
// Tabs / running badges
// ---------------------------------------------------------------------------
function activateSubpanel(panel) {
  if (!panel) return;
  const mode = panel.startsWith("bg-") ? "bot" : "show";
  state.activeSub[mode] = panel;
  document.querySelectorAll(".subpanel").forEach((el) => {
    el.classList.toggle("active", el.dataset.subpanel === panel);
  });
  document.querySelectorAll(".subnav-button").forEach((button) => {
    button.classList.toggle("active", button.dataset.subpanel === panel);
  });
  restoreGlobalState(mode);
  if (panel === "show-logs") refreshShowLog();
  if (panel === "bg-logs") refreshBotLog();
}

function activateTab(tab) {
  state.tab = tab === "bot" ? "bot" : "show";
  document.querySelectorAll(".nav-button").forEach((button) => {
    button.classList.toggle("active", button.dataset.tab === state.tab);
  });
  document.querySelectorAll(".tab-panel").forEach((panel) => {
    panel.classList.toggle("active", panel.dataset.panel === state.tab);
  });
  const showSubnav = byId("subnav-show");
  const botSubnav = byId("subnav-bot");
  if (showSubnav) showSubnav.hidden = state.tab !== "show";
  if (botSubnav) botSubnav.hidden = state.tab !== "bot";
  activateSubpanel(state.activeSub[state.tab]);
  if (state.tab === "show") refreshShowLog();
  if (state.tab === "bot") refreshBotLog();
  renderRunning();
}

function bindTabs() {
  document.querySelectorAll(".nav-button").forEach((button) => {
    button.addEventListener("click", () => activateTab(button.dataset.tab));
  });
  document.querySelectorAll(".subnav-button").forEach((button) => {
    button.addEventListener("click", () => activateSubpanel(button.dataset.subpanel));
  });
}

function renderRunning() {
  const pill = byId("running-pill");
  pill.textContent = state.runningBot ? "Running" : "Stopped";
  pill.className = `pill ${state.runningBot ? "running" : "stopped"}`;

  const badge = byId("show-run-badge");
  badge.textContent = state.runningShow ? "Running" : "Idle";
  badge.classList.toggle("live", state.runningShow);

  byId("sm-run").disabled = state.runningShow || state.busy.show;
  byId("bg-start").disabled = state.runningBot || state.busy.bot;
  byId("bg-stop").disabled = (!state.runningBot && !state.busy.bot) || state.busy.bot;
}

function setBusy(which, busy) {
  state.busy[which] = busy;
  renderRunning();
}

// ---------------------------------------------------------------------------
// Settings: first message + timing (per page prefix)
// ---------------------------------------------------------------------------
function firstValues(settings) {
  const s = settings || {};
  return {
    mode: s.mode || "bot",
    bot_first_percent: s.bot_first_percent ?? 70,
    wait_timeout: !!s.wait_timeout,
    wait_min_seconds: s.wait_min_seconds ?? 30,
    wait_max_seconds: s.wait_max_seconds ?? 60,
  };
}

function updateFirst(prefix) {
  const mode = byId(`${prefix}-first-mode`).value;
  const wait = byId(`${prefix}-first-wait`).value === "skip";
  byId(`${prefix}-first-percent-row`).hidden = mode !== "random";
  byId(`${prefix}-first-percent`).disabled = mode !== "random";
  byId(`${prefix}-first-wait-row`).hidden = mode === "bot";
  byId(`${prefix}-first-wait`).disabled = mode === "bot";
  byId(`${prefix}-first-timeout-row`).hidden = mode === "bot" || !wait;
  byId(`${prefix}-first-min`).disabled = mode === "bot" || !wait;
  byId(`${prefix}-first-max`).disabled = mode === "bot" || !wait;
}

function fillFirst(prefix, settings) {
  const v = firstValues(settings);
  byId(`${prefix}-first-mode`).value = v.mode;
  byId(`${prefix}-first-percent`).value = v.bot_first_percent;
  byId(`${prefix}-first-wait`).value = v.wait_timeout ? "skip" : "unlimited";
  byId(`${prefix}-first-min`).value = v.wait_min_seconds;
  byId(`${prefix}-first-max`).value = v.wait_max_seconds;
  updateFirst(prefix);
}

function readFirst(prefix) {
  const mode = byId(`${prefix}-first-mode`).value;
  if (!["bot", "user", "random"].includes(mode)) throw new Error("Choose a first-message policy.");
  const percent = Number(byId(`${prefix}-first-percent`).value);
  if (!Number.isInteger(percent) || percent < 0 || percent > 100) {
    throw new Error("Bot first chance must be a whole number from 0 to 100.");
  }
  const wait = byId(`${prefix}-first-wait`).value === "skip";
  const min = Number(byId(`${prefix}-first-min`).value);
  const max = Number(byId(`${prefix}-first-max`).value);
  if (!Number.isInteger(min) || !Number.isInteger(max) || min < 0 || max < min || max > 86400) {
    throw new Error("First-message timeout: use whole seconds, 0 ≤ min ≤ max ≤ 86400.");
  }
  if (mode !== "bot" && wait && min < 1) throw new Error("A first-message timeout needs a positive minimum.");
  return {
    mode,
    bot_first_percent: percent,
    wait_timeout: mode !== "bot" && wait,
    wait_min_seconds: min,
    wait_max_seconds: max,
  };
}

function timingPair(minId, maxId, label, floor) {
  const min = numberValue(minId);
  const max = numberValue(maxId);
  if (!Number.isInteger(min) || !Number.isInteger(max) || min < floor || max < min || max > 86400) {
    throw new Error(`${label}: use whole seconds with ${floor} ≤ min ≤ max ≤ 86400.`);
  }
}

// ---------------------------------------------------------------------------
// Show Mode settings
// ---------------------------------------------------------------------------
function fillShowSettings(settings) {
  const s = settings || {};
  fillFirst("sm", s.first_message);
  byId("sm-reply-min").value = s.reply_delay_min_seconds ?? 0;
  byId("sm-reply-max").value = s.reply_delay_max_seconds ?? 0;
  byId("sm-auto-skip").checked = !!s.auto_skip;
  byId("sm-noreply-min").value = s.no_reply_min_seconds ?? 60;
  byId("sm-noreply-max").value = s.no_reply_max_seconds ?? 60;
  byId("sm-next-min").value = s.next_chat_min_seconds ?? 0;
  byId("sm-next-max").value = s.next_chat_max_seconds ?? 0;
}

function readShowSettings() {
  const first = readFirst("sm");
  timingPair("sm-reply-min", "sm-reply-max", "Reply delay", 0);
  timingPair("sm-noreply-min", "sm-noreply-max", "No-reply timeout", 0);
  timingPair("sm-next-min", "sm-next-max", "Next-chat wait", 0);
  const autoSkip = byId("sm-auto-skip").checked;
  if (autoSkip && numberValue("sm-noreply-min") < 1) {
    throw new Error("Auto skip needs a positive no-reply timeout.");
  }
  return {
    first_message: first,
    reply_delay_min_seconds: numberValue("sm-reply-min"),
    reply_delay_max_seconds: numberValue("sm-reply-max"),
    auto_skip: autoSkip,
    no_reply_min_seconds: numberValue("sm-noreply-min"),
    no_reply_max_seconds: numberValue("sm-noreply-max"),
    next_chat_min_seconds: numberValue("sm-next-min"),
    next_chat_max_seconds: numberValue("sm-next-max"),
  };
}

// ---------------------------------------------------------------------------
// Background Bot settings
// ---------------------------------------------------------------------------
function fillBotSettings(general) {
  const g = general || {};
  fillFirst("bg", g.first_message);
  byId("bg-poll-min").value = g.poll_min_seconds ?? 20;
  byId("bg-poll-max").value = g.poll_max_seconds ?? 45;
  byId("bg-action-min").value = g.action_min_seconds ?? 20;
  byId("bg-action-max").value = g.action_max_seconds ?? 45;
  byId("bg-skip-min").value = g.match_reply_timeout_min_seconds ?? 60;
  byId("bg-skip-max").value = g.match_reply_timeout_max_seconds ?? 60;
  byId("bg-close-min").value = g.match_close_wait_min_seconds ?? 0;
  byId("bg-close-max").value = g.match_close_wait_max_seconds ?? 0;
  byId("bg-reply-min").value = g.reply_delay_min_seconds ?? 0;
  byId("bg-reply-max").value = g.reply_delay_max_seconds ?? 0;
}

function readBotSettings() {
  const first = readFirst("bg");
  timingPair("bg-poll-min", "bg-poll-max", "Poll", 1);
  timingPair("bg-action-min", "bg-action-max", "Action", 1);
  timingPair("bg-skip-min", "bg-skip-max", "Skip timeout", 0);
  timingPair("bg-close-min", "bg-close-max", "Chat closed wait", 0);
  timingPair("bg-reply-min", "bg-reply-max", "Worker reply delay", 0);
  return {
    first_message: first,
    poll_min_seconds: numberValue("bg-poll-min"),
    poll_max_seconds: numberValue("bg-poll-max"),
    action_min_seconds: numberValue("bg-action-min"),
    action_max_seconds: numberValue("bg-action-max"),
    match_reply_timeout_min_seconds: numberValue("bg-skip-min"),
    match_reply_timeout_max_seconds: numberValue("bg-skip-max"),
    match_close_wait_min_seconds: numberValue("bg-close-min"),
    match_close_wait_max_seconds: numberValue("bg-close-max"),
    reply_delay_min_seconds: numberValue("bg-reply-min"),
    reply_delay_max_seconds: numberValue("bg-reply-max"),
  };
}

// ---------------------------------------------------------------------------
// Proxy (shared store, both pages)
// ---------------------------------------------------------------------------
function fillProxy(prefix, proxy) {
  byId(`${prefix}-proxy-enabled`).checked = !!(proxy || {}).enabled;
  byId(`${prefix}-proxy-entries`).value = (proxy || {}).entries || "";
}

function proxyInputLines(prefix) {
  const raw = byId(`${prefix}-proxy-entries`).value;
  if (byId(`${prefix}-proxy-scheme`).value !== "socks5") return raw;
  return raw.split(/\r?\n/).map((line) => {
    const text = line.trim();
    return text && !/^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? "socks5://" + text : line;
  }).join("\n");
}

function readProxy(prefix) {
  return { enabled: byId(`${prefix}-proxy-enabled`).checked, entries: proxyInputLines(prefix) };
}

async function testProxy(prefix) {
  const button = byId(`${prefix}-test-proxy`);
  const statusId = `${prefix}-proxy-status`;
  const results = byId(`${prefix}-proxy-results`);
  if (button.disabled) return;
  const raw = proxyInputLines(prefix);
  results.replaceChildren();
  if (!raw.trim()) {
    setStatus(statusId, "Enter at least one proxy to test.", "error");
    return;
  }
  button.disabled = true;
  button.textContent = "Testing…";
  setStatus(statusId, "Testing HTTPS access through each line (four at a time)…");
  try {
    const rows = await invoke("TestProxyEntries", raw);
    let passed = 0;
    let reachable = 0;
    for (const row of rows || []) {
      if (row.ok) passed++;
      if (row.tunnel_ok) reachable++;
      const item = document.createElement("p");
      item.className = row.ok ? "proxy-test-pass" : row.tunnel_ok ? "proxy-test-warn" : "proxy-test-fail";
      item.textContent = `Line ${row.line}: ${row.ok ? "PASS" : row.tunnel_ok ? "REACHABLE / SITE RESTRICTED" : "FAIL"} — ${row.label || "Invalid entry"} — ${row.latency_ms} ms${row.status ? ` — HTTP ${row.status}` : ""} — ${row.detail}`;
      if (row.provider_message) item.textContent += `\nProvider said: “${row.provider_message}”`;
      if (row.proxy_authenticate) item.textContent += `\nAuth challenge: ${row.proxy_authenticate}`;
      if (row.hint) item.textContent += `\nNext step: ${row.hint}`;
      results.appendChild(item);
    }
    const total = (rows || []).length;
    setStatus(statusId, `${passed}/${total} passed; ${reachable} HTTPS tunnel(s) verified. A test is not a browser-login guarantee.`, passed === total ? "ok" : "");
    if (proxyInputLines(prefix) !== raw) {
      byId(statusId).textContent += " List edited: these results are for the previous values; test again.";
    }
  } catch (error) {
    setStatus(statusId, error.message || String(error), "error");
  } finally {
    button.disabled = false;
    button.textContent = "Test Proxy";
  }
}

// ---------------------------------------------------------------------------
// Message settings (shared store, both pages)
// ---------------------------------------------------------------------------
function setChatMode(prefix, mode) {
  const valid = ["single", "folder", "eva_chat_flow"].includes(mode) ? mode : "single";
  for (const [key, modeValue] of [["single", "single"], ["folder", "folder"], ["evachat", "eva_chat_flow"]]) {
    const active = modeValue === valid;
    byId(`${prefix}-chat-mode-${key}`).checked = active;
    byId(`${prefix}-mode-option-${key}`).classList.toggle("active", active);
    byId(`${prefix}-section-${key}-mode`).style.display = active ? "block" : "none";
  }
}

function selectedChatMode(prefix) {
  return document.querySelector(`input[name="${prefix}-chat-mode"]:checked`)?.value || "single";
}

function fillMessage(prefix, message) {
  const m = message || {};
  setChatMode(prefix, m.chat_mode || "single");
  byId(`${prefix}-sequence`).value = m.sequence || "";
  byId(`${prefix}-usernames`).value = m.usernames || "";
  byId(`${prefix}-ai-enabled`).checked = !!m.ai_enabled;
  byId(`${prefix}-ai-base-url`).value = m.ai_base_url || "";
  byId(`${prefix}-ai-model`).value = m.ai_model || "";
  byId(`${prefix}-ai-api-key`).value = m.ai_api_key || "";
  const loaded = (m.loaded_sequences || []).length;
  state.loadedSequences[prefix] = loaded;
  byId(`${prefix}-loaded-status`).textContent = `${loaded} sequence${loaded === 1 ? "" : "s"} loaded`;
}

function messagePayload(prefix) {
  const mode = selectedChatMode(prefix);
  return {
    chat_mode: mode,
    use_loaded_sequences: mode === "folder",
    sequence: byId(`${prefix}-sequence`).value,
    usernames: byId(`${prefix}-usernames`).value,
    ai_enabled: byId(`${prefix}-ai-enabled`).checked,
    ai_base_url: value(`${prefix}-ai-base-url`),
    ai_model: value(`${prefix}-ai-model`),
    ai_api_key: byId(`${prefix}-ai-api-key`).value,
  };
}

function assertFolderReady(prefix) {
  if (selectedChatMode(prefix) === "folder" && state.loadedSequences[prefix] === 0) {
    throw new Error("Load a folder of sequences before using Folder Sequences mode.");
  }
}

async function loadFolder(prefix) {
  try {
    const sequences = await invoke("LoadMessageFolder");
    if (sequences && sequences.length > 0) {
      await refreshState();
      setChatMode(prefix, "folder");
      setStatus(`${prefix}-loaded-status`, `${sequences.length} sequences loaded.`, "ok");
    }
  } catch (error) {
    setStatus(`${prefix}-loaded-status`, error.message || String(error), "error");
  }
}

async function cleanLoaded(prefix) {
  try {
    await invoke("ClearLoadedMessages");
    await refreshState();
    setChatMode(prefix, "single");
    setStatus(`${prefix}-loaded-status`, "Loaded sequences cleared.", "ok");
  } catch (error) {
    setStatus(`${prefix}-loaded-status`, error.message || String(error), "error");
  }
}

async function openEvaFolder(prefix) {
  try {
    const summary = await invoke("OpenEVAChatDataFolder");
    if (summary) setStatus(`${prefix}-eva-status`, summary, "ok");
  } catch (error) {
    setStatus(`${prefix}-eva-status`, error.message || String(error), "error");
  }
}

async function testAi(prefix) {
  try {
    setStatus(`${prefix}-ai-status`, "Testing AI API...");
    const payload = { ...messagePayload(prefix), chat_mode: "eva_chat_flow" };
    const answer = await invoke("TestEVAChatAI", payload);
    setStatus(`${prefix}-ai-status`, answer || "AI OK", "ok");
  } catch (error) {
    setStatus(`${prefix}-ai-status`, `AI test failed: ${error.message || String(error)}`, "error");
  }
}

// ---------------------------------------------------------------------------
// Browser settings
// ---------------------------------------------------------------------------
function fillBrowser(prefix, browser) {
  const b = browser || {};
  if (prefix === "sm") {
    state.browserProfile = b.profile || "default";
    state.browserSession = !!b.use_saved_session;
  } else {
    byId(`${prefix}-profile`).value = b.profile || "default";
  }
  if (byId(`${prefix}-backend`)) byId(`${prefix}-backend`).value = b.backend === "chromedp" ? "chromedp" : "rod";
  if (byId(`${prefix}-headless`)) byId(`${prefix}-headless`).checked = !!b.headless;
}

// Show Mode profile: two radio choices, exactly as on screen.
//   default = fresh browser (manual login; session saved afterwards)
//   session = default account + its newest saved session (auto-login)
function selectedProfileChoice() {
  const checked = document.querySelector('input[name="sm-profile-choice"]:checked');
  return checked ? checked.value : "default";
}

function updateProfileStatus(prefix) {
  const status = byId(`${prefix}-profile-session`);
  if (!status) return;
  if (prefix === "sm") {
    const selection = selectedProfileChoice();
    const defaultProfile = state.profiles.find((p) => p.name.toLowerCase() === "default") || { name: "default" };
    if (selection === "session") {
      if (defaultProfile.has_session) {
        status.textContent = `Saved default session${defaultProfile.saved_at ? ` (${defaultProfile.saved_at})` : ""} — Run Browser logs in automatically.`;
        status.classList.add("status-ok");
      } else {
        status.textContent = "No saved default session yet — manual login will save one for the session option.";
        status.classList.remove("status-ok");
      }
    } else {
      status.textContent = "Fresh browser — manual login; the session is saved for the session option.";
      status.classList.remove("status-ok");
    }
    return;
  }
  const name = value(`${prefix}-profile`) || "default";
  const profile = state.profiles.find((p) => p.name.toLowerCase() === name.toLowerCase());
  if (profile && profile.has_session) {
    status.textContent = `Saved session${profile.saved_at ? ` (${profile.saved_at})` : ""} — Run Browser will log in automatically.`;
    status.classList.add("status-ok");
  } else {
    status.textContent = "No saved session — the browser will wait for manual login.";
    status.classList.remove("status-ok");
  }
}

function fillProfileOptions() {
  const defaultRadio = byId("sm-profile-default");
  const sessionRadio = byId("sm-profile-session-choice");
  if (defaultRadio && sessionRadio) {
    const wantSession = !!state.browserSession;
    defaultRadio.checked = !wantSession;
    sessionRadio.checked = wantSession;
    updateProfileStatus("sm");
  }
  const bgList = byId("bg-profile-options");
  if (bgList) {
    const fragment = document.createDocumentFragment();
    for (const profile of state.profiles) {
      const option = document.createElement("option");
      option.value = profile.name;
      option.label = profile.has_session ? `${profile.name} · session ${profile.saved_at || ""}`.trim() : `${profile.name} · no session`;
      fragment.appendChild(option);
    }
    bgList.replaceChildren(fragment);
    updateProfileStatus("bg");
  }
}

async function refreshProfiles() {
  try {
    const profiles = await invoke("ListProfiles");
    state.profiles = Array.isArray(profiles) ? profiles : [];
    fillProfileOptions();
  } catch (_) { /* old build or transient read error */ }
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
function renderStats(home) {
  const h = home || {};
  byId("bg-stat-running").textContent = String(h.running_accounts || 0);
  byId("bg-stat-profiles").textContent = String(h.bot_accounts || 0);
  byId("bg-stat-messages").textContent = String(h.messages_sent || 0);
  byId("bg-stat-traffic").textContent = String(h.traffic_users || 0);
  byId("bg-stat-snap").textContent = String(h.snap_tg_sent_count || 0);
  byId("bg-stat-dead").textContent = String(h.dead_accounts || 0);
}

function fillForms(snapshot) {
  state.snapshot = snapshot;
  state.runningShow = !!snapshot.show_mode_running;
  state.runningBot = !!snapshot.running;

  fillBrowser("sm", snapshot.browser);
  fillBrowser("bg", snapshot.browser);
  fillProxy("sm", snapshot.proxy);
  fillProxy("bg", snapshot.proxy);
  fillMessage("sm", snapshot.message);
  fillMessage("bg", snapshot.message);
  fillShowSettings(snapshot.show_mode);
  fillBotSettings(snapshot.general);
  renderStats(snapshot.home || {});
  renderRunning();

  const steps = String(snapshot.message.sequence || "").split(/\r?\n/).map((l) => l.trim()).filter(Boolean).length;
  byId("bg-summary").textContent = `Backend: ${snapshot.browser.backend} · Profiles auto-start · Message steps: ${steps}`;
  captureGlobalState();

  if (snapshot.license_invalid) {
    byId("license-overlay").style.display = "flex";
    byId("license-message").textContent = snapshot.license_reason || "Your license is invalid or expired.";
    byId("license-key-input").value = snapshot.license_key || "UNKNOWN";
    return;
  }
  document.querySelector(".shell")?.classList.add("visible");
}

async function refreshState() {
  const snapshot = await invoke("GetInitialState");
  fillForms(snapshot);
  await refreshProfiles();
  await refreshShowLog();
  await refreshBotLog();
}

async function refreshStatsOnly() {
  try {
    const snapshot = await invoke("GetInitialState");
    renderStats(snapshot.home || {});
    state.runningBot = !!snapshot.running;
    state.runningShow = !!snapshot.show_mode_running;
    renderRunning();
  } catch (_) { /* transient */ }
}

// ---------------------------------------------------------------------------
// Live logs
// ---------------------------------------------------------------------------
function logKindLabel(kind) {
  return { in: "IN", out: "OUT", skip: "SKIP", system: "SYS", error: "ERR" }[kind] || kind.toUpperCase();
}

function logFilterMatch(entry, filter) {
  if (filter === "sms") return entry.kind === "in" || entry.kind === "out";
  if (filter === "rules") return entry.kind === "out" || entry.kind === "skip" || entry.kind === "system";
  if (filter === "errors") return entry.kind === "error";
  return true;
}

function renderShowLog() {
  const list = byId("sm-log-list");
  const filter = byId("sm-log-filter").value;
  const nearBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 60;
  const fragment = document.createDocumentFragment();
  let shown = 0;
  for (const entry of state.showLog.entries) {
    if (!logFilterMatch(entry, filter)) continue;
    shown += 1;
    const row = document.createElement("div");
    row.className = `log-row log-${entry.kind}`;

    const time = document.createElement("span");
    time.className = "log-time";
    time.textContent = entry.time || "";

    const badge = document.createElement("span");
    badge.className = "log-badge";
    badge.textContent = logKindLabel(entry.kind);

    const body = document.createElement("div");
    body.className = "log-body";
    const text = document.createElement("p");
    text.className = "log-text";
    if (entry.kind === "in" || entry.kind === "out") {
      text.textContent = `"${entry.text || ""}"`;
    } else {
      text.textContent = entry.text || "";
    }
    body.appendChild(text);

    const metaParts = [];
    if (entry.rule) metaParts.push(entry.rule);
    if (entry.detail) metaParts.push(entry.detail);
    if (entry.kind === "out") metaParts.push(entry.setting_ok ? "✓" : "✗");
    if (metaParts.length) {
      const meta = document.createElement("p");
      meta.className = "log-meta";
      meta.textContent = metaParts.join(" · ");
      body.appendChild(meta);
    }

    row.append(time, badge, body);
    fragment.appendChild(row);
  }
  list.replaceChildren(fragment);
  byId("sm-log-empty").hidden = shown > 0;
  if (nearBottom) list.scrollTop = list.scrollHeight;
}

async function refreshShowLog() {
  if (!backend() || typeof backend().GetShowModeLog !== "function") return;
  if (state.showLog.pending) return;
  state.showLog.pending = true;
  try {
    const snapshot = await invoke("GetShowModeLog", state.showLog.after);
    if (snapshot.entries && snapshot.entries.length) {
      state.showLog.entries.push(...snapshot.entries);
      state.showLog.after = snapshot.version;
      if (state.showLog.entries.length > 600) {
        state.showLog.entries.splice(0, state.showLog.entries.length - 600);
      }
      renderShowLog();
    }
  } catch (_) { /* retry next tick */ }
  finally { state.showLog.pending = false; }
}

async function clearShowLog() {
  try {
    await invoke("ClearShowModeLog");
    state.showLog.entries = [];
    renderShowLog();
    setStatus("sm-run-status", "Live log cleared (logs.txt is kept).", "ok");
  } catch (error) {
    setStatus("sm-run-status", error.message || String(error), "error");
  }
}

function renderBotLog() {
  const list = byId("bg-log-list");
  const filter = byId("bg-log-filter").value;
  const nearBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 60;
  const fragment = document.createDocumentFragment();
  let shown = 0;
  for (const entry of state.botLog.entries) {
    if (!logFilterMatch(entry, filter)) continue;
    shown += 1;
    const row = document.createElement("div");
    row.className = `log-row log-${entry.kind}`;

    const time = document.createElement("span");
    time.className = "log-time";
    time.textContent = entry.time || "";

    const badge = document.createElement("span");
    badge.className = "log-badge";
    badge.textContent = entry.profile ? `${logKindLabel(entry.kind)} · ${entry.profile}` : logKindLabel(entry.kind);

    const body = document.createElement("div");
    body.className = "log-body";
    const text = document.createElement("p");
    text.className = "log-text";
    if (entry.kind === "in" || entry.kind === "out") {
      text.textContent = `"${entry.text || ""}"`;
    } else {
      text.textContent = entry.text || "";
    }
    body.appendChild(text);

    const metaParts = [];
    if (entry.rule) metaParts.push(entry.rule);
    if (entry.detail) metaParts.push(entry.detail);
    if (entry.kind === "out") metaParts.push(entry.setting_ok ? "✓" : "✗");
    if (metaParts.length) {
      const meta = document.createElement("p");
      meta.className = "log-meta";
      meta.textContent = metaParts.join(" · ");
      body.appendChild(meta);
    }

    row.append(time, badge, body);
    fragment.appendChild(row);
  }
  list.replaceChildren(fragment);
  byId("bg-log-empty").hidden = shown > 0;
  if (nearBottom) list.scrollTop = list.scrollHeight;
}

async function refreshBotLog() {
  if (!backend() || typeof backend().GetBotLog !== "function") return;
  if (state.botLog.pending) return;
  state.botLog.pending = true;
  try {
    const snapshot = await invoke("GetBotLog", state.botLog.after);
    if (snapshot.entries && snapshot.entries.length) {
      state.botLog.entries.push(...snapshot.entries);
      state.botLog.after = snapshot.version;
      if (state.botLog.entries.length > 600) {
        state.botLog.entries.splice(0, state.botLog.entries.length - 600);
      }
      renderBotLog();
    }
  } catch (_) { /* retry next tick */ }
  finally { state.botLog.pending = false; }
}

async function clearBotLog() {
  try {
    await invoke("ClearBotLog");
    state.botLog.entries = [];
    renderBotLog();
    setStatus("bg-status", "Live log cleared (logs.txt is kept).", "ok");
  } catch (error) {
    setStatus("bg-status", error.message || String(error), "error");
  }
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------
async function runShowBrowser() {
  if (state.runningShow || state.busy.show) return;
  setBusy("show", true);
  try {
    const show = readShowSettings();
    const message = messagePayload("sm");
    assertFolderReady("sm");
    const proxy = readProxy("sm");
    const selection = selectedProfileChoice();
    const profile = "default";
    const useSession = selection === "session";

    setStatus("sm-run-status", "Applying Setting, Proxy and Message…");
    await invoke("SaveShowModeSettings", show);
    await invoke("SaveProxySettings", proxy);
    await invoke("SaveMessageSettings", message);
    await invoke("SaveBrowserSettings", { profile, backend: "rod", headless: false, use_saved_session: useSession });

    state.showLog.entries = [];
    state.showLog.after = 0;
    renderShowLog();
    setStatus("sm-run-status", useSession ? "Restoring the saved profile session…" : "Launching a fresh browser (manual login)…");
    await invoke("RunBrowser", { profile, backend: "show_mode", headless: false, use_saved_session: useSession }, 1);
    await refreshState();
    setStatus("sm-run-status", "Browser requested. Log in manually; live logs will appear here. Close the browser to stop.", "ok");
  } catch (error) {
    setStatus("sm-run-status", error.message || String(error), "error");
  } finally {
    setBusy("show", false);
  }
}

async function startBackgroundBot() {
  if (state.runningBot || state.busy.bot) return;
  setBusy("bot", true);
  try {
    const general = readBotSettings();
    const message = messagePayload("bg");
    assertFolderReady("bg");
    const proxy = readProxy("bg");
    const profile = value("bg-profile") || "default";
    const backendValue = value("bg-backend") || "rod";
    const headless = byId("bg-headless").checked;

    setStatus("bg-status", "Saving browser, proxy, message and settings…");
    await invoke("SaveBrowserSettings", { profile, backend: backendValue, headless, use_saved_session: !!state.snapshot?.browser?.use_saved_session });
    await invoke("SaveProxySettings", proxy);
    await invoke("SaveMessageSettings", message);
    await invoke("SaveGeneralSettings", general);
    setStatus("bg-status", "Starting workers…");
    await invoke("StartBot");
    await refreshState();
    setStatus("bg-status", "Started. One worker runs per saved profile.", "ok");
  } catch (error) {
    setStatus("bg-status", error.message || String(error), "error");
  } finally {
    setBusy("bot", false);
  }
}

async function stopBackgroundBot() {
  try {
    await invoke("StopBot");
    await refreshState();
    setStatus("bg-status", "Stopped.", "ok");
  } catch (error) {
    setStatus("bg-status", error.message || String(error), "error");
  }
}

async function launchLogin() {
  try {
    const settings = {
      profile: value("bg-profile") || "default",
      backend: value("bg-backend") || "rod",
      headless: byId("bg-headless").checked,
    };
    setStatus("bg-login-status", `Launching ${settings.backend} for login…`);
    await invoke("RunBrowser", settings, 1);
    await refreshState();
    setStatus("bg-login-status", "Launch requested. Finish the login in the browser; it closes after the session is saved.", "ok");
  } catch (error) {
    setStatus("bg-login-status", error.message || String(error), "error");
  }
}

// ---------------------------------------------------------------------------
// Wiring
// ---------------------------------------------------------------------------
function bindForms() {
  for (const prefix of ["sm", "bg"]) {
    document.querySelectorAll(`input[name="${prefix}-chat-mode"]`).forEach((radio) => {
      radio.addEventListener("change", (event) => setChatMode(prefix, event.target.value));
    });
    for (const key of ["single", "folder", "evachat"]) {
      byId(`${prefix}-mode-option-${key}`).addEventListener("click", () => {
        setChatMode(prefix, key === "evachat" ? "eva_chat_flow" : key);
      });
    }
    byId(`${prefix}-first-mode`).addEventListener("change", () => updateFirst(prefix));
    byId(`${prefix}-first-wait`).addEventListener("change", () => updateFirst(prefix));
    byId(`${prefix}-profile`).addEventListener("input", () => updateProfileStatus(prefix));
    byId(`${prefix}-profile`).addEventListener("change", () => updateProfileStatus(prefix));
    if (prefix === "sm") {
      document.querySelectorAll('input[name="sm-profile-choice"]').forEach((radio) => {
        radio.addEventListener("change", () => updateProfileStatus("sm"));
      });
    }
  }
}

function bindActions() {
  byId("show-refresh").addEventListener("click", async () => {
    await refreshState();
    setStatus("sm-run-status", "Refreshed.", "ok");
  });
  byId("sm-run").addEventListener("click", runShowBrowser);
  byId("sm-test-proxy").addEventListener("click", () => testProxy("sm"));
  byId("sm-load-folder").addEventListener("click", () => loadFolder("sm"));
  byId("sm-clean-loaded").addEventListener("click", () => cleanLoaded("sm"));
  byId("sm-open-eva").addEventListener("click", () => openEvaFolder("sm"));
  byId("sm-ai-test").addEventListener("click", () => testAi("sm"));
  byId("sm-log-clear").addEventListener("click", clearShowLog);
  byId("sm-log-filter").addEventListener("change", renderShowLog);
  byId("bg-log-clear").addEventListener("click", clearBotLog);
  byId("bg-log-filter").addEventListener("change", renderBotLog);

  byId("bg-refresh").addEventListener("click", async () => {
    await refreshState();
    setStatus("bg-status", "Refreshed.", "ok");
  });
  byId("bg-start").addEventListener("click", startBackgroundBot);
  byId("bg-stop").addEventListener("click", stopBackgroundBot);
  byId("bg-launch-login").addEventListener("click", launchLogin);
  byId("bg-test-proxy").addEventListener("click", () => testProxy("bg"));
  byId("bg-load-folder").addEventListener("click", () => loadFolder("bg"));
  byId("bg-clean-loaded").addEventListener("click", () => cleanLoaded("bg"));
  byId("bg-open-eva").addEventListener("click", () => openEvaFolder("bg"));
  byId("bg-ai-test").addEventListener("click", () => testAi("bg"));
}

function bindEvents() {
  const runtime = runtimeEvents();
  if (typeof runtime.EventsOn !== "function") return;
  runtime.EventsOn("ui:showlog", () => refreshShowLog());
  runtime.EventsOn("ui:botlog", () => refreshBotLog());
  runtime.EventsOn("ui:show_running", (running) => {
    state.runningShow = !!running;
    renderRunning();
  });
  runtime.EventsOn("ui:running", (running) => {
    state.runningBot = !!running;
    renderRunning();
  });
  runtime.EventsOn("ui:log", (entry) => {
    if (typeof entry.message === "string" && /\[[^\]]+ fresh \d+\/\d+\]/.test(entry.message)
        && !entry.message.includes("] Removed temporary browser data from ")) {
      setStatus("bg-login-status", entry.message, entry.level === "error" ? "error" : "");
    }
  });
  runtime.EventsOn("ui:license_invalid", (data) => {
    byId("license-overlay").style.display = "flex";
    byId("license-message").textContent = data.reason || "Your license became invalid or expired.";
    byId("license-key-input").value = data.key || "UNKNOWN";
  });

  const copyBtn = byId("copy-license-btn");
  if (copyBtn) {
    copyBtn.addEventListener("click", () => {
      const input = byId("license-key-input");
      input.select();
      input.setSelectionRange(0, 99999);
      navigator.clipboard.writeText(input.value).then(() => {
        const original = copyBtn.textContent;
        copyBtn.textContent = "Copied!";
        setTimeout(() => { copyBtn.textContent = original; }, 2000);
      });
    });
  }
}

async function bootstrap() {
  bindTabs();
  bindForms();
  bindGlobalState();
  bindActions();
  bindEvents();
  activateTab("show");

  let attempts = 0;
  while (attempts < 50) {
    attempts += 1;
    try {
      await refreshState();
      setInterval(refreshStatsOnly, 3000);
      setInterval(refreshProfiles, 10000);
      setInterval(() => { if (state.tab === "show") refreshShowLog(); }, 1000);
      setInterval(() => { if (state.tab === "bot") refreshBotLog(); }, 1000);
      return;
    } catch (error) {
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
  }
  setStatus("sm-run-status", "Failed to connect to Wails bindings.", "error");
}

window.addEventListener("error", (event) => {
  reportFrontendError("Uncaught error", event.error || event.message);
});
window.addEventListener("unhandledrejection", (event) => {
  reportFrontendError("Unhandled promise rejection", event.reason);
});

window.addEventListener("DOMContentLoaded", () => {
  bootstrap().catch((error) => {
    reportFrontendError("Bootstrap failed", error);
    setStatus("sm-run-status", error.message || String(error), "error");
  });
});
