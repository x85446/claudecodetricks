// TESTMASTER: id=dag-web-2 tier=fast parallel=no
// The dashboard's Graph tab in headless Chrome over a fixture project whose
// head plan is parked: one node per plan and one edge per dependency, held
// plans read "waiting on <head>", no edge crosses another plan's box, no page
// horizontal scroll at 1280 or 390, selecting a node opens its plan page, and
// a project with no plans shows its empty state (dag-web-3). Exits 77 (skip)
// when Chrome, Go or WebSocket is absent.
//
// Run: node --experimental-websocket tests/conductor/graph-tab.test.mjs
import { execFileSync, spawn } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const skip = why => { console.log(`SKIP: ${why}`); process.exit(77); };
const wait = ms => new Promise(r => setTimeout(r, ms));

const chromeBin = [process.env.CHROME_BIN, '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', '/Applications/Chromium.app/Contents/MacOS/Chromium',
  '/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser'].find(p => p && existsSync(p));
if (!chromeBin) skip('no Chrome or Chromium found; set CHROME_BIN to run the browser checks');
if (typeof WebSocket === 'undefined') skip('this Node has no WebSocket; run it with --experimental-websocket');
try { execFileSync('go', ['version'], { stdio: 'ignore' }); } catch { skip('go is not on PATH; the test builds iterate-run from this checkout'); }

const root = realpathSync(mkdtempSync(join(tmpdir(), 'graph-tab-')));  // the dashboard keys projects by canonical path
const home = join(root, 'home'), chain = join(root, 'izchain'), empty = join(root, 'empty');
const procs = [];
const cleanup = () => { for (const p of procs) try { p.kill('SIGTERM'); } catch {} try { rmSync(root, { recursive: true, force: true }); } catch {} };
process.on('exit', cleanup);

const bin = join(root, 'iterate-run');
execFileSync('make', ['bin/iterate-run'], { cwd: repo, stdio: 'inherit' });
copyFileSync(join(repo, 'bin/iterate-run'), bin);

const plan = (name, keys) => `# Iterate Task — ${name}\n\nname: ${name}\nStarted: 2026-09-11T02:06:12Z (planned)\n${keys.join('\n')}\nrunning: false\n\n## Goal\nFixture plan ${name}\n\n## Steps\n- [ ] 1. work\n\n## Validation\n- [ ] 1. verify\n`;
for (const d of [join(chain, '.claude/iterate/plans'), join(chain, '.claude/iterate/archive'), join(empty, '.claude/iterate/plans'), join(home, '.claude/iterate-run')]) mkdirSync(d, { recursive: true });
writeFileSync(join(chain, '.claude/iterate/archive/2026-10-09T05-24-17Z-baboon-done.md'), plan('baboon', ['phase: executing']));
const plans = {
  caiman: ['phase: executing', 'status: blocked-on-operator: owned assignment interrupted', 'depends: baboon'],
  dace: ['phase: executing', 'status: unblocked', 'depends: caiman'],
  earthworm: ['phase: executing', 'status: unblocked', 'depends: dace'],
  fairywren: ['phase: planned', 'status: queued: 2026-10-22T00:00:00Z by /ip stage', 'depends: earthworm, independent'],
  independent: ['phase: planned', 'status: queued: 2026-10-22T00:00:00Z by /ip stage'],
};
for (const [n, k] of Object.entries(plans)) writeFileSync(join(chain, `.claude/iterate/plans/${n}.md`), plan(n, k));
writeFileSync(join(home, '.claude/iterate-run/projects.json'), JSON.stringify([chain, empty]));

const freePort = () => new Promise(r => { const s = createServer().listen(0, '127.0.0.1', () => { const p = s.address().port; s.close(() => r(p)); }); });
const port = await freePort();
const env = { ...process.env, HOME: home, ITERATE_CONDUCTOR_DIR: join(root, 'conductor'), ITERATE_RUN_RECORD_DIR: join(root, 'records') };
const server = spawn(bin, ['serve', '--port', String(port)], { env, stdio: 'ignore' });
procs.push(server);
for (let i = 0; ; i++) {
  try { if ((await fetch(`http://localhost:${port}/healthz`)).ok) break; } catch {}
  if (i > 100) { console.error('FAIL: the dashboard never answered /healthz'); process.exit(1); }
  await wait(100);
}

const debugPort = await freePort();
const profile = join(root, 'chrome');
const chrome = spawn(chromeBin, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-gpu', `--remote-debugging-port=${debugPort}`, `--user-data-dir=${profile}`, '--window-size=1280,900', 'about:blank'], { stdio: 'ignore' });
procs.push(chrome);
let target;
for (let i = 0; i < 100 && !target; i++) {
  try { target = (await (await fetch(`http://127.0.0.1:${debugPort}/json/list`)).json()).find(t => t.type === 'page'); } catch {}
  if (!target) await wait(100);
}
if (!target) { console.error('FAIL: headless Chrome started but exposed no page target'); process.exit(1); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise(r => ws.addEventListener('open', r, { once: true }));
let seq = 0; const pending = new Map(); const pageErrors = [];
ws.addEventListener('message', e => {
  const m = JSON.parse(e.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === 'Runtime.exceptionThrown') pageErrors.push(m.params.exceptionDetails.text + ' ' + (m.params.exceptionDetails.exception?.description || ''));
  if (m.method === 'Runtime.consoleAPICalled' && m.params.type === 'error') pageErrors.push(m.params.args.map(a => a.value ?? a.description).join(' '));
});
const send = (method, params = {}) => new Promise(r => { const id = ++seq; pending.set(id, r); ws.send(JSON.stringify({ id, method, params })); });
const js = async expr => (await send('Runtime.evaluate', { expression: expr, awaitPromise: true, returnByValue: true })).result?.result?.value;
const size = (w, h) => send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: w < 600 });
await send('Runtime.enable'); await send('Page.enable');

