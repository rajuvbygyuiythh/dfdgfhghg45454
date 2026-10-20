#!/usr/bin/env node
// Re-apply the CrushFling dashboard overlay to cmd/chitchatui/frontend/dist.
//
// `dist/` is generated output: it is not part of workspace snapshots and a
// fresh checkout only contains the stock dashboard. The overlay itself lives
// in cmd/chitchatui/frontend/overlay/ (source of truth) and this script copies
// it in. Safe to run repeatedly; run it after every fresh checkout before
// `npm test` or a Wails build.
const fs = require('node:fs');
const path = require('node:path');

const root = path.join(__dirname, '..');
const src = path.join(root, 'cmd/chitchatui/frontend/overlay');
const dist = path.join(root, 'cmd/chitchatui/frontend/dist');

function read(p) { return fs.readFileSync(p, 'utf8'); }

function replaceBlock(html, name, content) {
  const start = `<!-- ${name}:start -->`;
  const end = `<!-- ${name}:end -->`;
  const block = `${start}\n${content.trimEnd()}\n${end}`;
  const re = new RegExp(`${start}[\\s\\S]*?${end}`);
  if (re.test(html)) return html.replace(re, () => block);
  return null;
}

function main() {
  const check = process.argv.includes('--check');
  const files = ['crushfling.js', 'crushfling.css'];
  const drift = [];

  for (const name of files) {
    const from = path.join(src, name);
    const to = path.join(dist, name);
    const want = read(from);
    if (check) {
      if (!fs.existsSync(to) || read(to) !== want) drift.push(name);
      continue;
    }
    fs.writeFileSync(to, want);
  }

  const htmlPath = path.join(dist, 'index.html');
  let html = read(htmlPath);
  const overlay = read(path.join(src, 'overlay.html'));
  const sidebar = read(path.join(src, 'sidebar.html'));

  const withOverlay = replaceBlock(html, 'crushfling-overlay', overlay);
  const withSidebar = replaceBlock(html, 'crushfling-sidebar', sidebar);

  if (check) {
    const normalize = (t) => t.replace(/[ \t]+\n/g, '\n');
    if (!withOverlay || normalize(withOverlay) !== normalize(html)) drift.push('index.html overlay block');
    if (!withSidebar || normalize(withSidebar) !== normalize(html)) drift.push('index.html sidebar block');
    if (!html.includes('./crushfling.css?v=1')) drift.push('index.html css link');
    if (!html.includes('./crushfling.js?v=1')) drift.push('index.html script tag');
    if (drift.length) {
      console.error('CrushFling overlay is out of sync: ' + drift.join(', '));
      console.error('Run: npm run overlay:sync');
      process.exit(1);
    }
    console.log('CrushFling overlay is in sync.');
    return;
  }

  // A stock index.html has no markers, so insert at stable positions.
  if (withSidebar) {
    html = withSidebar;
  } else {
    const block = `<!-- crushfling-sidebar:start -->\n${sidebar.trimEnd()}\n<!-- crushfling-sidebar:end -->`;
    html = html.replace(/<\/nav>/, (m) => `${m}\n${block}`);
  }
  if (withOverlay) {
    html = withOverlay;
  } else {
    const block = `<!-- crushfling-overlay:start -->\n${overlay.trimEnd()}\n<!-- crushfling-overlay:end -->\n\n`;
    // Keep the load order used by the hand-edited page: overlay markup and
    // crushfling.js run before app.js.
    const appScript = html.match(/^[ \t]*<script src="\.\/app\.js[^>]*><\/script>\s*$/m);
    if (appScript) {
      html = html.slice(0, appScript.index) + block + html.slice(appScript.index);
    } else {
      html = html.replace('</body>', block + '</body>');
    }
  }
  if (!html.includes('./crushfling.css?v=1')) {
    html = html.replace('</head>', '  <link rel="stylesheet" href="./crushfling.css?v=1">\n</head>');
  }
  if (!html.includes('./crushfling.js?v=1')) {
    const tag = '  <script src="./crushfling.js?v=1"></script>\n';
    if (html.includes('<script src="./app.js')) {
      html = html.replace('  <script src="./app.js', tag + '  <script src="./app.js');
    } else {
      html = html.replace('</body>', tag + '</body>');
    }
  }
  fs.writeFileSync(htmlPath, html);
  console.log('CrushFling overlay restored into cmd/chitchatui/frontend/dist.');
}

main();
