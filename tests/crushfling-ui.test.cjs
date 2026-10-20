// CrushFling overlay: it must not disturb the two-page dashboard and must call
// only the App bindings that exist in Go.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('jsdom');

const assets = path.join(__dirname, '../cmd/chitchatui/frontend/dist');

// `dist/` is generated output (and is not part of snapshots), so a fresh
// checkout can be missing the overlay. Restore it from the tracked source
// before reading anything; the sync is idempotent.
if (!fs.existsSync(path.join(assets, 'crushfling.js')) || !fs.existsSync(path.join(assets, 'crushfling.css'))) {
  require('node:child_process').execFileSync(process.execPath, [
    path.join(__dirname, '../scripts/crushfling-overlay.cjs'),
  ], { stdio: 'ignore' });
}

const html = fs.readFileSync(path.join(assets, 'index.html'), 'utf8');
const crushJs = fs.readFileSync(path.join(assets, 'crushfling.js'), 'utf8');
const tick = (ms = 0) => new Promise((resolve) => setTimeout(resolve, ms));

function setup(api = {}, url = 'http://localhost/') {
  const dom = new JSDOM(html, { runScripts: 'outside-only', url });
  const w = dom.window;
  w.go = {
    main: {
      App: {
        GetStats: async () => ({ total: 0, success: 0, failed: 0, running: false }),
        GetLogs: async () => [],
        GetPictureCount: async () => 0,
        GetCaptionCount: async () => 0,
        ListAccounts: async () => [],
        StartBulk: async () => undefined,
        StopBulk: async () => undefined,
        PreviewProfile: async () => ({ username: 'preview' }),
        ...api,
      },
    },
  };
  w.URL.createObjectURL = () => 'blob:test';
  w.URL.revokeObjectURL = () => {};
  w.eval(crushJs);
  const close = () => {
    try {
      if (w.__crushfling) w.__crushfling.stop();
    } catch (_) { /* already closed */ }
    try { dom.window.close(); } catch (_) { /* already closed */ }
  };
  return { dom, w, d: w.document, close };
}

test('CrushFling is an overlay and the dashboard keeps its two pages', () => {
  const { d, close } = setup();
  try {
    const overlay = d.getElementById('crushfling-overlay');
    assert.ok(overlay, 'overlay exists');
    assert.equal(overlay.hidden, true, 'hidden until opened');
    assert.equal(d.querySelectorAll('.tab-panel').length, 2);
    assert.deepEqual([...d.querySelectorAll('.nav-button')].map((b) => b.dataset.tab), ['show', 'bot']);
    assert.notEqual(d.getElementById('open-crushfling'), null);
    // The overlay controls are not part of either dashboard form slice.
    assert.equal(d.querySelector('[data-panel="show"] #cf-start, [data-panel="bot"] #cf-start'), null);
  } finally { close(); }
});

test('sidebar link opens the panel and ?view=crushfling deep-links to it', () => {
  const first = setup();
  try {
    first.d.getElementById('open-crushfling').click();
    assert.equal(first.d.getElementById('crushfling-overlay').hidden, false);
    first.d.getElementById('cf-close').click();
    assert.equal(first.d.getElementById('crushfling-overlay').hidden, true);
  } finally { first.close(); }

  const deep = setup({}, 'http://localhost/?view=crushfling');
  try {
    assert.equal(deep.d.getElementById('crushfling-overlay').hidden, false);
  } finally { deep.close(); }
});

