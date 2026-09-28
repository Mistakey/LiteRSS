// Browser forensics over the Chrome DevTools protocol (docs/TESTING.md「浏览器取证」).
//
//   node scripts/browser-forensics.mjs <devtools endpoint> <url> [screenshot.png] [--csp-probe]
//
// <devtools endpoint> is a --remote-debugging-port listener: chrome-headless-shell
// for the browser channel, or the dev instance's WebView2 when it was started
// with LITERSS_WEBVIEW2_DEBUG_PORT. The script loads <url> in the first page
// target and prints every request with its status and CSP header, console
// output, and any resource outside the page's origin. --csp-probe then injects
// an inline <script>, an inline <style> and a data: <object>; with the CSP
// header enforced all three are refused and reported as violations.
// Requires Node 22+ (global WebSocket).
import { writeFileSync } from 'node:fs';

const args = process.argv.slice(2);
const cspProbe = args.includes('--csp-probe');
const [endpoint, url, screenshot] = args.filter((a) => !a.startsWith('--'));
if (!endpoint || !url) {
  console.error(
    'usage: node scripts/browser-forensics.mjs <devtools endpoint> <url> [screenshot.png] [--csp-probe]'
  );
  process.exit(2);
}

const targets = await (await fetch(`${endpoint}/json/list`)).json();
const target = targets.find((t) => t.type === 'page');
if (!target) throw new Error(`no page target at ${endpoint}`);
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((resolve) => ws.addEventListener('open', resolve, { once: true }));

let seq = 0;
const pending = new Map();
const events = [];
ws.addEventListener('message', (message) => {
  const msg = JSON.parse(message.data);
  if (msg.id && pending.has(msg.id)) {
    pending.get(msg.id)(msg);
    pending.delete(msg.id);
  } else if (msg.method) {
    events.push(msg);
  }
});
const send = (method, params = {}) =>
  new Promise((resolve) => {
    const id = ++seq;
    pending.set(id, resolve);
    ws.send(JSON.stringify({ id, method, params }));
  });
const evaluate = async (expression) =>
  (await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })).result
    .result.value;
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

function printLog(from) {
  for (const e of events.slice(from)) {
    if (e.method === 'Log.entryAdded')
      console.log(`  ${e.params.entry.level}: ${e.params.entry.text}`);
    if (e.method === 'Runtime.consoleAPICalled')
      console.log(
        `  console.${e.params.type}: ${e.params.args.map((a) => a.value ?? a.description).join(' ')}`
      );
    if (e.method === 'Runtime.exceptionThrown')
      console.log(`  exception: ${e.params.exceptionDetails.text}`);
  }
}

for (const domain of ['Network', 'Log', 'Runtime', 'Page']) await send(`${domain}.enable`);
await send('Page.navigate', { url });
await sleep(2500);

const requests = new Map();
for (const e of events) {
  const r = requests.get(e.params?.requestId) ?? {};
  if (e.method === 'Network.requestWillBeSent') r.url = e.params.request.url;
  else if (e.method === 'Network.responseReceived') {
    r.url ??= e.params.response.url;
    r.status = e.params.response.status;
    const headers = Object.entries(e.params.response.headers);
    r.csp = headers.find(([k]) => k.toLowerCase() === 'content-security-policy')?.[1] ?? null;
  } else if (e.method === 'Network.loadingFailed')
    r.failed = e.params.blockedReason ?? e.params.errorText;
  else continue;
  requests.set(e.params.requestId, r);
}
console.log('== requests');
for (const r of requests.values())
  console.log(`  ${r.status ?? r.failed} ${r.url}\n      csp: ${r.csp}`);
console.log('== console during load');
printLog(0);
console.log('== page');
console.log(
  await evaluate(`JSON.stringify({
    origin: location.origin,
    title: document.title,
    external: performance.getEntriesByType('resource').map((e) => e.name).filter((n) => !n.startsWith(location.origin)),
  })`)
);

if (screenshot) {
  const { result } = await send('Page.captureScreenshot', { format: 'png' });
  writeFileSync(screenshot, Buffer.from(result.data, 'base64'));
  console.log(`== screenshot ${screenshot}`);
}

if (cspProbe) {
  const before = events.length;
  console.log('== CSP probe (all three must be refused)');
  console.log(
    await evaluate(`(async () => {
      const violations = [];
      document.addEventListener('securitypolicyviolation', (e) => violations.push(e.violatedDirective));
      const script = document.createElement('script');
      script.textContent = 'window.__cspProbeRan = true';
      document.head.appendChild(script);
      const style = document.createElement('style');
      style.textContent = 'body { outline: 7px solid red }';
      document.head.appendChild(style);
      const object = document.createElement('object');
      object.data = 'data:text/html,probe';
      document.body.appendChild(object);
      await new Promise((r) => setTimeout(r, 500));
      return JSON.stringify({
        inlineScriptRan: window.__cspProbeRan === true,
        inlineStyleApplied: getComputedStyle(document.body).outlineWidth === '7px',
        violations,
      });
    })()`)
  );
  printLog(before);
}
ws.close();
