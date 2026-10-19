// Two-page UI: Show Mode (5 sections + live log) and Background Bot.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('jsdom');

const assets = path.join(__dirname, '../cmd/chitchatui/frontend/dist');
const html = fs.readFileSync(path.join(assets, 'index.html'), 'utf8');
const appJs = fs.readFileSync(path.join(assets, 'app.js'), 'utf8');
const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

function snapshot(over = {}) {
  return {
    running: false,
    show_mode_running: false,
    license_invalid: false,
    license_key: '',
    license_reason: '',
    home: { bot_accounts: 1, running_accounts: 0, messages_sent: 6, traffic_users: 2, snap_tg_sent_count: 0, dead_accounts: 0 },
    browser: { profile: 'default', backend: 'rod', headless: false, use_saved_session: false },
    message: { chat_mode: 'single', sequence: 'hello\nwait', usernames: '@u', loaded_sequences: [], ai_enabled: false, ai_base_url: '', ai_model: '', ai_api_key: '' },
    proxy: { enabled: false, entries: '' },
    general: {
      first_message: { mode: 'bot', bot_first_percent: 70, wait_timeout: false, wait_min_seconds: 30, wait_max_seconds: 60 },
      poll_min_seconds: 20, poll_max_seconds: 45,
      action_min_seconds: 20, action_max_seconds: 45,
      match_reply_timeout_min_seconds: 60, match_reply_timeout_max_seconds: 60,
      match_close_wait_min_seconds: 0, match_close_wait_max_seconds: 0,
      reply_delay_min_seconds: 5, reply_delay_max_seconds: 9,
    },
    show_mode: {
      first_message: { mode: 'bot', bot_first_percent: 70, wait_timeout: false, wait_min_seconds: 30, wait_max_seconds: 60 },
      reply_delay_min_seconds: 5, reply_delay_max_seconds: 9,
      auto_skip: true, no_reply_min_seconds: 60, no_reply_max_seconds: 90,
      next_chat_min_seconds: 3, next_chat_max_seconds: 7,
    },
    browser_options: [],
    logs: [],
    ...over,
  };
}

function setup(api = {}, snap = snapshot()) {
  const dom = new JSDOM(html, { runScripts: 'outside-only', url: 'http://localhost/' });
  const w = dom.window;
  w.addEventListener = () => {};
  w.go = { main: { App: { GetInitialState: async () => snap, ...api } } };
  w.eval(appJs);
  w.bindForms();
  w.bindGlobalState();
  w.bindActions();
  return { dom, w, d: w.document, snap };
}

test('the app is two pages: Show Mode first, Background Bot second', () => {
  const { dom, d } = setup();
  try {
    assert.deepEqual([...d.querySelectorAll('.nav-button')].map((b) => b.dataset.tab), ['show', 'bot']);
    assert.equal(d.querySelectorAll('.tab-panel').length, 2);
    assert.ok(d.querySelector('[data-panel="show"]').classList.contains('active'));
    const headings = [...d.querySelectorAll('[data-panel="show"] h2')].map((h) => h.textContent.trim());
    assert.ok(headings.some((h) => h.startsWith('1 · Setting')));
    assert.ok(headings.some((h) => h.startsWith('2 · Proxy')));
    assert.ok(headings.some((h) => h.startsWith('3 · Message')));
    assert.ok(headings.some((h) => h.startsWith('4 · Run Browser')));
    assert.ok(headings.some((h) => h.startsWith('5 · Live Logs')));
    assert.notEqual(d.getElementById('sm-run'), null);
    assert.equal(d.querySelector('#start-bot, #start-show-mode, #dashboard-chooser, #home-chat'), null);
    assert.notEqual(d.getElementById('bg-start'), null);
    assert.notEqual(d.getElementById('bg-launch-login'), null);
  } finally { dom.window.close(); }
});

test('sidebar switches between the two pages', () => {
  const { dom, w, d } = setup();
  try {
    w.activateTab('bot');
    assert.ok(d.querySelector('[data-panel="bot"]').classList.contains('active'));
    assert.ok(!d.querySelector('[data-panel="show"]').classList.contains('active'));
    assert.ok(d.querySelector('.nav-button[data-tab="bot"]').classList.contains('active'));
    w.activateTab('show');
    assert.ok(d.querySelector('[data-panel="show"]').classList.contains('active'));
  } finally { dom.window.close(); }
});

