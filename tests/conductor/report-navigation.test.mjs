// TESTMASTER: id=conductor.report-navigation tier=standard parallel=no
// Plan report navigation in headless Chrome over the sanitized Albatross,
// Gemsbok and Waterbuck records: reading order, bounded Conductor activity,
// history loading that keeps open disclosures, focus and the selected
// requirement, keyboard reachability, 320px without horizontal scroll, and
// read-only rendering. Exits 77 (skip) when Chrome, Go or WebSocket is absent.
//
// Run: node --experimental-websocket tests/conductor/report-navigation.test.mjs
import { execFileSync, spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const fixtures = join(repo, 'src/internal/iterrun/testdata/reports');
const skip = why => { console.log(`SKIP: ${why}`); process.exit(77); };
const wait = ms => new Promise(r => setTimeout(r, ms));

const chromeBin = [process.env.CHROME_BIN, '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', '/Applications/Chromium.app/Contents/MacOS/Chromium',
  '/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser'].find(p => p && existsSync(p));
if (!chromeBin) skip('no Chrome or Chromium found; set CHROME_BIN to run the browser checks');
if (typeof WebSocket === 'undefined') skip('this Node has no WebSocket; run it with --experimental-websocket');
try { execFileSync('go', ['version'], { stdio: 'ignore' }); } catch { skip('go is not on PATH; the test builds iterate-run from this checkout'); }

// ---- Fixture: one project holding the three executions, an isolated HOME,
// run record and Conductor store, and a long hook log for the live plan.
const root = mkdtempSync(join(tmpdir(), 'report-nav-'));
const project = join(root, 'project'), home = join(root, 'home'), records = join(root, 'records'), conductorDir = join(root, 'conductor');
const procs = [];
const cleanup = () => { for (const p of procs) try { p.kill('SIGTERM'); } catch {} try { rmSync(root, { recursive: true, force: true }); } catch {} };
process.on('exit', cleanup);

const bin = join(root, 'iterate-run');
execFileSync('go', ['build', '-o', bin, './src/cmd/iterate-run'], { cwd: repo, stdio: 'inherit' });

for (const d of ['plans', 'archive']) mkdirSync(join(project, '.claude/iterate', d), { recursive: true });
cpSync(join(fixtures, 'integrity/waterbuck'), join(project, '.claude/iterate/plans'), { recursive: true });
cpSync(join(fixtures, 'integrity/gemsbok'), join(project, '.claude/iterate/archive'), { recursive: true });
cpSync(join(fixtures, 'integrity/albatross'), join(project, '.claude/iterate/archive'), { recursive: true });
const sub = s => s.replaceAll('{{PROJECT}}', project);
const copyTree = (from, to) => {
  if (!existsSync(from)) return;
  for (const e of readdirSync(from, { withFileTypes: true })) {
    const src = join(from, e.name), dst = join(to, e.name);
    if (e.isDirectory()) { mkdirSync(dst, { recursive: true }); copyTree(src, dst); }
    else if (e.name !== 'events.jsonl') writeFileSync(dst, sub(readFileSync(src, 'utf8')));
  }
};
const store = join(home, '.claude/iterate-run');
mkdirSync(store, { recursive: true });
let events = '';
for (const s of ['albatross', 'gemsbok', 'waterbuck']) {
  copyTree(join(fixtures, 'accounting', s, 'home'), home);
  events += sub(readFileSync(join(fixtures, 'accounting', s, 'home/.claude/iterate-run/events.jsonl'), 'utf8'));
}
const t0 = Date.parse('2026-10-21T00:00:00Z');
for (let i = 0; i < 2000; i++) {
  const at = new Date(t0 + i * 30000), id = `bulk-${i}`;
  events += JSON.stringify({ ts: at.toISOString(), hook: 'pre', session_id: 'wb-coord', tool_name: 'Bash', tool_use_id: id, plan: 'waterbuck', cwd: project }) + '\n';
  events += JSON.stringify({ ts: new Date(+at + 10000).toISOString(), hook: 'post', session_id: 'wb-coord', tool_name: 'Bash', tool_use_id: id, summary: `bulk step ${i}` }) + '\n';
}
writeFileSync(join(store, 'events.jsonl'), events);
writeFileSync(join(store, 'projects.json'), JSON.stringify([project]));

const port = await new Promise(r => { const s = createServer().listen(0, '127.0.0.1', () => { const p = s.address().port; s.close(() => r(p)); }); });
const env = { ...process.env, HOME: home, CODEX_HOME: join(home, '.codex'), ITERATE_RUN_RECORD_DIR: records, ITERATE_CONDUCTOR_DIR: conductorDir };
const server = spawn(bin, ['serve', '--port', String(port)], { env, stdio: 'ignore' });
procs.push(server);
for (let i = 0; ; i++) {
  try { if ((await fetch(`http://localhost:${port}/healthz`)).ok) break; } catch {}
  if (i > 100) { console.error('FAIL: the preview server never answered /healthz'); process.exit(1); }
  await wait(100);
}
await wait(500);

// Everything a read-only check must leave byte-identical.
const digest = () => {
  const out = {};
  const walk = d => { if (!existsSync(d)) return; for (const e of readdirSync(d)) { const p = join(d, e); if (statSync(p).isDirectory()) walk(p); else out[p] = createHash('sha256').update(readFileSync(p)).digest('hex'); } };
  for (const d of [join(project, '.claude'), records, conductorDir, store]) walk(d);
  return out;
};
const before = digest();

// ---- Headless Chrome over the DevTools protocol.
const profile = mkdtempSync(join(root, 'chrome-'));
const debugPort = await new Promise(r => { const s = createServer().listen(0, '127.0.0.1', () => { const p = s.address().port; s.close(() => r(p)); }); });
const chrome = spawn(chromeBin, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-gpu', `--remote-debugging-port=${debugPort}`, `--user-data-dir=${profile}`, '--window-size=1280,900', 'about:blank'], { stdio: 'ignore' });
procs.push(chrome);
let targets = [];
for (let i = 0; i < 150 && !targets.some(t => t.type === 'page'); i++) { try { targets = await (await fetch(`http://127.0.0.1:${debugPort}/json`)).json(); } catch {} await wait(100); }
const target = targets.find(t => t.type === 'page');
if (!target) { console.error('FAIL: headless Chrome started but exposed no page target'); process.exit(1); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise(r => ws.addEventListener('open', r, { once: true }));
let seq = 0; const calls = new Map(); const listeners = new Set(); const consoleErrors = [];
ws.addEventListener('message', e => {
  const m = JSON.parse(e.data);
  if (m.id && calls.has(m.id)) { const c = calls.get(m.id); calls.delete(m.id); m.error ? c.reject(Error(m.error.message)) : c.resolve(m.result); }
  else if (m.method) for (const l of listeners) l(m);
});
const send = (method, params = {}) => new Promise((res, rej) => { const id = ++seq; calls.set(id, { resolve: res, reject: rej }); ws.send(JSON.stringify({ id, method, params })); });
listeners.add(m => {
  if (m.method === 'Runtime.exceptionThrown') consoleErrors.push(m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text);
  if (m.method === 'Runtime.consoleAPICalled' && m.params.type === 'error') consoleErrors.push(m.params.args.map(a => a.value ?? a.description).join(' '));
});
await send('Page.enable'); await send('Runtime.enable');
const evaluate = async expr => {
  const r = await send('Runtime.evaluate', { expression: `(async()=>{${expr}})()`, awaitPromise: true, returnByValue: true });
  if (r.exceptionDetails) throw Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text);
  return r.result.value;
};
const goto = async path => {
  const loaded = new Promise(r => { const l = m => { if (m.method === 'Page.loadEventFired') { listeners.delete(l); r(); } }; listeners.add(l); });
  await send('Page.navigate', { url: `http://localhost:${port}${path}` });
  await loaded;
};
const key = async (k, code, vk) => {
  await send('Input.dispatchKeyEvent', { type: 'keyDown', key: k, code, windowsVirtualKeyCode: vk, ...(k === 'Enter' ? { text: '\r' } : {}) });
  await send('Input.dispatchKeyEvent', { type: 'keyUp', key: k, code, windowsVirtualKeyCode: vk });
};
const historyLoaded = async () => {
  for (let i = 0; i < 300; i++) {
    const s = await evaluate(`return document.querySelector('#plan-history-status')?.textContent ?? 'none'`);
    if (/loaded|could not|none/.test(s)) return s;
    await wait(100);
  }
  return 'timed out';
};

const failures = [];
let checks = 0;
const check = (ok, what, detail) => { checks++; if (!ok) failures.push(`${what}${detail === undefined ? '' : `\n    got: ${typeof detail === 'string' ? detail : JSON.stringify(detail)}`}`); };

const q = (k, v) => new URLSearchParams({ project, [k]: v }).toString();
const pages = {
  waterbuck: { path: `/plan?${q('name', 'waterbuck')}`, live: true },
  gemsbok: { path: `/archive?${q('file', '20261020T094038Z-gemsbok.md')}`, live: false },
  albatross: { path: `/archive?${q('file', '2026-09-11T02-27-19Z-albatross-done.md')}`, live: false },
};

try {
  // 1. Reading order, explicit section states and resolving links, wide.
  for (const [name, p] of Object.entries(pages)) {
    await goto(p.path);
    const status = await historyLoaded();
    check(status.includes('loaded'), `${name}: tool history loads`, status);
    const r = await evaluate(`
      const ids = ['main','plan-elapsed','report-progress','report-evidence','plan-timeline','report-time','report-ownership','report-rework','report-usage','report-coverage','report-delivery'${p.live ? ",'conductor-live-detail'" : ''}];
      const tops = ids.map(id => { const el = document.getElementById(id); return el ? Math.round(el.getBoundingClientRect().top + scrollY) : null; });
      const h1 = document.querySelector('h1'), toc = document.querySelector('.report-toc');
      const states = ['report-progress','report-evidence','report-time','report-ownership','report-rework','report-usage','report-coverage','report-delivery']
        .filter(id => !document.querySelector('#' + id + ' .rs-state, #' + id + ' [data-status], #' + id + ' h2 .ra-badge'));
      const broken = [...document.querySelectorAll('main a[href^="#"]')].map(a => a.getAttribute('href').slice(1)).filter(id => !document.getElementById(id));
      const first = [...document.querySelectorAll('a[href], button, summary, [tabindex]:not([tabindex="-1"])')][0];
      return { tops, h1: h1 && Math.round(h1.getBoundingClientRect().top + scrollY), toc: toc && Math.round(toc.getBoundingClientRect().top + scrollY),
        crumbs: Math.round(document.querySelector('.conductor-detail-nav').getBoundingClientRect().top + scrollY), states, broken,
        first: first && first.className, mains: document.querySelectorAll('main').length,
        replaced: ['.stats','.runbox','.gapmark','.gaplist','.tk-top','#note-modal','#bd-detail','[onclick]'].filter(s => document.querySelector(s)) };`);
    check(r.tops.every(t => t !== null), `${name}: every section renders`, r.tops);
    check(r.tops.every((t, i) => i === 0 || t >= r.tops[i - 1]), `${name}: sections read summary, timeline, breakdowns${p.live ? ', then Conductor activity' : ''}`, r.tops);
    check(r.crumbs < r.h1 && r.h1 < r.toc && r.toc < r.tops[2], `${name}: breadcrumbs, plan header and section list come first`, r);
    check(r.first === 'skip-link', `${name}: the skip link is the first focusable element`, r.first);
    check(r.mains === 1, `${name}: one main landmark`, r.mains);
    check(r.states.length === 0, `${name}: every metric section states available, partial or unavailable`, r.states);
    check(r.broken.length === 0, `${name}: every in-page link has a target`, r.broken);
    check(r.replaced.length === 0, `${name}: no unlabelled stat cards, severe-gap marks, token panel, note modal or click handlers`, r.replaced);
  }

  // 2. History loading keeps what the reader did: hold the history request,
  // interact, then release it.
  await send('Fetch.enable', { patterns: [{ urlPattern: '*history=1*', requestStage: 'Request' }] });
  let paused = null;
  const onPause = m => { if (m.method === 'Fetch.requestPaused') paused = m.params.requestId; };
  listeners.add(onPause);
  await goto(pages.gemsbok.path);
  for (let i = 0; i < 100 && !paused; i++) await wait(50);
  check(!!paused, 'gemsbok: the page requests tool history after it renders');
  const pre = await evaluate(`
    document.querySelector('#report-progress .pg-more').open = true;
    document.querySelector('#report-time details').open = true;
    return { status: document.querySelector('#plan-history-status').textContent,
      usage: document.querySelector('#report-usage').textContent.includes('until tool history loads') };`);
  check(/Loading tool history/.test(pre.status), 'gemsbok: the status line says history is loading', pre.status);
  check(pre.usage, 'gemsbok: before history, Usage says it waits for tool history', pre);
  await evaluate(`document.querySelector('a.bd-cell[href="#evidence-req-2"]').focus()`);
  await key('Enter', 'Enter', 13);
  await wait(200);
  const chosen = await evaluate(`
    const li = document.getElementById('evidence-req-2');
    return { hash: location.hash, open: li.querySelector('details').open, focused: document.activeElement === li.querySelector('summary'),
      selected: document.querySelector('a.bd-cell[href="#evidence-req-2"]').classList.contains('bd-selected') };`);
  check(chosen.hash === '#evidence-req-2' && chosen.open && chosen.focused && chosen.selected,
    'gemsbok: Enter on requirement cell 2 opens its evidence, focuses it and marks the cell', chosen);
  await send('Fetch.continueRequest', { requestId: paused });
  const status = await historyLoaded();
  listeners.delete(onPause);
  await send('Fetch.disable');
  check(status.includes('loaded'), 'gemsbok: history loads after release', status);
  const post = await evaluate(`
    const li = document.getElementById('evidence-req-2');
    return { usage: document.querySelector('#report-usage').textContent.includes('until tool history loads'),
      coverage: document.querySelector('#report-coverage').textContent.includes('until tool history loads'),
      more: document.querySelector('#report-progress .pg-more').open, defs: document.querySelector('#report-time details').open,
      open: li.querySelector('details').open, focused: document.activeElement === li.querySelector('summary'),
      selected: document.querySelector('a.bd-cell[href="#evidence-req-2"]').classList.contains('bd-selected') };`);
  check(!post.usage && !post.coverage, 'gemsbok: after history loads, Usage and Tracking coverage no longer wait for it', post);
  check(post.more && post.defs, 'gemsbok: open disclosures stay open through the history swap', post);
  check(post.open && post.focused && post.selected, 'gemsbok: the selected requirement keeps its open evidence, focus and grid mark', post);

  // 3. Keyboard: skip link, then Tab reaches every requirement cell, the
  // section list and the Conductor event log, each with a visible focus ring.
  await goto(pages.waterbuck.path);
  await historyLoaded();
  await key('Tab', 'Tab', 9);
  const skipFocused = await evaluate(`return document.activeElement.className`);
  check(skipFocused === 'skip-link', 'waterbuck: the first Tab lands on the skip link', skipFocused);
  await key('Enter', 'Enter', 13);
  await wait(100);
  const inMain = await evaluate(`return document.activeElement.id`);
  check(inMain === 'main', 'waterbuck: the skip link moves focus to the report', inMain);
  const total = await evaluate(`return document.querySelectorAll('a[href], summary, [tabindex="0"], button').length`);
  const reached = new Set(); let ringless = [];
  for (let i = 0; i < total + 10; i++) {
    await key('Tab', 'Tab', 9);
    const f = await evaluate(`const a = document.activeElement; const cs = getComputedStyle(a);
      return { id: a.getAttribute('href') || a.className || a.tagName, cell: a.classList.contains('bd-cell'), toc: !!a.closest('.report-toc'),
        log: a.classList.contains('conductor-event-scroll'), ring: cs.outlineStyle !== 'none' && parseFloat(cs.outlineWidth) >= 2 };`);
    if (f.cell || f.toc || f.log) { reached.add((f.cell ? 'cell' : f.toc ? 'toc' : 'log') + ':' + f.id); if (!f.ring) ringless.push(f.id); }
    if (f.log) break;
  }
  const want = await evaluate(`return { cells: document.querySelectorAll('a.bd-cell').length, toc: document.querySelectorAll('.report-toc a').length }`);
  const got = { cells: [...reached].filter(k => k.startsWith('cell:')).length, toc: [...reached].filter(k => k.startsWith('toc:')).length, log: [...reached].some(k => k.startsWith('log:')) };
  check(got.cells === want.cells && got.toc === want.toc && got.log, 'waterbuck: Tab reaches every requirement cell, section link and the event log', { got, want });
  check(ringless.length === 0, 'waterbuck: every keyboard-focused control shows a focus ring', ringless);

  // 4. The Conductor activity stays bounded below the report however long
  // its log grows.
  await wait(1500);
  const act = await evaluate(`
    const log = document.querySelector('.conductor-event-scroll');
    for (let i = 0; i < 600; i++) { const p = document.createElement('p'); p.textContent = 'injected event ' + i; log.append(p); }
    const sec = document.getElementById('conductor-live-detail');
    return { logH: log.clientHeight, logScroll: log.scrollHeight, secH: Math.round(sec.getBoundingClientRect().height), vh: innerHeight,
      below: sec.getBoundingClientRect().top > document.getElementById('report-delivery').getBoundingClientRect().top };`);
  check(act.logH <= Math.min(260, act.vh * 0.4) + 1 && act.logScroll > act.logH * 4 && act.secH < 480 && act.below,
    'waterbuck: a 600-line event log scrolls inside a bounded region after the report', act);

  // 5. 320px: no horizontal scroll with every disclosure open, and a usable
  // gantt track.
  await send('Emulation.setDeviceMetricsOverride', { width: 320, height: 800, deviceScaleFactor: 1, mobile: true });
  for (const [name, p] of Object.entries(pages)) {
    await goto(p.path);
    await historyLoaded();
    const n = await evaluate(`document.querySelectorAll('details').forEach(d => d.open = true);
      const de = document.documentElement;
      const wide = [...document.querySelectorAll('main *')].filter(e => { const r = e.getBoundingClientRect(); return r.width > 0 && r.right > de.clientWidth + 1 && !e.closest('.vh,.conductor-event-scroll'); }).slice(0, 5).map(e => e.tagName + '.' + e.className);
      const tracks = [...document.querySelectorAll('.gantt .track')].map(t => Math.round(t.getBoundingClientRect().width));
      return { scrollW: de.scrollWidth, clientW: de.clientWidth, wide, minTrack: Math.min(...tracks) };`);
    check(n.scrollW <= n.clientW && n.wide.length === 0, `${name}: no horizontal scroll at 320px`, n);
    check(n.minTrack >= 200, `${name}: gantt tracks keep their width at 320px`, n.minTrack);
  }

  check(consoleErrors.length === 0, 'no console errors or uncaught exceptions', consoleErrors);
} catch (e) {
  failures.push(`the browser run stopped: ${e.stack || e}`);
}

// 6. Reading changed nothing.
const after = digest();
const changed = [...new Set([...Object.keys(before), ...Object.keys(after)])].filter(p => before[p] !== after[p]);
check(changed.length === 0, 'plan files, the run record, the Conductor store and the hook log are byte-identical after the checks', changed);

ws.close();
if (failures.length) {
  console.error(`FAIL: ${failures.length} of ${checks} report navigation checks failed`);
  for (const f of failures) console.error(`  - ${f}`);
  process.exit(1);
}
console.log(`ok: ${checks} report navigation checks passed`);
process.exit(0);