test('Start sends the Go config shape and stats render', async () => {
  const seen = [];
  const { d, close } = setup({
    StartBulk: async (cfg) => { seen.push(cfg); },
    GetStats: async () => ({ total: 4, success: 2, failed: 1, running: true }),
    GetPictureCount: async () => 9,
    GetCaptionCount: async () => 7,
  });
  try {
    d.getElementById('open-crushfling').click();
    d.getElementById('cf-count').value = '4';
    d.getElementById('cf-conc').value = '2';
    d.getElementById('cf-email').value = '1secmail';
    d.getElementById('cf-start').click();
    await tick(30);

    assert.equal(seen.length, 1);
    const cfg = seen[0];
    assert.equal(cfg.count, 4);
    assert.equal(cfg.concurrency, 2);
    assert.equal(cfg.email_provider, '1secmail');
    assert.equal(cfg.data_dir, '', 'blank means the Go service data folder');
    assert.equal(cfg.avatar_dir, '', 'blank means <data>/pictures');
    assert.equal(typeof cfg.mail_poll_sec, 'number');
    assert.equal(typeof cfg.use_browser, 'boolean');
    assert.equal(typeof cfg.browser_verify, 'boolean');
    assert.equal(cfg.debug, true, 'debug logging on by default');
    assert.equal(cfg.trace_http, false, 'body tracing stays opt-in');
    assert.equal(cfg.custom_email_hook, '', 'custom mail hook is forwarded');
    assert.notEqual(d.getElementById('cf-log-filter'), null);
    assert.notEqual(d.getElementById('cf-mail-hook'), null);

    assert.equal(d.getElementById('cf-ok').textContent, '2');
    assert.equal(d.getElementById('cf-fail').textContent, '1');
    assert.equal(d.getElementById('cf-running').textContent, 'Running');
    assert.equal(d.getElementById('cf-start').disabled, true);
    assert.equal(d.getElementById('cf-stop').disabled, false);
    assert.equal(d.getElementById('cf-pic-count').textContent, '9');
    assert.equal(d.getElementById('cf-cap-count').textContent, '7');
  } finally { close(); }
});

test('live log is plain text and preview appends JSON', async () => {
  const { d, close } = setup({
    GetLogs: async () => ['[12:00:01] info: inbox created', '<img src=x onerror=alert(1)>'],
  });
  try {
    d.getElementById('open-crushfling').click();
    await tick(30);
    const log = d.getElementById('cf-log');
    assert.match(log.textContent, /inbox created/);
    assert.equal(d.querySelector('#cf-log img, #cf-log script'), null);
    assert.match(log.textContent, /<img src=x/);

    d.getElementById('cf-preview').click();
    await tick(30);
    assert.match(d.getElementById('cf-log').textContent, /"username": "preview"/);
  } finally { close(); }
});

