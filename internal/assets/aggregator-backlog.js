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
    detail: null  // { project, id, data } when a detail panel is open
  };

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

    var svg = svgEl('svg', { viewBox: '0 0 ' + W + ' ' + H, class: 'bk-svg' });
    svg.appendChild(svgEl('defs', {}, [
      svgEl('marker', { id: 'bk-arr', markerWidth: '9', markerHeight: '9', refX: '7', refY: '3', orient: 'auto' }, [
        svgEl('path', { class: 'bk-arr-head', d: 'M0,0 L7,3 L0,6 z' })
      ])
    ]));
    edges.forEach(function (e) {
      var a = pos[e.from], b = pos[e.to];
      if (!a || !b) return;
      svg.appendChild(svgEl('line', {
        class: 'bk-edge' + (e.kind === 'parent' ? ' parent' : ''),
        x1: a.x + NW, y1: a.y + NH / 2, x2: b.x - 3, y2: b.y + NH / 2,
        'marker-end': 'url(#bk-arr)'
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
        state.detail = { project: project, id: id, data: d };
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

  // re-apply cursor + detail after a core render rebuilt the view DOM
  function paint() {
    applyHighlight();
    if (state.detail) paintDetail();
  }

  // mouse: click a row → open its detail; click ✕ → close.
  document.addEventListener('click', function (e) {
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
    openFocused: openFocused, closeDetail: closeDetail
  };
})();
