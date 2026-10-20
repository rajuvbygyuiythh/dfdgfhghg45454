// CrushFling Auto Register — dashboard overlay.
//
// The panel lives inside index.html (Wails injects the runtime only into that
// page) and talks to the Go App bindings. Every user-visible string is written
// with textContent: log lines and profile data contain server responses, so
// they must never be interpreted as markup.
(function () {
  "use strict";

  const POLL_MS = 2000;
  const state = {
    open: false,
    timer: null,
    lines: [],
    extra: [],
    offset: 0,
    stats: null,
    summary: null,
    busy: false,
    tab: "pictures",
  };

  function $(id) {
    return typeof document !== "undefined" ? document.getElementById(id) : null;
  }

  function documentAlive() {
    return typeof document !== "undefined" && !!document.body;
  }

  function app() {
    const w = typeof window !== "undefined" ? window : globalThis;
    return w && w.go && w.go.main && w.go.main.App ? w.go.main.App : null;
  }

  function has(name) {
    const a = app();
    return !!a && typeof a[name] === "function";
  }

  function call(name, ...args) {
    const a = app();
    if (!a || typeof a[name] !== "function") {
      return Promise.reject(new Error("binding " + name + " is not available"));
    }
    return Promise.resolve(a[name](...args));
  }

  function errorText(err) {
    if (!err) return "unknown error";
    return String(err.message || err);
  }

  function text(id, value) {
    const el = $(id);
    if (el) el.textContent = String(value);
  }

  function val(id, fallback) {
    const el = $(id);
    if (!el) return fallback === undefined ? "" : fallback;
    return String(el.value);
  }

  function setVal(id, value) {
    const el = $(id);
    if (el) el.value = value === undefined || value === null ? "" : String(value);
  }

  function num(id, fallback) {
    const parsed = Number(val(id, ""));
    return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : fallback;
  }

  function checked(id) {
    const el = $(id);
    return !!(el && el.checked);
  }

  function hint(message, kind) {
    const el = $("cf-hint");
    if (!el) return;
    el.textContent = message || "";
    el.classList.toggle("cf-error-text", kind === "error");
    el.classList.toggle("cf-ok-text", kind === "ok");
  }

  // Write a status line into one section (not the global hint), so a button
  // pressed in Proxy does not overwrite a warning from the run.
  function status(id, message, kind) {
    const el = $(id);
    if (!el) return;
    el.textContent = message || "";
    el.classList.toggle("cf-error-text", kind === "error");
    el.classList.toggle("cf-ok-text", kind === "ok");
  }

  // ------------------------------------------------------------------- tabs
  function showTab(name) {
    if (!documentAlive()) return;
    state.tab = name || "settings";
    const tabs = document.querySelectorAll(".cf-tab");
    for (let i = 0; i < tabs.length; i += 1) {
      const tab = tabs[i];
      const active = tab.getAttribute("data-cf-pane") === state.tab;
      tab.classList.toggle("active", active);
      tab.setAttribute("aria-selected", active ? "true" : "false");
    }
    const panes = document.querySelectorAll(".cf-pane");
    for (let i = 0; i < panes.length; i += 1) {
      const pane = panes[i];
      pane.hidden = pane.getAttribute("data-cf-pane") !== state.tab;
    }
  }

  // ------------------------------------------------------------------ stats
  function applyStats(stats) {
    if (!stats) return;
    state.stats = stats;
    text("cf-ok", stats.success || 0);
    text("cf-fail", stats.failed || 0);
    text("cf-total", stats.total || 0);
    const running = !!stats.running;
    const pill = $("cf-running");
    if (pill) {
      pill.textContent = running ? "Running" : "Stopped";
      pill.className = "pill " + (running ? "running" : "stopped");
    }
    const codes = stats.by_code && typeof stats.by_code === "object"
      ? Object.keys(stats.by_code).sort().map((key) => key + "=" + stats.by_code[key]).join(" · ")
      : "";
    const parts = [];
    if (stats.last_error) parts.push("Last error: " + stats.last_error);
    if (codes) parts.push("Failure codes: " + codes);
    text("cf-last-error", parts.join("  |  "));
    const start = $("cf-start");
    const stop = $("cf-stop");
    if (start) start.disabled = running || state.busy;
    if (stop) stop.disabled = !running || state.busy;
  }

  function applySummary(summary) {
    if (!summary) return;
    state.summary = summary;
    text("cf-accounts", (summary.accounts || 0) + (summary.verified ? " (" + summary.verified + " ✓)" : ""));
    text("cf-browsers-open", summary.browsers || 0);
    if (summary.bio_file) {
      setVal("cf-bio-file", summary.bio_file);
      if ($("cf-bio-file")) $("cf-bio-file").title = summary.bio_file;
    }
    if (typeof summary.bios === "number") text("cf-cap-count", summary.bios);
    if (typeof summary.pictures === "number") text("cf-pic-count", summary.pictures);
    if (summary.stats) applyStats(summary.stats);
  }

  // -------------------------------------------------------------------- log
  function lineLevel(line) {
    const match = /(?:^|\]\s|\b)(debug|info|warn|warning|error):/i.exec(String(line));
    if (!match) return "info";
    const level = match[1].toLowerCase();
    return level === "warning" ? "warn" : level;
  }

  function renderLog() {
    const el = $("cf-log");
    if (!el) return;
    const everything = state.lines.slice(state.offset).concat(state.extra);
    const filter = val("cf-log-filter", "all") || "all";
    const min = { all: 0, debug: 0, info: 1, warn: 2, error: 3 }[filter] || 0;
    const rank = { debug: 0, info: 1, warn: 2, error: 3 };
    const visible = everything.filter((line) => (rank[lineLevel(line)] || 0) >= min);
    el.textContent = visible.join("\n");
    el.scrollTop = el.scrollHeight;
  }

  async function refresh(withCounts) {
    if (!documentAlive()) return;
    try {
      if (has("GetCrushSummary")) {
        const summary = await call("GetCrushSummary");
        if (documentAlive()) applySummary(summary);
      } else if (has("GetStats")) {
        const stats = await call("GetStats");
        if (documentAlive()) applyStats(stats);
      }
      if (has("GetLogs")) {
        const logs = await call("GetLogs", 400);
        if (documentAlive() && Array.isArray(logs)) {
          state.lines = logs.map((line) => String(line));
          if (state.offset > state.lines.length) state.offset = 0;
        }
      }
      if (documentAlive()) renderLog();
      if (withCounts) await refreshCounts();
    } catch (err) {
      if (documentAlive()) hint("Refresh failed: " + errorText(err), "error");
    }
  }

  async function refreshCounts() {
    if (has("GetPictureCount")) {
      try { text("cf-pic-count", await call("GetPictureCount")); } catch (_) { /* non-fatal */ }
    }
    if (has("GetCaptionCount")) {
      try { text("cf-cap-count", await call("GetCaptionCount")); } catch (_) { /* non-fatal */ }
    }
    if (has("GetCrushDebugInfo") && documentAlive()) {
      try {
        const info = await call("GetCrushDebugInfo");
        if (info && info.log_path) text("cf-log-path", "Run log: " + info.log_path);
      } catch (_) { /* non-fatal */ }
    }
    await refreshBio(false);
  }

  // ------------------------------------------------------------- bio list
  function renderBio(info) {
    if (!info) return;
    if (info.path) setVal("cf-bio-file", info.path);
    const preview = $("cf-bio-preview");
    if (preview) {
      while (preview.firstChild) preview.removeChild(preview.firstChild);
      const lines = Array.isArray(info.preview) ? info.preview : [];
      for (let i = 0; i < lines.length; i += 1) {
        const item = document.createElement("li");
        item.textContent = String(lines[i]);
        preview.appendChild(item);
      }
      if (lines.length === 0) {
        const item = document.createElement("li");
        item.textContent = "(no bio left — load a bio.txt file)";
        preview.appendChild(item);
      }
    }
    if (typeof info.remaining === "number") text("cf-cap-count", info.remaining);
    status("cf-bio-status",
      (info.file_name || "bio list") + ": " + (info.remaining || 0) + " left of " + (info.total || 0),
      "ok");
  }

  async function refreshBio(announce) {
    if (!has("GetCrushBioInfo")) return;
    try {
      renderBio(await call("GetCrushBioInfo"));
    } catch (err) {
      if (announce) status("cf-bio-status", "Could not read the bio list: " + errorText(err), "error");
    }
  }

  async function loadBioFile() {
    try {
      const info = await call("PickCrushBioFile");
      if (!info || !info.path) {
        status("cf-bio-status", "No file selected.");
        return;
      }
      renderBio(info);
      hint("Bio list loaded: " + info.total + " line(s).", "ok");
    } catch (err) {
      status("cf-bio-status", "Load failed: " + errorText(err), "error");
    }
  }

  // -------------------------------------------------------------- pictures
  async function browsePictures() {
    try {
      const dir = await call("PickCrushPictureFolder");
      if (!dir) return;
      setVal("cf-avatar", dir);
      status("cf-pic-status", "Picture folder: " + dir, "ok");
      if (has("GetPictureCount")) {
        try { text("cf-pic-count", await call("GetPictureCount")); } catch (_) { /* non-fatal */ }
      }
    } catch (err) {
      status("cf-pic-status", "Folder picker failed: " + errorText(err), "error");
    }
  }

  async function openPictures() {
    try {
      if (has("OpenCrushPicturesFolder")) {
        status("cf-pic-status", String(await call("OpenCrushPicturesFolder")), "ok");
        return;
      }
      if (has("OpenCrushflingDataFolder")) {
        status("cf-pic-status", String(await call("OpenCrushflingDataFolder")), "ok");
        return;
      }
      status("cf-pic-status", "Folder binding is not available.", "error");
    } catch (err) {
      status("cf-pic-status", "Could not open the folder: " + errorText(err), "error");
    }
  }

  // ----------------------------------------------------------------- proxy
  async function browseProxy() {
    try {
      const path = await call("PickCrushProxyFile");
      if (!path) return;
      setVal("cf-proxy-file", path);
      status("cf-proxy-status", "Proxy file: " + path, "ok");
    } catch (err) {
      status("cf-proxy-status", "File picker failed: " + errorText(err), "error");
    }
  }

  async function validateProxy() {
    try {
      if (!has("ValidateCrushProxies")) {
        status("cf-proxy-status", "Validation binding is not available.", "error");
        return;
      }
      const result = await call("ValidateCrushProxies", val("cf-proxy-file", "").trim(), val("cf-proxy-entries", ""));
      const errors = result && Array.isArray(result.errors) ? result.errors : [];
      const bad = result ? (result.total || 0) - (result.valid || 0) : 0;
      if (!result || (result.total || 0) === 0) {
        status("cf-proxy-status", "No proxy line found (file empty and the text box is blank).", "error");
        return;
      }
      const head = (result.valid || 0) + "/" + result.total + " valid";
      status("cf-proxy-status", errors.length ? head + " — " + errors.join(" | ") : head, bad ? "error" : "ok");
    } catch (err) {
      status("cf-proxy-status", "Validate failed: " + errorText(err), "error");
    }
  }

  // --------------------------------------------------------------- browsers
  async function closeBrowsers() {
    try {
      const closed = await call("CloseCrushBrowsers");
      text("cf-browsers-open", await call("GetCrushBrowsersOpen"));
      status("cf-browser-status", "Closed " + (closed || 0) + " browser(s).", "ok");
    } catch (err) {
      status("cf-browser-status", "Could not close browsers: " + errorText(err), "error");
    }
  }

  // ------------------------------------------------------------------ panel
  function setOpen(open) {
    if (!documentAlive()) return;
    const overlay = $("crushfling-overlay");
    if (!overlay) return;
    overlay.hidden = !open;
    state.open = open;
    if (open) {
      if (!state.timer) {
        state.timer = setInterval(() => {
          if (!state.open) return;
          refresh(false);
        }, POLL_MS);
      }
      refresh(true);
    } else if (state.timer) {
      clearInterval(state.timer);
      state.timer = null;
    }
  }

  async function startRun() {
    if (state.busy) return;
    state.busy = true;
    hint("Starting…");
    applyStats({ ...(state.stats || {}), running: true });
    try {
      await call("StartBulk", buildConfig());
      hint("Run started — logs stream in the Logs tab.", "ok");
      showTab("logs");
    } catch (err) {
      hint("Start failed: " + errorText(err), "error");
    } finally {
      state.busy = false;
      await refresh(true);
    }
  }

  async function stopRun() {
    try {
      await call("StopBulk");
      hint("Stop requested — every browser is being closed.", "ok");
      text("cf-browsers-open", 0);
    } catch (err) {
      hint("Stop failed: " + errorText(err), "error");
    }
    await refresh(false);
  }

  function buildConfig() {
    // Empty means "let Go use the service data folder"; only a typed value
    // overrides it. This keeps logs/dumps/accounts in the same place the
    // dashboard's Open folder buttons use.
    const dataDir = val("cf-data", "").trim();
    const avatarDir = val("cf-avatar", "").trim();
    return {
      data_dir: dataDir,
      count: num("cf-count", 1),
      concurrency: num("cf-conc", 1),
      headless: checked("cf-headless"),
      use_browser: checked("cf-browser"),
      browser_verify: checked("cf-browser-verify"),
      email_provider: val("cf-email", "") || "mailtm",
      custom_email: val("cf-custom-email", "").trim(),
      custom_email_hook: val("cf-mail-hook", "").trim(),
      proxy_file: val("cf-proxy-file", "").trim(),
      proxy_entries: val("cf-proxy-entries", "").trim(),
      avatar_dir: avatarDir,
      captcha_api_key: val("cf-captcha", "").trim(),
      pasted_link: val("cf-pasted-link", "").trim(),
      manual_link: false,
      debug: checked("cf-debug"),
      trace_http: checked("cf-trace"),
      mail_poll_sec: num("cf-poll", 3),
      mail_timeout_sec: num("cf-timeout", 120),
      max_retries: num("cf-retries", 3),
      min_delay_ms: num("cf-min-delay", 1500),
      max_delay_ms: num("cf-max-delay", 3500),
    };
  }

  async function previewProfile() {
    try {
      const profile = await call("PreviewProfile");
      state.extra.push(JSON.stringify(profile, null, 2));
      renderLog();
      showTab("logs");
      hint("Preview generated.", "ok");
    } catch (err) {
      hint("Preview failed: " + errorText(err), "error");
    }
  }

  async function exportAccounts() {
    try {
      const accounts = await call("ListAccounts");
      const blob = new Blob([JSON.stringify(accounts, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = "crushfling-accounts.json";
      document.body.appendChild(link);
      link.click();
      link.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      hint("Exported " + (Array.isArray(accounts) ? accounts.length : 0) + " account(s).", "ok");
    } catch (err) {
      hint("Export failed: " + errorText(err), "error");
    }
  }

  async function openLogFolder() {
    try {
      let message = "";
      if (!has("OpenCrushLogFolder") && !has("OpenCrushflingDataFolder")) {
        throw new Error("log folder binding is not available");
      }
      if (has("OpenCrushLogFolder")) {
        message = await call("OpenCrushLogFolder");
      } else if (has("OpenCrushflingDataFolder")) {
        message = await call("OpenCrushflingDataFolder");
      }
      hint(String(message || "Folder opened."), "ok");
    } catch (err) {
      hint("Could not open the folder: " + errorText(err), "error");
    }
  }

  async function debugReport() {
    try {
      const path = await call("CreateCrushDebugReport");
      state.extra.push("debug report: " + path);
      renderLog();
      hint("Debug report written: " + path, "ok");
    } catch (err) {
      hint("Debug report failed: " + errorText(err), "error");
    }
  }

  function clearLog() {
    state.offset = state.lines.length;
    state.extra = [];
    renderLog();
    hint("Log view cleared (new lines still arrive).", "ok");
  }

  function stopTimers() {
    if (state.timer) {
      clearInterval(state.timer);
      state.timer = null;
    }
    state.open = false;
  }

  // ------------------------------------------------------------------- init
  let initialized = false;

  function init() {
    if (initialized || !documentAlive()) return;
    initialized = true;

    const open = $("open-crushfling");
    if (open) open.addEventListener("click", () => setOpen(true));
    const close = $("cf-close");
    if (close) close.addEventListener("click", () => setOpen(false));

    const overlay = $("crushfling-overlay");
    if (overlay) {
      overlay.addEventListener("click", (event) => {
        if (event.target === overlay) setOpen(false);
      });
    }

    const bind = (id, handler) => {
      const el = $(id);
      if (el) el.addEventListener("click", handler);
    };
    bind("cf-start", startRun);
    bind("cf-stop", stopRun);
    bind("cf-refresh", () => refresh(true));
    bind("cf-preview", previewProfile);
    bind("cf-export", exportAccounts);
    bind("cf-open-logs", openLogFolder);
    bind("cf-debug-report", debugReport);
    bind("cf-clear", clearLog);
    bind("cf-pic-browse", browsePictures);
    bind("cf-pic-open", openPictures);
    bind("cf-proxy-browse", browseProxy);
    bind("cf-proxy-validate", validateProxy);
    bind("cf-bio-load", loadBioFile);
    bind("cf-bio-refresh", () => refreshBio(true));
    bind("cf-close-browsers", closeBrowsers);

    const tabs = document.querySelectorAll(".cf-tab");
    for (let i = 0; i < tabs.length; i += 1) {
      const tab = tabs[i];
      tab.addEventListener("click", () => showTab(tab.getAttribute("data-cf-pane")));
    }

    const filter = $("cf-log-filter");
    if (filter) filter.addEventListener("change", renderLog);

    const email = $("cf-email");
    if (email) {
      const syncCustom = () => {
        const isCustom = email.value === "custom";
        const custom = $("cf-custom-email");
        const hook = $("cf-mail-hook");
        if (custom) custom.disabled = !isCustom;
        if (hook) hook.disabled = !isCustom;
      };
      email.addEventListener("change", syncCustom);
      syncCustom();
    }

    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape") setOpen(false);
    });

    try {
      const view = new URLSearchParams(window.location.search).get("view");
      if (view === "crushfling") setOpen(true);
    } catch (_) { /* no URL support */ }
  }

  init();
  if (typeof document !== "undefined" && document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  }

  window.__crushfling = {
    open: () => setOpen(true),
    close: () => setOpen(false),
    refresh: () => refresh(true),
    tab: showTab,
    stop: stopTimers,
  };
})();