test('chat mode reveals exactly one editor per page, independently', () => {
  const { dom, w, d, snap } = setup();
  try {
    w.fillForms(snap);
    const visible = (id) => d.getElementById(id).style.display !== 'none';
    assert.ok(visible('sm-section-single-mode'));
    assert.ok(!visible('sm-section-folder-mode'));

    w.setChatMode('sm', 'folder');
    assert.ok(visible('sm-section-folder-mode'));
    assert.ok(!visible('sm-section-single-mode'));
    assert.equal(d.getElementById('sm-chat-mode-folder').checked, true);

    assert.equal(d.getElementById('bg-chat-mode-single').checked, true);
    w.setChatMode('bg', 'eva_chat_flow');
    assert.ok(visible('bg-section-evachat-mode'));
    assert.equal(d.getElementById('bg-chat-mode-evachat').checked, true);
  } finally { dom.window.close(); }
});

test('Show Mode live log renders SMS, rules, settings and literal text', async () => {
  const entries = [
    { seq: 1, time: '14:03:21', kind: 'in', text: 'hey what up', detail: 'partner · pending=1' },
    { seq: 2, time: '14:03:23', kind: 'out', text: 'hi', rule: 'EVA · greeting', detail: 'opener · no delay', setting_ok: true },
    { seq: 3, time: '14:03:44', kind: 'out', text: 'f 20 u?', rule: 'EVA · age_gender', detail: 'reply #2 · delay 3.2s (setting 3.0s–6.0s)', setting_ok: true },
    { seq: 4, time: '14:04:10', kind: 'skip', text: 'skipped conversation', detail: 'no-reply auto-skip · waited 12.0s (setting 10.0s–20.0s)' },
    { seq: 5, time: '14:04:11', kind: 'error', text: 'SEND UNCONFIRMED', detail: 'inspect the page' },
    { seq: 6, time: '14:04:12', kind: 'in', text: '<img src=x onerror=alert(1)>', detail: 'partner' },
  ];
  const { dom, w, d } = setup({ GetShowModeLog: async () => ({ version: 6, entries }) });
  try {
    await w.refreshShowLog();
    const rows = [...d.querySelectorAll('#sm-log-list .log-row')];
    assert.equal(rows.length, 6);
    assert.match(d.getElementById('sm-log-list').textContent, /"hey what up"/);
    assert.match(d.getElementById('sm-log-list').textContent, /EVA · age_gender/);
    assert.match(d.getElementById('sm-log-list').textContent, /delay 3\.2s \(setting 3\.0s–6\.0s\)/);
    assert.match(d.getElementById('sm-log-list').textContent, /no-reply auto-skip/);
    assert.equal(d.querySelector('#sm-log-list img, #sm-log-list script'), null);
    assert.match(d.getElementById('sm-log-list').textContent, /<img src=x/);
    assert.equal(d.getElementById('sm-log-empty').hidden, true);

    d.getElementById('sm-log-filter').value = 'sms';
    d.getElementById('sm-log-filter').dispatchEvent(new w.Event('change'));
    assert.equal(d.querySelectorAll('#sm-log-list .log-row').length, 4);

    d.getElementById('sm-log-filter').value = 'errors';
    d.getElementById('sm-log-filter').dispatchEvent(new w.Event('change'));
    assert.equal(d.querySelectorAll('#sm-log-list .log-row').length, 1);
  } finally { dom.window.close(); }
});

test('live log fetches only new entries and advances the sequence', async () => {
  const calls = [];
  const { dom, w, d } = setup({
    GetShowModeLog: async (after) => {
      calls.push(after);
      if (after === 0) {
        return { version: 2, entries: [
          { seq: 1, time: '10:00:00', kind: 'system', text: 'settings applied', detail: 'profile=default' },
          { seq: 2, time: '10:00:01', kind: 'in', text: 'hello', detail: 'partner' },
        ] };
      }
      return { version: 2, entries: [] };
    },
  });
  try {
    await w.refreshShowLog();
    assert.deepEqual(calls, [0]);
    assert.equal(d.querySelectorAll('#sm-log-list .log-row').length, 2);

    await w.refreshShowLog();
    assert.deepEqual(calls, [0, 2]);
    assert.equal(d.querySelectorAll('#sm-log-list .log-row').length, 2);
  } finally { dom.window.close(); }
});

