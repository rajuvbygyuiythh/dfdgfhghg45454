// Packages the standalone CrushFling app into one Windows zip.
//
// Input : ./standalone (generated) with CrushFling.exe + crushfling-cli.exe
//         already built at its root (CI builds them on windows-latest;
//         install.bat produces the same files locally).
// Output: release/CrushFling-Only-Windows-x64.zip
//
// The zip contains only CrushFling: the two executables, the launcher scripts,
// the full editable source of this small app (so it can be rebuilt) and a
// SHA-256 manifest. No ChitChat Bot files, no accounts, no logs.
//
// Usage: node scripts/package-crushfling-standalone.cjs [--out path.zip] [--allow-missing]
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { zipSync } = require("fflate");

const root = path.resolve(__dirname, "..");
const src = path.join(root, "standalone");
const TOP = "CrushFling";
const BINARIES = ["CrushFling.exe", "crushfling-cli.exe"];
const DEFAULT_OUT = path.join(root, "release", "CrushFling-Only-Windows-x64.zip");

function sha256(data) {
  return crypto.createHash("sha256").update(data).digest("hex");
}

function collect(allowMissing) {
  const files = {};
  const add = (rel, data) => {
    files[`${TOP}/${rel.split(path.sep).join("/")}`] = data;
  };
  function visit(dir, prefix = "") {
    for (const entry of fs
      .readdirSync(dir, { withFileTypes: true })
      .sort((a, b) => a.name.localeCompare(b.name, "en"))) {
      const rel = prefix + entry.name;
      // Runtime data, build leftovers and the module cache never ship.
      if (["data", "build", "node_modules", ".git"].includes(entry.name)) continue;
      if (/\.(log|exe~)$/i.test(entry.name)) continue;
      if (entry.isSymbolicLink()) throw new Error(`symlink not allowed: ${rel}`);
      if (entry.isDirectory()) {
        visit(path.join(dir, entry.name), rel + "/");
        continue;
      }
      if (entry.isFile()) add(rel, fs.readFileSync(path.join(dir, entry.name)));
    }
  }
  visit(src);
  for (const name of BINARIES) {
    const file = path.join(src, name);
    if (!fs.existsSync(file)) {
      if (!allowMissing) throw new Error(`missing ${name} - build the standalone app first`);
      console.warn(`warn: ${name} missing, zip will not be runnable without install.bat`);
      continue;
    }
    const bytes = fs.readFileSync(file);
    if (bytes.subarray(0, 2).toString() !== "MZ") throw new Error(`${name} is not a Windows PE file`);
    files[`${TOP}/${name}`] = bytes;
  }
  return files;
}

function main() {
  const outIndex = process.argv.indexOf("--out");
  const out = outIndex > 0 ? path.resolve(process.argv[outIndex + 1]) : DEFAULT_OUT;
  const allowMissing = process.argv.includes("--allow-missing");
  const files = collect(allowMissing);

  const rows = BINARIES.filter((n) => files[`${TOP}/${n}`]).map((n) => {
    const data = files[`${TOP}/${n}`];
    return `  ${n.padEnd(20)} ${String(data.length).padStart(10)} bytes  sha256 ${sha256(data)}`;
  });
  files[`${TOP}/BUILD-INFO.txt`] = Buffer.from(
    [
      "CrushFling Auto Register - standalone Windows x64 bundle",
      `Built (UTC): ${new Date().toISOString()}`,
      "",
      "How to run: extract the whole zip, then double-click run.bat",
      "Rebuild    : install.bat (Go 1.25+, WebView2 runtime required by Windows)",
      "",
      ...rows,
      "",
    ].join("\r\n"),
    "utf8",
  );

  const manifest = {
    schema: 1,
    product: "CrushFling Auto Register (standalone)",
    top: TOP,
    created: new Date().toISOString(),
    sha256: Object.fromEntries(Object.keys(files).sort().map((k) => [k, sha256(files[k])])),
  };
  files["SOURCE-MANIFEST.json"] = Buffer.from(JSON.stringify(manifest, null, 2));

  fs.mkdirSync(path.dirname(out), { recursive: true });
  const bytes = zipSync(files, { level: 9 });
  fs.writeFileSync(out, bytes);
  console.log(`crushfling zip: ${out}`);
  console.log(`files         : ${Object.keys(files).length}`);
  console.log(`size          : ${(bytes.length / 1024 / 1024).toFixed(1)} MB`);
}

main();