const failures = []; let checks = 0;
const check = (ok, what, detail) => { checks++; if (!ok) failures.push(`${what}${detail === undefined ? '' : `\n    got: ${JSON.stringify(detail)}`}`); };
const open = async (project, tab) => {
  await send('Page.navigate', { url: `http://localhost:${port}/project?${new URLSearchParams({ project, tab })}` });
  for (let i = 0; i < 80; i++) { if (await js("!!document.querySelector('#graph-body svg, #graph-body .empty:not(:empty)') && !document.querySelector('#graph-body')?.textContent.includes('Loading')")) return; await wait(100); }
};

await size(1280, 900);
await open(chain, 'graph');
const nodes = await js("document.querySelectorAll('rect.graph-node').length");
const edges = await js("document.querySelectorAll('path.graph-edge').length");
check(nodes === 6, 'one node per plan, the archived predecessor included', nodes);
check(edges === 5, 'one edge per dependency', edges);
check(await js("document.querySelectorAll('path.graph-edge.unmet').length") === 4, 'unfinished edges are drawn dashed');
const held = await js("document.querySelector('.graph-held')?.textContent || ''");
check(held.includes('dace') && held.includes('waiting on caiman'), 'the held list names dace waiting on caiman', held);
check(held.includes('held by caiman'), 'deeper plans name the root that holds them', held);
check((await js("document.querySelector('.graph-summary')?.textContent || ''")).includes('5 plans · 1 ready · 3 waiting · 1 blocked'), 'summary counts by displayed status');
check((await js("document.querySelector('.graph-summary')?.textContent || ''")).includes('Next: independent'), 'Next names the ready unblocked plan');
const graphAPI = await (await fetch(`http://localhost:${port}/api/graph?${new URLSearchParams({project:chain})}`)).json();
const graphDocument = graphAPI.nodes ? graphAPI : graphAPI.data;
for (const n of graphDocument.nodes.filter(n=>!n.archived)) {
  check(await js(`Array.from(document.querySelectorAll('.graph-name')).some(x=>x.textContent.includes(${JSON.stringify(n.label+' · '+n.name)}))`), 'node shows canonical execution label '+n.name);
}
if (process.env.NENE_GRAPH_SCREENSHOT) {
 const shot = await send('Page.captureScreenshot', {format:'png',captureBeyondViewport:false});
 writeFileSync(process.env.NENE_GRAPH_SCREENSHOT, Buffer.from(shot.result.data,'base64'));
}
const through = await js(`(()=>{const rects=[...document.querySelectorAll('.graph-item')].map(g=>({n:g.querySelector('title').textContent.split(' · ')[0],b:g.querySelector('rect').getBBox()}));const hits=[];for(const p of document.querySelectorAll('path.graph-edge')){const [f,t]=p.querySelector('title').textContent.split(' · ')[0].split(' → ');const L=p.getTotalLength();for(let s=0;s<=60;s++){const q=p.getPointAtLength(L*s/60);for(const r of rects){if(r.n===f||r.n===t)continue;const b=r.b;if(q.x>b.x+2&&q.x<b.x+b.width-2&&q.y>b.y+2&&q.y<b.y+b.height-2)hits.push(f+'→'+t+' through '+r.n)}}}return [...new Set(hits)]})()`);
check(through.length === 0, 'no edge crosses another plan\'s box', through);
check(await js("document.querySelectorAll('rect.graph-node').length === new Set([...document.querySelectorAll('rect.graph-node')].map(r=>r.getAttribute('x')+','+r.getAttribute('y'))).size"), 'no two boxes share a position');
check(await js('document.scrollingElement.scrollWidth <= innerWidth'), 'no page horizontal scroll at 1280');
await size(390, 844); await wait(300);
check(await js('document.scrollingElement.scrollWidth <= innerWidth'), 'no page horizontal scroll at 390');
check(await js("!document.querySelector('.graph-scroll-hint').hidden"), 'a graph wider than its panel says to scroll sideways');
await size(1280, 900); await wait(200);
await js("[...document.querySelectorAll('a.graph-item')].find(a=>a.getAttribute('href').endsWith('name=dace')).dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true,view:window}))");
for (let i = 0; i < 60; i++) { if ((await js('location.pathname')) === '/plan') break; await wait(100); }
check(await js("location.pathname + '?' + new URLSearchParams(location.search).get('name')") === '/plan?dace', 'selecting a node opens its plan page');

await open(chain, 'plans');
const badge = await js("document.querySelector('article.plan-row[aria-label^=\"dace \"] .badge')?.textContent");
check(badge === 'Waiting', 'the Plans tab badges a held plan Waiting', badge);

// dag-web-3: a project with no plans.
await open(empty, 'graph');
const emptyText = await js("document.querySelector('#graph-body')?.textContent.trim()");
check(emptyText === 'No plans in this project.', 'an empty project shows its empty state', emptyText);
check(pageErrors.length === 0, 'no page errors', pageErrors);

ws.close();
if (failures.length) { console.error(`FAIL: ${failures.length} of ${checks} graph-tab checks\n  - ${failures.join('\n  - ')}`); process.exit(1); }
console.log(`ok: ${checks} graph-tab checks`);
process.exit(0);
