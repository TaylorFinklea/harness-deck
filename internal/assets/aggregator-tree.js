// Sidebar tree keyboard navigation (Space-e "tree focus" mode) for the
// aggregator shell.
//
// A separate-IIFE module exposing window.HDTree. It owns the focus state
// (treeFocused + treeActiveKey) and drives the NeoVim-style file-explorer mode:
// while focused, j/k walk the visible report rows, Enter/o opens, p pins, Esc
// exits. It operates purely on the rendered #tree DOM (built by the core's
// renderTree) plus window.HDPins — no core helpers, no module data — so the
// surface the core depends on is just this object.
//
// LOAD ORDER: unlike the help module, paint() is called during the core's init
// (render → refresh at startup), so this module must be concatenated BEFORE the
// core IIFE — window.HDTree has to exist by the time core init runs. The core
// references it as HDTree.* from render(), the keydown handler, the :pin
// command, and the pins-changed listener.
(function () {
  'use strict';

  /* treeFocused: Space-e toggles the sidebar tree into a NeoVim-style
     file-explorer mode. While focused, j/k walks visible report rows
     and Enter opens the active one. Esc or Space-e exits. */
  var treeFocused = false;
  var treeActiveKey = null;

  function treeRows() {
    return Array.prototype.slice.call(document.querySelectorAll('#tree .row.run'));
  }

  function treeKeyOf(row) {
    var url = row.dataset.url || '';
    var m = /^\/r\/([^\/]+)\/([^\/?#]+)/.exec(url);
    if (!m) return '';
    // Section-scoped key so a pinned report's entry in the PINNED
    // list and its appearance in the main project tree don't collide
    // — findIndex would otherwise always snap to the pinned row and
    // the cursor could never reach the deeper tree entry.
    var section = row.classList.contains('pinned') ? 'p' : 't';
    return section + ':' + decodeURIComponent(m[1]) + '\x00' + decodeURIComponent(m[2]);
  }

  function paint() {
    var tree = document.querySelector('#tree .tree');
    if (!tree) return;
    tree.classList.toggle('tree-focused', treeFocused);
    document.querySelectorAll('#tree .row.tree-active').forEach(function (r) {
      r.classList.remove('tree-active');
    });
    if (!treeFocused) return;
    var rows = treeRows();
    if (!rows.length) return;
    // Snap to the first row if our remembered key disappeared.
    var match = rows.find(function (r) { return treeKeyOf(r) === treeActiveKey; });
    if (!match) {
      match = rows[0];
      treeActiveKey = treeKeyOf(match);
    }
    match.classList.add('tree-active');
    var rect = match.getBoundingClientRect();
    if (rect.top < 80 || rect.bottom > window.innerHeight - 40) {
      match.scrollIntoView({ block: 'nearest', behavior: 'instant' });
    }
  }

  function moveCursor(delta) {
    var rows = treeRows();
    if (!rows.length) return;
    var idx = rows.findIndex(function (r) { return treeKeyOf(r) === treeActiveKey; });
    if (idx < 0) idx = 0;
    var next = Math.max(0, Math.min(rows.length - 1, idx + delta));
    treeActiveKey = treeKeyOf(rows[next]);
    paint();
  }

  function activeRow() {
    return treeRows().find(function (r) { return treeKeyOf(r) === treeActiveKey; });
  }

  function open() {
    var row = activeRow();
    if (row && row.dataset.url) window.location.href = row.dataset.url;
  }

  // activeReport returns { project, run } parsed from the active row's key, or
  // null. The core's :pin command resolves the full record via this; the title
  // it needs is already on the row label, so the module never touches data.
  function activeReport() {
    var key = treeActiveKey;
    if (!key || key.length < 3) return null;
    var parts = key.slice(2).split('\x00');
    return parts.length === 2 ? { project: parts[0], run: parts[1] } : null;
  }

  function pin() {
    var row = activeRow();
    var a = activeReport();
    if (!row || !a) return;
    // The row label text is r.title || r.run — the same title togglePin uses.
    var label = row.querySelector('.label');
    var title = label ? label.textContent : a.run;
    if (window.HDPins) window.HDPins.toggle(a.project, a.run, title);
  }

  function enter() {
    treeFocused = true;
    var rows = treeRows();
    if (rows.length && !treeActiveKey) treeActiveKey = treeKeyOf(rows[0]);
    paint();
  }
  function exit() { treeFocused = false; paint(); }
  function toggle() { if (treeFocused) exit(); else enter(); }
  function isFocused() { return treeFocused; }

  window.HDTree = {
    paint: paint,
    moveCursor: moveCursor,
    open: open,
    pin: pin,
    enter: enter,
    exit: exit,
    toggle: toggle,
    isFocused: isFocused,
    activeReport: activeReport,
  };
})();