test('crushfling.js never injects markup', () => {
  assert.doesNotMatch(crushJs, /innerHTML|outerHTML|insertAdjacentHTML|document\.write/);
  assert.doesNotMatch(crushJs, /https?:\/\//, 'no hard-coded remote endpoint in the UI layer');
});

test('debug buttons degrade gracefully when their bindings are missing', async () => {
  const { d, close } = setup();
  try {
    d.getElementById('open-crushfling').click();
    await tick(30);

    d.getElementById('cf-debug-report').click();
    await tick(30);
    assert.match(d.getElementById('cf-hint').textContent, /Debug report failed/);

    d.getElementById('cf-open-logs').click();
    await tick(30);
    assert.match(d.getElementById('cf-hint').textContent, /Could not open the folder/);

    // The panel is still usable; no binding error escaped as an exception.
    assert.equal(d.getElementById('crushfling-overlay').hidden, false);
  } finally { close(); }
});

test('overlay source of truth stays in sync with the generated dist copy', () => {
  const src = path.join(__dirname, '../cmd/chitchatui/frontend/overlay');
  assert.equal(
    crushJs,
    fs.readFileSync(path.join(src, 'crushfling.js'), 'utf8'),
    'run npm run overlay:sync after editing the overlay',
  );
  assert.equal(
    fs.readFileSync(path.join(assets, 'crushfling.css'), 'utf8'),
    fs.readFileSync(path.join(src, 'crushfling.css'), 'utf8'),
  );
  assert.match(html, /<!-- crushfling-overlay:start -->/);
  assert.match(html, /<!-- crushfling-sidebar:start -->/);
  assert.match(html, /\.\/crushfling\.css\?v=1/);
  assert.match(html, /\.\/crushfling\.js\?v=1/);
});

test('the panel is organised in six sections with Start/Stop at the top', () => {
  const { d, close } = setup();
  try {
    d.getElementById('open-crushfling').click();
    const tabs = [...d.querySelectorAll('.cf-tab')].map((t) => t.dataset.cfPane);
    assert.deepEqual(tabs, ['pictures', 'proxy', 'bio', 'browser', 'settings', 'logs']);
    // Each section has its controls (the operator asked for all of these).
    for (const [pane, ids] of Object.entries({
      pictures: ['cf-avatar', 'cf-pic-browse', 'cf-pic-open'],
      proxy: ['cf-proxy-file', 'cf-proxy-entries', 'cf-proxy-browse', 'cf-proxy-validate'],
      bio: ['cf-bio-file', 'cf-bio-load', 'cf-bio-refresh', 'cf-bio-preview'],
      browser: ['cf-headless', 'cf-browser', 'cf-browser-verify', 'cf-conc', 'cf-close-browsers'],
      settings: ['cf-count', 'cf-email', 'cf-data', 'cf-poll', 'cf-timeout', 'cf-retries', 'cf-min-delay', 'cf-max-delay'],
      logs: ['cf-log', 'cf-log-filter', 'cf-open-logs', 'cf-debug-report', 'cf-clear'],
    })) {
      for (const id of ids) assert.notEqual(d.getElementById(id), null, pane + ' is missing ' + id);
    }
    // Start/Stop live in the header, not buried in a tab.
    const header = d.querySelector('.cf-head');
    assert.notEqual(header.querySelector('#cf-start'), null);
    assert.notEqual(header.querySelector('#cf-stop'), null);

    // Clicking a tab shows exactly one pane.
    d.getElementById('cf-tab-bio').click();
    assert.equal(d.getElementById('cf-pane-bio').hidden, false);
    assert.equal(d.getElementById('cf-pane-pictures').hidden, true);
  } finally { close(); }
});

test('browser close, bio load and proxy validate call their Go bindings', async () => {
  const calls = [];
  const { d, close } = setup({
    CloseCrushBrowsers: async () => { calls.push('close'); return 2; },
    GetCrushBrowsersOpen: async () => 0,
    PickCrushBioFile: async () => ({ path: '/data/bio.txt', file_name: 'bio.txt', remaining: 3, total: 5, preview: ['one', 'two'] }),
    ValidateCrushProxies: async (file, raw) => { calls.push('proxy:' + file + ':' + raw); return { total: 2, valid: 1, errors: ['line 2: bad'] }; },
    PickCrushPictureFolder: async () => '/pics',
    GetPictureCount: async () => 4,
  });
  try {
    d.getElementById('open-crushfling').click();

    d.getElementById('cf-close-browsers').click();
    await tick(30);
    assert.match(d.getElementById('cf-browser-status').textContent, /Closed 2 browser/);
    assert.equal(d.getElementById('cf-browsers-open').value, '0');

    d.getElementById('cf-bio-load').click();
    await tick(30);
    assert.equal(d.getElementById('cf-bio-file').value, '/data/bio.txt');
    assert.match(d.getElementById('cf-bio-status').textContent, /3 left of 5/);
    assert.equal(d.querySelectorAll('#cf-bio-preview li').length, 2);
    assert.equal(d.querySelector('#cf-bio-preview li').textContent, 'one');

    d.getElementById('cf-proxy-file').value = 'proxies.txt';
    d.getElementById('cf-proxy-entries').value = '';
    d.getElementById('cf-proxy-validate').click();
    await tick(30);
    assert.ok(calls.includes('proxy:proxies.txt:'));
    assert.match(d.getElementById('cf-proxy-status').textContent, /1\/2 valid/);

    d.getElementById('cf-pic-browse').click();
    await tick(30);
    assert.equal(d.getElementById('cf-avatar').value, '/pics');
    assert.equal(d.getElementById('cf-pic-count').textContent, '4');
  } finally { close(); }
});

test('Start and Stop both go through the Go bindings from the header', async () => {
  const seen = [];
  let running = false;
  const { d, close } = setup({
    StartBulk: async () => { seen.push('start'); running = true; },
    StopBulk: async () => { seen.push('stop'); running = false; },
    GetStats: async () => ({ total: 1, success: 0, failed: 0, running }),
  });
  try {
    d.getElementById('open-crushfling').click();
    d.getElementById('cf-start').click();
    await tick(30);
    assert.equal(seen[0], 'start');
    d.getElementById('cf-stop').click();
    await tick(30);
    assert.equal(seen[1], 'stop');
  } finally { close(); }
});
