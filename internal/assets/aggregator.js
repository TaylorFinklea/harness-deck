/* harness-deck aggregator — fetches the report index and renders the sidebar
   tree and the four switchable home views (inbox / overview / latest /
   projects). vim-nav.js handles scroll/search/command; this owns the data.

   The DOM is built with createElement + textContent only (never innerHTML),
   so report content authored by an agent can never inject markup. */
(function () {
  'use strict';

  /* v0.2.0 IA: two top-level views.
       - inbox: actionable now (awaiting + open-asks); archive is a filter, not a view
       - projects: long-term / strategic per-project surface
     Settings becomes an overlay (`,s` in phase 3; gear button on the
     titlebar in phase 2 for mouse access). Old "overview / latest /
     archive / settings" tabs are folded — see migrateLegacyURL(). */
  var VIEWS = [
    { id: 'inbox', label: 'inbox' },
    { id: 'projects', label: 'projects' },
    { id: 'activity', label: 'activity' }
  ];
  // Backlog view (beads/bd issues) is opt-in: the shell sets window.HD_BEADS
  // from config.beads.enabled, so a disabled deployment shows no extra tab.
  if (window.HD_BEADS) VIEWS.push({ id: 'backlog', label: 'backlog' });

  /* activeReports — non-archived only. Every default view operates on
     these so the inbox stays the "things you actually care about" list.
     The archive view reads data.reports directly and filters the other
     way. */
  function activeReports() {
    return (data.reports || []).filter(function (r) { return !r.archived; });
  }
  function archivedReports() {
    return (data.reports || []).filter(function (r) { return r.archived; });
  }
  var data = { reports: [], errors: [], projects: [], discovered: [] };
  var beadsData = { repos: [], available: false }; // Backlog view data (/api/beads)
  var currentView = 'inbox';
  var trackedOpen = false;     // is the "tracked projects" panel expanded?
  var draggedName = null;      // project name currently being dragged, or null
  /* archiveFilter: when true the inbox shows archived reports instead
     of active ones. Replaces the old standalone archive view. The 'a'
     keybinding (toggle archive) still works in either mode — it just
     means "archive" in one and "unarchive" in the other. */
  var archiveFilter = false;
  /* inboxSort — sort pivot for the inbox panel. Persisted in sessionStorage
     so the user's chosen sort survives a page reload mid-triage.
     'recent'  → created desc (default)
     'asks'    → open_asks desc, tie-break created desc
     'project' → project asc, tie-break created desc */
  var inboxSort = (function () {
    try { return sessionStorage.getItem('hd:inboxSort') || 'recent'; } catch (_) { return 'recent'; }
  })();
  /* collapsedSecs — project section collapse state. Keyed by
     project + '\x00' + sectionName → true when collapsed. */
  var collapsedSecs = (function () {
    try { return JSON.parse(sessionStorage.getItem('hd:collapsedSecs') || '{}'); } catch (_) { return {}; }
  })();
  /* Sidebar tree-focus mode (Space-e) lives in its own module,
     aggregator-tree.js, exposing window.HDTree. It owns treeFocused +
     treeActiveKey and all the j/k/Enter/p/Esc handling; the core just calls
     HDTree.paint() after each render and routes keys to it. */

  // Shared no-innerHTML DOM helpers (hd-dom.js, loaded first). Bound to locals
  // so the rest of this file reads unchanged.
  var el = window.HDDom.el,
    htmlToNodes = window.HDDom.htmlToNodes;

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

  /* isLive — true when a report has telemetry whose `updated` timestamp
     is within the live window. Same 60s window the report-page banner
     uses so the inbox dot and the banner go stale together. */
  var LIVE_WINDOW_MS = 60 * 1000;
  function isLive(r) {
    if (!r.live || !r.live.updated) return false;
    var t = Date.parse(r.live.updated);
    return isFinite(t) && (Date.now() - t) < LIVE_WINDOW_MS;
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
    var live = isLive(r);
    if (live && r.live.step) {
      sub.push(el('br'));
      sub.push(el('span', { class: 'live-mini', text: '● ' + r.live.step }));
    }
    var rowCls = 'inbox-item' + (live ? ' live' : '');
    return el('div', { class: rowCls, data: { url: reportURL(r), key: rowKey(r) } }, [
      el('span', { class: 'dot ' + r.status + (live ? ' live' : '') }),
      el('div', { class: 'main' }, [
        el('div', { class: 'title', text: r.title || r.run }),
        el('div', { class: 'sub' }, sub)
      ]),
      el('div', { class: 'aside' }, aside),
      el('button', {
        class: 'inbox-close',
        title: 'close (mark done)',
        data: { project: r.project, run: r.run }
      }, ['✕'])
    ]);
  }

  /* --- inbox cursor (v0.2.0 phase 1) ---
     Vim-style focused-row state for keyboard triage. The cursor is a
     report-id ("project\x00run"), not a row index — so archive / close
     / sort changes don't slide it onto the wrong row. Restored across
     refreshes via sessionStorage so the muscle memory survives a
     reload mid-triage. */
  var focusedKey = null;
  var ddArmedAt = 0; // timestamp of the first `d` in a `dd` sequence

  /* rowKey builds the stable id for one inbox row. Uses NUL as the
     separator because project + run never contain it. */
  function rowKey(r) { return r.project + '\x00' + r.run; }

  /* visibleRows is the list the cursor traverses on the current view.
     Mirrors the filter each view applies in renderContent — keeps the
     cursor in sync with what the user sees. Projects view has no row
     list; the cursor falls through to vim-nav scroll there. */
  function visibleRows() {
    if (currentView !== 'inbox') return [];
    if (archiveFilter) return archivedReports();
    return activeReports().filter(function (r) {
      return r.status === 'awaiting-review' || r.open_asks > 0;
    });
  }

  /* ensureFocused snaps focusedKey to the first visible row when the
     current target no longer exists in this view (archived, answered,
     filtered out). Returning null means "no rows to focus." */
  function ensureFocused() {
    var rows = visibleRows();
    if (!rows.length) { focusedKey = null; return null; }
    var found = rows.find(function (r) { return rowKey(r) === focusedKey; });
    if (!found) {
      focusedKey = rowKey(rows[0]);
      found = rows[0];
    }
    persistFocus();
    return found;
  }

  /* applyFocusHighlight toggles the .focused class on the row whose
     data-key matches focusedKey. JS-side equality match because data-key
     contains a NUL separator and CSS attribute selectors don't accept
     control characters. Cheap O(rows) DOM walk; called on every render
     + every cursor move. */
  function applyFocusHighlight() {
    document.querySelectorAll('.inbox-item.focused').forEach(function (el) {
      el.classList.remove('focused');
    });
    if (!focusedKey) return;
    var rows = document.querySelectorAll('.inbox-item[data-key]');
    for (var i = 0; i < rows.length; i++) {
      if (rows[i].dataset.key === focusedKey) {
        rows[i].classList.add('focused');
        scrollFocusedIntoView(rows[i]);
        return;
      }
    }
  }

  /* scrollFocusedIntoView keeps the cursor on screen without scrolling
     the whole page to the row's top — uses 'nearest' so j/k feel
     responsive instead of jumpy. */
  function scrollFocusedIntoView(el) {
    var rect = el.getBoundingClientRect();
    var winH = window.innerHeight;
    if (rect.top < 80 || rect.bottom > winH - 40) {
      el.scrollIntoView({ block: 'nearest', behavior: 'instant' });
    }
  }

  /* moveCursor advances focusedKey by delta within visibleRows. Clamps
     at the ends rather than wrapping — wrap would lose the user when
     a long list scrolls past where they expected. */
  function moveCursor(delta) {
    var rows = visibleRows();
    if (!rows.length) return;
    var current = ensureFocused();
    var idx = current ? rows.findIndex(function (r) { return rowKey(r) === focusedKey; }) : 0;
    var next = Math.max(0, Math.min(rows.length - 1, idx + delta));
    focusedKey = rowKey(rows[next]);
    persistFocus();
    applyFocusHighlight();
  }

  /* moveCursorTo jumps to top or bottom of the list — bound to gg / G. */
  function moveCursorTo(where) {
    var rows = visibleRows();
    if (!rows.length) return;
    focusedKey = rowKey(rows[where === 'top' ? 0 : rows.length - 1]);
    persistFocus();
    applyFocusHighlight();
  }

  /* openFocused navigates to the report page for the focused row. The
     :hover row never wins over the focused row — once you've moved the
     cursor by keyboard you stay in keyboard mode. */
  function openFocused() {
    var row = ensureFocused();
    if (row) window.location.href = reportURL(row);
  }

  /* actOnFocused dispatches close / archive / delete against the
     focused row. Uses the existing report-action endpoints so behavior
     stays identical to the mouse-driven buttons. After mutation the
     refresh() re-renders and ensureFocused snaps to the next row. */
  function actOnFocused(action) {
    var row = ensureFocused();
    if (!row) return;
    if (action === 'close') return closeReport(row.project, row.run);
    if (action === 'archive') {
      return reportAction(row.project, row.run, row.archived ? 'unarchive' : 'archive', 'POST');
    }
    if (action === 'delete') {
      if (!confirm('Delete "' + (row.title || row.run) + '"? This removes the run directory from disk.')) return;
      return reportAction(row.project, row.run, '', 'DELETE');
    }
  }

  /* persistFocus / restoreFocus — sessionStorage so the cursor
     survives F5 mid-triage. View-scoped so each view has its own last
     position. */
  function persistFocus() {
    try { sessionStorage.setItem('hd:focus:' + currentView, focusedKey || ''); } catch (_) {}
  }
  function restoreFocus() {
    try {
      var v = sessionStorage.getItem('hd:focus:' + currentView);
      if (v) focusedKey = v;
    } catch (_) {}
  }

  /* --- sidebar tree ---
     Two stacked sections: PINNED (the user's curated quick-jump list,
     keyed by digit 1-9 minus the dashboard) on top, REPORTS (the full
     project-grouped tree) below. Pinned items show as a flat list —
     no project grouping — because that's what makes 1..9 map cleanly
     to "the item at position N." */
  function pinnedItems() { return (window.HDPins && HDPins.load()) || []; }
  function isPinned(r) {
    return window.HDPins && HDPins.isPinned(r.project, r.run);
  }
  function togglePin(r) {
    if (!window.HDPins || !r) return;
    HDPins.toggle(r.project, r.run, r.title || r.run);
  }

  function renderTree() {
    var byProj = {};
    activeReports().forEach(function (r) {
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
        var rowKids = [
          el('span', { class: 'st ' + r.status }),
          el('span', { class: 'label', text: r.title || r.run }),
          el('span', { class: 'hbadge', text: r.harness })
        ];
        if (isPinned(r)) {
          rowKids.unshift(el('span', { class: 'pin-glyph', text: '★', title: 'pinned' }));
        }
        tree.appendChild(el('div', { class: 'row run' + (isPinned(r) ? ' is-pinned' : ''), data: { url: reportURL(r) } }, rowKids));
      });
    });

    /* PINNED section — flat list of the user's manually-pinned reports.
       Each row is a navigable .row.run (same shape as the main tree),
       so tree-focus j/k walks them naturally. The leading digit chip
       teaches the 2-9 shortcut. Section is hidden when no pins. */
    var pins = pinnedItems();
    var pinnedTree = null;
    if (pins.length) {
      pinnedTree = el('div', { class: 'tree pinned-tree' });
      pins.forEach(function (p, i) {
        pinnedTree.appendChild(el('div', {
          class: 'row run pinned', data: { url: reportURL(p) }
        }, [
          el('span', { class: 'pin-num', text: String(i + 2) }),
          el('span', { class: 'label', text: p.title || p.run }),
          el('span', { class: 'hbadge', text: p.project })
        ]));
      });
    }

    /* SAVED section — flat list of the user's named search queries. Each row
       carries data-saved (the query string) instead of data-url so it never
       navigates directly; clicks are handled by the delegated handler below.
       Section is hidden when no saved searches exist. */
    var savedItems = (window.HDSaved && HDSaved.list()) || [];
    var savedTree = null;
    if (savedItems.length) {
      savedTree = el('div', { class: 'tree saved-tree' });
      savedItems.forEach(function (item) {
        savedTree.appendChild(el('div', {
          class: 'row saved', data: { saved: item.query }
        }, [
          el('span', { class: 'saved-glyph', text: '⌕' }),
          el('span', { class: 'label', text: item.name }),
          el('button', { class: 'saved-del', title: 'remove' }, ['✕'])
        ]));
      });
    }

    var sections = [];
    if (savedTree) {
      sections.push(el('div', { class: 'sidebar-section' }, [
        el('div', { class: 'sidebar-title', text: 'saved' }), savedTree
      ]));
    }
    if (pinnedTree) {
      sections.push(el('div', { class: 'sidebar-section' }, [
        el('div', { class: 'sidebar-title', text: 'pinned · p' }), pinnedTree
      ]));
    }
    sections.push(el('div', { class: 'sidebar-section' }, [
      el('div', { class: 'sidebar-title', text: 'reports' }), tree
    ]));

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

  /* metricChip — one cell in an operational metric strip. value is the
     bold number; label is the small caption beside it. When chip.onClick
     is set, the chip becomes a navigable filter pre-set. */
  function metricChip(value, label, opts) {
    opts = opts || {};
    var attrs = { class: 'metric-chip' + (opts.active ? ' active' : '') + (opts.click ? ' clickable' : '') };
    if (opts.click) attrs.data = { action: opts.click };
    return el('div', attrs, [
      el('span', { class: 'value', text: String(value) }),
      el('span', { class: 'label', text: label })
    ]);
  }

  function metricStrip(chips) {
    return el('div', { class: 'metric-strip' }, chips);
  }

  /* createdInLast24h — true when a report's `created` (RFC3339) is
     within the last 24 hours of now. Defensive about parse failures
     (an unparseable timestamp is treated as old). */
  function createdInLast24h(iso) {
    if (!iso) return false;
    var t = Date.parse(iso);
    return isFinite(t) && (Date.now() - t) < 24 * 60 * 60 * 1000;
  }

  function viewInbox() {
    if (archiveFilter) return viewInboxArchived();
    var active = activeReports();
    var items = active.filter(function (r) {
      return r.status === 'awaiting-review' || r.open_asks > 0;
    });
    var openAsks = active.reduce(function (s, r) { return s + (r.open_asks || 0); }, 0);
    var inFlight = active.filter(isLive).length;
    var today = active.filter(function (r) { return createdInLast24h(r.created); }).length;
    var archivedCount = archivedReports().length;

    var strip = metricStrip([
      metricChip(items.length, 'awaiting'),
      metricChip(openAsks, 'open asks'),
      metricChip(inFlight, 'in-flight'),
      metricChip(today, 'today'),
      metricChip(archivedCount, 'archived', { click: 'toggle-archive' })
    ]);

    /* apply inboxSort */
    items = items.slice().sort(function (a, b) {
      if (inboxSort === 'asks') {
        var diff = (b.open_asks || 0) - (a.open_asks || 0);
        if (diff !== 0) return diff;
      } else if (inboxSort === 'project') {
        var pc = (a.project || '').localeCompare(b.project || '');
        if (pc !== 0) return pc;
      }
      return (b.created || '').localeCompare(a.created || '');
    });
    var sortToggle = el('span', { class: 'sort-toggle', text: 'sort: ' + inboxSort });
    var body = items.length
      ? items.map(itemRow)
      : [emptyState([el('b', { text: 'nothing needs you.' }), el('br'),
        'no reports are awaiting review.'])];
    var rightParts = items.length ? [sortToggle, pill(items.length + ' awaiting', 'warn')] : [sortToggle];
    var rightWrap = el('div', { class: 'inbox-panel-right' }, rightParts);
    return [strip, panel('inbox — needs you', rightWrap, body)];
  }

  /* viewInboxArchived — the archive filter version of viewInbox. Same
     strip, but the action chip says "back to inbox" instead, the panel
     title reads "archive", and the row list is the archived set. */
  function viewInboxArchived() {
    var rs = archivedReports();
    var active = activeReports();
    var openAsks = active.reduce(function (s, r) { return s + (r.open_asks || 0); }, 0);

    var strip = metricStrip([
      metricChip(active.filter(function (r) { return r.status === 'awaiting-review' || r.open_asks > 0; }).length, 'awaiting'),
      metricChip(openAsks, 'open asks'),
      metricChip(active.filter(isLive).length, 'in-flight'),
      metricChip(active.filter(function (r) { return createdInLast24h(r.created); }).length, 'today'),
      metricChip(rs.length, 'archived', { click: 'toggle-archive', active: true })
    ]);

    if (!rs.length) {
      return [strip, panel('archive — empty', null, [emptyState([
        'nothing archived. ',
        el('b', { text: 'tip:' }),
        ' archive instead of delete to keep a report\'s files around in case you need them.'
      ])])];
    }
    return [strip, panel('archive', pill(rs.length + ' archived'), rs.map(itemRow))];
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

    /* metric strip: at-a-glance state across all tracked projects.
       "updated this week" counts projects whose history has any entry
       created in the last 7 days; "with asks" counts projects whose
       roadmap reports or history entries carry open asks. Latest is
       the most-recent any-kind report's project name. */
    var latestReport = (data.reports || []).slice().sort(function (a, b) {
      return (b.created || '').localeCompare(a.created || '');
    })[0];
    var weekAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
    function projectIsFresh(p) {
      return (p.history || []).some(function (h) { return Date.parse(h.created) > weekAgo; });
    }
    function projectHasAsks(p) {
      return (p.history || []).some(function (h) { return h.open_asks > 0; });
    }
    nodes.push(metricStrip([
      metricChip(projects.length, 'projects'),
      metricChip(projects.filter(projectIsFresh).length, 'updated this week'),
      metricChip(projects.filter(projectHasAsks).length, 'with asks'),
      metricChip(latestReport ? latestReport.project : '—', 'latest update')
    ]));

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
        var stateKey = p.project + '\x00current state';
        var stateCollapsed = !!collapsedSecs[stateKey];
        body.push(el('div', {
          class: 'proj-sec collapsible',
          data: { proj: p.project, sec: 'current state' }
        }, [
          el('span', { class: 'sec-sigil', text: stateCollapsed ? '▸ ' : '▾ ' }),
          'current state'
        ]));
        if (!stateCollapsed) {
          body.push(el('div', { class: 'roadmap-md' }, htmlToNodes(p.current_state_html)));
        }
      }
      if (p.has_roadmap) {
        var roadmapKey = p.project + '\x00roadmap';
        var roadmapCollapsed = !!collapsedSecs[roadmapKey];
        body.push(el('div', {
          class: 'proj-sec collapsible',
          data: { proj: p.project, sec: 'roadmap' }
        }, [
          el('span', { class: 'sec-sigil', text: roadmapCollapsed ? '▸ ' : '▾ ' }),
          'roadmap'
        ]));
        if (!roadmapCollapsed) {
          body.push(el('div', { class: 'roadmap-md' }, htmlToNodes(p.roadmap_html)));
        }
      }
      if (!p.has_state && !p.has_roadmap) {
        body.push(emptyState(['no ', el('code', { text: '.docs/ai' }),
          ' docs for this project']));
      }
      (p.reports || []).forEach(function (r) { body.push(itemRow(r)); });
      var hist = p.history || [];
      if (hist.length) {
        var total = p.history_total || hist.length;
        var label = total > hist.length
          ? 'history — newest ' + hist.length + ' of ' + total
          : 'history';
        body.push(el('div', { class: 'proj-sec', text: label }));
        body.push(el('div', { class: 'history' }, hist.map(historyRow)));
      }
      var n = (p.reports || []).length;
      var right = n ? pill(n + ' roadmap report' + (n === 1 ? '' : 's')) : null;
      nodes.push(panel(p.project, right, body));
    });
    return nodes;
  }

  /* historyRow — one run in the project-history timeline. Unlike itemRow
     (which is triage-focused: close button, link to report), this row is a
     retrospective surface: the answers the user recorded show inline so the
     project panel doubles as a record of what was decided. */
  function historyRow(r) {
    var meta = [el('span', { class: 'hist-time', text: shortTime(r.created) })];
    if (r.kind) meta.push(el('span', { class: 'hist-kind', text: r.kind }));
    if (r.harness) meta.push(el('span', { class: 'hist-harness', text: r.harness }));
    if (r.archived) meta.push(el('span', { class: 'hist-flag archived', text: 'archived' }));
    if (r.open_asks > 0) meta.push(el('span', { class: 'hist-flag asks', text: r.open_asks + ' open' }));

    var head = el('div', { class: 'hist-head', data: { url: reportURL(r) } }, [
      el('span', { class: 'dot ' + r.status }),
      el('div', { class: 'hist-title', text: r.title || r.run }),
      el('div', { class: 'hist-meta' }, meta)
    ]);

    var kids = [head];
    var resps = r.responses || [];
    if (resps.length) {
      kids.push(el('div', { class: 'hist-resps' }, resps.map(function (a) {
        var bits = [
          el('span', { class: 'r-block', text: a.block }),
          el('span', { class: 'r-arrow', text: ' → ' }),
          el('span', { class: 'r-value', text: a.value || '—' })
        ];
        if (a.note) {
          bits.push(el('span', { class: 'r-note', text: ' · ' + a.note }));
        }
        if (a.at) bits.push(el('span', { class: 'r-at', text: ' · ' + shortTime(a.at) }));
        return el('div', { class: 'hist-resp' }, bits);
      })));
    }
    return el('div', { class: 'hist-row' }, kids);
  }

  /* Settings overlay (v0.2.0): the same content viewSettings() builds,
     but rendered into a modal instead of a top-level view. The function
     stays here so we can also point an inline-link from elsewhere if
     ever needed; openSettingsOverlay() drives the modal lifecycle.

     openSettingsOverlay creates the DOM lazily on first open so it
     never costs paint time for users who never trigger it. */
  function openSettingsOverlay() {
    var existing = document.getElementById('settings-overlay');
    if (existing) {
      existing.style.display = 'flex';
      return;
    }
    var body = el('div', { class: 'settings-modal-body' }, viewSettings());
    var modal = el('div', { class: 'settings-modal' }, [
      el('div', { class: 'settings-modal-head' }, [
        el('div', { class: 'settings-modal-title', text: 'settings' }),
        el('button', { type: 'button', class: 'settings-modal-close', title: 'close (Esc)' }, ['✕'])
      ]),
      body
    ]);
    var overlay = el('div', { id: 'settings-overlay', class: 'settings-overlay' }, [modal]);
    document.body.appendChild(overlay);
  }
  function closeSettingsOverlay() {
    var o = document.getElementById('settings-overlay');
    if (o) o.style.display = 'none';
  }
  function toggleSettingsOverlay() {
    var o = document.getElementById('settings-overlay');
    if (o && o.style.display !== 'none') closeSettingsOverlay();
    else openSettingsOverlay();
  }

  /* The context-aware help overlay (the `?` cheat sheet) lives in its own
     module, aggregator-help.js, exposing window.HDHelp = { open, close }. The
     overlay shares the dismiss vocabulary with settings (Esc, click scrim, ✕
     button) — those handlers stay here; only the builder + open/close moved. */

  /* viewActivity — cross-project, cross-harness chronological list of all
     non-archived reports, grouped by day (newest first). Answers "what has
     all my AI work been doing lately?". */
  function viewActivity() {
    var reports = (data.reports || []).filter(function (r) { return !r.archived; });
    reports = reports.slice().sort(function (a, b) {
      return (b.created || '').localeCompare(a.created || '');
    });

    /* unique project + harness counts */
    var projectSet = {};
    var harnessSet = {};
    var todayCount = 0;
    reports.forEach(function (r) {
      if (r.project) projectSet[r.project] = true;
      if (r.harness) harnessSet[r.harness] = true;
      if (createdInLast24h(r.created)) todayCount++;
    });

    var strip = metricStrip([
      metricChip(reports.length, 'reports'),
      metricChip(Object.keys(projectSet).length, 'projects'),
      metricChip(Object.keys(harnessSet).length, 'harnesses'),
      metricChip(todayCount, 'today')
    ]);

    if (!reports.length) {
      return [strip, emptyState([el('b', { text: 'no activity yet.' }), el('br'),
        'reports will appear here as AI sessions run.'])];
    }

    /* group by day (YYYY-MM-DD from r.created) */
    var pad2 = function (n) { return String(n).padStart(2, '0'); };
    /* Local day key — matches the local HH:MM shown on each row, so a report
       written near midnight isn't bucketed under a UTC date that disagrees
       with its displayed time. Unparseable timestamps fall back to the slice. */
    function localDay(iso) {
      if (!iso) return 'unknown';
      var d = new Date(iso);
      if (isNaN(d.getTime())) return iso.slice(0, 10) || 'unknown';
      return d.getFullYear() + '-' + pad2(d.getMonth() + 1) + '-' + pad2(d.getDate());
    }
    var days = [];      /* ordered list of day strings */
    var byDay = {};     /* day → [report, …] */
    reports.forEach(function (r) {
      var day = localDay(r.created);
      if (!byDay[day]) { days.push(day); byDay[day] = []; }
      byDay[day].push(r);
    });

    var nodes = [strip];
    days.forEach(function (day) {
      var dayReports = byDay[day];
      /* day header */
      nodes.push(el('div', { class: 'activity-day' }, [
        el('span', { class: 'activity-day-label', text: day }),
        el('span', { class: 'activity-day-count', text: dayReports.length + '' })
      ]));
      /* one row per report */
      dayReports.forEach(function (r) {
        var timeStr = '';
        if (r.created) {
          var d = new Date(r.created);
          if (!isNaN(d.getTime())) {
            timeStr = pad2(d.getHours()) + ':' + pad2(d.getMinutes());
          }
        }
        var kids = [
          el('span', { class: 'act-time', text: timeStr }),
          el('span', { class: 'act-project', text: r.project || '' })
        ];
        if (r.harness) kids.push(el('span', { class: 'hbadge act-harness', text: r.harness }));
        kids.push(el('span', { class: 'st ' + (r.status || '') }));
        if (r.kind) kids.push(el('span', { class: 'act-kind', text: r.kind }));
        kids.push(el('span', { class: 'act-title', text: r.title || r.run }));
        nodes.push(el('div', { class: 'activity-row', data: { url: reportURL(r) } }, kids));
      });
    });
    return nodes;
  }

  /* BUILDERS maps view id → builder function. v0.2.0 keeps just the
     two top-level views; viewSettings is now rendered into a modal
     overlay (settingsOverlayBody) instead. */
  /* viewBacklog — the beads Backlog view. Delegates all rendering (lists, the
     inline-SVG dependency graph, drill-in) to the HDBacklog module, which owns
     its own cursor + detail state. */
  function viewBacklog() {
    return window.HDBacklog ? HDBacklog.render(beadsData) : [emptyState(['backlog unavailable'])];
  }

  var BUILDERS = { inbox: viewInbox, projects: viewProjects, activity: viewActivity, backlog: viewBacklog };

  function renderContent() {
    var tabs = el('div', { class: 'view-tabs' }, VIEWS.map(function (v) {
      return el('div', { class: 'view-tab', data: { view: v.id } }, [v.label]);
    }));
    var nodes = [tabs];
    VIEWS.forEach(function (v) {
      nodes.push(el('div', { class: 'view', id: 'view-' + v.id }, BUILDERS[v.id]()));
    });
    var content = document.getElementById('content');
    content.replaceChildren.apply(content, nodes);
  }

  function showView(id) {
    var changed = currentView !== id;
    currentView = id;
    document.querySelectorAll('.view').forEach(function (v) {
      v.classList.toggle('active', v.id === 'view-' + id);
    });
    document.querySelectorAll('.view-tab').forEach(function (t) {
      t.classList.toggle('active', t.dataset.view === id);
    });
    if (changed) {
      // Each view has its own remembered cursor position; reload it
      // when the user switches in.
      focusedKey = null;
      restoreFocus();
      ensureFocused();
      applyFocusHighlight();
    }
  }

  /* refreshAsksCount surfaces the aggregate open-asks count as a chip in
     the titlebar and in document.title — visible everywhere, including
     the browser/PWA tab strip and (on iOS) the home-screen badge area. */
  function refreshAsksCount() {
    var n = (data.reports || []).reduce(function (acc, r) {
      return acc + (r.open_asks || 0);
    }, 0);
    var chip = document.getElementById('title-asks');
    if (chip) {
      if (n > 0) {
        chip.textContent = ' · ' + n + ' open';
        chip.classList.add('has-asks');
      } else {
        chip.textContent = '';
        chip.classList.remove('has-asks');
      }
    }
    document.title = n > 0 ? '(' + n + ') harness-deck' : 'harness-deck';
  }

  function render() {
    renderTree();
    renderContent();
    showView(currentView);
    refreshAsksCount();
    // Cursor-state lives across renders. ensureFocused snaps to a valid
    // row if the previous one got filtered out, then the highlight
    // gets painted on the freshly-built DOM.
    ensureFocused();
    applyFocusHighlight();
    // Tree-focus state survives renders too — repaint after the new
    // tree DOM is in place.
    HDTree.paint();
    // Backlog cursor + open detail survive renders too.
    if (window.HDBacklog) HDBacklog.paint();
  }

  /* one delegated click handler for every navigable row */
  document.addEventListener('click', function (e) {
    var tab = e.target.closest('.view-tab');
    if (tab) { showView(tab.dataset.view); return; }
    var secHead = e.target.closest('.proj-sec.collapsible');
    if (secHead) {
      var secKey = secHead.dataset.proj + '\x00' + secHead.dataset.sec;
      collapsedSecs[secKey] = !collapsedSecs[secKey];
      try { sessionStorage.setItem('hd:collapsedSecs', JSON.stringify(collapsedSecs)); } catch (_) {}
      render();
      return;
    }
    var th = e.target.closest('.tracked-head');
    if (th) { trackedOpen = !trackedOpen; render(); return; }
    var closeBtn = e.target.closest('.inbox-close');
    if (closeBtn) {
      e.stopPropagation();
      closeReport(closeBtn.dataset.project, closeBtn.dataset.run);
      return;
    }
    var restoreBtn = e.target.closest('.archive-restore');
    if (restoreBtn) {
      e.stopPropagation();
      reportAction(restoreBtn.dataset.project, restoreBtn.dataset.run, 'unarchive', 'POST');
      return;
    }
    var hardDelBtn = e.target.closest('.archive-delete');
    if (hardDelBtn) {
      e.stopPropagation();
      if (!confirm('Permanently delete this archived report? This removes the run directory from disk.')) return;
      reportAction(hardDelBtn.dataset.project, hardDelBtn.dataset.run, '', 'DELETE');
      return;
    }
    if (e.target.id === 'push-on') { enablePushHere(); return; }
    if (e.target.id === 'push-off') { disablePushHere(); return; }
    var themeBtn = e.target.closest('.theme-btn');
    if (themeBtn) { setTheme(themeBtn.dataset.theme); return; }
    var sortBtn = e.target.closest('.sort-toggle');
    if (sortBtn) {
      e.stopPropagation();
      var sorts = ['recent', 'asks', 'project'];
      inboxSort = sorts[(sorts.indexOf(inboxSort) + 1) % sorts.length];
      try { sessionStorage.setItem('hd:inboxSort', inboxSort); } catch (_) {}
      render();
      return;
    }
    var chip = e.target.closest('.metric-chip.clickable');
    if (chip) {
      e.stopPropagation();
      if (chip.dataset.action === 'toggle-archive') {
        archiveFilter = !archiveFilter;
        focusedKey = null; // archive set is disjoint from active set
        try { sessionStorage.removeItem('hd:focus:inbox'); } catch (_) {}
        render();
      }
      return;
    }
    if (e.target.closest('#titlebar-settings')) {
      e.stopPropagation();
      openSettingsOverlay();
      return;
    }
    if (e.target.closest('.help-overlay .settings-modal-close') || e.target.classList.contains('help-overlay')) {
      HDHelp.close();
      return;
    }
    if (e.target.closest('.settings-modal-close') || e.target.classList.contains('settings-overlay')) {
      closeSettingsOverlay();
      return;
    }
    var testBtn = e.target.closest('.dest-test');
    if (testBtn) { e.stopPropagation(); testDestination(testBtn.dataset.name); return; }
    var removeBtn = e.target.closest('.dest-remove');
    if (removeBtn) { e.stopPropagation(); removeDestination(removeBtn.dataset.name); return; }
    var del = e.target.closest('.saved-del');
    if (del) {
      var r = del.closest('[data-saved]');
      if (r && window.HDSaved) HDSaved.remove(r.dataset.saved);
      return;
    }
    var sv = e.target.closest('[data-saved]');
    if (sv) { if (window.HDSearch) HDSearch.open(sv.dataset.saved); return; }
    var row = e.target.closest('[data-url]');
    if (row) { window.location.href = row.dataset.url; }
  });

  /* destination-form submit: POST then refresh + reset on success */
  document.addEventListener('submit', function (e) {
    var form = e.target.closest('#dest-form');
    if (!form) return;
    e.preventDefault();
    var submitBtn = form.querySelector('button[type="submit"]');
    if (submitBtn) { submitBtn.disabled = true; submitBtn.textContent = 'saving…'; }
    addDestination(new FormData(form))
      .then(function () {
        form.reset();
        refreshDestinations();
      })
      .catch(function (err) { alert('add failed: ' + err.message); })
      .finally(function () {
        if (submitBtn) { submitBtn.disabled = false; submitBtn.textContent = 'add destination'; }
      });
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

  /* --- v0.2.0 phase 3: chord state machine ---
     The dashboard supports two prefix chords:
       Space (leader): Space + s/t/? → settings / theme / cheat
       g       (jump): g + i/p/a/g → inbox / projects / archive / top
     Pending chord clears automatically after 700ms so a stale half-
     chord never poisons the next keystroke. We track this in capture
     phase so vim-nav never sees the chord-prefix keys (its own gg /
     space-scroll bindings would otherwise fire alongside). */
  var pendingChord = '';
  var chordTimeoutId = 0;
  function setPendingChord(c) {
    pendingChord = c;
    if (chordTimeoutId) clearTimeout(chordTimeoutId);
    // 1500ms matches vim's default `timeoutlen` — long enough for a
    // deliberate two-key chord, short enough that a stale prefix from
    // a misfire doesn't poison the next intentional keystroke.
    if (c) chordTimeoutId = setTimeout(function () { pendingChord = ''; }, 1500);
  }

  /* --- tree-explorer focus (Space-e) ---
     Mirrors a NeoVim file-tree workflow: the sidebar becomes the
     keyboard-focused surface, j/k walks visible report rows, Enter
     opens the active one (real navigation), Space-e or Esc returns
     focus to the main content. The "active row" survives renders by
     report id (same pattern as the inbox cursor). */

  /* cycleTheme — Space+t shortcut. Walks system → dark → light → system,
     touching the same localStorage key the picker uses so persistence
     stays consistent. */
  function cycleTheme() {
    var cur = (function () {
      try { return localStorage.getItem('harness-deck:theme') || 'system'; }
      catch (_) { return 'system'; }
    })();
    var next = cur === 'system' ? 'dark' : cur === 'dark' ? 'light' : 'system';
    setTheme(next);
  }

  /* in-app tabs (1-9): switch to tab N when on the dashboard. In-report
     contexts already bind digits to ask-option picking (triage.js); we
     defer to that by skipping digits when window.HD_REPORT is set. */
  function switchToTabN(n) {
    if (window.HD_REPORT) return false; // report page owns digits
    try {
      var tabs = pinnedItems();
      // Tab 1 is the dashboard itself (always present), tabs 2..N+1 are
      // the pinned reports in order. So digit i picks index i-1 across
      // the combined list (dashboard first).
      var idx = n - 1;
      if (idx === 0) { window.location.href = '/'; return true; }
      var tab = tabs[idx - 1];
      if (tab) { window.location.href = reportURL(tab); return true; }
    } catch (_) {}
    return false;
  }

  /* g-chord completions for the dashboard. tabs.js is the single
     g-chord state machine (it loads on both the shell and report
     pages); we register only the in-place completions it should
     prefer over its URL-jump built-ins. The registry bootstrap is
     identical in both files — whichever loads first creates it. */
  var HDKeys = (window.HDKeys = window.HDKeys || {
    pendingPrefix: '',
    _chords: {},
    chord: function (prefix, key, fn) { this._chords[prefix + key] = fn; },
  });
  HDKeys.chord('g', 'i', function () { archiveFilter = false; render(); showView('inbox'); });
  HDKeys.chord('g', 'p', function () { showView('projects'); });
  HDKeys.chord('g', 'a', function () { showView('inbox'); archiveFilter = true; render(); });
  HDKeys.chord('g', 'l', function () { showView('activity'); });
  if (window.HD_BEADS) HDKeys.chord('g', 'b', function () { showView('backlog'); refreshBeads(); });
  HDKeys.chord('g', 'g', function () { window.scrollTo({ top: 0, behavior: 'instant' }); });

  /* Inbox cursor key handler — capture phase so we intercept j/k
     before vim-nav's page-scroll binding fires. Also owns the Space
     leader and digit-to-tab routing; the g chord belongs to tabs.js. */
  document.addEventListener('keydown', function (e) {
    var t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
    if (window.VimNav && VimNav.getMode && VimNav.getMode() !== 'NORMAL') return;
    // Modifier keys belong to the browser / OS — don't intercept.
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    // A pending g-chord belongs to tabs.js — stand down so the
    // completion key reaches its bubble-phase handler untouched
    // (otherwise our single-key row bindings eat it in capture phase).
    if (HDKeys.pendingPrefix) return;

    function consume() { e.preventDefault(); e.stopImmediatePropagation(); }

    // --- chord dispatch: a pending prefix (Space or g) consumes the
    // current key as the chord completion. ---
    if (pendingChord === ' ') {
      setPendingChord('');
      switch (e.key) {
        case 's': toggleSettingsOverlay(); consume(); return;
        case 't': cycleTheme(); consume(); return;
        case 'e': HDTree.toggle(); consume(); return;
        case '?': HDHelp.open(); consume(); return;
        case 'Escape': consume(); return; // cancel
      }
      // Any other key after Space: just consume; user can re-type.
      consume();
      return;
    }
    // --- tree focus mode owns j/k/Enter/Esc when active ---
    if (HDTree.isFocused()) {
      switch (e.key) {
        case 'j': HDTree.moveCursor(+1); consume(); return;
        case 'k': HDTree.moveCursor(-1); consume(); return;
        case 'Enter':
        case 'o': HDTree.open(); consume(); return;
        case 'p': HDTree.pin(); consume(); return;
        case 'Escape': HDTree.exit(); consume(); return;
      }
      // Pass other keys (including Space for the leader) through so
      // Space-e can toggle the tree off.
    }

    // --- single-key dispatch ---
    // Digit → in-app tab N.
    if (/^[1-9]$/.test(e.key)) {
      if (switchToTabN(parseInt(e.key, 10))) { consume(); return; }
    }
    if (e.key === ' ') { setPendingChord(' '); consume(); return; }
    // `g` deliberately NOT consumed: it bubbles to tabs.js, the single
    // g-chord owner, which arms and dispatches (our registered
    // completions above included).
    if (e.key === '?') { HDHelp.open(); consume(); return; }

    // --- Backlog view owns j/k/G/Enter/Esc over its own issue rows ---
    if (currentView === 'backlog' && window.HDBacklog) {
      switch (e.key) {
        case 'j': HDBacklog.moveCursor(+1); consume(); return;
        case 'k': HDBacklog.moveCursor(-1); consume(); return;
        case 'G': HDBacklog.moveCursorTo('bottom'); consume(); return;
        case 'Enter':
        case 'o': HDBacklog.openFocused(); consume(); return;
        case 'Escape': if (HDBacklog.closeDetail()) { consume(); return; } break;
        // writable-only actions (c claim, x close, n new)
        case 'c': if (window.HD_BEADS_WRITABLE) { HDBacklog.claimFocused(); consume(); return; } break;
        case 'x': if (window.HD_BEADS_WRITABLE) { HDBacklog.closeFocused(); consume(); return; } break;
        case 'n': if (window.HD_BEADS_WRITABLE) { HDBacklog.newFocusedRepo(); consume(); return; } break;
      }
    }

    // Below: row actions, only meaningful when a row list is visible.
    if (!visibleRows().length) return;

    switch (e.key) {
      case 'j': moveCursor(+1); consume(); return;
      case 'k': moveCursor(-1); consume(); return;
      case 'G': moveCursorTo('bottom'); consume(); return;
      case 'Enter':
      case 'o': openFocused(); consume(); return;
      case 'a': actOnFocused('archive'); consume(); return;
      case 'x': actOnFocused('close'); consume(); return;
      case 'p':
        // Pin/unpin the inbox-focused row.
        var fr = ensureFocused();
        if (fr) togglePin(fr);
        consume();
        return;
      case 'd':
        if (Date.now() - ddArmedAt < 500) {
          ddArmedAt = 0;
          actOnFocused('delete');
        } else {
          ddArmedAt = Date.now();
        }
        consume();
        return;
    }
  }, true);

  /* closeReport marks a report done server-side, then re-syncs the list. */
  function closeReport(project, run) {
    fetch('/r/' + encodeURIComponent(project) + '/' + encodeURIComponent(run) + '/close', {
      method: 'POST'
    }).then(function (r) {
      if (!r.ok) throw new Error('close HTTP ' + r.status);
    }).catch(function (err) {
      console.error('harness-deck: close failed', err);
    }).then(refresh);
  }

  /* archive endpoints mirror close — fire-and-refresh. */
  function reportAction(project, run, action, method) {
    var base = '/r/' + encodeURIComponent(project) + '/' + encodeURIComponent(run);
    fetch(action ? base + '/' + action : base, { method: method }).then(function (r) {
      if (!r.ok) throw new Error(action + ' HTTP ' + r.status);
    }).catch(function (err) {
      console.error('harness-deck:', action, 'failed', err);
    }).then(refresh);
  }

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

  /* refreshBeads pulls the /api/beads snapshot for the Backlog view. Fired on
     the SSE 'beads' event, on entering the view, and once at startup. Tolerant:
     a failure leaves the last good data. Always re-renders so the (pre-built)
     backlog view picks up fresh data even from another view; render() preserves
     the inbox cursor, so the extra render is cheap and harmless. */
  function refreshBeads() {
    fetch('/api/beads', { cache: 'no-store' })
      .then(function (r) { return r.ok ? r.json() : { repos: [], available: false }; })
      .then(function (d) {
        beadsData = d || { repos: [], available: false };
        if (window.HDBacklog) HDBacklog.setData(beadsData);
        // Re-render so the (pre-built) backlog view picks up fresh data even
        // when the user is on another view. render() preserves the inbox cursor.
        render();
      })
      .catch(function () {});
  }

  // After a backlog write, HDBacklog fires this for an immediate refresh
  // (belt-and-suspenders alongside the server's 'beads' SSE broadcast).
  document.addEventListener('hd:beads-refresh', function () { if (window.HD_BEADS) refreshBeads(); });

  /* live updates — the server pushes a 'change' event when the report index
     changes on disk; EventSource reconnects on its own if the stream drops. */
  function connectEvents() {
    if (typeof EventSource === 'undefined') return;
    var es = new EventSource('/events');
    es.addEventListener('change', function () { refresh(); });
    // 'beads' fires when the beads snapshot changes on disk (bd writes).
    es.addEventListener('beads', function () { refreshBeads(); });
  }

  /* migrateLegacyURL — old `?v=overview|latest|archive|settings`
     URLs now map onto the v0.2.0 IA. Settings becomes the overlay;
     archive becomes the filter; overview/latest just redirect to
     inbox. Rewrites the URL via history.replaceState so refreshes
     don't keep re-triggering the migration. */
  function migrateLegacyURL() {
    var params = new URLSearchParams(window.location.search);
    var v = params.get('v');
    var openSettings = false;
    var enableArchive = params.get('archive') === '1';
    if (v === 'archive') { enableArchive = true; params.delete('v'); }
    else if (v === 'settings') { openSettings = true; params.delete('v'); }
    else if (v === 'overview' || v === 'latest') { params.delete('v'); }
    else if (v === 'projects') {
      // Land on the projects view — applied via currentView before the
      // first render so we don't briefly flash inbox.
      currentView = 'projects';
      params.delete('v');
    }
    else if (v === 'activity') {
      currentView = 'activity';
      params.delete('v');
    }
    else if (v === 'inbox') {
      currentView = 'inbox';
      params.delete('v');
    }
    if (enableArchive) {
      archiveFilter = true;
      // Once consumed the param is purely cosmetic; drop it from the URL
      // so a refresh after the user un-toggles archive doesn't snap them
      // back in.
      params.delete('archive');
    }
    if (params.get('settings') === '1') {
      openSettings = true;
      params.delete('settings');
    }
    var search = params.toString();
    history.replaceState(null, '', window.location.pathname + (search ? '?' + search : ''));
    if (openSettings) setTimeout(openSettingsOverlay, 0);
  }

  /* Esc closes the settings overlay when it's the topmost layer. The
     vim-nav Esc handler runs too but only resets its own prompt state;
     no conflict. */
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape') return;
    var help = document.getElementById('help-overlay');
    if (help && help.style.display !== 'none') {
      HDHelp.close();
      e.preventDefault();
      return;
    }
    var o = document.getElementById('settings-overlay');
    if (o && o.style.display !== 'none') {
      closeSettingsOverlay();
      e.preventDefault();
    }
  });

  /* Register `:`-prefixed commands with vim-nav so the command palette
     (Tab-autocomplete prompt) can drive every overlay + view switch.
     The names track the help overlay (`:cheat`) so users see a single
     vocabulary. */
  function registerCommands() {
    if (!(window.VimNav && VimNav.addCommand)) return;
    VimNav.addCommand('inbox', function () { archiveFilter = false; render(); showView('inbox'); }, 'go to inbox');
    VimNav.addCommand('projects', function () { showView('projects'); }, 'go to projects');
    VimNav.addCommand('activity', function () { showView('activity'); }, 'go to activity timeline');
    if (window.HD_BEADS) VimNav.addCommand('backlog', function () { showView('backlog'); refreshBeads(); }, 'go to the beads backlog');
    VimNav.addCommand('archive', function () { showView('inbox'); archiveFilter = !archiveFilter; render(); }, 'toggle archive filter on inbox');
    VimNav.addCommand('settings', function () { openSettingsOverlay(); }, 'open the settings overlay');
    VimNav.addCommand('cheat', function () { HDHelp.open(); }, 'open the keymap cheat sheet');
    VimNav.addCommand('pin', function () {
      var r = ensureFocused() || (HDTree.isFocused() && (function () {
        var a = HDTree.activeReport();
        return a ? (data.reports || []).find(function (x) { return x.project === a.project && x.run === a.run; }) : null;
      })());
      if (r) togglePin(r);
    }, 'pin or unpin the focused report');
    VimNav.addCommand('theme', function (arg) {
      if (arg === 'dark' || arg === 'light' || arg === 'system') setTheme(arg);
      else cycleTheme();
    }, 'cycle theme (or: theme dark/light/system)');
  }

  /* The pins list mutating (toggle / pin / unpin) fires the
     'hd:pins-changed' DOM event from tabs.js. Re-render the tree so the
     PINNED section and the in-tree ★ markers stay in sync without a full data
     refresh. A DOM event (vs. the old single-slot window.HDPinsChanged
     callback) lets any number of listeners react independently. */
  window.addEventListener('hd:pins-changed', function () {
    renderTree();
    HDTree.paint();
  });
  window.addEventListener('hd:saved-changed', function () { renderTree(); });

  window.HarnessDeck = { reload: refresh, openSettings: openSettingsOverlay, toggleSettings: toggleSettingsOverlay };
  migrateLegacyURL();
  registerCommands();
  // Seed the inbox cursor from sessionStorage before the first render so
  // ensureFocused() (inside refresh → render) keeps the saved row instead
  // of snapping to the top on initial load.
  restoreFocus();
  refresh();
  if (window.HD_BEADS) refreshBeads();
  connectEvents();
