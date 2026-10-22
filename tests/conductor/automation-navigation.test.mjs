// TESTMASTER: id=conductor.automation-navigation tier=standard parallel=no
// Named Automations navigation in headless Chrome against a seeded, isolated
// Conductor: the sidebar shows the full list from an open, edited editor;
// Save persists a band after an unrelated revision; Cancel keeps saved values;
// create, duplicate, Back/Forward and reload never leave a stale editor or
// drop an unsaved edit; a saved rename reaches the list, the project picker
// and attached rows, an unsaved one shows as a draft; keyboard attach and
// detach on Project Associations persist, and a duplicate attach writes
// nothing. Every click is a pointer press held as long as a person's, and a
// press that produces no click fails: Save or Cancel straight after typing
// a name (F61), a press held across the 3-second refresh (F62), and a press
// straight after typing a project priority (F63). Exits 77 (skip) when
// Chrome, Go or WebSocket is absent.
//
// Run: node --experimental-websocket tests/conductor/automation-navigation.test.mjs
import { execFileSync, spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, realpathSync, rmSync, statSync } from 'node:fs';
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

// ---- Isolated state: HOME, run record and Conductor store under one temp
// root, three project directories, and Chrome and the server in their own
// process groups so every helper process dies with them.
const root = realpathSync(mkdtempSync(join(tmpdir(), 'automation-nav-')));
const home = join(root, 'home'), records = join(root, 'records'), conductorDir = join(root, 'conductor');
const procs = [];
const kill = () => { for (const p of procs) try { process.kill(-p.pid, 'SIGKILL'); } catch {} };
const cleanup = () => {
  kill();
  try { rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 }); } catch (e) { console.error(`cleanup: could not remove ${root}: ${e.message}`); }
};
// Kills both process groups and waits until no member is left, because a
// Chrome helper still writing its profile makes the temp root unremovable.
const stop = async () => {
  kill();
  const alive = p => { try { process.kill(-p.pid, 0); return true; } catch { return false; } };
  for (let i = 0; i < 100 && procs.some(alive); i++) await wait(50);
  cleanup();
};
process.on('exit', cleanup);
for (const sig of ['SIGINT', 'SIGTERM', 'SIGHUP']) process.on(sig, () => stop().finally(() => process.exit(1)));

const bin = join(root, 'iterate-run');
execFileSync('go', ['build', '-o', bin, './src/cmd/iterate-run'], { cwd: repo, stdio: 'inherit' });

const dirs = { filemaster: join(root, 'projects/filemaster'), izcros: join(root, 'projects/izcros'), symmail: join(root, 'projects/symmail') };
for (const d of [home, records, conductorDir]) mkdirSync(d, { recursive: true });
for (const d of Object.values(dirs)) mkdirSync(join(d, '.claude/iterate/plans'), { recursive: true });

const freePort = () => new Promise(r => { const s = createServer().listen(0, '127.0.0.1', () => { const p = s.address().port; s.close(() => r(p)); }); });
const port = await freePort();
const origin = `http://localhost:${port}`;
const env = { ...process.env, HOME: home, CODEX_HOME: join(home, '.codex'), ITERATE_RUN_RECORD_DIR: records, ITERATE_CONDUCTOR_DIR: conductorDir };
const server = spawn(bin, ['serve', '--port', String(port)], { env, stdio: 'ignore', detached: true });
procs.push(server);
for (let i = 0; ; i++) {
  try { if ((await fetch(`${origin}/healthz`)).ok) break; } catch {}
  if (i > 100) { console.error('FAIL: the dashboard server never answered /healthz'); process.exit(1); }
  await wait(100);
}

// ---- Seed through the dashboard's own API: three projects, three named
// automations, one attachment each.
const config = async () => (await (await fetch(`${origin}/api/conductor/config`)).json()).data;
const postConfig = async c => {
  const r = await fetch(`${origin}/api/conductor/config`, { method: 'POST', headers: { 'Content-Type': 'application/json', Origin: origin }, body: JSON.stringify({ expected_revision: c.revision, config: c }) });
  const j = await r.json();
  if (!r.ok || j.ok === false) throw Error(`config write refused: ${JSON.stringify(j)}`);
  return j.data;
};
{
  const c = await config();
  c.subscriptions.find(s => s.id === 'codex').sequences.push({ id: 'codex-fast', name: 'fast', model: '', reasoning: 'low', preset_id: '', args: [] });
  const project = (id, name, path, enabled, paused, tags) => ({ id, name, path, tags, enabled, paused, max_attempts: 5, no_progress_seconds: 2700, watch_bound: 10, max_import: 10 });
  c.projects = [
    project('project-filemaster', 'Filemaster', dirs.filemaster, true, false, ['work']),
    project('project-izcros', 'izcrOS', dirs.izcros, false, false, []),
    project('project-symmail', 'Symmail', dirs.symmail, true, true, ['personal']),
  ];
  const auto = (id, name, entries) => ({ id, name, enabled: false, mode: 'keep_working', entries: entries.map(([s, b], i) => ({ id: `${id}-e${i}`, sequence_id: s, max_band: b })), participants: [], honor_engine_overdrive: false, only_during_overdrive: false, run_limit: 'drain_queue', on_window_end: 'finish_checkpoint' });
  c.automations = [
    auto('automation-run-yellow', 'Run yellow (Codex)', [['codex-build', 'yellow']]),
    auto('automation-yellow-go', 'yellow go', [['claude-build', 'green'], ['codex-fast', 'yellow']]),
    auto('automation-green', 'Green and go', [['codex-build', 'green']]),
  ];
  c.automation_attachments = [
    { id: 'attachment-fm-yellow-go', definition_id: 'automation-yellow-go', project_id: 'project-filemaster', enabled: true, priority: 10 },
    { id: 'attachment-iz-run-yellow', definition_id: 'automation-run-yellow', project_id: 'project-izcros', enabled: false, priority: 10 },
    { id: 'attachment-sy-green', definition_id: 'automation-green', project_id: 'project-symmail', enabled: false, priority: 5 },
  ];
  try { await postConfig(c); } catch (e) { console.error(`FAIL: could not seed the isolated Conductor: ${e.message}`); process.exit(1); }
}
const projectPaths = Object.fromEntries((await config()).projects.map(p => [p.id, p.path]));
const storeDigest = () => {
  const out = {};
  const walk = d => { for (const e of readdirSync(d)) { const p = join(d, e); if (statSync(p).isDirectory()) walk(p); else out[p.slice(conductorDir.length)] = createHash('sha256').update(readFileSync(p)).digest('hex'); } };
  walk(conductorDir);
  return out;
};

