const http = require('http');
const WebSocket = require('ws');
const port = process.argv[2];
http.get(`http://127.0.0.1:${port}/json/list`, (res) => {
  let body = '';
  res.on('data', (c) => { body += c; });
  res.on('end', () => {
    const page = JSON.parse(body).find((t) => t.type === 'page');
    const ws = new WebSocket(page.webSocketDebuggerUrl);
    ws.on('open', () => ws.send(JSON.stringify({ id: 1, method: 'Runtime.evaluate', params: { expression: `JSON.stringify({url:location.href,text:document.body.innerText.replace(/\\s+/g,' ').slice(0,200)})`, returnByValue: true } })));
    ws.on('message', (m) => {
      const j = JSON.parse(m);
      if (j.id === 1) { console.log(j.result.result.value); ws.close(); }
    });
  });
});