test('Clear removes rendered entries without calling the browser', async () => {
  let cleared = 0;
  const { dom, w, d } = setup({
    ClearShowModeLog: async () => { cleared++; },
    GetShowModeLog: async () => ({ version: 1, entries: [{ seq: 1, time: '10:00:00', kind: 'in', text: 'hello', detail: 'partner' }] }),
  });
  try {
    await w.refreshShowLog();
    d.getElementById('sm-log-clear').click();
    await tick();
    assert.equal(cleared, 1);
    assert.equal(d.querySelectorAll('#sm-log-list .log-row').length, 0);
    assert.equal(d.getElementById('sm-log-empty').hidden, false);
  } finally { dom.window.close(); }
});

test('Test Proxy renders literal per-line results and handles empty input', async () => {
  const calls = [];
  const { dom, w, d } = setup({
    TestProxyEntries: async (raw) => {
      calls.push(raw);
      return [
        { line: 1, label: 'socks5://host:1080', ok: true, status: 200, latency_ms: 12, detail: 'PASS' },
        { line: 2, label: '<img src=x>', ok: false, tunnel_ok: true, status: 403, latency_ms: 9, detail: '<script>bad</script>', provider_message: 'quota <b>exhausted</b>', proxy_authenticate: 'Basic realm="proxy"', hint: 'Try another plan.' },
      ];
    },
  });
  try {
    d.getElementById('sm-proxy-entries').value = '';
    d.getElementById('sm-test-proxy').click();
    await tick();
    assert.equal(calls.length, 0);
    assert.match(d.getElementById('sm-proxy-status').textContent, /Enter at least one proxy/);

    d.getElementById('sm-proxy-entries').value = 'socks5://host:1080';
    d.getElementById('sm-test-proxy').click();
    await new Promise((resolve) => setTimeout(resolve, 20));
    assert.equal(calls.length, 1);
    assert.equal(d.querySelector('#sm-proxy-results img, #sm-proxy-results script'), null);
    assert.match(d.getElementById('sm-proxy-results').textContent, /REACHABLE \/ SITE RESTRICTED.*HTTP 403/);
    assert.match(d.getElementById('sm-proxy-results').textContent, /Provider said: “quota <b>exhausted<\/b>”/);
    assert.equal(d.getElementById('sm-test-proxy').disabled, false);
  } finally { dom.window.close(); }
});

test('profile list marks saved sessions and explains auto-login', async () => {
  const { dom, w, d, snap } = setup({
    ListProfiles: async () => [
      { name: 'default', has_session: true, saved_at: '03:32' },
      { name: 'work', has_session: true, saved_at: '03:35' },
    ],
  });
  try {
    w.fillForms(snap);
    await w.refreshProfiles();
    assert.equal(d.getElementById('sm-profile-default').checked, true);
    assert.equal(d.getElementById('sm-profile-session-choice').checked, false);
    assert.match(d.getElementById('sm-profile-session').textContent, /Fresh browser/);

    d.getElementById('sm-profile-session-choice').checked = true;
    d.getElementById('sm-profile-session-choice').dispatchEvent(new w.Event('change'));
    assert.match(d.getElementById('sm-profile-session').textContent, /Saved default session/);
    assert.ok(d.getElementById('sm-profile-session').classList.contains('status-ok'));

    d.getElementById('sm-profile-default').checked = true;
    d.getElementById('sm-profile-default').dispatchEvent(new w.Event('change'));
    assert.match(d.getElementById('sm-profile-session').textContent, /session is saved for the session option/);
  } finally { dom.window.close(); }
});

test('nested subnav lists the required sections per mode', () => {
  const { dom, d } = setup();
  try {
    const show = [...d.querySelectorAll('#subnav-show .subnav-button')].map((b) => b.dataset.subpanel);
    assert.deepEqual(show, ['show-setting', 'show-proxy', 'show-message', 'show-run', 'show-logs']);
    const bot = [...d.querySelectorAll('#subnav-bot .subnav-button')].map((b) => b.dataset.subpanel);
    assert.deepEqual(bot, ['bg-login', 'bg-proxy', 'bg-message', 'bg-settings', 'bg-logs']);
    assert.equal(d.querySelector('#subnav-show').hidden, false);
    assert.equal(d.querySelector('#subnav-bot').hidden, true);
  } finally { dom.window.close(); }
});

