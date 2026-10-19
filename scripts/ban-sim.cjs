const http = require('http');
const WebSocket = require('ws');
const port = process.argv[2];
http.get(`http://127.0.0.1:${port}/json/list`, (res) => {
  let body = '';
  res.on('data', (c) => { body += c; });
  res.on('end', () => {
    const page = JSON.parse(body).find((t) => t.type === 'page');
    const ws = new WebSocket(page.webSocketDebuggerUrl);
    ws.on('open', () => {
      const expression = `(() => {
        history.pushState({}, '', '/banned');
        document.body.innerHTML = '<main><h1>Account suspended</h1><p>Your access to Chitchat has been restricted.</p><div>Ban ID</div><div>test-ban-1</div><div>Reason</div><div>Breaking our community guidelines and terms of service.</div><div>Remaining</div><div>Permanent</div></main>';
        return location.href;
      })()`;
      ws.send(JSON.stringify({ id: 1, method: 'Runtime.evaluate', params: { expression, returnByValue: true } }));
    });
    ws.on('message', (m) => {
      const j = JSON.parse(m);
      if (j.id === 1) { console.log('simulated:', j.result.result.value); ws.close(); }
    });
  });
});
