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
    { id: 'projects', label: 'projects' }
  ];

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
  var currentView = 'inbox';
  var trackedOpen = false;     // is the "tracked projects" panel expanded?
  var draggedName = null;      // project name currently being dragged, or null
  /* archiveFilter: when true the inbox shows archived reports instead
     of active ones. Replaces the old standalone archive view. The 'a'
     keybinding (toggle archive) still works in either mode — it just
     means "archive" in one and "unarchive" in the other. */
  var archiveFilter = false;

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

  /* --- sidebar tree --- */
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

    var body = items.length
      ? items.map(itemRow)
      : [emptyState([el('b', { text: 'nothing needs you.' }), el('br'),
        'no reports are awaiting review.'])];
    var right = items.length ? pill(items.length + ' awaiting', 'warn') : null;
    return [strip, panel('inbox — needs you', right, body)];
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
      var hist = p.history || [];
      if (hist.length) {
        body.push(el('div', { class: 'proj-sec', text: 'history' }));
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

  /*
     phone push notifications. The whole view renders synchronously with
     placeholder copy, then async fetches (server status, browser
     subscription state) overwrite the relevant cells via id refs. */
  function viewSettings() {
    var statusValue = el('span', { id: 'set-push-status', text: 'checking…' });
    var subValue = el('span', { id: 'set-push-sub', text: 'checking…' });
    var actions = el('div', { class: 'respond-actions', id: 'set-push-actions' }, [
      el('div', { class: 'sub', text: 'loading…' })
    ]);

    var body = [
      el('div', { class: 'kv' }, [
        el('div', null, [el('b', { text: 'push status: ' }), statusValue]),
        el('div', null, [el('b', { text: 'this browser: ' }), subValue])
      ]),
      el('div', { style: 'margin-top: 14px;' }, [actions]),
      el('div', { class: 'sub', style: 'margin-top: 18px;',
        text: 'Push needs a VAPID keypair (run `harness-deck vapid` once) and HTTPS — iOS will not show notifications over plain http.' })
    ];

    // Kick off the async refresh after render returns.
    setTimeout(refreshSettings, 0);

    return [
      panel('phone notifications', null, body),
      panel('notification destinations', null, [destinationsBody()]),
      panel('theme', null, [themePicker()]),
      panel('about', null, [
        el('div', null, [el('b', { text: 'tip: ' }),
          'open this page on your phone (over Tailscale) and tap Add to Home Screen to install harness-deck as an app.'])
      ])
    ];
  }

  /* destinationsBody — renders the "notification destinations" settings
     panel: a list of configured Slack / Discord / webhook endpoints with
     per-row test + remove buttons, and an add form below. The list is
     async-fetched after render so the panel itself is synchronous. */
  function destinationsBody() {
    var listEl = el('div', { id: 'dest-list' }, [
      el('div', { class: 'sub', text: 'loading…' })
    ]);
    setTimeout(refreshDestinations, 0);
    return el('div', null, [
      listEl,
      el('div', { class: 'sub', style: 'margin: 14px 0 6px; font-size: 10px; text-transform: uppercase; letter-spacing: 0.12em; color: var(--tn-fg-gutter);', text: 'add destination' }),
      destinationForm(),
      el('div', { class: 'sub', style: 'margin-top: 12px;', text:
        'Fan-out fires for every new ask alongside Web Push. Set public_url in config.json so the link in Slack/Discord resolves externally (defaults to bind+port, which is "0.0.0.0:7420" on open binds).'
      })
    ]);
  }

  /* destinationForm — the add-destination form. POSTs the four fields
     to /api/notifications; on success, refreshes the list and resets. */
  function destinationForm() {
    return el('form', { id: 'dest-form', class: 'dest-form' }, [
      el('div', { class: 'dest-form-row' }, [
        el('select', { name: 'type', class: 'hd-input' }, [
          el('option', { value: 'slack', text: 'slack' }),
          el('option', { value: 'discord', text: 'discord' }),
          el('option', { value: 'webhook', text: 'webhook' })
        ]),
        el('input', { name: 'name', class: 'hd-input', placeholder: 'name (e.g. team-alerts)', required: 'required' })
      ]),
      el('input', { name: 'url', class: 'hd-input', placeholder: 'https://hooks.slack.com/services/...', required: 'required' }),
      el('input', { name: 'projects', class: 'hd-input', placeholder: 'projects allowlist (comma-separated, empty = all)' }),
      el('div', { class: 'dest-form-actions' }, [
        el('button', { type: 'submit', class: 'hd-btn' }, ['add destination'])
      ])
    ]);
  }

  /* refreshDestinations — pull the current list, render rows. Called
     after add / delete so the UI matches server state. */
  function refreshDestinations() {
    var list = document.getElementById('dest-list');
    if (!list) return;
    fetch('/api/notifications').then(function (r) { return r.json(); }).then(function (data) {
      var dests = data.destinations || [];
      if (!dests.length) {
        list.replaceChildren(el('div', { class: 'sub', text: 'no destinations configured.' }));
        return;
      }
      list.replaceChildren.apply(list, dests.map(destinationRow));
    }).catch(function (e) {
      list.replaceChildren(el('div', { class: 'sub', text: 'failed to load: ' + e }));
    });
  }

  /* destinationRow — one entry in the configured list. URL host only
     (the server redacts secrets); test + remove buttons wire via data-
     attributes to the delegated click handler. */
  function destinationRow(d) {
    var kids = [
      el('div', { class: 'dest-main' }, [
        el('div', null, [
          el('b', { text: d.name }),
          el('span', { class: 'sub', text: ' · ' + d.type + (d.url_host ? ' · ' + d.url_host : '') })
        ])
      ]),
      el('div', { class: 'dest-actions' }, [
        el('button', { type: 'button', class: 'hd-btn dest-test', data: { name: d.name } }, ['test']),
        el('button', { type: 'button', class: 'hd-btn danger dest-remove', data: { name: d.name } }, ['remove'])
      ])
    ];
    if (d.projects && d.projects.length) {
      kids[0].appendChild(el('div', { class: 'sub', text: 'projects: ' + d.projects.join(', ') }));
    }
    return el('div', { class: 'dest-row' }, kids);
  }

  function addDestination(formData) {
    var body = {
      name: (formData.get('name') || '').trim(),
      type: formData.get('type'),
      url: (formData.get('url') || '').trim(),
    };
    var projects = (formData.get('projects') || '').trim();
    if (projects) {
      body.projects = projects.split(',').map(function (s) { return s.trim(); }).filter(Boolean);
    }
    return fetch('/api/notifications', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t || ('http ' + r.status)); });
      return r.json();
    });
  }

  function testDestination(name) {
    var btn = document.querySelector('.dest-test[data-name="' + cssEscape(name) + '"]');
    if (btn) { btn.disabled = true; btn.textContent = 'testing…'; }
    fetch('/api/notifications/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: name })
    }).then(function (r) { return r.json(); }).then(function (data) {
      if (btn) {
        btn.disabled = false;
        btn.textContent = data.ok ? 'sent ✓' : 'failed';
        btn.classList.toggle('ok', !!data.ok);
        setTimeout(function () {
          btn.textContent = 'test';
          btn.classList.remove('ok');
        }, 2500);
      }
      if (!data.ok && data.error) alert('test failed: ' + data.error);
    }).catch(function (e) {
      if (btn) { btn.disabled = false; btn.textContent = 'test'; }
      alert('test failed: ' + e);
    });
  }

  function removeDestination(name) {
    if (!confirm('Remove destination "' + name + '"? This rewrites config.json.')) return;
    fetch('/api/notifications/' + encodeURIComponent(name), { method: 'DELETE' })
      .then(function (r) {
        if (!r.ok) throw new Error('http ' + r.status);
        refreshDestinations();
      })
      .catch(function (e) { alert('remove failed: ' + e); });
  }

  /* cssEscape — minimal substitute for CSS.escape (Safari has it but
     older WebViews may not). Only needs to handle what dest names can
     contain (safePathComponent: alphanumeric + . _ -). */
  function cssEscape(s) { return s.replace(/["\\]/g, '\\$&'); }

  /* themePicker — three-way segmented control (system / dark / light)
     persisted to localStorage. "system" follows prefers-color-scheme
     via the @media query in tokyo-night.css. The dark/light buttons
     set data-theme on <html> immediately so the page repaints. */
  function themePicker() {
    var current = 'system';
    try {
      var t = localStorage.getItem('harness-deck:theme');
      if (t === 'light' || t === 'dark') current = t;
    } catch (_) {}
    function btn(key, label) {
      return el('button', {
        type: 'button',
        class: 'theme-btn' + (current === key ? ' active' : ''),
        data: { theme: key },
        text: label,
      });
    }
    var bar = el('div', { class: 'theme-bar' }, [
      btn('system', 'system'),
      btn('dark', 'dark'),
      btn('light', 'light'),
    ]);
    return el('div', null, [
      bar,
      el('div', { class: 'sub', style: 'margin-top: 10px;',
        text: 'system follows your OS preference (prefers-color-scheme). dark / light override it for this browser; choice persists across sessions.' }),
    ]);
  }

  function setTheme(choice) {
    try {
      if (choice === 'system') localStorage.removeItem('harness-deck:theme');
      else localStorage.setItem('harness-deck:theme', choice);
    } catch (_) {}
    if (choice === 'dark' || choice === 'light') {
      document.documentElement.setAttribute('data-theme', choice);
    } else {
      document.documentElement.removeAttribute('data-theme');
    }
    // Update active states without re-rendering the whole view.
    document.querySelectorAll('.theme-btn').forEach(function (b) {
      b.classList.toggle('active', b.dataset.theme === choice);
    });
  }

  /* refreshSettings — pulls /api/push/status and the in-browser
     PushSubscription, then re-renders the settings cells. Safe to call
     repeatedly; each call replaces the action button(s). */
  function refreshSettings() {
    var statusEl = document.getElementById('set-push-status');
    var subEl = document.getElementById('set-push-sub');
    var actEl = document.getElementById('set-push-actions');
    if (!statusEl || !subEl || !actEl) return; // not on the settings view

    fetch('/api/push/status').then(function (r) { return r.json(); }).then(function (s) {
      if (!s.enabled) {
        statusEl.textContent = 'disabled';
        statusEl.className = 'pill warn';
        actEl.replaceChildren(el('div', { class: 'sub',
          text: 'No VAPID key found. Run `harness-deck vapid` in a terminal, then restart the server.' }));
        return;
      }
      statusEl.textContent = 'ready · ' + s.subscription_count + ' device(s)';
      statusEl.className = 'pill ok';
      if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
        subEl.textContent = 'unsupported in this browser';
        actEl.replaceChildren();
        return;
      }
      navigator.serviceWorker.ready.then(function (reg) {
        return reg.pushManager.getSubscription().then(function (sub) {
          if (sub) {
            subEl.textContent = 'subscribed';
            subEl.className = 'pill ok';
            actEl.replaceChildren(
              el('button', { type: 'button', id: 'push-off', text: '⌦ unsubscribe this browser' })
            );
          } else {
            subEl.textContent = 'not subscribed';
            subEl.className = 'pill';
            actEl.replaceChildren(
              el('button', { type: 'button', id: 'push-on', text: '🔔 enable notifications on this browser' })
            );
          }
        });
      }).catch(function (err) {
        subEl.textContent = 'error: ' + err.message;
      });
    }).catch(function (err) {
      statusEl.textContent = 'error: ' + err.message;
    });
  }

  /* enablePushHere — prompt for notification permission, ask the SW for
     a PushManager subscription using the server's VAPID public key,
     and POST it. Surfaces the user-facing error inline rather than via
     alert() so the settings view stays usable. */
  function enablePushHere() {
    fetch('/api/push/vapid-key').then(function (r) {
      if (!r.ok) throw new Error('vapid-key HTTP ' + r.status);
      return r.json();
    }).then(function (j) {
      return navigator.serviceWorker.ready.then(function (reg) {
        return reg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: urlBase64ToUint8Array(j.key)
        });
      });
    }).then(function (sub) {
      return fetch('/api/push/subscribe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(sub)
      });
    }).then(function (r) {
      if (!r.ok) throw new Error('subscribe HTTP ' + r.status);
      refreshSettings();
    }).catch(function (err) {
      var sub = document.getElementById('set-push-sub');
      if (sub) { sub.textContent = 'enable failed: ' + err.message; }
    });
  }

  /* disablePushHere — unsubscribe in the browser, then tell the server
     to drop the stored Subscription. */
  function disablePushHere() {
    navigator.serviceWorker.ready.then(function (reg) {
      return reg.pushManager.getSubscription();
    }).then(function (sub) {
      if (!sub) return null;
      var endpoint = sub.endpoint;
      return sub.unsubscribe().then(function () {
        return fetch('/api/push/unsubscribe', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ endpoint: endpoint })
        });
      });
    }).then(refreshSettings).catch(function (err) {
      var sub = document.getElementById('set-push-sub');
      if (sub) { sub.textContent = 'disable failed: ' + err.message; }
    });
  }

  /* urlBase64ToUint8Array — converts the base64url VAPID key the server
     returns into the Uint8Array format PushManager.subscribe wants. */
  function urlBase64ToUint8Array(s) {
    var pad = '='.repeat((4 - (s.length % 4)) % 4);
    var b64 = (s + pad).replace(/-/g, '+').replace(/_/g, '/');
    var raw = atob(b64);
    var out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out;
  }

  /* BUILDERS maps view id → builder function. v0.2.0 keeps just the
     two top-level views; viewSettings is now rendered into a modal
     overlay (settingsOverlayBody) instead. */
  var BUILDERS = { inbox: viewInbox, projects: viewProjects };

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
  }

  /* one delegated click handler for every navigable row */
  document.addEventListener('click', function (e) {
    var tab = e.target.closest('.view-tab');
    if (tab) { showView(tab.dataset.view); return; }
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
    if (e.target.closest('.settings-modal-close') || e.target.classList.contains('settings-overlay')) {
      closeSettingsOverlay();
      return;
    }
    var testBtn = e.target.closest('.dest-test');
    if (testBtn) { e.stopPropagation(); testDestination(testBtn.dataset.name); return; }
    var removeBtn = e.target.closest('.dest-remove');
    if (removeBtn) { e.stopPropagation(); removeDestination(removeBtn.dataset.name); return; }
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

  /* number keys 1-4 switch views, unless vim-nav owns the keystroke */
  document.addEventListener('keydown', function (e) {
    var t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
    if (window.VimNav && VimNav.getMode && VimNav.getMode() !== 'NORMAL') return;
    var v = VIEWS.find(function (x) { return x.key === e.key; });
    if (v) { showView(v.id); e.preventDefault(); }
  });

  /* Inbox cursor key handler — capture phase so we intercept j/k
     before vim-nav's page-scroll binding fires. Only owns those keys
     when there's actually a row list to navigate; otherwise lets the
     event bubble through to vim-nav. */
  document.addEventListener('keydown', function (e) {
    var t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
    if (window.VimNav && VimNav.getMode && VimNav.getMode() !== 'NORMAL') return;
    if (!visibleRows().length) return; // current view doesn't have a row list
    // Modifier keys belong to the browser / OS — don't intercept.
    if (e.metaKey || e.ctrlKey || e.altKey) return;

    function consume() { e.preventDefault(); e.stopImmediatePropagation(); }

    switch (e.key) {
      case 'j': moveCursor(+1); consume(); return;
      case 'k': moveCursor(-1); consume(); return;
      case 'G': moveCursorTo('bottom'); consume(); return;
      case 'Enter':
      case 'o': openFocused(); consume(); return;
      case 'a': actOnFocused('archive'); consume(); return;
      case 'x': actOnFocused('close'); consume(); return;
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

  /* live updates — the server pushes a 'change' event when the report index
     changes on disk; EventSource reconnects on its own if the stream drops. */
  function connectEvents() {
    if (typeof EventSource === 'undefined') return;
    var es = new EventSource('/events');
    es.addEventListener('change', function () { refresh(); });
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
    else if (v === 'inbox' || v === 'projects') { /* still valid */ }
    if (enableArchive) {
      archiveFilter = true;
      params.set('archive', '1');
    }
    if (params.get('settings') === '1') openSettings = true;
    var search = params.toString();
    history.replaceState(null, '', window.location.pathname + (search ? '?' + search : ''));
    if (openSettings) setTimeout(openSettingsOverlay, 0);
  }

  /* Esc closes the settings overlay when it's the topmost layer. The
     vim-nav Esc handler runs too but only resets its own prompt state;
     no conflict. */
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape') return;
    var o = document.getElementById('settings-overlay');
    if (o && o.style.display !== 'none') {
      closeSettingsOverlay();
      e.preventDefault();
    }
  });

  window.HarnessDeck = { reload: refresh, openSettings: openSettingsOverlay, toggleSettings: toggleSettingsOverlay };
  migrateLegacyURL();
  refresh();
  connectEvents();
})();