test('sub-pages render one section and keep typed values across switches', () => {
  const { dom, w, d, snap } = setup();
  try {
    w.fillForms(snap);
    w.bindGlobalState();

    d.getElementById('sm-reply-min').value = '42';
    d.getElementById('sm-reply-min').dispatchEvent(new w.Event('input'));
    d.getElementById('sm-proxy-entries').value = 'socks5://kept:1080';
    d.getElementById('sm-proxy-entries').dispatchEvent(new w.Event('input'));

    w.activateSubpanel('show-proxy');
    assert.ok(d.querySelector('[data-subpanel="show-proxy"]').classList.contains('active'));
    assert.ok(!d.querySelector('[data-subpanel="show-setting"]').classList.contains('active'));
    w.activateSubpanel('show-setting');
    assert.equal(d.getElementById('sm-reply-min').value, '42');
    w.activateSubpanel('show-proxy');
    assert.equal(d.getElementById('sm-proxy-entries').value, 'socks5://kept:1080');

    // Background slice keeps its own values independently.
    w.activateTab('bot');
    assert.equal(d.querySelector('#subnav-bot').hidden, false);
    d.getElementById('bg-proxy-entries').value = 'http://bg-kept:8080';
    d.getElementById('bg-proxy-entries').dispatchEvent(new w.Event('input'));
    w.activateSubpanel('bg-message');
    assert.ok(!d.querySelector('[data-subpanel="bg-proxy"]').classList.contains('active'));
    w.activateSubpanel('bg-proxy');
    assert.equal(d.getElementById('bg-proxy-entries').value, 'http://bg-kept:8080');
    // Stats stay outside the sub-pages and remain in the DOM.
    assert.equal(d.getElementById('bg-stat-running').textContent, '0');
  } finally { dom.window.close(); }
});

test('running state disables Run Browser and drives the badges', () => {
  const { dom, w, d } = setup();
  try {
    w.fillForms(snapshot({ show_mode_running: true, running: true }));
    assert.equal(d.getElementById('sm-run').disabled, true);
    assert.equal(d.getElementById('show-run-badge').textContent, 'Running');
    assert.equal(d.getElementById('running-pill').textContent, 'Running');
    assert.equal(d.getElementById('bg-start').disabled, true);
    assert.equal(d.getElementById('bg-stop').disabled, false);

    w.fillForms(snapshot());
    assert.equal(d.getElementById('sm-run').disabled, false);
    assert.equal(d.getElementById('show-run-badge').textContent, 'Idle');
  } finally { dom.window.close(); }
});

test('events update the badges, licence overlay and login status', () => {
  const events = {};
  const { dom, w, d } = setup();
  try {
    w.runtime = { EventsOn: (name, fn) => { events[name] = fn; } };
    w.bindEvents();
    events['ui:show_running'](true);
    assert.equal(d.getElementById('show-run-badge').textContent, 'Running');
    assert.equal(d.getElementById('sm-run').disabled, true);
    events['ui:running'](true);
    assert.equal(d.getElementById('running-pill').textContent, 'Running');
    events['ui:log']({ level: 'info', message: '[default fresh 1/1] Login saved to login_info.json' });
    assert.match(d.getElementById('bg-login-status').textContent, /Login saved/);
    events['ui:license_invalid']({ key: 'K1', reason: 'expired' });
    assert.equal(d.getElementById('license-overlay').style.display, 'flex');
    assert.equal(d.getElementById('license-message').textContent, 'expired');
  } finally { dom.window.close(); }
});

test('backend failures are forwarded without their arguments', async () => {
  const { dom, w } = setup({ RunBrowser: async () => { throw new Error('synthetic backend failure'); } });
  const messages = [];
  try {
    w.runtime = { LogError: (message) => messages.push(message) };
    await assert.rejects(w.invoke('RunBrowser', { password: 'DO_NOT_LOG_ARGUMENTS' }), /synthetic backend failure/);
    assert.equal(messages.length, 1);
    assert.doesNotMatch(messages[0], /DO_NOT_LOG_ARGUMENTS/);
  } finally { dom.window.close(); }
});