// ---- Headless Chrome over the DevTools protocol.
const profile = mkdtempSync(join(root, 'chrome-'));
const debugPort = await freePort();
const chrome = spawn(chromeBin, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-gpu', `--remote-debugging-port=${debugPort}`, `--user-data-dir=${profile}`, '--window-size=1280,900', 'about:blank'], { stdio: 'ignore', detached: true });
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
let dragData = null;
listeners.add(m => { if (m.method === 'Input.dragIntercepted') dragData = m.params.data; });
await send('Page.enable'); await send('Runtime.enable'); await send('Network.enable');
// The dashboard's own requests, so a check can wait for one full poll.
const sent = new Map(), finished = [];
listeners.add(m => {
  if (m.method === 'Network.requestWillBeSent') sent.set(m.params.requestId, { url: m.params.request.url.split('?')[0], at: Date.now() });
  if (m.method === 'Network.loadingFinished' && sent.has(m.params.requestId)) finished.push({ ...sent.get(m.params.requestId), done: Date.now() });
});
// Resolves once every endpoint the 3-second poll reads has answered a
// request sent after the call, and the page has had time to act on it.
const pollEndpoints = ['config', 'status', 'projects', 'usage', 'runs', 'visibility'];
const nextPoll = async (ms = 7000) => {
  const from = Date.now(), end = from + ms;
  while (Date.now() < end) {
    const seen = new Set(finished.filter(f => f.at >= from).map(f => f.url.replace(/^.*\/api\/conductor\//, '')));
    if (pollEndpoints.every(e => seen.has(e))) { await wait(250); return true; }
    await wait(50);
  }
  return false;
};

const evaluate = async expr => {
  const r = await send('Runtime.evaluate', { expression: `(async()=>{${expr}})()`, awaitPromise: true, returnByValue: true });
  if (r.exceptionDetails) throw Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text);
  return r.result.value;
};
const loadFired = () => new Promise(r => { const l = m => { if (m.method === 'Page.loadEventFired') { listeners.delete(l); r(); } }; listeners.add(l); });
const goto = async path => { const loaded = loadFired(); await send('Page.navigate', { url: origin + path }); await loaded; };
const reload = async () => { const loaded = loadFired(); await send('Page.reload'); await loaded; };
const historyGo = async dir => {
  const h = await send('Page.getNavigationHistory'), entry = h.entries[h.currentIndex + dir];
  if (!entry) throw Error(`the browser has no history entry ${dir > 0 ? 'forward' : 'back'}`);
  await send('Page.navigateToHistoryEntry', { entryId: entry.id });
};
// Polls a page expression until it is truthy; returns its last value.
const until = async (expr, ms = 8000) => {
  const end = Date.now() + ms; let v;
  do { v = await evaluate(`return (${expr})`); if (v) return v; await wait(50); } while (Date.now() < end);
  return v;
};
// A real pointer click at the element's center once it holds still (the
// editor's tester renders after the form and moves the buttons below it),
// refused when something else would receive it. The button is held for a
// person's 120 ms, or until `during` resolves, so whatever the press sets off
// (a blur, a save, a background refresh) lands before the release. A press
// that produces no click on the element stops the section: that is the
// "Save appears to do nothing" failure.
const click = async (sel, what, during = () => wait(120)) => {
  let p, prev;
  for (let i = 0; i < 25; i++) {
    p = await evaluate(`const el = document.querySelector(${JSON.stringify(sel)}); if (!el) return { missing: true };
      el.scrollIntoView({ block: 'center' }); const r = el.getBoundingClientRect(), x = r.left + r.width / 2, y = r.top + r.height / 2, hit = document.elementFromPoint(x, y);
      window.__press = { el, clicked: false };
      return { x, y, ok: !!hit && (hit === el || el.contains(hit)), by: hit ? hit.tagName + (hit.id ? '#' + hit.id : '') + (hit.className ? '.' + hit.className : '') : 'nothing' };`);
    if (prev && !p.missing && p.ok && p.x === prev.x && p.y === prev.y) break;
    prev = p;
    await wait(80);
  }
  if (p.missing) throw Error(`${what}: nothing on the page matches ${sel} (at ${await evaluate('return location.pathname + location.search')})`);
  if (!p.ok) throw Error(`${what}: ${sel} is covered by ${p.by}`);
  await evaluate(`window.__pressSeen = e => { if (e.target.closest?.(${JSON.stringify(sel)})) window.__press.clicked = true; };
    document.addEventListener('click', window.__pressSeen, true);`);
  await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: p.x, y: p.y });
  await send('Input.dispatchMouseEvent', { type: 'mousePressed', x: p.x, y: p.y, button: 'left', buttons: 1, clickCount: 1 });
  await Promise.all([wait(120), during()]);
  const at = await evaluate(`const el = window.__press.el, h = document.elementFromPoint(${p.x}, ${p.y});
    return { replaced: !el.isConnected, under: h ? h.tagName + (h.id ? '#' + h.id : '') + (h.className ? '.' + h.className : '') : 'nothing' };`);
  await send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: p.x, y: p.y, button: 'left', buttons: 0, clickCount: 1 });
  const clicked = await evaluate(`document.removeEventListener('click', window.__pressSeen, true); return window.__press.clicked;`);
  if (!clicked) throw Error(`${what}: the press on ${sel} produced no click; ${at.replaced ? 'the page replaced the element during the press and ' : ''}the release landed on ${at.under}`);
};
const key = async (k, code, vk, text) => {
  await send('Input.dispatchKeyEvent', { type: 'keyDown', key: k, code, windowsVirtualKeyCode: vk, ...(text ? { text } : {}) });
  await send('Input.dispatchKeyEvent', { type: 'keyUp', key: k, code, windowsVirtualKeyCode: vk });
};
// Replaces a field's text the way a person does: focus, select all, type.
const typeInto = async (sel, text) => {
  await evaluate(`const el = document.querySelector(${JSON.stringify(sel)}); el.focus(); el.select();`);
  await send('Input.insertText', { text });
};
// Native select popups are outside the page in headless Chrome, so a band is
// chosen by value with the change event a picked option fires.
const chooseBand = async (row, value) => {
  await evaluate(`const s = document.querySelectorAll('#automation-form [data-band]')[${row}]; s.value = ${JSON.stringify(value)}; s.dispatchEvent(new Event('change', { bubbles: true }));`);
  await until(testerDone);
};
// A real pointer drag: Chrome starts a native drag, CDP intercepts it and
// delivers the drop where a person would release it.
const drag = async (fromSel, toSel, what) => {
  const center = sel => evaluate(`const el = document.querySelector(${JSON.stringify(sel)}); if (!el) return null; el.scrollIntoView({ block: 'center' });
    const r = el.getBoundingClientRect(); return { x: r.left + Math.min(r.width / 2, 40), y: r.top + r.height / 2 };`);
  await send('Input.setInterceptDrags', { enabled: true });
  try {
    const a = await center(fromSel);
    if (!a) throw Error(`${what}: nothing on the page matches ${fromSel}`);
    dragData = null;
    await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: a.x, y: a.y });
    await send('Input.dispatchMouseEvent', { type: 'mousePressed', x: a.x, y: a.y, button: 'left', buttons: 1, clickCount: 1 });
    for (const d of [8, 30]) await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: a.x + d, y: a.y + d, button: 'left', buttons: 1 });
    for (let i = 0; i < 60 && !dragData; i++) await wait(50);
    if (!dragData) throw Error(`${what}: Chrome never started a drag from ${fromSel}`);
    const b = await center(toSel);
    if (!b) throw Error(`${what}: nothing on the page matches ${toSel}`);
    for (const type of ['dragEnter', 'dragOver']) await send('Input.dispatchDragEvent', { type, x: b.x, y: b.y, data: dragData });
    await send('Input.dispatchDragEvent', { type: 'drop', x: b.x, y: b.y, data: dragData });
    await send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: b.x, y: b.y, button: 'left', buttons: 0, clickCount: 1 });
  } finally { await send('Input.setInterceptDrags', { enabled: false }); }
};

