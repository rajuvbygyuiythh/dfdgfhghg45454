const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {JSDOM}=require('jsdom');
const root=path.join(__dirname,'..');
const read=p=>fs.readFileSync(path.join(root,p),'utf8');
const ui=read('cmd/chitchatui/frontend/dist/app.js');
const go=read('app.go');
const goAll=go+read('show_log.go')+read('bot_log.go');
const bindings=read('cmd/chitchatui/frontend/wailsjs/go/main/App.js');
const dts=read('cmd/chitchatui/frontend/wailsjs/go/main/App.d.ts');

test('every literal UI backend invocation has a public Go method',()=>{
 const invoked=[...ui.matchAll(/invoke\("(\w+)"/g)].map(m=>m[1]);
 assert.ok(invoked.includes('RunBrowser'));
 assert.ok(invoked.includes('GetShowModeLog'));
 assert.ok(invoked.includes('ClearShowModeLog'));
 for(const method of invoked) assert.match(goAll,new RegExp(`func \\(a \\*App\\) ${method}\\(`),method);
});

test('Show Mode Run saves its own stores and launches show_mode in one click',()=>{
 assert.match(ui,/invoke\("SaveShowModeSettings", show\)/);
 assert.match(ui,/invoke\("SaveProxySettings", proxy\)/);
 assert.match(ui,/invoke\("SaveMessageSettings", message\)/);
 assert.match(ui,/invoke\("RunBrowser",\s*\{\s*profile,\s*backend:\s*"show_mode",\s*headless:\s*false[^}]*\},\s*1\)/);
 assert.match(go,/func \(a \*App\) RunBrowser\(settings model\.BrowserSettings, count int\) error/);
 assert.match(go,/if settings\.Backend == "show_mode" \{\s*settings\.Backend = "rod"\s*settings\.Headless = false/);
 assert.match(bindings,/RunBrowser\(arg1, arg2\)[\s\S]*?\['RunBrowser'\]\(arg1, arg2\)/);
 assert.match(dts,/RunBrowser\(arg1:model\.BrowserSettings,\s*arg2:number\):Promise<void>/);
 assert.match(bindings,/SaveShowModeSettings\(arg1\)/);
});

test('Show Mode exposes a bounded, rule-aware live log feed',()=>{ assert.match(read('show_log.go'),/const showLogCapacity = 500/);
 assert.match(goAll,/func \(a \*App\) GetShowModeLog\(/);
 assert.match(goAll,/func \(a \*App\) ClearShowModeLog\(\)/);
 assert.match(read('live_chat.go'),/type liveReply struct/);
 assert.match(read('live_chat.go'),/eventf\s+func\(kind, text, rule, detail string, settingOK bool\)/);
 assert.match(read('live_chat.go'),/r\.emit\("out", line, reply\.Rule, detail, ok\)/);
 assert.match(read('live_chat.go'),/r\.emit\("in", row\.Text/);
 assert.match(read('live_chat.go'),/r\.emit\("skip", "skipped conversation"/);
 assert.match(bindings,/GetShowModeLog\(arg1\)/);
 assert.match(dts,/GetShowModeLog\(arg1:number\):Promise<main\.ShowLogSnapshot>/);
 assert.match(read('internal/model/ui.go'),/ShowModeRunning bool\s+`json:"show_mode_running"`/);
});

test('saved browser profiles are listed and restored server-side',()=>{
 assert.match(goAll,/func \(a \*App\) ListProfiles\(\)/);
 assert.match(read('internal/browser/backend.go'),/type SessionSeeder interface/);
 assert.match(read('internal/browser/rod.go'),/func \(b \*RodBackend\) SeedSession\(/);
 assert.match(ui,/invoke\("ListProfiles"\)/);
 assert.match(bindings,/ListProfiles\(\)/);
 assert.match(goAll,/func \(a \*App\) restoreProfileSession\(/);
 assert.doesNotMatch(bindings,/Cookies\[|SeedSession/); // cookies stay server-side
});

test('the two timing profiles keep independent database keys',()=>{
 assert.match(read('internal/uistore/store.go'),/keyGeneral\s*=\s*"settings\/general"/);
 assert.match(read('internal/uistore/show_mode.go'),/keyShowMode\s*=\s*"settings\/show_mode\/v1"/);
 assert.match(ui,/invoke\("SaveShowModeSettings"/);
 assert.match(ui,/invoke\("SaveGeneralSettings"/);
});

test('all literal and templated DOM IDs exist, and HTML IDs are unique',()=>{
 const dom=new JSDOM(read('cmd/chitchatui/frontend/dist/index.html'));
 const doc=dom.window.document;
 const ids=[...doc.querySelectorAll('[id]')].map(n=>n.id);
 assert.equal(new Set(ids).size,ids.length,'duplicate HTML IDs');
 for(const m of ui.matchAll(/byId\("([^"]+)"\)/g)) assert.ok(ids.includes(m[1]),`missing #${m[1]}`);
 for(const prefix of ['sm','bg']){
  for(const suffix of ['profile','profile-session','first-mode','first-percent','first-wait','first-min','first-max','first-percent-row','first-wait-row','first-timeout-row','chat-mode-single','chat-mode-folder','chat-mode-evachat','mode-option-single','mode-option-folder','mode-option-evachat','section-single-mode','section-folder-mode','section-evachat-mode','sequence','usernames','ai-enabled','ai-base-url','ai-model','ai-api-key','ai-test','ai-status','loaded-status','eva-status','load-folder','clean-loaded','open-eva','proxy-enabled','proxy-scheme','proxy-entries','test-proxy','proxy-status','proxy-results']){
   assert.ok(ids.includes(`${prefix}-${suffix}`),`missing #${prefix}-${suffix}`);
  }
 }
 for(const id of ['sm-run','sm-run-status','sm-log-list','sm-log-filter','sm-log-clear','bg-profile-options','bg-start','bg-stop','bg-status','bg-login-status','bg-poll-min','bg-poll-max','bg-action-min','bg-action-max','bg-skip-min','bg-skip-max','bg-close-min','bg-close-max','bg-reply-min','bg-reply-max']) assert.ok(ids.includes(id),`missing #${id}`);
 dom.window.close();
});

test('advertised drivers match factory and Show Mode resolves to visible Rod',()=>{
 const models=read('internal/model/ui.go').split('func DefaultBrowserOptions()')[1].split('func DefaultBrowserSettings()')[0];
 const offered=[...models.matchAll(/Key:\s*"([^"]+)"/g)].map(m=>m[1]).sort();
 const available=[...read('internal/browser/backend.go').matchAll(/case "([^"]+)":/g)].map(m=>m[1]).sort();
 assert.deepEqual(offered.filter(key=>key!=='show_mode'),available);
 assert.ok(offered.includes('show_mode'));
});

test('live browser integration test is opt-in, never part of the default suite',()=>{
 assert.ok(read('internal/browser/rod_proxy_auth_test.go').startsWith('//go:build browser_live\n'));
});

test('desktop startup records logs before UI startup even without a GUI console',()=>{
 const main=read('main.go');
 assert.match(main,/log\.SetOutput\(io\.MultiWriter\(applog\.Logger\{[^}]+\}, os\.Stderr\)\)/);
 assert.ok(main.indexOf('log.SetOutput(')<main.indexOf('NewApp(dataDir)'));
 assert.match(main,/Logger:\s+diskLogger/);
 const logFn=go.slice(go.indexOf('func (a *App) log('));
 assert.ok(logFn.indexOf('disk.Log(level, message)')<logFn.indexOf('a.uiStore.AppendLog(level, message)'));
 assert.match(logFn,/disk\.Error\(fmt\.Sprintf\("UI log persistence failed/);
});

test('process capture, crash hook and per-copy WebView2 profile are desktop-startup only',()=>{
 const main=read('main.go'),capture=read('internal/applog/capture.go');
 assert.ok(main.indexOf('applog.CaptureOutput(')<main.indexOf('NewApp(dataDir)'));
 assert.match(main,/capture\.Close\(\)/);
 assert.match(capture,/debug\.SetCrashOutput\(crash, debug\.CrashOptions\{\}\)/);
 assert.match(capture,/os\.Stdout = outWrite/);
 assert.match(capture,/os\.Stderr = errWrite/);
 assert.match(main,/LogLevelProduction:\s*logger\.INFO/);
 assert.match(main,/WebviewUserDataPath:\s+webviewDir/);
 assert.match(main,/filepath\.Join\(dataDir, "webview"\)/);
});
