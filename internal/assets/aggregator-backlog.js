// aggregator-backlog.js — window.HDBacklog. Renders the read-only Backlog view:
// per-repo ready/blocked lists, a hand-rolled inline-SVG dependency graph, a
// keyboard cursor, and a drill-in detail panel. A separate IIFE (like
// aggregator-tree.js), prepended before the core so the core's viewBacklog() and
// paint() can call it during init. It owns its own cursor + detail state and
// talks to the core only through this namespace and the DOM. No innerHTML: DOM
// via HDDom.el, SVG via svgEl (createElementNS). Colors come from CSS classes so
// the graph follows the light/dark theme.
window.HDBacklog = (function () {
  'use strict';

  var el = window.HDDom.el;
  var SVGNS = 'http://www.w3.org/2000/svg';

  var state = {
    data: { repos: [], available: false },
    cursor: null, // bkey of the focused row
    detail: null, // { project, id, data, reason } when a detail panel is open
    form: null    // { project, title, type, priority, description } when the create form is open
  };
  var graphSeq = 0; // monotonic id source so each graph's arrowhead marker is unique

  function svgEl(tag, attrs, kids) {
    var n = document.createElementNS(SVGNS, tag);
    attrs = attrs || {};
    Object.keys(attrs).forEach(function (k) { n.setAttribute(k, attrs[k]); });
    (kids || []).forEach(function (c) {
      if (c == null) return;
      n.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    });
    return n;
  }

  function trunc(s, n) { s = s || ''; return s.length > n ? s.slice(0, n - 1) + '…' : s; }
  function bkey(repo, id) { return repo + '\x00' + id; }
  function shortID(repo, id) { return (id || '').indexOf(repo + '-') === 0 ? id.slice(repo.length + 1) : id; }
  function idSet(list) { var m = {}; (list || []).forEach(function (i) { m[i.id] = true; }); return m; }
  function pri(i) { var p = i && i.priority; return (p >= 0 && p <= 4) ? p : 0; }

  // classify an issue for coloring given the repo's ready/blocked id sets.
  function classify(issue, readySet, blockedSet) {
    if (issue.issue_type === 'epic') return 'epic';
    if (blockedSet[issue.id]) return 'blocked';
    if (issue.status === 'in_progress') return 'inprog';
    if (readySet[issue.id]) return 'ready';
    return 'other';
  }

  // --- list rows ---
  function issueRow(repoName, issue, cls, meta) {
    return el('div', {
      class: 'bk-row',
      data: { bkey: bkey(repoName, issue.id), project: repoName, id: issue.id }
    }, [
      el('span', { class: 'bk-dot ' + cls }),
      el('span', { class: 'bk-id', text: shortID(repoName, issue.id) }),
      el('span', { class: 'bk-pri p' + pri(issue), text: 'P' + pri(issue) }),
      el('span', { class: 'bk-title', text: issue.title || issue.id }),
      el('span', { class: 'bk-meta', text: meta || '' })
    ]);
  }

  function sechead(label, n) {
    return el('div', { class: 'bk-h' }, [
      el('span', { text: label }),
      el('span', { class: 'bk-count', text: String(n) })
    ]);
  }

  function repoCard(repo) {
    var readySet = idSet(repo.ready), blockedSet = idSet(repo.blocked);
    var c = repo.counts || {};
    var header = el('div', { class: 'bk-strip' }, [
      el('span', { class: 'bk-repo', text: repo.name }),
      el('span', { class: 'bk-c', text: 'ready ' + (c.ready || 0) }),
      el('span', { class: 'bk-c', text: 'blocked ' + (c.blocked || 0) }),
      el('span', { class: 'bk-c', text: 'open ' + (c.open || 0) })
    ]);
    if (window.HD_BEADS_WRITABLE) {
      header.appendChild(el('button', { class: 'bk-newbtn', title: 'new issue (n)', 'data-bknew': repo.name }, ['+ new']));
    }
    if (repo.err) {
      return el('section', { class: 'panel bk-card' }, [
        header, el('div', { class: 'bk-carderr', text: 'bd error: ' + repo.err })
      ]);
    }
    var readyRows = (repo.ready || []).map(function (i) {
      return issueRow(repo.name, i, classify(i, readySet, blockedSet),
        i.dependent_count ? 'blocks ' + i.dependent_count : '');
    });
    var blockedRows = (repo.blocked || []).map(function (i) {
      var by = (i.blocked_by || []).map(function (b) { return shortID(repo.name, b); }).join(', ');
      return issueRow(repo.name, i, 'blocked', by ? '← ' + by : '');
    });
    var none = function () { return el('div', { class: 'bk-none', text: 'none' }); };
    var cols = el('div', { class: 'bk-cols' }, [
      el('div', { class: 'bk-sec' }, [sechead('READY', c.ready || readyRows.length)].concat(
        readyRows.length ? readyRows : [none()])),
      el('div', { class: 'bk-sec' }, [sechead('BLOCKED', c.blocked || blockedRows.length)].concat(
        blockedRows.length ? blockedRows : [none()]))
    ]);
    var kids = [header, cols];
    var graph = graphSVG(repo, readySet, blockedSet);
    if (graph) kids.push(graph);
    return el('section', { class: 'panel bk-card', 'data-vim-section': '' }, kids);
  }

  // --- inline SVG dependency graph (longest-path layered DAG) ---
  var NW = 150, NH = 44, COLW = 172, ROWH = 62, PADX = 8, PADY = 10, MAXLAYER = 12;

  function graphSVG(repo, readySet, blockedSet) {
    var edges = repo.edges || [];
    if (!edges.length) return null;

    var nodes = {};
    (repo.all || []).forEach(function (i) { nodes[i.id] = i; });
    var inGraph = {};
    edges.forEach(function (e) {
      inGraph[e.from] = true; inGraph[e.to] = true;
      if (!nodes[e.from]) nodes[e.from] = { id: e.from, title: e.from, priority: 4, status: '' };
      if (!nodes[e.to]) nodes[e.to] = { id: e.to, title: e.to, priority: 4, status: '' };
    });
    var ids = Object.keys(inGraph);
    if (!ids.length) return null;

    // longest-path layering via bounded relaxation (DAG-safe; capped for cycles).
    var layer = {};
    ids.forEach(function (id) { layer[id] = 0; });
    for (var pass = 0; pass < ids.length; pass++) {
      var moved = false;
      edges.forEach(function (e) {
        if (layer[e.to] < layer[e.from] + 1) {
          layer[e.to] = Math.min(layer[e.from] + 1, MAXLAYER);
          moved = true;
        }
      });
      if (!moved) break;
    }

    var byLayer = {}, maxRow = 0, maxLayer = 0;
    ids.forEach(function (id) {
      var L = layer[id];
      (byLayer[L] = byLayer[L] || []).push(id);
      if (L > maxLayer) maxLayer = L;
    });
    var pos = {};
    Object.keys(byLayer).forEach(function (L) {
      byLayer[L].forEach(function (id, idx) {
        pos[id] = { x: PADX + Number(L) * COLW, y: PADY + idx * ROWH };
        if (idx > maxRow) maxRow = idx;
      });
    });
    var W = PADX + (maxLayer + 1) * COLW, H = PADY * 2 + (maxRow + 1) * ROWH;

    // Unique marker id per graph: multiple repo graphs share one HTML document,
    // so a fixed id would duplicate and every arrow would point at the first.
    var arrID = 'bk-arr-' + (graphSeq++);
    var svg = svgEl('svg', { viewBox: '0 0 ' + W + ' ' + H, class: 'bk-svg' });
    svg.appendChild(svgEl('defs', {}, [
      svgEl('marker', { id: arrID, markerWidth: '9', markerHeight: '9', refX: '7', refY: '3', orient: 'auto' }, [
        svgEl('path', { class: 'bk-arr-head', d: 'M0,0 L7,3 L0,6 z' })
      ])
    ]));
    edges.forEach(function (e) {
      var a = pos[e.from], b = pos[e.to];
      if (!a || !b) return;
      svg.appendChild(svgEl('line', {
        class: 'bk-edge' + (e.kind === 'parent' ? ' parent' : ''),
        x1: a.x + NW, y1: a.y + NH / 2, x2: b.x - 3, y2: b.y + NH / 2,
        'marker-end': 'url(#' + arrID + ')'
      }));
    });
    ids.forEach(function (id) {
      var n = nodes[id], p = pos[id], cls = classify(n, readySet, blockedSet);
      var g = svgEl('g', {});
      g.appendChild(svgEl('rect', { class: 'bk-node ' + cls, x: p.x, y: p.y, width: NW, height: NH, rx: 7 }));
      g.appendChild(ntext(p.x + 10, p.y + 18, shortID(repo.name, id), 'bk-nid'));
      g.appendChild(svgEl('rect', { class: 'bk-npill p' + pri(n), x: p.x + NW - 34, y: p.y + 7, width: 26, height: 15, rx: 3 }));
      g.appendChild(ntext(p.x + NW - 21, p.y + 18, 'P' + pri(n), 'bk-npill-t p' + pri(n), 'middle'));
      g.appendChild(ntext(p.x + 10, p.y + 35, trunc(n.title, 20), 'bk-ntitle'));
      svg.appendChild(g);
    });

    return el('div', { class: 'bk-graph' }, [
      el('div', { class: 'bk-h' }, [el('span', { text: 'dependency graph' })]),
      svg,
      legend()
    ]);
  }

  function ntext(x, y, s, cls, anchor) {
    var a = { class: cls, x: x, y: y };
    if (anchor) a['text-anchor'] = anchor;
    var t = svgEl('text', a);
    t.textContent = s;
    return t;
  }

  function legend() {
    function seg(cls, label) { return el('span', { class: 'bk-lg' }, [el('i', { class: 'bk-sw ' + cls }), label]); }
    return el('div', { class: 'bk-legend' }, [
      seg('ready', 'ready'), seg('blocked', 'blocked'), seg('epic', 'epic'),
      seg('inprog', 'in-prog'), el('span', { text: '→ blocks · ⇢ parent' })
    ]);
  }

  // --- view render (called by core viewBacklog) ---
  function render(data) {
    state.data = data || { repos: [], available: false };
    if (!state.data.available) {
      return [el('div', { class: 'empty' }, ['Backlog is off — set beads.enabled in config.'])];
    }
    var repos = sortedRepos();
    if (!repos.length) {
      return [el('div', { class: 'empty' }, ['No beads repos found under scan_roots.'])];
    }
    return repos.map(repoCard);
  }

  function setData(data) { state.data = data || { repos: [], available: false }; }

  // sortedRepos — most-active first (open desc); empty/errored sink to the
  // bottom. Used for BOTH the render order and the cursor row order so j/k
  // walks rows in the same order they appear.
  function sortedRepos() {
    return (state.data.repos || []).slice().sort(function (a, b) {
      return ((b.counts && b.counts.open) || 0) - ((a.counts && a.counts.open) || 0);
    });
  }

  // --- cursor over the flattened ready+blocked rows ---
  function rowList() {
    var out = [];
    sortedRepos().forEach(function (repo) {
      (repo.ready || []).concat(repo.blocked || []).forEach(function (i) {
        out.push({ project: repo.name, id: i.id, bkey: bkey(repo.name, i.id) });
      });
    });
    return out;
  }

  function applyHighlight() {
    document.querySelectorAll('.bk-row.focused').forEach(function (n) { n.classList.remove('focused'); });
    if (!state.cursor) return;
    var rows = document.querySelectorAll('.bk-row[data-bkey]');
    for (var i = 0; i < rows.length; i++) {
      if (rows[i].dataset.bkey === state.cursor) {
        rows[i].classList.add('focused');
        rows[i].scrollIntoView({ block: 'nearest', behavior: 'instant' });
        return;
      }
    }
  }

  function moveCursor(delta) {
    var rows = rowList();
    if (!rows.length) return;
    var idx = rows.findIndex(function (r) { return r.bkey === state.cursor; });
    idx = idx < 0 ? 0 : Math.max(0, Math.min(rows.length - 1, idx + delta));
    state.cursor = rows[idx].bkey;
    applyHighlight();
  }
  function moveCursorTo(where) {
    var rows = rowList();
    if (!rows.length) return;
    state.cursor = rows[where === 'top' ? 0 : rows.length - 1].bkey;
    applyHighlight();
  }
  function focusedRow() {
    return rowList().find(function (r) { return r.bkey === state.cursor; }) || null;
  }
  function openFocused() {
    var r = focusedRow();
    if (r) detail(r.project, r.id);
  }

  // --- drill-in detail ---
  function detail(project, id) {
    fetch('/api/beads/' + encodeURIComponent(project) + '/' + encodeURIComponent(id), { cache: 'no-store' })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (d) {
        if (!d) return;
        // preserve a half-typed close reason across a re-fetch of the same issue
        var prevReason = (state.detail && state.detail.id === id) ? state.detail.reason : '';
        state.detail = { project: project, id: id, data: d, reason: prevReason || '' };
        paintDetail();
      })
      .catch(function () {});
  }

  function closeDetail() {
    if (!state.detail) return false;
    state.detail = null;
    var p = document.getElementById('bk-detail');
    if (p) p.remove();
    return true;
  }

  function detailBlock(label, text) {
    text = (text || '').trim();
    if (!text) return null;
    return el('div', { class: 'bk-detail-block' }, [
      el('div', { class: 'bk-detail-label', text: label }),
      el('pre', { class: 'bk-pre', text: text })
    ]);
  }

  // detailActions builds the writable Claim/Close controls for an open issue.
  function detailActions(issue) {
    if (!writable() || !state.detail || issue.status === 'closed') return null;
    var reason = el('input', { class: 'bk-reason', type: 'text', placeholder: 'close reason (optional)' });
    reason.value = state.detail.reason || ''; // restore across a wholesale re-render
    reason.addEventListener('input', function () { state.detail.reason = reason.value; });
    reason.addEventListener('keydown', function (ev) {
      if (ev.key === 'Enter') { ev.preventDefault(); closeIssue(state.detail.project, state.detail.id, reason.value); }
      else if (ev.key === 'Escape') { ev.preventDefault(); reason.blur(); closeDetail(); }
    });
    return el('div', { class: 'bk-actions' }, [
      el('button', { class: 'bk-btn', 'data-bkact': 'claim' }, ['claim']),
      reason,
      el('button', { class: 'bk-btn bk-btn-danger', 'data-bkact': 'close' }, ['close']),
      el('span', { class: 'bk-err', id: 'bk-detail-err' })
    ]);
  }

  function paintDetail() {
    var host = document.getElementById('view-backlog');
    if (!host || !state.detail) return;
    var d = state.detail.data, issue = d.issue || {};
    var panel = el('section', { class: 'panel bk-detail', id: 'bk-detail' }, [
      el('div', { class: 'bk-detail-head' }, [
        el('span', { class: 'bk-id', text: state.detail.id }),
        el('span', { class: 'bk-pri p' + pri(issue), text: 'P' + pri(issue) }),
        el('span', { class: 'bk-detail-title', text: issue.title || '' }),
        el('button', { class: 'bk-x', title: 'close (Esc)', 'data-bkclose': '1' }, ['✕'])
      ]),
      el('div', { class: 'bk-detail-meta', text: [issue.status, issue.issue_type, issue.owner].filter(Boolean).join(' · ') }),
      detailActions(issue),
      issue.description ? el('div', { class: 'bk-detail-desc', text: issue.description }) : null,
      detailBlock('blocked by', d.blockers),
      detailBlock('dependents', d.dependents),
      detailBlock('comments', d.comments)
    ]);
    var existing = document.getElementById('bk-detail');
    if (existing) existing.replaceWith(panel);
    else host.insertBefore(panel, host.firstChild);
    panel.scrollIntoView({ block: 'nearest', behavior: 'instant' });
  }

  // ensureCursor re-snaps to the first row when the focused issue vanished from
  // a live refresh (e.g. bd closed it), so the cursor stays visible rather than
  // silently disappearing — mirrors the inbox's ensureFocused.
  function ensureCursor() {
    if (!state.cursor) return;
    var rows = rowList();
    if (!rows.length) { state.cursor = null; return; }
    if (!rows.some(function (r) { return r.bkey === state.cursor; })) state.cursor = rows[0].bkey;
  }

  // re-apply cursor + detail + open create-form after a core render rebuilt the
  // view DOM (a cross-project 'change' SSE rebuilds #view-backlog wholesale, so
  // without this an open detail/form would be silently discarded).
  function paint() {
    ensureCursor();
    applyHighlight();
    if (state.detail) paintDetail();
    if (state.form) paintForm();
  }

  // --- actions (Phase 2, writable only). The server broadcasts 'beads' after a
  // write so the lists refresh via the core SSE listener; we also fire a
  // hd:beads-refresh event as a belt-and-suspenders immediate refresh. ---
  function writable() { return !!window.HD_BEADS_WRITABLE; }
  function enc(s) { return encodeURIComponent(s); }
  function triggerRefresh() { document.dispatchEvent(new CustomEvent('hd:beads-refresh')); }

  function postAction(url, body, onOk, onErr) {
    fetch(url, {
      method: 'POST',
      headers: body ? { 'Content-Type': 'application/json' } : {},
      body: body ? JSON.stringify(body) : undefined,
      cache: 'no-store'
    }).then(function (r) {
      if (r.ok) { r.json().then(function (j) { if (onOk) onOk(j || {}); }).catch(function () { if (onOk) onOk({}); }); return; }
      r.text().then(function (t) { if (onErr) onErr((t || '').trim() || ('HTTP ' + r.status)); });
    }).catch(function (e) { if (onErr) onErr(String(e)); });
  }

  // toast surfaces a transient message when there's no inline error slot (e.g. a
  // keyboard `c` claim that fails with no detail panel open) — so an action error
  // is never silently swallowed.
  function toast(msg) {
    var t = document.getElementById('bk-toast');
    if (!t) { t = el('div', { id: 'bk-toast', class: 'bk-toast' }); document.body.appendChild(t); }
    t.textContent = msg;
    t.classList.add('show');
    clearTimeout(t._timer);
    t._timer = setTimeout(function () { t.classList.remove('show'); }, 4500);
  }
  // err surfaces an action error: always to the console, inline when the slot
  // exists, else as a toast. Never silent.
  function err(slotId, msg) {
    console.error('harness-deck: beads action: ' + msg);
    var e = slotId && document.getElementById(slotId);
    if (e) e.textContent = msg; else toast(msg);
  }
  function detailErr(msg) { err('bk-detail-err', msg); }

  function claim(project, id) {
    postAction('/api/beads/' + enc(project) + '/' + enc(id) + '/claim', null,
      function () { triggerRefresh(); detail(project, id); }, detailErr);
  }
  function closeIssue(project, id, reason) {
    postAction('/api/beads/' + enc(project) + '/' + enc(id) + '/close', { reason: reason || '' },
      function () { closeDetail(); triggerRefresh(); }, detailErr);
  }
  function createIssue(project, fields, onOk, onErr) {
    postAction('/api/beads/' + enc(project) + '/create', fields, onOk, onErr);
  }

  // --- create form. State lives in state.form so a wholesale render() (fired by
  // any cross-project 'change' SSE) can't discard a half-typed issue — paint()
  // re-renders it from the tracked values. ---
  function fieldRow(label, control) {
    return el('label', { class: 'bk-frow' }, [el('span', { class: 'bk-flabel', text: label }), control]);
  }
  function buildForm(f) {
    var title = el('input', { class: 'bk-fi', type: 'text', placeholder: 'title' });
    var type = el('select', { class: 'bk-fs' },
      ['task', 'feature', 'bug', 'chore', 'epic'].map(function (t) { return el('option', { value: t }, [t]); }));
    var prio = el('select', { class: 'bk-fs' },
      ['0', '1', '2', '3', '4'].map(function (p) { return el('option', { value: p }, ['P' + p]); }));
    var desc = el('textarea', { class: 'bk-ft', placeholder: 'description (optional)', rows: '3' });
    title.value = f.title; type.value = f.type; prio.value = f.priority; desc.value = f.description;
    title.addEventListener('input', function () { f.title = title.value; });
    type.addEventListener('change', function () { f.type = type.value; });
    prio.addEventListener('change', function () { f.priority = prio.value; });
    desc.addEventListener('input', function () { f.description = desc.value; });
    return el('section', { class: 'panel bk-form', id: 'bk-form' }, [
      el('div', { class: 'bk-detail-head' }, [
        el('span', { class: 'bk-detail-title', text: 'new issue · ' + f.project }),
        el('button', { class: 'bk-x', title: 'cancel (Esc)', 'data-bkact': 'create-cancel' }, ['✕'])
      ]),
      fieldRow('title', title), fieldRow('type', type), fieldRow('priority', prio), fieldRow('description', desc),
      el('div', { class: 'bk-actions' }, [
        el('button', { class: 'bk-btn', 'data-bkact': 'create-submit' }, ['create']),
        el('span', { class: 'bk-err', id: 'bk-form-err' })
      ])
    ]);
  }
  function paintForm() {
    var host = document.getElementById('view-backlog');
    if (!host || !state.form) return;
    var panel = buildForm(state.form);
    var existing = document.getElementById('bk-form');
    if (existing) existing.replaceWith(panel); else host.insertBefore(panel, host.firstChild);
    return panel;
  }
  function openCreate(project) {
    if (!writable()) return;
    state.form = { project: project, title: '', type: 'task', priority: '2', description: '' };
    var panel = paintForm();
    if (panel) { var t = panel.querySelector('.bk-fi'); if (t) t.focus(); }
  }
  function closeForm() {
    state.form = null;
    var f = document.getElementById('bk-form'); if (f) f.remove();
  }
  function submitCreate() {
    var f = state.form; if (!f) return;
    createIssue(f.project, { title: f.title, type: f.type || 'task', priority: f.priority || '2', description: f.description },
      function () { closeForm(); triggerRefresh(); }, function (msg) { err('bk-form-err', msg); });
  }

  // keyboard-exposed actions (act on the focused row)
  function claimFocused() { var r = focusedRow(); if (r && writable()) claim(r.project, r.id); }
  function closeFocused() {
    var r = focusedRow();
    if (!r || !writable()) return;
    detail(r.project, r.id); // open detail so a reason can be typed; Enter submits
    setTimeout(function () { var ri = document.querySelector('#bk-detail .bk-reason'); if (ri) ri.focus(); }, 350);
  }
  function newFocusedRepo() { var r = focusedRow(); if (r && writable()) openCreate(r.project); }

  // mouse: row → detail; ✕ → close; action buttons → claim/close/create.
  document.addEventListener('click', function (e) {
    var actBtn = e.target.closest('[data-bkact]');
    if (actBtn) {
      var act = actBtn.dataset.bkact;
      if (act === 'claim' && state.detail) { claim(state.detail.project, state.detail.id); return; }
      if (act === 'close' && state.detail) {
        var ri = document.querySelector('#bk-detail .bk-reason');
        closeIssue(state.detail.project, state.detail.id, ri ? ri.value : '');
        return;
      }
      if (act === 'create-submit') { submitCreate(); return; }
      if (act === 'create-cancel') { closeForm(); return; }
    }
    if (e.target.closest('[data-bknew]')) { openCreate(e.target.closest('[data-bknew]').dataset.bknew); return; }
    if (e.target.closest('[data-bkclose]')) { closeDetail(); return; }
    var row = e.target.closest('.bk-row');
    if (row && row.dataset.project) {
      state.cursor = row.dataset.bkey;
      applyHighlight();
      detail(row.dataset.project, row.dataset.id);
    }
  });

  return {
    render: render, setData: setData, paint: paint,
    moveCursor: moveCursor, moveCursorTo: moveCursorTo,
    openFocused: openFocused, closeDetail: closeDetail,
    claimFocused: claimFocused, closeFocused: closeFocused, newFocusedRepo: newFocusedRepo
  };
})();
