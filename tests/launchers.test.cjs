const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const root = path.resolve(__dirname, "..");
const read = (name) => fs.readFileSync(path.join(root, name));

test("run.bat and install.bat exist as pure CRLF ASCII batch files", () => {
  for (const name of ["run.bat", "install.bat"]) {
    const bytes = read(name);
    const text = bytes.toString("ascii");
    assert.ok(bytes.length > 200, `${name} is too small`);
    assert.equal(bytes.includes(0), false, `${name} has NUL bytes`);
    assert.equal(text.includes("\r\n"), true, `${name} is not CRLF`);
    assert.equal(
      text.split("\r\n").join("").includes("\n"),
      false,
      `${name} has a bare LF (cmd.exe can mis-parse it)`,
    );
    assert.equal(
      /[^\x09\x0a\x0d\x20-\x7e]/.test(text),
      false,
      `${name} must stay ASCII for cmd.exe`,
    );
  }
});

test("run.bat launches the dashboard, can fall back to install.bat and exposes the CLI", () => {
  const text = read("run.bat").toString("ascii").toLowerCase();
  assert.match(text, /build\\bin\\chitchatui\.exe/);
  assert.match(text, /start "" \/d "%root%"/);
  assert.match(text, /call "%root%\\install\.bat" build/);
  assert.match(text, /build\\bin\\crushfling\.exe/);
  assert.match(text, /run\.bat cli/);
  // A missing exe must not silently open an empty window.
  assert.match(text, /was not found/);
});

test("install.bat builds the dashboard and both CLIs and warms the module cache", () => {
  const text = read("install.bat").toString("ascii").toLowerCase();
  assert.match(text, /wails build -clean -webview2 error/);
  assert.match(text, /go build -tags desktop,production/);
  assert.match(text, /cmd\\chitchatbot/);
  assert.match(text, /cmd\\crushfling/);
  assert.match(text, /build\\bin\\crushfling\.exe/);
  assert.match(text, /go mod download/);
  assert.match(text, /double-click run\.bat/);
});

test("every goto/call target in the batch files exists", () => {
  for (const name of ["run.bat", "install.bat"]) {
    const text = read(name).toString("ascii");
    const labels = new Set(
      [...text.matchAll(/^:([a-z0-9_]+)/gim)].map((m) => m[1].toLowerCase()),
    );
    const refs = [
      ...text.matchAll(/(?:goto|call)\s+:([a-z0-9_]+)/gim),
    ].map((m) => m[1].toLowerCase());
    for (const ref of refs) assert.ok(labels.has(ref), `${name}: :${ref} missing`);
  }
});
