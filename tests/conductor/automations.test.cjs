// TESTMASTER: id=conductor.automations-ui tier=fast parallel=yes
// Named Automations UI: saved identity and labels, drafts, the band tester request and
// summary, and Project Associations writes (disabled by default, idempotent, rebased).
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

const elements = new Map();
const element = () => ({ innerHTML: '', textContent: '', className: '', value: '', dataset: {}, style: {}, disabled: false, focused: false, focus() { this.focused = true; }, classList: { add() {}, remove() {} }, querySelector: () => null, querySelectorAll: () => [], addEventListener() {}, closest: () => null });
const node = s => { if (!elements.has(s)) elements.set(s, element()); return elements.get(s); };
const storage = new Map();
const sessionStorage = { getItem: k => storage.get(k) ?? null, setItem: (k, v) => storage.set(k, String(v)), removeItem: k => storage.delete(k) };

const seed = () => ({
  revision: 7, enabled: false, paused: false, power_bands: [], time_windows: [],
  projects: [{ id: 'p-a', name: 'Alpha', path: '/a' }, { id: 'p-b', name: 'Beta', path: '/b' }],
  subscriptions: [{ id: 'codex', name: 'Codex', sequences: [{ id: 'codex-build', name: 'build' }] }],
  automations: [
    { id: 'automation-run-yellow', name: 'Run yellow (Codex)', mode: 'keep_working', entries: [{ id: 'e1', sequence_id: 'codex-build', max_band: 'yellow' }], participants: [] },
    { id: 'automation-yellow-go', name: 'yellow go', mode: 'keep_working', entries: [{ id: 'e2', sequence_id: 'codex-build', max_band: 'green' }], participants: [] },
    { id: 'automation-1234abcd', name: 'yellow go', mode: 'keep_working', entries: [], participants: [] },
  ],
  automation_attachments: [{ id: 'att-keep', definition_id: 'automation-yellow-go', project_id: 'p-a', enabled: true, priority: 3 }],
});
let server = seed();
const posts = [];
let failNext = '';
const reply = (data, ok = true, status = 200) => Promise.resolve({ ok: ok && status < 400, status, statusText: 'x', json: async () => (ok ? { ok: true, data } : { ok: false, error: { message: data } }) });
const fetch = (path, opts = {}) => {
  const name = path.replace('/api/conductor/', '');
  const body = opts.body ? JSON.parse(opts.body) : undefined;
  if (body) posts.push({ name, body });
  if (name === 'config' && body) {
    if (failNext) { const m = failNext; failNext = ''; return reply(m, false, 500); }
    if (body.expected_revision !== server.revision) return reply('configuration revision conflict', false, 409);
    server = { ...structuredClone(body.config), revision: server.revision + 1 };
    return reply(structuredClone(server));
  }
  if (name === 'config') return reply(structuredClone(server));
  if (name === 'automation-preview') return reply({ band: body.band, maximum: 'yellow', outcome: 'launch', reason: 'Row 1 · Codex · build launches: Green + Yellow permits Yellow', rows: [], bands: [] });
  return reply(name === 'status' ? { state: 'watching' } : name === 'usage' ? {} : []);
};
const sandbox = {
  AbortController, structuredClone, URLSearchParams, URL, Intl, console, fetch, sessionStorage, localStorage: sessionStorage,
  crypto: require('node:crypto').webcrypto, CSS: { escape: s => String(s) },
  location: { search: '?section=associations', pathname: '/settings' },
  history: { state: null, pushState() {}, replaceState() {}, back() {} },
  scrollY: 0, scrollTo() {}, requestAnimationFrame: f => f(), addEventListener() {}, setInterval() {}, setTimeout() {}, clearTimeout() {},
  document: { querySelector: node, querySelectorAll: () => [], addEventListener() {}, activeElement: null, body: { classList: { add() {}, remove() {} } } },
};
let source = fs.readFileSync('src/internal/iterrun/conductor_web/app.js', 'utf8');
source = source.replace('refresh();setInterval(', 'setInterval(');
source = source.replace(/\}\)\(\);\s*$/, `globalThis.subject={automationLabel,namedAutomationsView,associationsView,bandSummary,makeDraft,draftDirty,evaluateAutomation,attachAssociation,detachAssociation,automationsView,automationSummary,draftNote,state:()=>({cfg,drafts,assocFeedback}),setCfg(c){cfg=c},setDraft(d){draft=d},setDrafts(d){drafts=d}};})();`);
vm.runInNewContext(source, sandbox);
const f = sandbox.subject;
const plain = v => JSON.parse(JSON.stringify(v));

