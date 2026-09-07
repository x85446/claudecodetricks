(function () {
  'use strict';

  // ── State ──────────────────────────────────────────────────────────────
  let currentProduct = null;
  let treeData = null;
  let iterators = [];
  let iteratorMap = {};       // name → {description, values}
  let openEditId = null;
  let speechRecognition = null;
  let aiContextMode = 'proofreader';
  let saveMode = 'manual'; // 'manual' or 'autosave'
  let autosaveTimers = {};  // keyed by node id
  let feedbackLog = [];     // { timestamp, entity, level, mode, engine, feedback, prompt, response }

  // ── DOM refs ───────────────────────────────────────────────────────────
  const productSelect = document.getElementById('product-select');
  const glossaryBtn = document.getElementById('iterator-glossary-btn');
  const modalOverlay = document.getElementById('iterator-modal');
  const modalClose = document.getElementById('iterator-modal-close');
  const modalBody = document.getElementById('iterator-modal-body');
  const treeContainer = document.getElementById('tree-container');
  const loadingEl = document.getElementById('loading');
  const emptyState = document.getElementById('empty-state');

  // Toast container
  const toastContainer = document.createElement('div');
  toastContainer.className = 'toast-container';
  document.body.appendChild(toastContainer);

  // ── API helpers ────────────────────────────────────────────────────────
  async function api(method, path, body) {
    const opts = { method, headers: { 'Content-Type': 'application/json' } };
    if (body !== undefined) opts.body = JSON.stringify(body);
    const res = await fetch(path, opts);
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`${method} ${path} → ${res.status}: ${text}`);
    }
    if (res.status === 204) return null;
    return res.json();
  }

  function toast(message, type) {
    const el = document.createElement('div');
    el.className = 'toast ' + (type || '');
    el.textContent = message;
    toastContainer.appendChild(el);
    setTimeout(function () {
      el.style.opacity = '0';
      el.style.transition = 'opacity 0.3s';
      setTimeout(function () { el.remove(); }, 300);
    }, 3000);
  }

  // ── Product loading ────────────────────────────────────────────────────
  async function loadProducts() {
    try {
      const products = await api('GET', '/api/products');
      productSelect.innerHTML = '<option value="">Select a product...</option>';
      products.forEach(function (p) {
        const opt = document.createElement('option');
        opt.value = p.code || p.product_code || p.id;
        opt.textContent = p.name || p.code || p.product_code || p.id;
        productSelect.appendChild(opt);
      });
    } catch (err) {
      toast('Failed to load products: ' + err.message, 'error');
    }
  }

  productSelect.addEventListener('change', function () {
    const code = productSelect.value;
    if (!code) {
      currentProduct = null;
      treeData = null;
      treeContainer.classList.add('hidden');
      emptyState.classList.remove('hidden');
      return;
    }
    currentProduct = code;
    loadTree(code);
    loadIterators(code);
  });

  // ── Tree loading ───────────────────────────────────────────────────────
  async function loadTree(code) {
    emptyState.classList.add('hidden');
    treeContainer.classList.add('hidden');
    loadingEl.classList.remove('hidden');
    try {
      treeData = await api('GET', '/api/' + encodeURIComponent(code) + '/tree');
      renderTree();
      treeContainer.classList.remove('hidden');
    } catch (err) {
      toast('Failed to load tree: ' + err.message, 'error');
      emptyState.classList.remove('hidden');
    } finally {
      loadingEl.classList.add('hidden');
    }
  }

  async function loadIterators(code) {
    try {
      iterators = await api('GET', '/api/' + encodeURIComponent(code) + '/iterators');
      iteratorMap = {};
      iterators.forEach(function (it) {
        // API returns values as [{value: "x86", position: 1}, ...] — flatten to strings
        if (it.values && it.values.length && typeof it.values[0] === 'object') {
          it.values = it.values.map(function (v) { return v.value; });
        }
        iteratorMap[it.name] = it;
      });
    } catch (err) {
      toast('Failed to load iterators: ' + err.message, 'error');
    }
  }

  // ── Expand / Collapse All ─────────────────────────────────────────────
  function expandAll() {
    treeContainer.querySelectorAll('.node-body').forEach(function (body) {
      body.classList.remove('collapsed');
    });
    treeContainer.querySelectorAll('.node-toggle').forEach(function (t) {
      t.classList.add('expanded');
    });
  }

  function collapseAll() {
    treeContainer.querySelectorAll('.node-body').forEach(function (body) {
      body.classList.add('collapsed');
    });
    treeContainer.querySelectorAll('.node-toggle').forEach(function (t) {
      t.classList.remove('expanded');
    });
  }

  // Wire up expand/collapse buttons (added to top bar in index.html)
  var expandBtn = document.getElementById('expand-all');
  var collapseBtn = document.getElementById('collapse-all');
  if (expandBtn) expandBtn.addEventListener('click', expandAll);
  if (collapseBtn) collapseBtn.addEventListener('click', collapseAll);

  // ── Tree rendering ─────────────────────────────────────────────────────
  function renderTree() {
    treeContainer.innerHTML = '';
    if (!treeData) return;
    var epics = treeData.epics || treeData;
    if (!Array.isArray(epics)) return;
    epics.forEach(function (epic) {
      treeContainer.appendChild(buildNode(epic, 'epic'));
    });
  }

  function buildNode(node, level) {
    var childLevel = { epic: 'feature', feature: 'requirement', requirement: 'test', test: null };
    var childKey = { epic: 'features', feature: 'requirements', requirement: 'tests' };
    var children = childKey[level] ? (node[childKey[level]] || []) : [];
    var hasChildren = children.length > 0;

    var wrapper = document.createElement('div');
    wrapper.className = 'tree-node';
    wrapper.setAttribute('data-level', level);
    wrapper.setAttribute('data-id', node.id);
    if (node.stale) wrapper.classList.add('stale');

    // Header row (always visible — collapsed summary)
    var header = document.createElement('div');
    header.className = 'node-header';

    // Toggle arrow
    var toggle = document.createElement('span');
    toggle.className = 'node-toggle';
    toggle.textContent = '\u25B6';
    header.appendChild(toggle);

    // Type label
    var typeLabel = document.createElement('span');
    typeLabel.className = 'badge badge-type badge-type-' + level;
    typeLabel.textContent = level.charAt(0).toUpperCase() + level.slice(1);
    header.appendChild(typeLabel);

    // Display name (read-only title in header)
    var displayName = node.name || node.title || node.short_desc || '(untitled)';
    var nameEl = document.createElement('span');
    nameEl.className = 'node-name';
    nameEl.textContent = displayName;
    header.appendChild(nameEl);

    // Version badge
    if (node.version !== undefined && node.version !== null) {
      var vBadge = document.createElement('span');
      vBadge.className = 'badge badge-version';
      vBadge.textContent = 'v' + node.version;
      header.appendChild(vBadge);
    }

    // Status badge
    if (node.status) {
      var sBadge = document.createElement('span');
      sBadge.className = 'badge badge-status ' + node.status.toLowerCase();
      sBadge.textContent = node.status;
      header.appendChild(sBadge);
    }

    // Stale icon
    if (node.stale) {
      var staleEl = document.createElement('span');
      staleEl.className = 'stale-icon';
      staleEl.textContent = '\u26A0';
      var tip = document.createElement('span');
      tip.className = 'stale-tooltip';
      tip.textContent = buildStaleMessage(node);
      staleEl.appendChild(tip);
      header.appendChild(staleEl);
    }

    // Approve / disapprove button
    var approveBtn = document.createElement('button');
    updateApproveButton(approveBtn, node);
    approveBtn.addEventListener('click', function (e) {
      e.stopPropagation();
      toggleApproval(node, level, approveBtn, wrapper);
    });
    header.appendChild(approveBtn);

    // Bulk approve (epic and feature only)
    if (level === 'epic' || level === 'feature') {
      var bulkBtn = document.createElement('button');
      bulkBtn.className = 'btn btn-bulk';
      bulkBtn.textContent = 'Approve All';
      bulkBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        bulkApprove(node, level, wrapper);
      });
      header.appendChild(bulkBtn);
    }

    wrapper.appendChild(header);

    // ── Expandable body (hidden when collapsed) ──
    var body = document.createElement('div');
    body.className = 'node-body collapsed';

    // Editable fields
    var fields = getEditableFields(node, level);
    fields.forEach(function (f) {
      var group = document.createElement('div');
      group.className = 'field-group';
      var label = document.createElement('label');
      label.textContent = f.label;
      group.appendChild(label);

      if (f.multiline) {
        var ta = document.createElement('textarea');
        ta.setAttribute('data-field', f.key);
        ta.value = f.value || '';
        group.appendChild(ta);
      } else {
        var inp = document.createElement('input');
        inp.type = 'text';
        inp.setAttribute('data-field', f.key);
        inp.value = f.value || '';
        group.appendChild(inp);
      }
      body.appendChild(group);
    });

    // Save button (hidden in autosave mode)
    var saveBtn = document.createElement('button');
    saveBtn.className = 'btn btn-save';
    saveBtn.textContent = 'Save';
    if (saveMode === 'autosave') saveBtn.classList.add('hidden');
    saveBtn.addEventListener('click', function (e) {
      e.stopPropagation();
      saveNode(body, node, level, wrapper);
    });
    body.appendChild(saveBtn);

    // Autosave: debounced save on field change
    var fieldInputs = body.querySelectorAll(':scope > .field-group [data-field]');
    fieldInputs.forEach(function (inp) {
      inp.addEventListener('input', function () {
        if (saveMode !== 'autosave') return;
        clearTimeout(autosaveTimers[node.id]);
        autosaveTimers[node.id] = setTimeout(function () {
          saveNode(body, node, level, wrapper);
        }, 1500);
      });
    });

    // AI Feedback row
    var fbGroup = document.createElement('div');
    fbGroup.className = 'field-group feedback-section';
    var fbLabel = document.createElement('label');
    fbLabel.textContent = 'AI Feedback';
    fbGroup.appendChild(fbLabel);

    var fbRow = document.createElement('div');
    fbRow.className = 'feedback-input-row';
    var fbTextarea = document.createElement('textarea');
    fbTextarea.placeholder = 'Enter feedback for AI regeneration...';
    fbTextarea.setAttribute('data-feedback', 'true');
    fbRow.appendChild(fbTextarea);

    var micBtn = document.createElement('button');
    micBtn.className = 'btn-mic';
    micBtn.textContent = '\uD83C\uDF99';
    micBtn.title = 'Voice input (click to start/stop)';
    micBtn.addEventListener('click', function (e) {
      e.stopPropagation();
      toggleVoiceInput(micBtn, fbTextarea);
    });
    fbRow.appendChild(micBtn);
    fbGroup.appendChild(fbRow);

    // Context-level send buttons
    var contextRow = document.createElement('div');
    contextRow.className = 'feedback-context-row';
    var contextLabel = document.createElement('label');
    contextLabel.textContent = 'Send feedback with context:';
    contextRow.appendChild(contextLabel);

    var contextBtns = document.createElement('div');
    contextBtns.className = 'feedback-context-buttons';
    var contextLevels = getContextLevels(level);
    contextLevels.forEach(function (cl) {
      var btn = document.createElement('button');
      btn.className = 'btn btn-context-level' + (cl.agent ? ' btn-context-agent' : '');
      btn.textContent = cl.label;
      btn.title = cl.hint;
      btn.addEventListener('click', function (e) {
        e.stopPropagation();
        sendFeedback(node, level, fbTextarea.value, body, wrapper, cl.mode);
      });
      contextBtns.appendChild(btn);
    });
    contextRow.appendChild(contextBtns);
    fbGroup.appendChild(contextRow);
    body.appendChild(fbGroup);

    // Children inside the body
    if (hasChildren) {
      var childContainer = document.createElement('div');
      childContainer.className = 'node-children';
      children.forEach(function (child) {
        var childNode = buildNode(child, childLevel[level]);
        if (node.stale && !child.stale) {
          childNode.classList.add('cascade-stale');
        }
        childContainer.appendChild(childNode);
      });
      body.appendChild(childContainer);
    }

    wrapper.appendChild(body);

    // Toggle expand/collapse on header click (not on buttons)
    header.addEventListener('click', function (e) {
      if (e.target.closest('button')) return;
      var isCollapsed = body.classList.toggle('collapsed');
      toggle.classList.toggle('expanded', !isCollapsed);
      // When collapsing, also collapse all descendants
      if (isCollapsed) {
        body.querySelectorAll('.node-body').forEach(function (b) {
          b.classList.add('collapsed');
        });
        body.querySelectorAll('.node-toggle').forEach(function (t) {
          t.classList.remove('expanded');
        });
      }
    });

    return wrapper;
  }

  function buildStaleMessage(node) {
    if (node.stale_reason) return node.stale_reason;
    if (node.base_version !== undefined && node.base_version !== null) {
      var parentType = 'Parent';
      return 'Based on ' + parentType + ' v' + node.base_version + ', now updated';
    }
    return 'This item is stale and may need review';
  }

  function updateApproveButton(btn, node) {
    if (node.human_approved) {
      btn.className = 'btn btn-disapprove';
      btn.textContent = '\u2717';
      btn.title = 'Click to disapprove';
    } else {
      btn.className = 'btn btn-approve';
      btn.textContent = '\u2713';
      btn.title = 'Click to approve';
    }
  }

  // ── Approval actions ───────────────────────────────────────────────────
  var entityEndpoint = { epic: 'epics', feature: 'features', requirement: 'requirements', test: 'tests' };

  async function toggleApproval(node, level, btn, wrapper) {
    var endpoint = entityEndpoint[level];
    var action = node.human_approved ? 'disapprove' : 'approve';
    try {
      await api('POST', '/api/' + endpoint + '/' + node.id + '/' + action);
      node.human_approved = !node.human_approved;
      node.status = node.human_approved ? 'approved' : 'draft';
      updateApproveButton(btn, node);
      // Update status badge
      var sBadge = wrapper.querySelector(':scope > .node-header .badge-status');
      if (sBadge) {
        sBadge.className = 'badge badge-status ' + node.status.toLowerCase();
        sBadge.textContent = node.status;
      }
      toast((node.name || node.title) + ' ' + action + 'd', 'success');
    } catch (err) {
      toast('Failed to ' + action + ': ' + err.message, 'error');
    }
  }

  async function bulkApprove(node, level, wrapper) {
    var endpoint = entityEndpoint[level];
    try {
      await api('POST', '/api/' + endpoint + '/' + node.id + '/bulk-approve');
      markApprovedRecursive(node, level);
      refreshNodeVisuals(wrapper, node, level);
      toast('Bulk approved: ' + (node.name || node.title), 'success');
    } catch (err) {
      toast('Bulk approve failed: ' + err.message, 'error');
    }
  }

  function markApprovedRecursive(node, level) {
    node.human_approved = true;
    node.status = 'approved';
    var childKey = { epic: 'features', feature: 'requirements', requirement: 'tests' };
    var childLevel = { epic: 'feature', feature: 'requirement', requirement: 'test' };
    var children = childKey[level] ? (node[childKey[level]] || []) : [];
    children.forEach(function (child) {
      markApprovedRecursive(child, childLevel[level]);
    });
  }

  function refreshNodeVisuals(wrapper, node, level) {
    // Update this node's button and badge
    var btn = wrapper.querySelector(':scope > .node-header .btn-approve, :scope > .node-header .btn-disapprove');
    if (btn) updateApproveButton(btn, node);
    var sBadge = wrapper.querySelector(':scope > .node-header .badge-status');
    if (sBadge) {
      sBadge.className = 'badge badge-status approved';
      sBadge.textContent = 'approved';
    }
    // Recurse into children (now inside .node-body)
    var childContainer = wrapper.querySelector(':scope > .node-body > .node-children');
    if (!childContainer) return;
    var childKey = { epic: 'features', feature: 'requirements', requirement: 'tests' };
    var childLevel = { epic: 'feature', feature: 'requirement', requirement: 'test' };
    var children = childKey[level] ? (node[childKey[level]] || []) : [];
    var childWrappers = childContainer.querySelectorAll(':scope > .tree-node');
    children.forEach(function (child, i) {
      if (childWrappers[i]) {
        refreshNodeVisuals(childWrappers[i], child, childLevel[level]);
      }
    });
  }

  // toggleEditPanel removed — editing is now inline in the expandable node body

  function getContextLevels(level) {
    // Returns buttons from narrowest (fast API) to broadest, plus /PM (Agent SDK)
    if (level === 'test') return [
      { label: 'Test', mode: 'proofreader', hint: 'Just this test — fast rewrite' },
      { label: 'Tests', mode: 'informed', hint: '+ sibling tests' },
      { label: 'Requirement', mode: 'scoped', hint: '+ parent requirement' },
      { label: 'Requirements', mode: 'deep', hint: '+ sibling requirements & feature' },
      { label: 'Product', mode: 'product', hint: 'Full product brief + tree + iterators' },
      { label: '/PM', mode: 'pm', hint: 'Agent SDK — invoke PM skill', agent: true },
    ];
    if (level === 'requirement') return [
      { label: 'Requirement', mode: 'proofreader', hint: 'Just this requirement — fast rewrite' },
      { label: 'Requirements', mode: 'informed', hint: '+ sibling requirements' },
      { label: 'Feature', mode: 'scoped', hint: '+ parent feature' },
      { label: 'Features', mode: 'deep', hint: '+ sibling features & epic' },
      { label: 'Product', mode: 'product', hint: 'Full product brief + tree + iterators' },
      { label: '/PM', mode: 'pm', hint: 'Agent SDK — invoke PM skill', agent: true },
    ];
    if (level === 'feature') return [
      { label: 'Feature', mode: 'proofreader', hint: 'Just this feature — fast rewrite' },
      { label: 'Features', mode: 'informed', hint: '+ sibling features' },
      { label: 'Epic', mode: 'scoped', hint: '+ parent epic' },
      { label: 'Product', mode: 'product', hint: 'Full product brief + tree + iterators' },
      { label: '/PM', mode: 'pm', hint: 'Agent SDK — invoke PM skill', agent: true },
    ];
    // epic
    return [
      { label: 'Epic', mode: 'proofreader', hint: 'Just this epic — fast rewrite' },
      { label: 'Epics', mode: 'informed', hint: '+ sibling epics' },
      { label: 'Product', mode: 'product', hint: 'Full product brief + tree + iterators' },
      { label: '/PM', mode: 'pm', hint: 'Agent SDK — invoke PM skill', agent: true },
    ];
  }

  function getEditableFields(node, level) {
    var fields = [];
    if (level === 'epic') {
      fields.push({ key: 'name', label: 'Title', value: node.name || '', multiline: false });
      fields.push({ key: 'description', label: 'Description', value: node.description || '', multiline: true });
    } else if (level === 'feature') {
      fields.push({ key: 'short_desc', label: 'Title', value: node.short_desc || '', multiline: false });
      fields.push({ key: 'detailed_desc', label: 'Description', value: node.detailed_desc || '', multiline: true });
    } else if (level === 'requirement') {
      fields.push({ key: 'title', label: 'Title', value: node.title || '', multiline: false });
      fields.push({ key: 'description', label: 'Description', value: node.description || '', multiline: true });
      fields.push({ key: 'acceptance_criteria', label: 'Acceptance Criteria', value: node.acceptance_criteria || '', multiline: true });
    } else if (level === 'test') {
      fields.push({ key: 'title', label: 'Title', value: node.title || '', multiline: false });
      fields.push({ key: 'detailed_desc', label: 'Description', value: node.detailed_desc || '', multiline: true });
    }
    return fields;
  }

  async function saveNode(bodyEl, node, level, wrapper) {
    var endpoint = entityEndpoint[level];
    var payload = {};
    // Only select this node's direct fields, not descendant node fields
    var fields = bodyEl.querySelectorAll(':scope > .field-group [data-field]');
    fields.forEach(function (inp) {
      var key = inp.getAttribute('data-field');
      payload[key] = inp.value;
    });

    if (Object.keys(payload).length === 0) {
      toast('No changes to save', 'error');
      return;
    }

    try {
      var updated = await api('PATCH', '/api/' + endpoint + '/' + node.id, payload);
      // Merge updated fields back
      if (updated) {
        Object.keys(updated).forEach(function (k) { node[k] = updated[k]; });
      } else {
        Object.keys(payload).forEach(function (k) { node[k] = payload[k]; });
      }
      // Update header name
      var nameEl = wrapper.querySelector(':scope > .node-header .node-name');
      if (nameEl) nameEl.textContent = node.name || node.title || node.short_desc || '(untitled)';
      // Update version badge
      if (updated && updated.version !== undefined) {
        var vBadge = wrapper.querySelector(':scope > .node-header .badge-version');
        if (vBadge) vBadge.textContent = 'v' + updated.version;
      }
      toast('Saved successfully', 'success');
    } catch (err) {
      toast('Save failed: ' + err.message, 'error');
    }
  }

  async function sendFeedback(node, level, text, bodyEl, wrapper, mode) {
    if (!text || !text.trim()) {
      toast('Please enter feedback text', 'error');
      return;
    }
    if (!currentProduct) return;

    // Show loading state on all context buttons
    var ctxButtons = bodyEl.querySelectorAll('.btn-context-level');
    ctxButtons.forEach(function (b) { b.disabled = true; });
    var clickedBtn = Array.from(ctxButtons).find(function (b) { return !b.disabled; }) || ctxButtons[0];
    // Find which button was clicked based on mode
    ctxButtons.forEach(function (b) {
      if (b.textContent === (mode || 'proofreader')) b.textContent = 'Regenerating...';
    });

    try {
      var result = await api('POST', '/api/' + encodeURIComponent(currentProduct) + '/feedback', {
        entity_type: level,
        entity_id: node.id,
        feedback_text: text.trim(),
        context_mode: mode || 'proofreader'
      });

      // Log the debug info
      if (result.debug) {
        addLogEntry({
          entity: node.name || node.title || node.short_desc || node.id,
          level: level,
          mode: mode || 'proofreader',
          engine: result.debug.engine || 'unknown',
          feedback: text.trim(),
          prompt: result.debug.prompt || '',
          response: result.debug.response || ''
        });
      }

      if (result.regenerated && result.updated) {
        Object.keys(result.updated).forEach(function (k) { node[k] = result.updated[k]; });

        var fields = bodyEl.querySelectorAll(':scope > .field-group [data-field]');
        fields.forEach(function (inp) {
          var key = inp.getAttribute('data-field');
          if (node[key] !== undefined) {
            inp.value = node[key];
          }
        });

        var nameEl = wrapper.querySelector(':scope > .node-header .node-name');
        if (nameEl) nameEl.textContent = node.name || node.title || node.short_desc || '(untitled)';

        var vBadge = wrapper.querySelector(':scope > .node-header .badge-version');
        if (vBadge && node.version !== undefined) vBadge.textContent = 'v' + node.version;

        var fbTextarea = bodyEl.querySelector('[data-feedback]');
        if (fbTextarea) fbTextarea.value = '';

        toast('AI regenerated content', 'success');
      } else {
        toast('Feedback stored (no AI key configured)', 'success');
      }
    } catch (err) {
      toast('Feedback failed: ' + err.message, 'error');
    } finally {
      // Restore context buttons
      var levels = getContextLevels(level);
      ctxButtons.forEach(function (b, i) {
        b.textContent = levels[i] ? levels[i].label : b.textContent;
        b.disabled = false;
      });
    }
  }

  // ── Iterator highlighting ──────────────────────────────────────────────
  function highlightIterators(text) {
    if (!text) return '';
    var escaped = escapeHtml(text);
    Object.keys(iteratorMap).forEach(function (name) {
      var it = iteratorMap[name];
      var valuesStr = (it.values || []).join(', ');
      var tooltipText = escapeHtml(it.description || '') + (valuesStr ? ' | Values: ' + escapeHtml(valuesStr) : '');
      var pattern = new RegExp('\\b' + escapeRegex(name) + '\\b', 'gi');
      escaped = escaped.replace(pattern, function (match) {
        return '<span class="iterator-ref">' + match +
          '<span class="iterator-tooltip">' + tooltipText + '</span></span>';
      });
    });
    return escaped;
  }

  function escapeHtml(str) {
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  function escapeRegex(str) {
    return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  }

  // ── Voice input ────────────────────────────────────────────────────────
  function toggleVoiceInput(btn, textarea) {
    var SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SpeechRecognition) {
      toast('Speech recognition not supported in this browser', 'error');
      return;
    }

    if (speechRecognition) {
      speechRecognition.stop();
      speechRecognition = null;
      btn.classList.remove('recording');
      return;
    }

    speechRecognition = new SpeechRecognition();
    speechRecognition.continuous = true;
    speechRecognition.interimResults = true;
    speechRecognition.lang = 'en-US';

    var finalTranscript = textarea.value;

    speechRecognition.onresult = function (event) {
      var interim = '';
      for (var i = event.resultIndex; i < event.results.length; i++) {
        if (event.results[i].isFinal) {
          finalTranscript += event.results[i][0].transcript + ' ';
        } else {
          interim += event.results[i][0].transcript;
        }
      }
      textarea.value = finalTranscript + interim;
    };

    speechRecognition.onend = function () {
      speechRecognition = null;
      btn.classList.remove('recording');
      textarea.value = finalTranscript.trim();
    };

    speechRecognition.onerror = function (event) {
      toast('Speech error: ' + event.error, 'error');
      speechRecognition = null;
      btn.classList.remove('recording');
    };

    speechRecognition.start();
    btn.classList.add('recording');
  }

  // ── Iterator glossary modal ────────────────────────────────────────────
  glossaryBtn.addEventListener('click', function () {
    renderGlossary();
    modalOverlay.classList.remove('hidden');
  });

  modalClose.addEventListener('click', function () {
    modalOverlay.classList.add('hidden');
  });

  modalOverlay.addEventListener('click', function (e) {
    if (e.target === modalOverlay) modalOverlay.classList.add('hidden');
  });

  function renderGlossary() {
    modalBody.innerHTML = '';

    iterators.forEach(function (it) {
      var card = document.createElement('div');
      card.className = 'iterator-card';

      // Header with name and delete button
      var cardHeader = document.createElement('div');
      cardHeader.className = 'iterator-card-header';
      var h3 = document.createElement('h3');
      h3.textContent = it.name;
      cardHeader.appendChild(h3);
      var deleteBtn = document.createElement('button');
      deleteBtn.className = 'btn btn-delete-iterator';
      deleteBtn.textContent = 'Delete';
      deleteBtn.addEventListener('click', function () {
        deleteIterator(it, card);
      });
      cardHeader.appendChild(deleteBtn);
      card.appendChild(cardHeader);

      if (it.description) {
        var desc = document.createElement('div');
        desc.className = 'iterator-desc';
        desc.textContent = it.description;
        card.appendChild(desc);
      }

      var valuesContainer = document.createElement('div');
      valuesContainer.className = 'iterator-values';
      (it.values || []).forEach(function (val) {
        var tag = document.createElement('span');
        tag.className = 'iterator-value-tag';
        tag.textContent = val + ' ';
        var removeBtn = document.createElement('button');
        removeBtn.className = 'remove-value';
        removeBtn.textContent = '\u00D7';
        removeBtn.addEventListener('click', function () {
          removeIteratorValue(it, val, tag, valuesContainer);
        });
        tag.appendChild(removeBtn);
        valuesContainer.appendChild(tag);
      });
      card.appendChild(valuesContainer);

      var addRow = document.createElement('div');
      addRow.className = 'iterator-add-row';
      var addInput = document.createElement('input');
      addInput.type = 'text';
      addInput.placeholder = 'Add value...';
      addRow.appendChild(addInput);
      var addBtn = document.createElement('button');
      addBtn.className = 'btn';
      addBtn.textContent = 'Add';
      addBtn.addEventListener('click', function () {
        var val = addInput.value.trim();
        if (!val) return;
        addIteratorValue(it, val, addInput, valuesContainer);
      });
      addInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') addBtn.click();
      });
      addRow.appendChild(addBtn);
      card.appendChild(addRow);

      modalBody.appendChild(card);
    });

    // Add new iterator form
    var addCard = document.createElement('div');
    addCard.className = 'iterator-card iterator-add-card';
    var addTitle = document.createElement('h3');
    addTitle.textContent = 'Add Iterator';
    addCard.appendChild(addTitle);

    var nameRow = document.createElement('div');
    nameRow.className = 'iterator-add-row';
    var nameInput = document.createElement('input');
    nameInput.type = 'text';
    nameInput.placeholder = 'Iterator name...';
    nameRow.appendChild(nameInput);
    addCard.appendChild(nameRow);

    var descRow = document.createElement('div');
    descRow.className = 'iterator-add-row';
    var descInput = document.createElement('input');
    descInput.type = 'text';
    descInput.placeholder = 'Description (optional)...';
    descRow.appendChild(descInput);
    addCard.appendChild(descRow);

    var createBtn = document.createElement('button');
    createBtn.className = 'btn btn-save';
    createBtn.textContent = 'Create Iterator';
    createBtn.addEventListener('click', function () {
      var name = nameInput.value.trim();
      if (!name) { toast('Enter an iterator name', 'error'); return; }
      createIterator(name, descInput.value.trim());
    });
    nameInput.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') createBtn.click();
    });
    addCard.appendChild(createBtn);
    modalBody.appendChild(addCard);
  }

  async function deleteIterator(iterator, cardEl) {
    try {
      await api('DELETE', '/api/iterators/' + iterator.id);
      iterators = iterators.filter(function (it) { return it.id !== iterator.id; });
      delete iteratorMap[iterator.name];
      cardEl.remove();
      toast('Deleted iterator "' + iterator.name + '"', 'success');
    } catch (err) {
      toast('Delete failed: ' + err.message, 'error');
    }
  }

  async function createIterator(name, description) {
    if (!currentProduct) return;
    try {
      var created = await api('POST', '/api/' + encodeURIComponent(currentProduct) + '/iterators', {
        name: name, description: description
      });
      created.values = [];
      iterators.push(created);
      iteratorMap[created.name] = created;
      renderGlossary();
      toast('Created iterator "' + name + '"', 'success');
    } catch (err) {
      toast('Create failed: ' + err.message, 'error');
    }
  }

  async function removeIteratorValue(iterator, value, tagEl, container) {
    try {
      await api('DELETE', '/api/iterators/' + iterator.id + '/values/' + encodeURIComponent(value));
      iterator.values = iterator.values.filter(function (v) { return v !== value; });
      iteratorMap[iterator.name] = iterator;
      tagEl.remove();
      toast('Removed "' + value + '"', 'success');
    } catch (err) {
      toast('Remove failed: ' + err.message, 'error');
    }
  }

  async function addIteratorValue(iterator, value, input, container) {
    try {
      await api('POST', '/api/iterators/' + iterator.id + '/values', { value: value });
      if (!iterator.values) iterator.values = [];
      iterator.values.push(value);
      iteratorMap[iterator.name] = iterator;
      input.value = '';

      // Add tag to DOM
      var tag = document.createElement('span');
      tag.className = 'iterator-value-tag';
      tag.textContent = value + ' ';
      var removeBtn = document.createElement('button');
      removeBtn.className = 'remove-value';
      removeBtn.textContent = '\u00D7';
      removeBtn.addEventListener('click', function () {
        removeIteratorValue(iterator, value, tag, container);
      });
      tag.appendChild(removeBtn);
      container.appendChild(tag);
      toast('Added "' + value + '"', 'success');
    } catch (err) {
      toast('Add failed: ' + err.message, 'error');
    }
  }

  // ── Log panel ──────────────────────────────────────────────────────
  var logBtn = document.getElementById('log-btn');
  var logCount = document.getElementById('log-count');
  var logPanel = document.getElementById('log-panel');
  var logPanelClose = document.getElementById('log-panel-close');
  var logPanelBody = document.getElementById('log-panel-body');

  logBtn.addEventListener('click', function () {
    logPanel.classList.toggle('hidden');
  });

  logPanelClose.addEventListener('click', function () {
    logPanel.classList.add('hidden');
  });

  function addLogEntry(entry) {
    entry.timestamp = new Date();
    feedbackLog.unshift(entry);
    logCount.textContent = feedbackLog.length;
    logCount.classList.remove('hidden');
    renderLog();
  }

  function renderLog() {
    logPanelBody.innerHTML = '';
    if (feedbackLog.length === 0) {
      logPanelBody.innerHTML = '<p class="log-empty">No feedback sent yet.</p>';
      return;
    }
    feedbackLog.forEach(function (entry, idx) {
      var el = document.createElement('div');
      el.className = 'log-entry';

      var header = document.createElement('div');
      header.className = 'log-entry-header';

      var summary = document.createElement('div');
      summary.className = 'log-entry-summary';

      var entitySpan = document.createElement('span');
      entitySpan.className = 'log-entity';
      entitySpan.textContent = entry.entity;
      summary.appendChild(entitySpan);

      var modeSpan = document.createElement('span');
      modeSpan.className = 'log-mode';
      modeSpan.textContent = entry.mode;
      summary.appendChild(modeSpan);

      var engineSpan = document.createElement('span');
      engineSpan.className = 'log-engine ' + (entry.engine === 'agent-sdk' ? 'log-engine-agent' : 'log-engine-fast');
      engineSpan.textContent = entry.engine === 'agent-sdk' ? 'SDK' : 'API';
      summary.appendChild(engineSpan);

      header.appendChild(summary);

      var timeSpan = document.createElement('span');
      timeSpan.className = 'log-entry-time';
      timeSpan.textContent = entry.timestamp.toLocaleTimeString();
      header.appendChild(timeSpan);

      var toggle = document.createElement('span');
      toggle.className = 'log-entry-toggle';
      toggle.textContent = '\u25B6';
      header.appendChild(toggle);

      header.addEventListener('click', function () {
        el.classList.toggle('open');
      });

      el.appendChild(header);

      var body = document.createElement('div');
      body.className = 'log-entry-body';

      var fbLabel = document.createElement('div');
      fbLabel.className = 'log-section-label';
      fbLabel.textContent = 'Feedback';
      body.appendChild(fbLabel);
      var fbCode = document.createElement('div');
      fbCode.className = 'log-code';
      fbCode.textContent = entry.feedback;
      body.appendChild(fbCode);

      var promptLabel = document.createElement('div');
      promptLabel.className = 'log-section-label';
      promptLabel.textContent = 'Prompt Sent';
      body.appendChild(promptLabel);
      var promptCode = document.createElement('div');
      promptCode.className = 'log-code';
      promptCode.textContent = entry.prompt;
      body.appendChild(promptCode);

      var respLabel = document.createElement('div');
      respLabel.className = 'log-section-label';
      respLabel.textContent = 'Response';
      body.appendChild(respLabel);
      var respCode = document.createElement('div');
      respCode.className = 'log-code';
      respCode.textContent = entry.response;
      body.appendChild(respCode);

      el.appendChild(body);
      logPanelBody.appendChild(el);
    });
  }

  // ── Product Brief modal ──────────────────────────────────────────────
  var productBriefBtn = document.getElementById('product-brief-btn');
  var productBriefModal = document.getElementById('product-brief-modal');
  var productBriefClose = document.getElementById('product-brief-close');
  var productBriefTextarea = document.getElementById('product-brief-textarea');
  var productBriefSave = document.getElementById('product-brief-save');

  productBriefBtn.addEventListener('click', async function () {
    if (!currentProduct) { toast('Select a product first', 'error'); return; }
    try {
      var data = await api('GET', '/api/' + encodeURIComponent(currentProduct) + '/brief');
      productBriefTextarea.value = data.product_brief || '';
      productBriefModal.classList.remove('hidden');
    } catch (err) {
      toast('Failed to load brief: ' + err.message, 'error');
    }
  });

  productBriefClose.addEventListener('click', function () {
    productBriefModal.classList.add('hidden');
  });

  productBriefModal.addEventListener('click', function (e) {
    if (e.target === productBriefModal) productBriefModal.classList.add('hidden');
  });

  productBriefSave.addEventListener('click', async function () {
    if (!currentProduct) return;
    try {
      await api('PATCH', '/api/' + encodeURIComponent(currentProduct) + '/brief', {
        product_brief: productBriefTextarea.value
      });
      productBriefModal.classList.add('hidden');
      toast('Product brief saved', 'success');
    } catch (err) {
      toast('Save failed: ' + err.message, 'error');
    }
  });

  // ── Settings modal ───────────────────────────────────────────────────
  var settingsBtn = document.getElementById('settings-btn');
  var settingsModal = document.getElementById('settings-modal');
  var settingsClose = document.getElementById('settings-modal-close');

  settingsBtn.addEventListener('click', function () {
    settingsModal.classList.remove('hidden');
  });

  settingsClose.addEventListener('click', function () {
    settingsModal.classList.add('hidden');
  });

  settingsModal.addEventListener('click', function (e) {
    if (e.target === settingsModal) settingsModal.classList.add('hidden');
  });

  settingsModal.querySelectorAll('input[name="save-mode"]').forEach(function (radio) {
    radio.addEventListener('change', function () {
      saveMode = this.value;
      // Toggle save buttons visibility across the tree
      document.querySelectorAll('.node-body > .btn-save').forEach(function (btn) {
        if (saveMode === 'autosave') {
          btn.classList.add('hidden');
        } else {
          btn.classList.remove('hidden');
        }
      });
      settingsModal.classList.add('hidden');
      toast('Save mode: ' + (saveMode === 'autosave' ? 'Auto Save' : 'Save Buttons'), 'success');
    });
  });

  // ── Keyboard shortcuts ─────────────────────────────────────────────────
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') {
      // Close modals
      if (!productBriefModal.classList.contains('hidden')) {
        productBriefModal.classList.add('hidden');
        return;
      }
      if (!logPanel.classList.contains('hidden')) {
        logPanel.classList.add('hidden');
        return;
      }
      if (!settingsModal.classList.contains('hidden')) {
        settingsModal.classList.add('hidden');
        return;
      }
      if (!modalOverlay.classList.contains('hidden')) {
        modalOverlay.classList.add('hidden');
      }
    }
  });

  // ── Init ───────────────────────────────────────────────────────────────
  loadProducts();
})();
