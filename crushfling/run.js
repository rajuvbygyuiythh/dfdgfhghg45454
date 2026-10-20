#!/usr/bin/env node
// CrushFling Auto Register - DIAGNOSTIC fallback (no Go needed).
//
// Scope: temp inbox -> magic-link request -> inbox poll -> click in the same
// HTTP session -> accounts.json. The avatar upload and profile creation steps
// live only in the Go implementation (internal/crushfling), so use this script
// to confirm that the mail + link part of the flow works from your network.
//
// Usage: node crushfling/run.js --count 1 --email mailtm [--verbose]
// Requires Node 18+ (built-in fetch).
//
// Like the Go worker, a caption line is reserved for the attempt and removed
// only after the account is verified. Secrets are masked in the console by
// default; use --verbose for request/status detail (never the raw token).

const fs = require('fs');
const path = require('path');

const BASE = 'https://crushfling.com';
const DEFAULT_UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36';

function parseArgs(argv){
  const out = {};
  for(let i=2;i<argv.length;i++){
    const a=argv[i];
    if(a==='--count') out.count=parseInt(argv[++i]);
    else if(a==='--email') out.emailProvider=argv[++i];
    else if(a==='--proxy') out.proxyFile=argv[++i];
    else if(a==='--data') out.dataDir=argv[++i];
    else if(a==='--avatars') out.avatarDir=argv[++i];
    else if(a==='--concurrency') out.concurrency=parseInt(argv[++i]);
    else if(a==='--verbose' || a==='-v') out.verbose=true;
  }
  return out;
}

let VERBOSE = false;
function vlog(...args){ if(VERBOSE) console.log('[debug]', ...args); }
function maskLink(url){
  return String(url || '').replace(/([?&](?:token|code|key|secret|sig|session)=)[^&\s]+/gi, '$1***');
}
function shortBody(body){
  return String(body || '').replace(/\s+/g, ' ').slice(0, 160);
}

async function createMailTmInbox(){
  // mail.tm: GET /domains -> POST /accounts -> POST /token -> GET /messages
  const domRes = await fetch('https://api.mail.tm/domains');
  if(!domRes.ok) throw new Error(`mail.tm domains ${domRes.status}: ${shortBody(await domRes.text())}`);
  const domJson = await domRes.json();
  if(!domJson['hydra:member'] || !domJson['hydra:member'].length) throw new Error('mail.tm returned no domains');
  const domain = domJson['hydra:member'][0].domain;
  vlog('mail.tm domain', domain);
  const local = Math.random().toString(36).slice(2,11);
  const email = `${local}@${domain}`;
  const password = Math.random().toString(36).slice(2,12) + '!A1';
  const accRes = await fetch('https://api.mail.tm/accounts', {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({address:email,password})});
  if(!accRes.ok) throw new Error(`mail.tm account ${accRes.status}: ${shortBody(await accRes.text())}`);
  const tokRes = await fetch('https://api.mail.tm/token', {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({address:email,password})});
  if(!tokRes.ok) throw new Error(`mail.tm token ${tokRes.status}: ${shortBody(await tokRes.text())}`);
  const tokJson = await tokRes.json();
  const token = tokJson.token;
  if(!token) throw new Error('mail.tm returned no token');
  console.log(`[mail.tm] inbox: ${email}`);
  return {
    email,
    token,
    getMessages: async()=>{
      const r = await fetch('https://api.mail.tm/messages?page=1', {headers:{Authorization:'Bearer '+token}});
      if(!r.ok) throw new Error(`mail.tm messages ${r.status}: ${shortBody(await r.text())}`);
      const j = await r.json();
      const msgs = j['hydra:member']||[];
      // fetch full body for each
      const out=[];
      for(const m of msgs){
        const fr = await fetch('https://api.mail.tm/messages/'+m.id, {headers:{Authorization:'Bearer '+token}});
        const fj = await fr.json();
        out.push({subject:m.subject, from:m.from?.address||'', body: fj.html||fj.text||m.intro||''});
      }
      return out;
    }
  };
}

