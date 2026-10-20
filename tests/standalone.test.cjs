// Contract tests for the standalone CrushFling app (the "only crushfling" zip).
const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { execFileSync } = require("node:child_process");
const { unzipSync } = require("fflate");

const root = path.resolve(__dirname, "..");
const standalone = path.join(root, "standalone");
const read = (rel) => fs.readFileSync(path.join(standalone, rel), "utf8");

test("standalone tree is generated and in sync with the templates", () => {
  const out = execFileSync(
    process.execPath,
    [path.join(root, "scripts/build-crushfling-standalone.cjs"), "--check"],
    { encoding: "utf8" },
  );
  assert.match(out, /in sync/);
  for (const rel of [
    "main.go",
    "app.go",
    "go.mod",
    "wails.json",
    "run.bat",
    "install.bat",
    "README.md",
    "frontend/dist/index.html",
    "frontend/dist/crushfling.js",
    "frontend/dist/crushfling.css",
    "frontend/dist/standalone.js",
    "frontend/dist/theme.css",
    "cmd/crushfling/main.go",
    "internal/crushfling/register.go",
    "internal/proxy/proxy.go",
  ]) {
    assert.ok(fs.existsSync(path.join(standalone, rel)), `${rel} missing`);
  }
});

test("standalone module is self-contained (no chitchatbot imports, no tests)", () => {
  const gomod = read("go.mod");
  assert.match(gomod, /^module crushfling$/m);
  assert.match(gomod, /github\.com\/wailsapp\/wails\/v2 v2\.12\.0/);
  assert.match(gomod, /github\.com\/go-rod\/rod v0\.116\.2/);
  const seen = [];
  (function walk(dir) {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.name.endsWith(".go")) seen.push(full);
    }
  })(standalone);
  assert.ok(seen.length >= 20, "expected the engine sources to be copied");
  for (const file of seen) {
    const body = fs.readFileSync(file, "utf8");
    assert.equal(body.includes("chitchatbot/"), false, `${file} still imports chitchatbot`);
    assert.equal(file.endsWith("_test.go"), false, `${file} is a test file`);
  }
});

test("standalone dashboard page carries the whole six-tab panel", () => {
  const html = read("frontend/dist/index.html");
  for (const id of [
    "crushfling-overlay",
    "cf-start",
    "cf-stop",
    "cf-tab-pictures",
    "cf-tab-proxy",
    "cf-tab-bio",
    "cf-tab-browser",
    "cf-tab-settings",
    "cf-tab-logs",
    "cf-pane-pictures",
    "cf-pane-proxy",
    "cf-pane-bio",
    "cf-pane-browser",
    "cf-pane-settings",
    "cf-pane-logs",
    "cf-log",
    "cf-bio-load",
    "cf-close-browsers",
  ]) {
    assert.match(html, new RegExp(`id="${id}"`), `${id} missing from standalone page`);
  }
  assert.match(html, /<script src="crushfling\.js"><\/script>/);
  assert.match(html, /<script src="standalone\.js"><\/script>/);
  assert.match(html, /href="crushfling\.css"/);
  assert.equal(html.includes("{{PANEL}}"), false);
});

test("standalone bootstrap opens the panel and cannot be dismissed", () => {
  const js = read("frontend/dist/standalone.js");
  assert.match(js, /__crushfling\.open/);
  assert.match(js, /cf-close/);
  assert.match(js, /Escape/);
  // It must not re-implement the panel logic; the shared file stays authoritative.
  const shared = read("frontend/dist/crushfling.js");
  assert.equal(js.includes("StartBulk"), false);
  assert.match(shared, /StartBulk/);
});

test("standalone batch files are CRLF ASCII and their labels resolve", () => {
  for (const name of ["run.bat", "install.bat"]) {
    const bytes = fs.readFileSync(path.join(standalone, name));
    const text = bytes.toString("ascii");
    assert.ok(bytes.length > 100, `${name} too small`);
    assert.equal(text.split("\r\n").join("").includes("\n"), false, `${name} has bare LF`);
    assert.equal(/[^\x09\x0a\x0d\x20-\x7e]/.test(text), false, `${name} must be ASCII`);
    const labels = new Set([...text.matchAll(/^:([a-z0-9_]+)/gim)].map((m) => m[1].toLowerCase()));
    for (const ref of [...text.matchAll(/(?:goto|call)\s+:([a-z0-9_]+)/gim)].map((m) => m[1].toLowerCase())) {
      assert.ok(labels.has(ref), `${name}: :${ref} missing`);
    }
  }
});

test("the crushfling zip holds only the CrushFling app plus executables", () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "cf-standalone-"));
  for (const name of ["CrushFling.exe", "crushfling-cli.exe"]) {
    fs.writeFileSync(path.join(standalone, name), Buffer.from("MZfake-binary-for-test"));
  }
  const zipPath = path.join(tmp, "out.zip");
  try {
    execFileSync(process.execPath, [
      path.join(root, "scripts/package-crushfling-standalone.cjs"),
      "--out",
      zipPath,
    ]);
    const files = unzipSync(fs.readFileSync(zipPath));
    const names = Object.keys(files);
    assert.deepEqual(
      [...new Set(names.map((n) => n.split("/")[0]))].sort(),
      ["CrushFling", "SOURCE-MANIFEST.json"],
    );
    for (const name of ["CrushFling/run.bat", "CrushFling/install.bat", "CrushFling/CrushFling.exe", "CrushFling/crushfling-cli.exe"]) {
      assert.ok(names.includes(name), `${name} missing from zip`);
    }
    assert.equal(names.some((n) => n.includes("/data/")), false);
    assert.equal(names.some((n) => n.endsWith(".log")), false);
    const manifest = JSON.parse(Buffer.from(files["SOURCE-MANIFEST.json"]).toString());
    assert.equal(manifest.top, "CrushFling");
    assert.ok(Object.keys(manifest.sha256).length > 20);
  } finally {
    for (const name of ["CrushFling.exe", "crushfling-cli.exe"]) {
      fs.rmSync(path.join(standalone, name), { force: true });
    }
    fs.rmSync(tmp, { recursive: true, force: true });
  }
});
