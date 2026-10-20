// Builds the clean, ready-to-run Windows release zip.
//
// Included: the whole source tree (minus runtime/build data) plus the three
// prebuilt Windows executables in build/bin. Excluded: git metadata, node
// modules, accounts/sessions/cookies, logs, verification reports and any
// earlier zip. The zip carries a SHA-256 manifest so tampering is detectable.
//
// Usage: node scripts/package-release.cjs [--out path.zip]
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { zipSync } = require("fflate");
const { excluded, hashes } = require("./source-files.cjs");

const root = path.resolve(__dirname, "..");
const TOP = "CrushFling-Bot";
const BINARIES = ["chitchatui.exe", "chitchatbot.exe", "crushfling.exe"];
const DEFAULT_OUT = path.join(
  root,
  "release",
  `CrushFling-Bot-Windows-x64.zip`,
);

function collect() {
  const files = {};
  const add = (rel, data) => {
    files[`${TOP}/${rel.replaceAll(path.sep, "/")}`] = data;
  };
  function visit(dir, prefix = "") {
    for (const entry of fs
      .readdirSync(dir, { withFileTypes: true })
      .sort((a, b) => a.name.localeCompare(b.name, "en"))) {
      const rel = prefix + entry.name;
      if (excluded(rel)) continue;
      if (entry.isSymbolicLink())
        throw new Error(`Release symlink is not allowed: ${rel}`);
      if (entry.isDirectory()) visit(path.join(dir, entry.name), rel + "/");
      else if (entry.isFile()) add(rel, fs.readFileSync(path.join(dir, entry.name)));
    }
  }
  visit(root);
  return files;
}

function addBinaries(files, requireAll) {
  for (const name of BINARIES) {
    const file = path.join(root, "build", "bin", name);
    if (!fs.existsSync(file)) {
      if (requireAll) throw new Error(`missing prebuilt binary: build/bin/${name}`);
      console.warn(`warn: build/bin/${name} missing, zip will build it on first run`);
      continue;
    }
    const bytes = fs.readFileSync(file);
    if (bytes.subarray(0, 2).toString() !== "MZ")
      throw new Error(`build/bin/${name} is not a Windows PE executable`);
    files[`${TOP}/build/bin/${name}`] = bytes;
  }
}

function buildInfo(files) {
  const rows = BINARIES.filter((n) => files[`${TOP}/build/bin/${n}`]).map((n) => {
    const data = files[`${TOP}/build/bin/${n}`];
    const sum = crypto.createHash("sha256").update(data).digest("hex");
    return `  ${n.padEnd(18)} ${String(data.length).padStart(10)} bytes  sha256 ${sum}`;
  });
  return [
    "CrushFling Bot - Windows x64 release bundle",
    `Built (UTC): ${new Date().toISOString()}`,
    "",
    "Prebuilt executables:",
    ...rows,
    "",
    "Start with run.bat. Rebuild with install.bat (Go 1.25+, Wails CLI, WebView2).",
    "Runtime data (accounts, sessions, logs) is never inside this zip.",
    "",
  ].join("\r\n");
}

function main() {
  const outIndex = process.argv.indexOf("--out");
  const out = outIndex > 0 ? path.resolve(process.argv[outIndex + 1]) : DEFAULT_OUT;
  const files = collect();
  addBinaries(files, true);
  files[`${TOP}/BUILD-INFO.txt`] = Buffer.from(buildInfo(files), "utf8");

  const manifest = {
    schema: 1,
    top: TOP,
    created: new Date().toISOString(),
    sha256: hashes(files),
  };
  files["SOURCE-MANIFEST.json"] = Buffer.from(JSON.stringify(manifest, null, 2));

  fs.mkdirSync(path.dirname(out), { recursive: true });
  const bytes = zipSync(files, { level: 9 });
  fs.writeFileSync(out, bytes);
  const names = Object.keys(files);
  console.log(`release: ${out}`);
  console.log(`files  : ${names.length} (+ manifest)`);
  console.log(`size   : ${(bytes.length / 1024 / 1024).toFixed(1)} MB`);
  const back = zipSync; // keep fflate import obvious for future changes
  void back;
}

main();