async function createOneSecMailInbox(){
  const domains=['1secmail.com','1secmail.net','1secmail.org'];
  const domain = domains[Math.floor(Math.random()*domains.length)];
  const local = Math.random().toString(36).slice(2,10);
  const email = `${local}@${domain}`;
  console.log(`[1secmail] inbox: ${email}`);
  return {
    email, login:local, domain,
    getMessages: async()=>{
      const r = await fetch(`https://www.1secmail.com/api/v1/?action=getMessages&login=${local}&domain=${domain}`);
      if(!r.ok) throw new Error(`1secmail messages ${r.status}: ${shortBody(await r.text())}`);
      const j = await r.json();
      const out=[];
      for(const m of j){
        const fr = await fetch(`https://www.1secmail.com/api/v1/?action=readMessage&login=${local}&domain=${domain}&id=${m.id}`);
        if(!fr.ok) throw new Error(`1secmail read ${fr.status}`);
        const fj = await fr.json();
        out.push({subject:m.subject, from:m.from, body: fj.htmlBody||fj.textBody||fj.body||''});
      }
      return out;
    }
  };
}

function extractMagicLink(body){
  const pats=[
    /https:\/\/crushfling\.com[^\s"'<>]+token=[^\s"'<>]+/g,
    /https:\/\/crushfling\.com\/api\/auth\/callback[^\s"'<>]+/g,
    /https:\/\/crushfling\.com\/verify[^\s"'<>]+/g,
  ];
  for(const re of pats){
    const m = body.match(re);
    if(m) return m[0].replace(/[.,;)"']+$/,'');
  }
  const any = body.match(/https?:\/\/[^\s"'<>]*crushfling[^\s"'<>]*/);
  if(any) return any[0];
  return null;
}

async function pollInbox(inbox, timeoutMs=120000, intervalMs=3000){
  const deadline = Date.now()+timeoutMs;
  while(Date.now()<deadline){
    let msgs;
    try{
      msgs = await inbox.getMessages();
    }catch(err){
      // A transient provider error must not abort the whole 120 s window.
      console.log(`[poll] inbox error: ${err.message}`);
      await new Promise(r=>setTimeout(r, intervalMs));
      continue;
    }
    vlog('poll messages', msgs.length);
    for(const m of msgs){
      const link = extractMagicLink(m.body) || extractMagicLink(m.subject);
      if(link){
        console.log(`[poll] magic link found: ${maskLink(link)}`);
        return link;
      }
    }
    console.log(`[poll] waiting... ${msgs.length} msgs, next in ${intervalMs}ms`);
    await new Promise(r=>setTimeout(r, intervalMs));
  }
  throw new Error('timeout waiting for magic link');
}

class Session {
  constructor(proxyUrl){
    this.cookies = new Map();
    this.ua = DEFAULT_UA;
    this.proxyUrl = proxyUrl;
  }
  async get(url){
    const headers = {
      'User-Agent': this.ua,
      'Accept': 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
      'Referer': BASE+'/account',
      'Cookie': this.cookieHeader()
    };
    const res = await fetch(url, {headers, redirect:'follow'});
    const setCookie = res.headers.get('set-cookie');
    if(setCookie) this.storeCookies(setCookie);
    const text = await res.text();
    // Also handle set-cookie via getSetCookie (Node 18+)
    if(res.headers.getSetCookie){
      for(const ck of res.headers.getSetCookie()) this.storeCookies(ck);
    }
    vlog('GET', maskLink(url.replace(BASE,'')), '->', res.status, 'cookies:', [...this.cookies.keys()].join(',')||'none');
    return {status: res.status, body: text, headers: res.headers};
  }
  async postForm(path, form){
    const url = path.startsWith('http')?path:BASE+path;
    const headers = {
      'User-Agent': this.ua,
      'Content-Type':'application/x-www-form-urlencoded',
      'Accept':'application/json',
      'Origin': BASE,
      'Referer': BASE+'/account',
      'Cookie': this.cookieHeader()
    };
    const res = await fetch(url, {method:'POST', headers, body: form.toString(), redirect:'follow'});
    if(res.headers.getSetCookie) for(const ck of res.headers.getSetCookie()) this.storeCookies(ck);
    const text = await res.text();
    vlog('POST', maskLink(url.replace(BASE,'')), '->', res.status, shortBody(text));
    return {status:res.status, body:text};
  }
  async postJson(path, payload){
    const url = path.startsWith('http')?path:BASE+path;
    const body = JSON.stringify(payload);
    const headers = {
      'User-Agent': this.ua,
      'Content-Type':'application/json',
      'Accept':'application/json',
      'Origin': BASE,
      'Referer': BASE+'/account',
      'Cookie': this.cookieHeader()
    };
    const res = await fetch(url, {method:'POST', headers, body, redirect:'follow'});
    const setCookie = res.headers.get('set-cookie');
    if(setCookie) this.storeCookies(setCookie);
    if(res.headers.getSetCookie) for(const ck of res.headers.getSetCookie()) this.storeCookies(ck);
    const text = await res.text();
    vlog('POST', maskLink(url.replace(BASE,'')), '->', res.status, shortBody(text));
    return {status:res.status, body:text};
  }
  cookieHeader(){
    return [...this.cookies.entries()].map(([k,v])=>k+'='+v).join('; ');
  }
  storeCookies(header){
    if(!header) return;
    const parts = header.split(/,\s*(?=[^;]+=[^;]+)/);
    for(const p of parts){
      const m = p.match(/^\s*([^=;]+)=([^;]*)/);
      if(m) this.cookies.set(m[1].trim(), m[2].trim());
    }
  }
}

function randomChoice(arr){return arr[Math.floor(Math.random()*arr.length)]}
function loadCaptions(dataDir){
  const p = path.join(dataDir, 'caption.txt');
  if(!fs.existsSync(p)) return [];
  return fs.readFileSync(p,'utf8').split('\n').map(s=>s.trim()).filter(s=>s && !s.startsWith('#'));
}
function pickCaption(dataDir){
  const lines = fs.readFileSync(path.join(dataDir,'caption.txt'),'utf8').split('\n');
  for(const l of lines){
    const t=l.trim();
    if(t && !t.startsWith('#')) return t;
  }
  return null;
}
function removeCaption(dataDir, caption){
  const p = path.join(dataDir,'caption.txt');
  const lines = fs.readFileSync(p,'utf8').split('\n');
  let removed=false;
  const kept=[];
  for(const l of lines){
    if(!removed && l.trim()===caption.trim()){ removed=true; continue; }
    kept.push(l);
  }
  if(removed) fs.writeFileSync(p, kept.join('\n'));
  return removed;
}
function pickPicture(avatarDir){
  if(!fs.existsSync(avatarDir)) return null;
  const files = fs.readdirSync(avatarDir).filter(f=>/\.(jpg|jpeg|png|webp)$/i.test(f) && !fs.statSync(path.join(avatarDir,f)).isDirectory());
  if(!files.length) return null;
  return path.join(avatarDir, randomChoice(files));
}
function generateProfile(email, caption){
  const firstNames=['Emma','Olivia','Ava','Sophia','Isabella','Mia','Luna','Aria','Chloe','Zoe','Maya','Aaliyah','Scarlett','Lily','Avery'];
  const lastNames=['Rose','Grace','Hope','Sweet','Star','Angel','Belle','Honey','Queen','Doll','Hot','Cute','Sassy','Flirty','Desire'];
  const fn=randomChoice(firstNames), ln=randomChoice(lastNames);
  const username=(fn+ln+Math.floor(1000+Math.random()*9000)).toLowerCase().replace(/\s/g,'');
  const countries=['US','GB','DE','CA','AU','FR','IT','ES','NL','SE','PL'];
  const country=randomChoice(countries);
  const platforms=['telegram','snapchat','discord','whatsapp'];
  const platform=randomChoice(platforms);
  const bio = caption ? caption.replace('@myhandle','@'+username).replace('{username}',username) : `Hi I'm new! Add me on ${platform} @${username}`;
  return {username, email, age: 19+Math.floor(Math.random()*7), gender:'woman', lookingFor:'guys', country, platform, platformUsername:username, bio};
}

async function registerOne(idx, opts){
  console.log(`\n=== [${idx}] Starting ===`);
  // inbox
  let inbox;
  if(opts.emailProvider==='1secmail') inbox=await createOneSecMailInbox();
  else inbox=await createMailTmInbox();
  const email=inbox.email;
  const caption=pickCaption(opts.dataDir) || 'Hi add me on telegram @myhandle';
  const profile=generateProfile(email, caption);
  const picPath=pickPicture(opts.avatarDir);
  console.log(`[${idx}] email=${email} user=${profile.username} pic=${picPath?path.basename(picPath):'none'} country=${profile.country}`);

  const sess=new Session();
  // CSRF
  let csrf='';
  try{
    const c=await sess.get(BASE+'/api/auth/csrf');
    const j=JSON.parse(c.body); csrf=j.csrfToken||'';
    console.log(`[${idx}] csrf: ${csrf ? csrf.length+' chars' : 'missing'}`);
  }catch(e){ console.log(`[${idx}] no csrf: ${e.message}`)}
  // Magic link request
  let ok=false;
  for(let attempt=0; attempt<3; attempt++){
    const form = new URLSearchParams({csrfToken:csrf, email, callbackUrl: BASE+'/', json: 'true'});
    const r=await sess.postForm('/api/auth/signin/email', form);
    console.log(`[${idx}] magic-link form POST ${r.status} ${shortBody(r.body)}`);
    if(r.status>=200 && r.status<300){ ok=true; break; }
    // JSON variant, then an alternate path (same order as the Go worker)
    const rj=await sess.postJson('/api/auth/signin/email', {email, csrfToken:csrf});
    if(rj.status>=200 && rj.status<300){ ok=true; break; }
    const r2=await sess.postJson('/api/auth/magic-link', {email});
    console.log(`[${idx}] alt magic-link POST ${r2.status} ${shortBody(r2.body)}`);
    if(r2.status>=200 && r2.status<300){ ok=true; break; }
    await new Promise(r=>setTimeout(r, 2000));
  }
  if(!ok) throw new Error('magic link request failed');
  console.log(`[${idx}] polling inbox...`);
  const magicLink=await pollInbox(inbox, 120000, 3000);
  console.log(`[${idx}] clicking magic link IN SAME SESSION: ${maskLink(magicLink)}`);
  const clicked=await sess.get(magicLink);
  const cookieNames=[...sess.cookies.keys()];
  console.log(`[${idx}] clicked ${clicked.status} cookies=${cookieNames.join(',')||'none'} bodyLen=${clicked.body.length}`);
  const sessionCookies=cookieNames.filter((name)=>/session/i.test(name));
  const verified = sessionCookies.length>0;
  console.log(`[${idx}] verified=${verified} sessionCookies=${sessionCookies.join(',')||'none'}`);
  if(verified && caption){ console.log(`[${idx}] caption ${removeCaption(opts.dataDir, caption) ? 'removed' : 'not removed'}`); }
  if(!verified){
    console.log(`[${idx}] not verified - account not saved, caption kept`);
    return {profile, verified:false};
  }
  // Save only verified accounts, exactly like the Go worker.
  const outPath = path.join(opts.dataDir, 'accounts.json');
  let accounts=[];
  if(fs.existsSync(outPath)) try{ accounts=JSON.parse(fs.readFileSync(outPath,'utf8')) }catch{}
  accounts.push({...profile, magicLink, cookies:Object.fromEntries(sess.cookies), verified, picPath});
  fs.writeFileSync(outPath, JSON.stringify(accounts,null,2));
  // Move picture to used
  if(picPath){
    const usedDir=path.join(opts.avatarDir,'used');
    fs.mkdirSync(usedDir,{recursive:true});
    const dest=path.join(usedDir, path.basename(picPath));
    try{ fs.renameSync(picPath, dest); console.log(`[${idx}] picture moved to used/${path.basename(picPath)}`);}catch{}
  }
  console.log(`[${idx}] DONE verified=${verified}`);
  return {profile, verified};
}

async function main(){
  const args=parseArgs(process.argv);
  const dataDir=args.dataDir||'crushfling/data';
  const avatarDir=args.avatarDir||path.join(dataDir,'pictures');
  const count=args.count||1;
  const provider=args.emailProvider||'mailtm';
  VERBOSE = !!args.verbose;
  console.log("CrushFling DIAGNOSTIC (Node): temp-mail + same-session magic link only.");
  console.log("Profile/avatar creation is implemented in the Go worker (cmd/crushfling).");
  console.log(`count=${count} provider=${provider} dataDir=${dataDir}`);
  fs.mkdirSync(dataDir,{recursive:true}); fs.mkdirSync(avatarDir,{recursive:true}); fs.mkdirSync(path.join(avatarDir,'used'),{recursive:true});
  // Ensure caption.txt exists
  const capPath=path.join(dataDir,'caption.txt');
  if(!fs.existsSync(capPath)){
    fs.writeFileSync(capPath, "Hi I'm at home and alone. Add Telegram >> @myhandle\nHey babe add me @myhandle\n");
  }
  for(let i=0;i<count;i++){
    try{
      await registerOne(i, {emailProvider:provider, dataDir, avatarDir});
    }catch(e){ console.error(`[${i}] FAILED: ${e.message}`)}
    if(i+1<count) await new Promise(r=>setTimeout(r, 2000+Math.random()*2000));
  }
  console.log('All done. See crushfling/data/accounts.json');
}
main().catch(e=>{console.error(e); process.exit(1)});
