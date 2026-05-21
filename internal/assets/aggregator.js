/* harness-deck aggregator — fetches the report index and renders the sidebar
   tree and the four switchable home views (inbox / overview / latest /
   roadmap). vim-nav.js handles scroll/search/command; this owns the data.

   The DOM is built with createElement + textContent only (never innerHTML),
   so report content authored by an agent can never inject markup. */
(function () {
  'use strict';

  var VIEWS = [
    { id: 'inbox', key: '1', label: 'inbox' },
    { id: 'overview', key: '2', label: 'overview' },
    { id: 'latest', key: '3', label: 'latest' },
    { id: 'roadmap', key: '4', label: 'roadmap' }
  ];
  var data = { reports: [], errors: [] };
  var currentView = 'inbox';

  /* el builds a DOM node. attrs: {class, text, data:{...}, <attr>:<val>}.
     kids: array of nodes or strings (strings become safe text nodes). */
  function el(tag, attrs, kids) {
    var n = document.createElement(tag);
    attrs = attrs || {};
    Object.keys(attrs).forEach(function (k) {
      if (k === 'text') n.textContent = attrs[k];
      else if (k === 'class') n.className = attrs[k];
      else if (k === 'data') Object.keys(attrs[k]).forEach(function (d) { n.dataset[d] = attrs[k][d]; });
      else n.setAttribute(k, attrs[k]);
    });
    (kids || []).forEach(function (c) {
      if (c == null) return;
      n.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    });
    return n;
  }

  function reportURL(r) {
    return '/r/' + encodeURIComponent(r.project) + '/' + encodeURIComponent(r.run);
  }

  function shortTime(iso) {
    if (!iso) return '';
    var d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    var p = function (n) { return String(n).padStart(2, '0'); };
    return p(d.getMonth() + 1) + '-' + p(d.getDate()) + ' ' + p(d.getHours()) + ':' + p(d.getMinutes());
  }

  function countBy(field) {
    return data.reports.reduce(function (m, r) {
      var k = r[field] || '—';
      m[k] = (m[k] || 0) + 1;
      return m;
    }, {});
  }

  function pill(text, level) {
    return el('span', { class: 'pill ' + (level || ''), text: text });
  }

  function panel(title, rightNode, bodyKids) {
    var head = [
      el('span', { class: 'sigil', text: '§' }), ' ',
      el('span', { class: 'title-text', text: title })
    ];
    if (rightNode) head.push(el('div', { class: 'right' }, [rightNode]));
    return el('section', { class: 'panel', 'data-vim-section': '' }, [
      el('div', { class: 'panel-head' }, head),
      el('div', { class: 'panel-body' }, bodyKids)
    ]);
  }

  function emptyState(kids) {
    return el('div', { class: 'empty' }, kids);
  }

  /* itemRow — one navigable report row, used by inbox / overview / latest. */
  function itemRow(r) {
    var sub = [el('span', { class: 'proj', text: r.project }), ' · ' + r.harness];
    if (r.kind) sub.push(' · ' + r.kind);
    var aside = [shortTime(r.created)];
    if (r.open_asks > 0) {
      aside.push(el('br'));
      aside.push(el('span', { class: 'asks', text: r.open_asks + ' open' }));
    }
    return el('div', { class: 'inbox-item', data: { url: reportURL(r) } }, [
      el('span', { class: 'dot ' + r.status }),
      el('div', { class: 'main' }, [
        el('div', { class: 'title', text: r.title || r.run }),
        el('div', { class: 'sub' }, sub)
      ]),
      el('div', { class: 'aside' }, aside)
    ]);
  }

  /* --- sidebar tree --- */
  function renderTree() {
    var byProj = {};
    data.reports.forEach(function (r) {
      (byProj[r.project] = byProj[r.project] || []).push(r);
    });
    var projects = Object.keys(byProj).sort();
    var tree = el('div', { class: 'tree' });
    if (!projects.length) {
      tree.appendChild(el('div', {
        class: 'empty', style: 'padding:12px 4px;text-align:left', text: 'no reports indexed'
      }));
    }
    projects.forEach(function (p) {
      tree.appendChild(el('div', { class: 'row proj' }, [
        el('span', { class: 'glyph', text: '▾ ' }),
        el('span', { class: 'dir', text: p + '/' }),
        el('span', { class: 'count', text: String(byProj[p].length) })
      ]));
      byProj[p].forEach(function (r) {
        tree.appendChild(el('div', { class: 'row run', data: { url: reportURL(r) } }, [
          el('span', { class: 'st ' + r.status }),
          el('span', { class: 'label', text: r.title || r.run }),
          el('span', { class: 'hbadge', text: r.harness })
        ]));
      });
    });

    var sections = [el('div', { class: 'sidebar-section' }, [
      el('div', { class: 'sidebar-title', text: 'reports' }), tree
    ])];
    if (data.errors && data.errors.length) {
      var errs = el('div', { class: 'scan-errs' },
        data.errors.map(function (e) { return el('div', { text: e }); }));
      sections.push(el('div', { class: 'sidebar-section' }, [
        el('div', { class: 'sidebar-title', text: 'scan errors' }), errs
      ]));
    }
    document.getElementById('tree').replaceChildren.apply(
      document.getElementById('tree'), sections);
  }

  /* --- home views --- each returns an array of nodes --- */
  function viewInbox() {
    var items = data.reports.filter(function (r) {
      return r.status === 'awaiting-review' || r.open_asks > 0;
    });
    var body = items.length
      ? items.map(itemRow)
      : [emptyState([el('b', { text: 'nothing needs you.' }), el('br'),
        'no reports are awaiting review.'])];
    var right = items.length ? pill(items.length + ' awaiting', 'warn') : null;
    return [panel('inbox — needs you', right, body)];
  }

  function viewOverview() {
    var rs = data.reports, st = countBy('status');
    var cells = [
      ['reports', rs.length],
      ['awaiting review', st['awaiting-review'] || 0],
      ['answered', st['answered'] || 0],
      ['projects', Object.keys(countBy('project')).length]
    ];
    var grid = el('div', { class: 'metric-grid' }, cells.map(function (c) {
      return el('div', { class: 'metric' }, [
        el('div', { class: 'label', text: c[0] }),
        el('div', { class: 'value', text: String(c[1]) })
      ]);
    }));
    var overview = el('section', { class: 'panel', 'data-vim-section': '' }, [
      el('div', { class: 'panel-head' }, [
        el('span', { class: 'sigil', text: '§' }), ' ',
        el('span', { class: 'title-text', text: 'overview' })
      ]),
      grid
    ]);
    var recent = rs.length
      ? rs.slice(0, 6).map(itemRow)
      : [emptyState(['no reports yet'])];
    return [overview, panel('recent activity', null, recent)];
  }

  function viewLatest() {
    if (!data.reports.length) {
      return [panel('latest', null, [emptyState(['no reports yet'])])];
    }
    var r = data.reports[0];
    return [panel('latest report', pill(r.status), [itemRow(r)])];
  }

  function viewRoadmap() {
    return [panel('roadmap', pill('phase 5'), [emptyState([
      'the roadmap view arrives in ', el('b', { text: 'Phase 5' }),
      ' — it will render each project’s ',
      el('code', { text: '.docs/ai/roadmap.md' }), '.'
    ])])];
  }

  var BUILDERS = { inbox: viewInbox, overview: viewOverview, latest: viewLatest, roadmap: viewRoadmap };

  function renderContent() {
    var tabs = el('div', { class: 'view-tabs' }, VIEWS.map(function (v) {
      return el('div', { class: 'view-tab', data: { view: v.id } }, [
        el('span', { class: 'k', text: v.key }), v.label
      ]);
    }));
    var nodes = [tabs];
    VIEWS.forEach(function (v) {
      nodes.push(el('div', { class: 'view', id: 'view-' + v.id }, BUILDERS[v.id]()));
    });
    var content = document.getElementById('content');
    content.replaceChildren.apply(content, nodes);
  }

  function showView(id) {
    currentView = id;
    document.querySelectorAll('.view').forEach(function (v) {
      v.classList.toggle('active', v.id === 'view-' + id);
    });
    document.querySelectorAll('.view-tab').forEach(function (t) {
      t.classList.toggle('active', t.dataset.view === id);
    });
  }

  function render() {
    renderTree();
    renderContent();
    showView(currentView);
  }

  /* one delegated click handler for every navigable row */
  document.addEventListener('click', function (e) {
    var tab = e.target.closest('.view-tab');
    if (tab) { showView(tab.dataset.view); return; }
    var row = e.target.closest('[data-url]');
    if (row) { window.location.href = row.dataset.url; }
  });

  /* number keys 1-4 switch views, unless vim-nav owns the keystroke */
  document.addEventListener('keydown', function (e) {
    var t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
    if (window.VimNav && VimNav.getMode && VimNav.getMode() !== 'NORMAL') return;
    var v = VIEWS.find(function (x) { return x.key === e.key; });
    if (v) { showView(v.id); e.preventDefault(); }
  });

  function load() {
    fetch('/api/reports')
      .then(function (resp) {
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        return resp.json();
      })
      .then(function (d) {
        data = { reports: d.reports || [], errors: d.errors || [] };
        render();
      })
      .catch(function (err) {
        document.getElementById('content').replaceChildren(
          emptyState(['failed to load reports: ' + String(err)]));
      });
  }

  // exposed so Phase 3's live updates can trigger a refresh
  window.HarnessDeck = { reload: load };
  load();
})();
