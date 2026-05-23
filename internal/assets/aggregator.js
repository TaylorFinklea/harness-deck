/* harness-deck aggregator — fetches the report index and renders the sidebar
   tree and the four switchable home views (inbox / overview / latest /
   projects). vim-nav.js handles scroll/search/command; this owns the data.

   The DOM is built with createElement + textContent only (never innerHTML),
   so report content authored by an agent can never inject markup. */
(function () {
  'use strict';

  var VIEWS = [
    { id: 'inbox', key: '1', label: 'inbox' },
    { id: 'overview', key: '2', label: 'overview' },
    { id: 'latest', key: '3', label: 'latest' },
    { id: 'projects', key: '4', label: 'projects' }
  ];
  var data = { reports: [], errors: [], projects: [], discovered: [] };
  var currentView = 'inbox';
  var trackedOpen = false;     // is the "tracked projects" panel expanded?
  var draggedName = null;      // project name currently being dragged, or null

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

  /* htmlToNodes parses a server-rendered HTML string into detached nodes.
     A DOMParser document never executes scripts, and the only HTML passed
     here is roadmap markdown rendered (and escaped) server-side. */
  function htmlToNodes(htmlString) {
    var doc = new DOMParser().parseFromString(htmlString || '', 'text/html');
    return Array.prototype.slice.call(doc.body.childNodes);
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
    // Sort the tree by the user's project order — falls back to alpha for
    // projects without a discovered entry (e.g. central-dir-only reports).
    var nameIdx = {};
    (data.discovered || []).forEach(function (d, i) { nameIdx[d.name] = i; });
    var projects = Object.keys(byProj).sort(function (a, b) {
      var ai = (a in nameIdx) ? nameIdx[a] : Infinity;
      var bi = (b in nameIdx) ? nameIdx[b] : Infinity;
      return ai !== bi ? ai - bi : a.localeCompare(b);
    });
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

  /* projectToggleRow — one discovered project with a drag handle and a
     visibility checkbox. The whole row is draggable so users can grab
     anywhere; the handle glyph is the visual cue. */
  function projectToggleRow(d) {
    var cb = el('input', { type: 'checkbox', class: 'proj-toggle', data: { name: d.name } });
    cb.checked = d.enabled;
    return el('label', {
      class: 'proj-row',
      draggable: 'true',
      data: { name: d.name }
    }, [
      el('span', { class: 'proj-handle', text: '⋮⋮' }),
      cb,
      el('span', { class: 'proj-name', text: d.name }),
      el('span', { class: 'proj-path', text: d.path })
    ]);
  }

  function viewProjects() {
    var discovered = data.discovered || [];
    var projects = data.projects || [];
    var onCount = discovered.filter(function (d) { return d.enabled; }).length;
    var nodes = [];

    /* collapsible "tracked projects" settings panel */
    var head = el('div', { class: 'panel-head tracked-head' }, [
      el('span', { class: 'sigil', text: trackedOpen ? '▾' : '▸' }), ' ',
      el('span', { class: 'title-text', text: 'tracked projects' }),
      el('div', { class: 'right' }, [pill(onCount + ' / ' + discovered.length + ' tracked')])
    ]);
    var panelKids = [head];
    if (trackedOpen) {
      var rows = discovered.length
        ? discovered.map(projectToggleRow)
        : [emptyState(['no projects discovered — add a ',
          el('code', { text: 'scan_roots' }), ' entry to your config.'])];
      panelKids.push(el('div', { class: 'panel-body' }, rows));
    }
    nodes.push(el('section', { class: 'panel', 'data-vim-section': '' }, panelKids));

    /* one panel per visible project — current-state above roadmap */
    if (!projects.length) {
      var msg = discovered.length
        ? [el('b', { text: 'all hidden.' }), el('br'),
          'every discovered project is unchecked — re-check one above.']
        : ['no projects discovered yet — add a ',
          el('code', { text: 'scan_roots' }), ' entry to ',
          el('code', { text: '~/.config/harness-deck/config.json' }), '.'];
      nodes.push(panel('projects', null, [emptyState(msg)]));
      return nodes;
    }
    projects.forEach(function (p) {
      var body = [];
      if (p.has_state) {
        body.push(el('div', { class: 'proj-sec', text: 'current state' }));
        body.push(el('div', { class: 'roadmap-md' }, htmlToNodes(p.current_state_html)));
      }
      if (p.has_roadmap) {
        body.push(el('div', { class: 'proj-sec', text: 'roadmap' }));
        body.push(el('div', { class: 'roadmap-md' }, htmlToNodes(p.roadmap_html)));
      }
      if (!p.has_state && !p.has_roadmap) {
        body.push(emptyState(['no ', el('code', { text: '.docs/ai' }),
          ' docs for this project']));
      }
      (p.reports || []).forEach(function (r) { body.push(itemRow(r)); });
      var n = (p.reports || []).length;
      var right = n ? pill(n + ' roadmap report' + (n === 1 ? '' : 's')) : null;
      nodes.push(panel(p.project, right, body));
    });
    return nodes;
  }

  var BUILDERS = { inbox: viewInbox, overview: viewOverview, latest: viewLatest, projects: viewProjects };

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
    var th = e.target.closest('.tracked-head');
    if (th) { trackedOpen = !trackedOpen; render(); return; }
    var row = e.target.closest('[data-url]');
    if (row) { window.location.href = row.dataset.url; }
  });

  /* a checkbox in the tracked-projects panel hides or shows a project */
  document.addEventListener('change', function (e) {
    var cb = e.target.closest('.proj-toggle');
    if (cb) { toggleProject(cb.dataset.name); }
  });

  /* drag-and-drop reordering of the tracked-projects rows. HTML5 native dnd
     so no library — the whole .proj-row is draggable; we compute the drop
     position from the cursor's Y within the target row. */
  function clearDropMarkers() {
    document.querySelectorAll('.proj-row.drop-above, .proj-row.drop-below').forEach(function (r) {
      r.classList.remove('drop-above', 'drop-below');
    });
  }
  document.addEventListener('dragstart', function (e) {
    var row = e.target.closest('.proj-row');
    if (!row) return;
    draggedName = row.dataset.name;
    e.dataTransfer.effectAllowed = 'move';
    try { e.dataTransfer.setData('text/plain', draggedName); } catch (_) {}
    row.classList.add('dragging');
  });
  document.addEventListener('dragover', function (e) {
    var row = e.target.closest('.proj-row');
    if (!row || !draggedName || row.dataset.name === draggedName) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    var rect = row.getBoundingClientRect();
    var below = (e.clientY - rect.top) > rect.height / 2;
    clearDropMarkers();
    row.classList.add(below ? 'drop-below' : 'drop-above');
  });
  document.addEventListener('drop', function (e) {
    var row = e.target.closest('.proj-row');
    if (!row || !draggedName || row.dataset.name === draggedName) return;
    e.preventDefault();
    var rect = row.getBoundingClientRect();
    var below = (e.clientY - rect.top) > rect.height / 2;
    var target = row.dataset.name;
    var order = (data.discovered || []).map(function (d) { return d.name; })
      .filter(function (n) { return n !== draggedName; });
    var idx = order.indexOf(target);
    if (idx < 0) return;
    order.splice(below ? idx + 1 : idx, 0, draggedName);
    reorderProjects(order);
  });
  document.addEventListener('dragend', function () {
    draggedName = null;
    document.querySelectorAll('.proj-row.dragging').forEach(function (r) {
      r.classList.remove('dragging');
    });
    clearDropMarkers();
  });

  /* number keys 1-4 switch views, unless vim-nav owns the keystroke */
  document.addEventListener('keydown', function (e) {
    var t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
    if (window.VimNav && VimNav.getMode && VimNav.getMode() !== 'NORMAL') return;
    var v = VIEWS.find(function (x) { return x.key === e.key; });
    if (v) { showView(v.id); e.preventDefault(); }
  });

  /* reorderProjects persists the user's drag-and-drop result. */
  function reorderProjects(order) {
    fetch('/api/projects/reorder', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ order: order })
    }).then(function (r) {
      if (!r.ok) throw new Error('reorder HTTP ' + r.status);
    }).catch(function (err) {
      console.error('harness-deck: reorder failed', err);
    }).then(refresh);
  }

  /* toggleProject hides or shows a project server-side, then re-syncs. */
  function toggleProject(name) {
    fetch('/api/projects/toggle', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: name })
    }).then(function (r) {
      if (!r.ok) throw new Error('toggle HTTP ' + r.status);
    }).catch(function (err) {
      console.error('harness-deck: project toggle failed', err);
    }).then(refresh); // re-sync the checkbox to server truth either way
  }

  /* refresh fetches the report index and the projects view, then re-renders. */
  function refresh() {
    Promise.all([
      fetch('/api/reports').then(function (r) {
        if (!r.ok) throw new Error('reports HTTP ' + r.status);
        return r.json();
      }),
      fetch('/api/projects').then(function (r) {
        return r.ok ? r.json() : { projects: [], discovered: [] };
      }).catch(function () { return { projects: [], discovered: [] }; })
    ]).then(function (res) {
      data = {
        reports: res[0].reports || [],
        errors: res[0].errors || [],
        projects: res[1].projects || [],
        discovered: res[1].discovered || []
      };
      render();
    }).catch(function (err) {
      document.getElementById('content').replaceChildren(
        emptyState(['failed to load: ' + String(err)]));
    });
  }

  /* live updates — the server pushes a 'change' event when the report index
     changes on disk; EventSource reconnects on its own if the stream drops. */
  function connectEvents() {
    if (typeof EventSource === 'undefined') return;
    var es = new EventSource('/events');
    es.addEventListener('change', function () { refresh(); });
  }

  window.HarnessDeck = { reload: refresh };
  refresh();
  connectEvents();
})();