const failures = [];
let checks = 0;
const check = (ok, what, detail) => { checks++; if (!ok) failures.push(`${what}${detail === undefined ? '' : `\n    got: ${typeof detail === 'string' ? detail : JSON.stringify(detail)}`}`); };
const section = async (name, body) => {
  try { await evaluate(`sessionStorage.clear()`).catch(() => {}); await body(); }
  catch (e) { failures.push(`${name} stopped: ${e.message}`); }
};

const EDIT = id => `[data-action="edit-automation"][data-id="${id}"]`;
const RAIL = '.rail a[href*="section=automations"]';
const SAVE = '#automation-form button[type=submit]';
// The editor counts as open once its band tester has answered, because the
// answer moves the Save and Cancel buttons.
const testerDone = `!!document.querySelector('#automation-result') && !/Evaluating/.test(document.querySelector('#automation-result').textContent)`;
const editorOpen = id => `new URLSearchParams(location.search).get('automation') === ${JSON.stringify(id)} && !!document.querySelector('#automation-form [name=name]') && ${testerDone}`;
// Waits for an automation's editor; the section stops with what is on
// screen when it never opens.
const editor = async (id, after) => {
  if (await until(editorOpen(id))) return;
  const v = await view();
  throw Error(`${after}: the editor for ${id} never opened; the page shows ${v.form ? `the editor for ${v.automation}` : `the ${v.heading || 'unknown'} view`} at ?${new URLSearchParams({ section: v.section || '', automation: v.automation || '' })}`);
};
const listOpen = `!document.querySelector('#automation-form') && !new URLSearchParams(location.search).get('automation') && !!document.querySelector('.settings-content .row')`;
// What a person sees in Settings: route, editor fields, and every list row
// with its badges, hints and whether anything covers it.
const view = () => evaluate(`
  const q = new URLSearchParams(location.search), form = document.querySelector('#automation-form');
  const rows = [...document.querySelectorAll('.settings-content .row')].map(r => {
    const s = r.querySelector('strong'); let covered = 'no name';
    if (s) { s.scrollIntoView({ block: 'nearest' }); const b = s.getBoundingClientRect(); const h = b.width ? document.elementFromPoint(b.left + 2, b.top + b.height / 2) : null;
      covered = !b.width ? 'not rendered' : h && r.contains(h) ? false : (h ? h.tagName + (h.id ? '#' + h.id : '') + (h.className ? '.' + h.className : '') : 'off screen'); }
    return { id: r.querySelector('[data-action="edit-automation"]')?.dataset.id, name: s?.textContent, draft: r.classList.contains('draft-row'),
      badges: [...r.querySelectorAll('.badge')].map(b => b.textContent), hints: [...r.querySelectorAll('.hint')].map(h => h.textContent), covered };
  });
  return { section: q.get('section'), automation: q.get('automation'), form: !!form, title: document.querySelector('#automation-title')?.textContent ?? null,
    name: form?.querySelector('[name=name]')?.value ?? null, bands: form ? [...form.querySelectorAll('[data-band]')].map(s => s.value) : null,
    note: document.querySelector('#draft-note')?.textContent ?? '', error: document.querySelector('#automation-error')?.textContent ?? '',
    cancel: !!document.querySelector('[data-action="cancel-automation"]'), heading: document.querySelector('.settings-content h2')?.textContent ?? '', rows };`);
