// Generates the standalone CrushFling project from this repository.
//
// Why generate instead of maintaining a second copy: the registration engine
// (internal/crushfling, internal/proxy, cmd/crushfling) and the dashboard
// (overlay/crushfling.{js,css}, overlay/overlay.html) must never diverge. The
// generator copies them into ./standalone, rewrites the module path, drops the
// tests, and adds the small Wails shell (scripts/standalone/*.tmpl).
//
// Usage: node scripts/build-crushfling-standalone.cjs [--check]
const fs = require("node:fs");
const path = require("node:path");

const root = path.resolve(__dirname, "..");
const out = path.join(root, "standalone");
const tpl = path.join(__dirname, "standalone");
const MODULE = "crushfling";

const files = {}; // relative path -> string content

function read(rel) {
  return fs.readFileSync(path.join(root, rel), "utf8");
}
function readTemplate(name) {
  return fs.readFileSync(path.join(tpl, name), "utf8");
}
function put(rel, content) {
  files[rel.replaceAll("\\", "/")] = content;
}

// Copy Go sources, dropping tests and rewriting the module path.
function copyGo(dir, prefix = "") {
  for (const entry of fs
    .readdirSync(path.join(root, dir), { withFileTypes: true })
    .sort((a, b) => a.name.localeCompare(b.name, "en"))) {
    const rel = `${dir}/${entry.name}`;
    if (entry.isDirectory()) {
      copyGo(`${rel}`, prefix);
      continue;
    }
    if (!entry.name.endsWith(".go") || entry.name.endsWith("_test.go")) continue;
    const body = read(rel).replaceAll("chitchatbot/", `${MODULE}/`);
    put(`standalone/${rel}${prefix}`, body);
  }
}

copyGo("internal/crushfling");
copyGo("internal/proxy");
// cmd/crushfling/main.go -> standalone/cmd/crushfling/main.go
put("standalone/cmd/crushfling/main.go", read("cmd/crushfling/main.go").replaceAll("chitchatbot/", `${MODULE}/`));

// Dashboard assets: one source of truth (the overlay used by ChitChat Bot).
put("standalone/frontend/dist/crushfling.js", read("cmd/chitchatui/frontend/overlay/crushfling.js"));
put("standalone/frontend/dist/crushfling.css", read("cmd/chitchatui/frontend/overlay/crushfling.css"));
put("standalone/frontend/dist/standalone.js", readTemplate("standalone.js.tmpl"));
put("standalone/frontend/dist/theme.css", readTemplate("theme.css.tmpl"));

// The panel markup is a fragment in the overlay; wrap it in a full page.
const overlay = read("cmd/chitchatui/frontend/overlay/overlay.html");
const start = overlay.indexOf('<div id="crushfling-overlay"');
const end = overlay.lastIndexOf("</div>");
if (start < 0 || end < start) throw new Error("overlay.html panel block not found");
const panel = overlay
  .slice(start, end + "</div>".length)
  .split("\n")
  .map((line) => (line.trim() === "" ? "" : "    " + line.trimStart()))
  .join("\n");
put("standalone/frontend/dist/index.html", readTemplate("index.html.tmpl").replace("{{PANEL}}", panel));

// Shell files.
put("standalone/main.go", readTemplate("main.go.tmpl"));
put("standalone/app.go", readTemplate("app.go.tmpl"));
put("standalone/go.mod", readTemplate("go.mod.tmpl"));
put("standalone/wails.json", readTemplate("wails.json.tmpl"));
put("standalone/README.md", readTemplate("README.md.tmpl"));
put("standalone/run.bat", readTemplate("run.bat.tmpl").replaceAll("\n", "\r\n"));
put("standalone/install.bat", readTemplate("install.bat.tmpl").replaceAll("\n", "\r\n"));
put("standalone/.gitignore", "data/\nbuild/\nbuild.log\ngo.sum\n*.exe\n");

const check = process.argv.includes("--check");

function sync() {
  if (check) {
    const problems = [];
    for (const [rel, content] of Object.entries(files)) {
      const file = path.join(root, rel);
      if (!fs.existsSync(file)) {
        problems.push(`missing ${rel}`);
        continue;
      }
      if (fs.readFileSync(file, "utf8") !== content) problems.push(`stale ${rel}`);
    }
    if (!fs.existsSync(out)) problems.push("missing standalone/");
    return problems;
  }
  fs.rmSync(out, { recursive: true, force: true });
  for (const [rel, content] of Object.entries(files)) {
    const file = path.join(root, rel);
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, content);
  }
  return [];
}

const problems = sync();
if (check) {
  if (problems.length) {
    console.error(`CrushFling standalone is out of date (${problems.length}):`);
    for (const p of problems.slice(0, 10)) console.error("  " + p);
    process.exit(1);
  }
  console.log(`CrushFling standalone is in sync (${Object.keys(files).length} files).`);
} else {
  const bytes = Object.values(files).reduce((n, c) => n + Buffer.byteLength(c), 0);
  console.log(`CrushFling standalone generated: ${Object.keys(files).length} files, ${(bytes / 1024).toFixed(1)} KiB`);
  console.log(`  ${path.relative(root, out)}/`);
}