(async () => {
  f.setCfg(structuredClone(server));
  // Saved names are the labels; equal names gain distinct stable-ID suffixes.
  assert.equal(f.automationLabel(server.automations[0]), 'Run yellow (Codex)');
  const a = f.automationLabel(server.automations[1]), b = f.automationLabel(server.automations[2]);
  assert.notEqual(a, b);
  assert.match(a, /^yellow go · #yellow/);
  assert.match(b, /^yellow go · #1234ab/);
  assert.match(f.automationSummary(server.automations[0]), /Codex · build through Green \+ Yellow/);

  // Drafts: new drafts and unsaved renames are explicit; untouched drafts are not.
  const clean = f.makeDraft('automation-run-yellow');
  assert.equal(f.draftDirty(clean), false);
  const renamed = f.makeDraft('automation-yellow-go');
  renamed.item.name = 'yellow go renamed';
  assert.equal(f.draftDirty(renamed), true);
  const fresh = f.makeDraft('');
  fresh.item.name = 'Night owl';
  f.setDrafts({ [clean.id]: clean, [renamed.id]: renamed, [fresh.id]: fresh });
  let html = f.namedAutomationsView();
  assert.match(html, /Night owl<\/strong> <span class="badge warn">Unsaved draft/);
  assert.match(html, /Draft name: “yellow go renamed”/);
  assert.equal((html.match(/Unsaved changes/g) || []).length, 1, 'only the edited saved automation is marked');
  assert.match(html, /data-action="discard-draft" class="link danger" data-id="automation-yellow-go"/);
  html = f.automationsView(server.projects[0]);
  assert.match(html, /yellow go · #yellow<\/strong>/, 'attached rows use the saved label');
  assert.match(html, /Unsaved changes/);
  assert.match(html, /<option value="automation-1234abcd" >yellow go · #1234ab<\/option>/, 'picker values are stable IDs');
  f.setDraft(renamed);
  assert.match(f.draftNote(), /saved name “yellow go”/);
  f.setDrafts({});

  // Tester: one request with the draft rules and sample band; no project or live config.
  const draft = f.makeDraft('automation-run-yellow');
  draft.sample.band = 'yellow';
  f.setDraft(draft);
  posts.length = 0;
  await f.evaluateAutomation();
  const req = posts.find(p => p.name === 'automation-preview').body;
  assert.equal(req.band, 'yellow');
  assert.equal(req.automation.id, 'automation-run-yellow');
  for (const key of ['project_id', 'draft', 'project_busy', 'queue_available']) assert.equal(key in req, false, key + ' is not a tester input');
  f.setDraft(null);
  const summary = f.bandSummary({ band: 'yellow', maximum: 'yellow', bands: [{ band: 'green', selected: 0, label: 'Codex · build' }, { band: 'yellow', selected: 0, label: 'Codex · build' }, { band: 'red', selected: -1, reason: 'Waits: no row permits Red; the highest row permits Green + Yellow' }], rows: [{ label: 'Codex · build', max_band: 'yellow', reason: 'Green + Yellow permits Yellow' }] });
  assert.equal((summary.match(/class="winner/g) || []).length, 1, 'only the sample band row is highlighted');
  assert.match(summary, /<tr class="winner current" aria-current="true">/);
  assert.match(summary, /Waits:<\/strong> no row permits Red/);
  assert.match(summary, /Rows permit up to Green \+ Yellow/);

  // Associations screen: drop targets, keyboard/touch alternatives, enablement text.
  html = f.associationsView();
  assert.equal((html.match(/data-drop="attach"/g) || []).length, 3);
  assert.match(html, /class="panel assoc-palette" data-drop="detach"/);
  assert.equal((html.match(/data-action="attach-association"/g) || []).length, 3);
  assert.match(html, /data-action="detach-association" class="link danger" data-id="att-keep"[^>]*aria-label="Detach Alpha with yellow go · #yellow"/);
  assert.match(html, /draggable="true" data-drag="source" data-project="p-b"/);
  assert.match(html, /Enabled · priority 3/);

  // Attach: disabled by default, idempotent, unrelated associations untouched.
  posts.length = 0;
  await f.attachAssociation('automation-run-yellow', 'p-b');
  let saved = server.automation_attachments;
  assert.equal(saved.length, 2);
  const added = saved.find(l => l.definition_id === 'automation-run-yellow' && l.project_id === 'p-b');
  assert.equal(added.enabled, false);
  assert.equal(added.priority, 10);
  assert.deepEqual(plain(saved.find(l => l.id === 'att-keep')), { id: 'att-keep', definition_id: 'automation-yellow-go', project_id: 'p-a', enabled: true, priority: 3 });
  assert.match(f.state().assocFeedback.text, /Attached Beta to Run yellow \(Codex\)\. It starts disabled/);
  posts.length = 0;
  await f.attachAssociation('automation-run-yellow', 'p-b');
  assert.equal(posts.filter(p => p.name === 'config').length, 0, 'duplicate drop writes nothing');
  assert.match(f.state().assocFeedback.text, /already uses .* Nothing changed/);
  assert.equal(server.automation_attachments.length, 2);

  // Conflict: another writer moved the revision; the attach rebases and keeps their change.
  server = { ...server, revision: server.revision + 1, projects: server.projects.map(p => p.id === 'p-a' ? { ...p, tags: ['concurrent'] } : p) };
  await f.attachAssociation('automation-yellow-go', 'p-b');
  assert.deepEqual(plain(server.projects[0].tags), ['concurrent']);
  assert.ok(server.automation_attachments.some(l => l.definition_id === 'automation-yellow-go' && l.project_id === 'p-b'));
  assert.equal(f.state().assocFeedback.error, false);

  // Failure: nothing is claimed; the screen shows the saved associations.
  const before = plain(server.automation_attachments);
  failNext = 'storage unavailable';
  await f.attachAssociation('automation-1234abcd', 'p-a');
  assert.equal(f.state().assocFeedback.error, true);
  assert.match(f.state().assocFeedback.text, /^Not saved: storage unavailable/);
  assert.deepEqual(plain(f.state().cfg.automation_attachments), before);

  // Detach removes only the chosen association.
  await f.detachAssociation(added.id);
  assert.equal(server.automation_attachments.some(l => l.id === added.id), false);
  assert.ok(server.automation_attachments.some(l => l.id === 'att-keep'));
  await f.detachAssociation(added.id);
  assert.match(f.state().assocFeedback.text, /already removed/);
  console.log('Automation labels, drafts, band tester request, and association writes pass.');
})().catch(e => { console.error(e); process.exitCode = 1; });