const savedNames = async () => (await config()).automations.map(a => a.name);
// The sidebar's list: no editor, no Cancel, every saved automation visible.
const expectList = async label => {
  await until(listOpen);
  const v = await view(), saved = await savedNames(), names = v.rows.filter(r => !r.draft).map(r => r.name);
  check(v.section === 'automations' && !v.automation && !v.form && !v.cancel && /^Named Automations/.test(v.heading),
    `${label}: Named Automations shows its list, with no editor, no Cancel and no ?automation= in the URL`, { ...v, rows: v.rows.length });
  check(JSON.stringify(names) === JSON.stringify(saved), `${label}: the list shows every saved automation`, { names, saved });
  check(v.rows.every(r => r.covered === false), `${label}: no list row is hidden or covered`, v.rows.filter(r => r.covered !== false));
  return v;
};
const row = (v, id) => v.rows.find(r => r.id === id && !r.draft);

try {
  // ---- 4b: the sidebar shows the list from an open, edited editor; Save,
  // Cancel, create, duplicate, Back/Forward and reload.
  await section('4b navigation', async () => {
    await goto('/settings?section=automations');
    await until(`!!document.querySelector('${EDIT('automation-run-yellow')}')`);
    await click(EDIT('automation-run-yellow'), 'open Run yellow (Codex)');
    await editor('automation-run-yellow', 'open Run yellow (Codex)');
    let v = await view();
    check(v.title === 'Edit automation · Run yellow (Codex)' && v.name === 'Run yellow (Codex)' && v.bands?.[0] === 'yellow',
      '4b: Edit opens Run yellow (Codex) at ?automation=automation-run-yellow with its saved name and band', v);
    await typeInto('#automation-form [name=name]', 'Run yellow (Codex) v2');
    await chooseBand(0, 'red');

    await click(RAIL, 'sidebar Named Automations from the edited editor');
    v = await expectList('4b: sidebar from an edited Run yellow (Codex)');
    const ry = row(v, 'automation-run-yellow');
    check(ry?.name === 'Run yellow (Codex)' && ry.badges.includes('Unsaved changes') && ry.hints.some(h => h.includes('Draft name: “Run yellow (Codex) v2”')),
      '4b: the list keeps the saved name and marks Run yellow (Codex) Unsaved changes with its draft name', ry);

    await click(EDIT('automation-run-yellow'), 'resume Run yellow (Codex)');
    await editor('automation-run-yellow', 'resume Run yellow (Codex)');
    v = await view();
    check(v.name === 'Run yellow (Codex) v2' && v.bands?.[0] === 'red', '4b: Resume editing restores the unsaved name and band', v);

    await typeInto('#automation-form [name=name]', 'Run yellow (Codex) v3');
    await click('[data-action="cancel-automation"]', 'Cancel pressed straight after typing a name');
    v = await expectList('4b: Cancel');
    let saved = (await config()).automations.find(a => a.id === 'automation-run-yellow');
    check(saved.name === 'Run yellow (Codex)' && saved.entries[0].max_band === 'yellow' && !row(v, 'automation-run-yellow')?.badges.includes('Unsaved changes'),
      '4b: Cancel keeps the saved name and band and clears the draft', { saved: [saved.name, saved.entries[0].max_band], row: row(v, 'automation-run-yellow') });

    await click(EDIT('automation-run-yellow'), 'reopen Run yellow (Codex)');
    await editor('automation-run-yellow', 'reopen Run yellow (Codex)');
    v = await view();
    check(v.name === 'Run yellow (Codex)' && v.bands?.[0] === 'yellow', '4b: after Cancel the editor reopens on the saved values', v);
    await chooseBand(0, 'red');
    const other = await config();
    other.projects.find(p => p.id === 'project-filemaster').tags.push('other-writer');
    const otherRev = (await postConfig(other)).revision;
    await click(SAVE, 'Save the red band');
    v = await expectList('4b: Save after an unrelated revision');
    const afterSave = await config();
    saved = afterSave.automations.find(a => a.id === 'automation-run-yellow');
    check(saved.entries[0].max_band === 'red' && afterSave.revision > otherRev && afterSave.projects.find(p => p.id === 'project-filemaster').tags.includes('other-writer'),
      '4b: Save persists band red over an unrelated revision and keeps the other writer’s change', { band: saved.entries[0].max_band, revision: afterSave.revision, otherRev });
    const focus = await until(`document.activeElement?.matches('${EDIT('automation-run-yellow')}')`, 3000);
    check(focus, '4b: Save returns focus to Run yellow (Codex)’s Edit button in the list', await evaluate(`return document.activeElement?.outerHTML.slice(0, 120)`));

    // Create: leave through the sidebar, resume the draft row, save.
    await click('[data-action="add-automation"]', '+ Add automation');
    const newID = await until(`!!document.querySelector('#automation-form [name=name]') && ${testerDone} && new URLSearchParams(location.search).get('automation')`);
    v = await view();
    check(!!newID && v.title === 'New automation', '4b: + Add automation opens an empty New automation editor at its own ?automation=', v);
    await typeInto('#automation-form [name=name]', 'Night owl');
    await click(RAIL, 'sidebar Named Automations from a new automation');
    v = await expectList('4b: sidebar from an unsaved new automation');
    check(v.rows.some(r => r.draft && r.name === 'Night owl' && r.badges.includes('Unsaved draft')), '4b: the unsaved new automation shows as an Unsaved draft row', v.rows);
    await click(`.draft-row ${EDIT(newID)}`, 'resume the Night owl draft');
    await editor(newID, 'resume the Night owl draft');
    check((await view()).name === 'Night owl', '4b: Resume editing restores the new automation’s typed name', await view());
    await click(SAVE, 'Save Night owl');
    v = await expectList('4b: Save of a new automation');
    const owl = (await config()).automations.find(a => a.name === 'Night owl');
    check(owl?.id === newID && !v.rows.some(r => r.draft), '4b: Night owl is saved under the ID its editor opened with and no draft row remains', { owl: owl?.id, newID });

    // Duplicate: same flow.
    await click(`[data-action="duplicate-automation"][data-id="${newID}"]`, 'Duplicate Night owl');
    const dupID = await until(`!!document.querySelector('#automation-form [name=name]') && ${testerDone} && new URLSearchParams(location.search).get('automation')`);
    v = await view();
    check(dupID && dupID !== newID && v.name === 'Night owl copy', '4b: Duplicate opens “Night owl copy” under a new ID', { dupID, newID, name: v.name });
    await click(RAIL, 'sidebar Named Automations from a duplicate');
    v = await expectList('4b: sidebar from an unsaved duplicate');
    check(v.rows.some(r => r.draft && r.name === 'Night owl copy'), '4b: the unsaved duplicate shows as a draft row', v.rows);
    await click(`.draft-row ${EDIT(dupID)}`, 'resume the duplicate');
    await editor(dupID, 'resume the duplicate');
    await click(SAVE, 'Save the duplicate');
    await expectList('4b: Save of a duplicate');
    const cfgDup = await config();
    check(cfgDup.automations.filter(a => a.name.startsWith('Night owl')).map(a => a.id).sort().join() === [newID, dupID].sort().join(),
      '4b: the duplicate is saved as a second automation and the original is unchanged', cfgDup.automations.map(a => [a.id, a.name]));

    // Back/Forward and reload.
    await click(EDIT('automation-yellow-go'), 'open yellow go');
    await editor('automation-yellow-go', 'open yellow go');
    await historyGo(-1);
    await expectList('4b: Back from the yellow go editor');
    await historyGo(1);
    check(await until(editorOpen('automation-yellow-go')), '4b: Forward reopens the yellow go editor', await view());
    await typeInto('#automation-form [name=name]', 'yellow go unsaved');
    await click(RAIL, 'sidebar Named Automations after Back/Forward');
    v = await expectList('4b: sidebar after Back/Forward');
    check(row(v, 'automation-yellow-go')?.badges.includes('Unsaved changes'), '4b: the list marks yellow go Unsaved changes', row(v, 'automation-yellow-go'));
    await historyGo(-1);
    await editor('automation-yellow-go', 'Back');
    check((await view()).name === 'yellow go unsaved', '4b: Back to the editor keeps the unsaved edit', await view());
    await reload();
    await editor('automation-yellow-go', 'reload');
    check((await view()).name === 'yellow go unsaved', '4b: reload keeps the unsaved edit', await view());
    await historyGo(1);
    v = await expectList('4b: Forward to the list after reload');
    await click(`[data-action="discard-draft"][data-id="automation-yellow-go"]`, 'Discard changes to yellow go');
    await until(`!document.querySelector('[data-action="discard-draft"]')`);
    v = await view();
    check(!row(v, 'automation-yellow-go')?.badges.includes('Unsaved changes') && (await config()).automations.find(a => a.id === 'automation-yellow-go').name === 'yellow go',
      '4b: Discard changes clears the draft and keeps the saved name', row(v, 'automation-yellow-go'));
  });

  // ---- 2b: a saved rename reaches the list, the project picker and attached
  // rows after reload; an unsaved rename shows as a draft everywhere.
  await section('2b saved and unsaved names', async () => {
    await goto('/settings?section=automations');
    await until(`!!document.querySelector('${EDIT('automation-yellow-go')}')`);
    await click(EDIT('automation-yellow-go'), 'open yellow go');
    await editor('automation-yellow-go', 'open yellow go');
    await typeInto('#automation-form [name=name]', 'yellow go (Claude)');
    await click(SAVE, 'Save the rename');
    check(await until(listOpen), '2b: Save of the rename returns to the list', await view());
    await reload();
    await until(listOpen);
    const v = await view();
    check(row(v, 'automation-yellow-go')?.name === 'yellow go (Claude)', '2b: after reload the list shows the saved name “yellow go (Claude)”', v.rows.map(r => r.name));
    const projectTab = id => `/project?${new URLSearchParams({ project: projectPaths[id], tab: 'automations' })}`;
    await goto(projectTab('project-filemaster'));
    const attached = await until(`document.querySelector('#project-body .row strong') && [...document.querySelectorAll('#project-body .row strong')].map(s => s.textContent)`);
    check(Array.isArray(attached) && attached.includes('yellow go (Claude)') && !attached.includes('yellow go'), '2b: Filemaster’s attached row shows “yellow go (Claude)”', attached);
    await goto(projectTab('project-symmail'));
    const picker = await until(`document.querySelector('[name="attach-definition"]') && [...document.querySelectorAll('[name="attach-definition"] option')].map(o => o.textContent)`);
    check(Array.isArray(picker) && picker.includes('yellow go (Claude)') && !picker.includes('yellow go'), '2b: Symmail’s attachment picker offers “yellow go (Claude)”', picker);

    await goto('/settings?section=automations');
    await until(`!!document.querySelector('${EDIT('automation-green')}')`);
    await click(EDIT('automation-green'), 'open Green and go');
    await editor('automation-green', 'open Green and go');
    await typeInto('#automation-form [name=name]', 'Green and go (draft)');
    // Pressing Save moves focus to it, the name field's change re-runs the
    // band tester, and Save and Cancel must not move while it answers.
    const buttons = () => evaluate(`return ['${SAVE}', '[data-action="cancel-automation"]'].map(s => Math.round(document.querySelector(s).getBoundingClientRect().top))`);
    await evaluate(`document.querySelector('${SAVE}').scrollIntoView({ block: 'center' })`);
    const still = [await buttons()];
    await evaluate(`document.querySelector('${SAVE}').focus()`);
    still.push(await buttons());
    await until(testerDone);
    still.push(await buttons());
    check(still.every(b => b.join() === still[0].join()), '2b: Save and Cancel stay put while the tester re-evaluates after focus leaves the name field', { before: still[0], focused: still[1], answered: still[2] });
    const ed = await view();
    check(ed.title === 'Edit automation · Green and go' && /Unsaved changes/.test(ed.note) && ed.note.includes('Draft name “Green and go (draft)”'),
      '2b: the editor keeps the saved name in its heading and notes the unsaved draft name', { title: ed.title, note: ed.note });
    await click(RAIL, 'sidebar Named Automations from the unsaved rename');
    const list = await expectList('2b: sidebar from an unsaved rename');
    const g = row(list, 'automation-green');
    check(g?.name === 'Green and go' && g.badges.includes('Unsaved changes') && g.hints.some(h => h.includes('Draft name: “Green and go (draft)”')),
      '2b: the list shows the saved name, an Unsaved changes badge and the draft name', g);
    await goto(projectTab('project-symmail'));
    const sy = await until(`document.querySelector('#project-body .row strong') && [...document.querySelectorAll('#project-body .row')].map(r => r.querySelector('strong')?.textContent + ' | ' + [...r.querySelectorAll('.badge')].map(b => b.textContent).join(','))`);
    check(Array.isArray(sy) && sy.includes('Green and go | Unsaved changes'), '2b: Symmail’s attached row keeps the saved name and shows Unsaved changes', sy);
    await goto(projectTab('project-filemaster'));
    const fp = await until(`document.querySelector('[name="attach-definition"]') && [...document.querySelectorAll('[name="attach-definition"] option')].map(o => o.textContent)`);
    check(Array.isArray(fp) && fp.includes('Green and go') && !fp.some(o => o.includes('(draft)')), '2b: Filemaster’s picker offers the saved name, not the draft', fp);
    check((await config()).automations.find(a => a.id === 'automation-green').name === 'Green and go', '2b: the unsaved rename writes nothing');
  });

  // ---- 3b: keyboard attach and detach on Project Associations persist after
  // reload; a duplicate attach writes nothing.
  await section('3b associations', async () => {
    const card = '.assoc-card[data-target="automation-run-yellow"]';
    const pairs = async () => (await config()).automation_attachments.map(l => `${l.definition_id}>${l.project_id}:${l.enabled ? 'on' : 'off'}:${l.priority}`).sort();
    const cardLinks = target => evaluate(`return [...document.querySelectorAll('.assoc-card[data-target="${target}"] .assoc-link')].map(l => l.querySelector('strong').textContent + (l.classList.contains('is-disabled') ? ' (disabled)' : ' (enabled)'))`);
    const feedback = () => evaluate(`return document.querySelector('#assoc-feedback')?.textContent || ''`);
    const before = await pairs();

    await goto('/settings?section=associations');
    await until(`!!document.querySelector('${card} select')`);
    await evaluate(`document.querySelector('${card} select').focus()`);
    await key('S', 'KeyS', 83, 'S');
    const chosen = await evaluate(`return document.querySelector('${card} select').value`);
    check(chosen === 'project-symmail', '3b: typing S in Run yellow (Codex)’s Attach a project picker chooses Symmail', chosen);
    if (chosen !== 'project-symmail') throw Error('the keyboard could not choose Symmail');
    await key('Tab', 'Tab', 9);
    const onAttach = await evaluate(`return document.activeElement?.matches('${card} [data-action="attach-association"]')`);
    check(onAttach, '3b: Tab moves from the picker to its Attach button', await evaluate(`return document.activeElement?.outerHTML.slice(0, 120)`));
    await key('Enter', 'Enter', 13, '\r');
    const said = await until(`/^Attached Symmail to Run yellow \\(Codex\\)/.test(document.querySelector('#assoc-feedback')?.textContent || '') && document.querySelector('#assoc-feedback').textContent`);
    check(!!said, '3b: Enter on Attach says Symmail is attached to Run yellow (Codex)', await feedback());
    let now = await pairs();
    check(JSON.stringify(now) === JSON.stringify([...before, 'automation-run-yellow>project-symmail:off:10'].sort()), '3b: the keyboard attach stores one disabled Symmail association and keeps the rest', now);

    await reload();
    await until(`!!document.querySelector('${card} .assoc-link')`);
    check((await cardLinks('automation-run-yellow')).includes('Symmail (disabled)'), '3b: after reload Run yellow (Codex) lists Symmail', await cardLinks('automation-run-yellow'));
    await goto('/settings?section=associations&by=projects');
    await until(`!!document.querySelector('.assoc-card[data-target="project-symmail"]')`);
    check((await cardLinks('project-symmail')).includes('Run yellow (Codex) (disabled)'), '3b: after reload Symmail lists Run yellow (Codex) by project', await cardLinks('project-symmail'));

    // Duplicate: drop Symmail on Run yellow (Codex) again.
    await goto('/settings?section=associations');
    await until(`!!document.querySelector('${card} .assoc-link[data-project="project-symmail"]')`);
    const rev = (await config()).revision, store = storeDigest();
    await drag('.assoc-chip[data-project="project-symmail"]', card, 'drop Symmail on Run yellow (Codex) again');
    const dupSaid = await until(`/already uses/.test(document.querySelector('#assoc-feedback')?.textContent || '') && document.querySelector('#assoc-feedback').textContent`);
    check(dupSaid === 'Symmail already uses Run yellow (Codex). Nothing changed.', '3b: a duplicate attach says nothing changed', await feedback());
    const afterDup = await config(), storeAfter = storeDigest();
    const changed = [...new Set([...Object.keys(store), ...Object.keys(storeAfter)])].filter(p => store[p] !== storeAfter[p]);
    check(afterDup.revision === rev && changed.length === 0 && JSON.stringify(await pairs()) === JSON.stringify(now),
      '3b: a duplicate attach writes nothing: same revision, same associations, Conductor store byte-identical', { rev, after: afterDup.revision, changed });

    // Keyboard detach.
    await evaluate(`document.querySelector('${card} .assoc-link[data-project="project-symmail"] [data-action="detach-association"]').focus()`);
    await key('Enter', 'Enter', 13, '\r');
    const detached = await until(`/^Detached Symmail from Run yellow \\(Codex\\)/.test(document.querySelector('#assoc-feedback')?.textContent || '')`);
    check(detached, '3b: Enter on Detach says Symmail is detached from Run yellow (Codex)', await feedback());
    now = await pairs();
    check(JSON.stringify(now) === JSON.stringify(before), '3b: the keyboard detach removes only the Symmail association', now);
    await reload();
    await until(`!!document.querySelector('${card} .assoc-link')`);
    const links = await cardLinks('automation-run-yellow');
    check(!links.some(l => l.startsWith('Symmail')) && links.includes('izcrOS (disabled)'), '3b: after reload Run yellow (Codex) no longer lists Symmail and keeps izcrOS', links);

    // The same Attach, Enabled and Detach controls with held pointer presses.
    const sw = `${card} .assoc-link[data-project="project-symmail"] input[role=switch]`;
    await evaluate(`document.querySelector('${card} select').focus()`);
    await key('S', 'KeyS', 83, 'S');
    await click(`${card} [data-action="attach-association"]`, 'Attach with the pointer');
    check(await until(`/^Attached Symmail to Run yellow/.test(document.querySelector('#assoc-feedback')?.textContent || '')`), '3b: a pointer press on Attach attaches Symmail', await feedback());
    await until(`!!document.querySelector('${sw}')`);
    await click(sw, 'Enabled switch with the pointer');
    check(await until(`/^Enabled Symmail with Run yellow/.test(document.querySelector('#assoc-feedback')?.textContent || '')`), '3b: a pointer press on Enabled enables the association', await feedback());
    now = await pairs();
    check(JSON.stringify(now) === JSON.stringify([...before, 'automation-run-yellow>project-symmail:on:10'].sort()), '3b: the pointer attach and enable store one enabled Symmail association', now);
    await reload();
    await until(`!!document.querySelector('${sw}')`);
    check((await cardLinks('automation-run-yellow')).includes('Symmail (enabled)'), '3b: after reload Run yellow (Codex) lists Symmail enabled', await cardLinks('automation-run-yellow'));
    await click(`${card} .assoc-link[data-project="project-symmail"] [data-action="detach-association"]`, 'Detach with the pointer');
    check(await until(`/^Detached Symmail from Run yellow/.test(document.querySelector('#assoc-feedback')?.textContent || '')`), '3b: a pointer press on Detach detaches Symmail', await feedback());
    check(JSON.stringify(await pairs()) === JSON.stringify(before), '3b: the pointer detach leaves the seeded associations', await pairs());
  });

  // ---- F62: a press held across the 3-second poll, after another writer
  // changed the configuration, still clicks the element it started on.
  await section('F62 background refresh during a press', async () => {
    await goto('/settings?section=automations');
    await until(`!!document.querySelector('${EDIT('automation-green')}')`);
    const c = await config();
    (c.projects.find(p => p.id === 'project-izcros').tags ||= []).push('other-writer');
    await postConfig(c);
    let polled = false;
    await click(EDIT('automation-green'), 'Edit Green and go held across a background refresh', async () => { polled = await nextPoll(); });
    check(polled, 'F62: the dashboard polled the changed configuration while the press was held');
    check(await until(editorOpen('automation-green')), 'F62: a press held across a background refresh still opens the Green and go editor', await view());

    // Space activates a button on release, so a held Space is a press too.
    await goto('/settings?section=automations');
    await until(`!!document.querySelector('${EDIT('automation-run-yellow')}')`);
    const d = await config();
    (d.projects.find(p => p.id === 'project-izcros').tags ||= []).push('other-writer-2');
    await postConfig(d);
    await evaluate(`document.querySelector('${EDIT('automation-run-yellow')}').focus()`);
    await send('Input.dispatchKeyEvent', { type: 'keyDown', key: ' ', code: 'Space', windowsVirtualKeyCode: 32, text: ' ' });
    polled = await nextPoll();
    await send('Input.dispatchKeyEvent', { type: 'keyUp', key: ' ', code: 'Space', windowsVirtualKeyCode: 32 });
    check(polled, 'F62: the dashboard polled the changed configuration while Space was held');
    check(await until(editorOpen('automation-run-yellow')), 'F62: Space held across a background refresh still opens the Run yellow (Codex) editor', await view());
  });

  // ---- F63: on a project's Automations tab, a typed priority saves when the
  // next press moves focus, and that press still lands.
  await section('F63 project Automations tab', async () => {
    const tab = `/project?${new URLSearchParams({ project: projectPaths['project-filemaster'], tab: 'automations' })}`;
    const prio = '[data-action="attachment-priority"][data-id="attachment-fm-yellow-go"]';
    const attachment = async () => (await config()).automation_attachments.find(a => a.id === 'attachment-fm-yellow-go');
    await goto(tab);
    await until(`!!document.querySelector('${prio}')`);
    await typeInto(prio, '3');
    await click(`#project-body ${EDIT('automation-yellow-go')}`, 'Edit shared rules pressed straight after typing a priority');
    check(await until(editorOpen('automation-yellow-go')), 'F63: Edit shared rules pressed straight after typing a priority opens the editor', await view());
    check((await attachment())?.priority === 3, 'F63: the typed priority 3 is saved', await attachment());

    await goto(tab);
    await until(`!!document.querySelector('${prio}')`);
    await typeInto(prio, '4');
    await click('[data-action="detach-automation"][data-id="attachment-fm-yellow-go"]', 'Remove from project pressed straight after typing a priority');
    let gone = false;
    for (let i = 0; i < 80 && !(gone = !(await attachment())); i++) await wait(100);
    check(gone, 'F63: Remove from project pressed straight after typing a priority removes the attachment', await attachment());
    check(await until(`/No automations attached/.test(document.querySelector('#project-body')?.textContent || '')`), 'F63: Filemaster’s tab shows no attached automations', await evaluate(`return document.querySelector('#project-body')?.textContent.slice(0, 200)`));
  });

  check(consoleErrors.length === 0, 'no console errors or uncaught exceptions', consoleErrors);
} catch (e) {
  failures.push(`the browser run stopped: ${e.stack || e}`);
}

ws.close();
await stop();
if (failures.length) {
  console.error(`FAIL: ${failures.length} of ${checks} automation navigation checks failed`);
  for (const f of failures) console.error(`  - ${f}`);
  process.exit(1);
}
console.log(`ok: ${checks} automation navigation checks passed`);
process.exit(0);
