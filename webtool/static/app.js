(function () {
  'use strict';

  // ── State ──────────────────────────────────────────────────────────────
  let currentProduct = null;
  let treeData = null;
  let iterators = [];
  let iteratorMap = {};       // name → {description, values}
  let openEditId = null;
  let speechRecognition = null;

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

    // Header row
    var header = document.createElement('div');
    header.className = 'node-header';

    // Toggle arrow
    var toggle = document.createElement('span');
    toggle.className = 'node-toggle' + (hasChildren ? '' : ' leaf');
    toggle.textContent = '\u25B6';
    header.appendChild(toggle);

    // Name
    var name = document.createElement('span');
    name.className = 'node-name';
    name.textContent = node.name || node.title || '(untitled)';
    name.addEventListener('click', function (e) {
      e.stopPropagation();
      toggleEditPanel(wrapper, node, level);
    });
    header.appendChild(name);

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

    // Children container
    if (hasChildren) {
      var childContainer = document.createElement('div');
      childContainer.className = 'node-children collapsed';
      children.forEach(function (child) {
        var childNode = buildNode(child, childLevel[level]);
        // Cascade stale
        if (node.stale && !child.stale) {
          childNode.classList.add('cascade-stale');
        }
        childContainer.appendChild(childNode);
      });
      wrapper.appendChild(childContainer);

      // Toggle expand/collapse on header click (but not on name or buttons)
      header.addEventListener('click', function (e) {
        if (e.target.closest('.node-name') || e.target.closest('button')) return;
        childContainer.classList.toggle('collapsed');
        toggle.classList.toggle('expanded');
      });
    }

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
    // Recurse into children
    var childContainer = wrapper.querySelector(':scope > .node-children');
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

  // ── Inline edit panel ──────────────────────────────────────────────────
  function toggleEditPanel(wrapper, node, level) {
    var existing = wrapper.querySelector(':scope > .edit-panel');
    if (existing) {
      existing.remove();
      openEditId = null;
      return;
    }
    // Close any other open panel
    var prev = document.querySelector('.edit-panel');
    if (prev) prev.remove();

    openEditId = node.id;
    var panel = document.createElement('div');
    panel.className = 'edit-panel';

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
        // Highlight iterators in description
        group.appendChild(ta);
      } else {
        var inp = document.createElement('input');
        inp.type = 'text';
        inp.setAttribute('data-field', f.key);
        inp.value = f.value || '';
        group.appendChild(inp);
      }
      panel.appendChild(group);
    });

    // If description field exists, show a preview with iterator highlights
    var descField = node.description || '';
    if (descField && Object.keys(iteratorMap).length > 0) {
      var preview = document.createElement('div');
      preview.className = 'field-group';
      var previewLabel = document.createElement('label');
      previewLabel.textContent = 'Description Preview';
      preview.appendChild(previewLabel);
      var previewContent = document.createElement('div');
      previewContent.style.fontSize = '13px';
      previewContent.style.lineHeight = '1.6';
      previewContent.innerHTML = highlightIterators(descField);
      preview.appendChild(previewContent);
      panel.appendChild(preview);
    }

    // Actions row
    var actions = document.createElement('div');
    actions.className = 'edit-actions';

    var saveBtn = document.createElement('button');
    saveBtn.className = 'btn btn-save';
    saveBtn.textContent = 'Save';
    saveBtn.addEventListener('click', function () {
      saveNode(panel, node, level, wrapper);
    });
    actions.appendChild(saveBtn);

    var cancelBtn = document.createElement('button');
    cancelBtn.className = 'btn btn-cancel';
    cancelBtn.textContent = 'Cancel';
    cancelBtn.addEventListener('click', function () {
      panel.remove();
      openEditId = null;
    });
    actions.appendChild(cancelBtn);

    panel.appendChild(actions);

    // Feedback section
    var feedbackSection = document.createElement('div');
    feedbackSection.className = 'feedback-section';

    var fbLabel = document.createElement('label');
    fbLabel.textContent = 'AI Feedback';
    feedbackSection.appendChild(fbLabel);

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
    micBtn.addEventListener('click', function () {
      toggleVoiceInput(micBtn, fbTextarea);
    });
    fbRow.appendChild(micBtn);

    feedbackSection.appendChild(fbRow);

    var fbActions = document.createElement('div');
    fbActions.className = 'feedback-actions';

    var sendFbBtn = document.createElement('button');
    sendFbBtn.className = 'btn btn-feedback';
    sendFbBtn.textContent = 'Send Feedback';
    sendFbBtn.addEventListener('click', function () {
      sendFeedback(node, level, fbTextarea.value);
    });
    fbActions.appendChild(sendFbBtn);

    feedbackSection.appendChild(fbActions);
    panel.appendChild(feedbackSection);

    // Insert after header
    var header = wrapper.querySelector(':scope > .node-header');
    header.insertAdjacentElement('afterend', panel);
  }

  function getEditableFields(node, level) {
    var fields = [];
    if (node.name !== undefined) fields.push({ key: 'name', label: 'Name', value: node.name, multiline: false });
    if (node.title !== undefined) fields.push({ key: 'title', label: 'Title', value: node.title, multiline: false });
    if (node.description !== undefined) fields.push({ key: 'description', label: 'Description', value: node.description, multiline: true });
    if (node.acceptance_criteria !== undefined) fields.push({ key: 'acceptance_criteria', label: 'Acceptance Criteria', value: node.acceptance_criteria, multiline: true });
    if (node.test_steps !== undefined) fields.push({ key: 'test_steps', label: 'Test Steps', value: node.test_steps, multiline: true });
    if (node.expected_result !== undefined) fields.push({ key: 'expected_result', label: 'Expected Result', value: node.expected_result, multiline: true });
    // Fallback: at minimum allow name/title editing
    if (fields.length === 0) {
      fields.push({ key: 'name', label: 'Name', value: node.name || node.title || '', multiline: false });
    }
    return fields;
  }

  async function saveNode(panel, node, level, wrapper) {
    var endpoint = entityEndpoint[level];
    var body = {};
    var inputs = panel.querySelectorAll('[data-field]');
    inputs.forEach(function (inp) {
      var key = inp.getAttribute('data-field');
      var val = inp.value;
      if (val !== (node[key] || '')) {
        body[key] = val;
      }
    });

    if (Object.keys(body).length === 0) {
      toast('No changes to save', 'error');
      return;
    }

    try {
      var updated = await api('PATCH', '/api/' + endpoint + '/' + node.id, body);
      // Merge updated fields back
      if (updated) {
        Object.keys(updated).forEach(function (k) { node[k] = updated[k]; });
      } else {
        Object.keys(body).forEach(function (k) { node[k] = body[k]; });
      }
      // Update header name
      var nameEl = wrapper.querySelector(':scope > .node-header .node-name');
      if (nameEl) nameEl.textContent = node.name || node.title || '(untitled)';
      // Update version badge
      if (updated && updated.version !== undefined) {
        var vBadge = wrapper.querySelector(':scope > .node-header .badge-version');
        if (vBadge) vBadge.textContent = 'v' + updated.version;
      }
      panel.remove();
      openEditId = null;
      toast('Saved successfully', 'success');
    } catch (err) {
      toast('Save failed: ' + err.message, 'error');
    }
  }

  async function sendFeedback(node, level, text) {
    if (!text || !text.trim()) {
      toast('Please enter feedback text', 'error');
      return;
    }
    if (!currentProduct) return;
    try {
      await api('POST', '/api/' + encodeURIComponent(currentProduct) + '/feedback', {
        entity_type: level,
        entity_id: node.id,
        feedback_text: text.trim()
      });
      toast('Feedback sent', 'success');
    } catch (err) {
      toast('Feedback failed: ' + err.message, 'error');
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
    if (!iterators || iterators.length === 0) {
      modalBody.innerHTML = '<p style="color:var(--text-muted)">No iterators found for this product.</p>';
      return;
    }

    iterators.forEach(function (it) {
      var card = document.createElement('div');
      card.className = 'iterator-card';

      var h3 = document.createElement('h3');
      h3.textContent = it.name;
      card.appendChild(h3);

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

  // ── Keyboard shortcuts ─────────────────────────────────────────────────
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') {
      // Close modal
      if (!modalOverlay.classList.contains('hidden')) {
        modalOverlay.classList.add('hidden');
        return;
      }
      // Close edit panel
      var panel = document.querySelector('.edit-panel');
      if (panel) {
        panel.remove();
        openEditId = null;
      }
    }
  });

  // ── Init ───────────────────────────────────────────────────────────────
  loadProducts();
})();
