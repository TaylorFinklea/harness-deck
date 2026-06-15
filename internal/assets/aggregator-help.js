// Context-aware help overlay (the `?` cheat sheet) for the aggregator shell.
//
// A true separate-IIFE module (not a fragment of the core IIFE): it binds the
// shared el() from hd-dom.js and exposes window.HDHelp = { open, close } for the
// core script's keydown handlers and the `:cheat` command. The core references
// HDHelp only from deferred callbacks, so this module can load after it.
//
// One sheet is built once and shown over the dashboard. We don't render
// different content per view — every section is always visible and the user
// scans for the one they need. The overlay shares the dismiss vocabulary with
// settings (Esc, click scrim, the ✕ button); those handlers live in the core.
(function () {
  'use strict';
  var el = window.HDDom.el;

  function helpSection(title, rows) {
    return el('div', { class: 'help-section' }, [
      el('div', { class: 'help-section-title', text: title }),
      el('div', { class: 'help-rows' }, rows.map(function (row) {
        return el('div', { class: 'help-row' }, [
          el('div', { class: 'help-keys' }, (Array.isArray(row[0]) ? row[0] : [row[0]]).map(function (k) {
            return el('kbd', { text: k });
          })),
          el('div', { class: 'help-desc', text: row[1] })
        ]);
      }))
    ]);
  }

  function helpContent() {
    var sections = [];
    sections.push(helpSection('movement', [
      [['j', 'k'], 'move cursor down / up on the inbox'],
      ['G', 'jump to last row'],
      [['g', 'g'], 'scroll to top of page'],
      [['Tab'], 'next unanswered ask (on a report)'],
      [['/', '⌘K'], 'page search · cross-report search'],
    ]));
    sections.push(helpSection('row actions (inbox)', [
      [['Enter'], 'open the focused report'],
      ['o', 'same as Enter'],
      ['a', 'archive (or unarchive in archive view)'],
      ['x', 'close (mark done)'],
      ['p', 'pin / unpin (sidebar PINNED section)'],
      [['d', 'd'], 'delete (with confirm)'],
    ]));
    sections.push(helpSection('jumps (g-prefix)', [
      [['g', 'i'], 'go to inbox'],
      [['g', 'p'], 'go to projects'],
      [['g', 'a'], 'go to inbox + archive filter on'],
      [['g', 'd'], 'go to dashboard (alias: g h, q on a report)'],
      [['g', 't'], 'cycle to next in-app tab (g T reverses)'],
      [['g', 'x'], 'close the current in-app tab'],
    ]));
    sections.push(helpSection('leader (Space)', [
      [['Space', 'e'], 'focus the sidebar tree (toggle)'],
      [['Space', 's'], 'open settings'],
      [['Space', 't'], 'cycle theme (system → dark → light)'],
      [['Space', '?'], 'this help (alias for ?)'],
    ]));
    sections.push(helpSection('pins (digits)', [
      [['p'], 'pin / unpin the focused / current report'],
      [['1'], 'go to dashboard'],
      [['2', '–', '9'], 'go to pinned report at position N−1'],
      [['g', 't'], 'cycle to next pin (g T reverses)'],
      [['g', 'x'], 'unpin the current report'],
    ]));
    sections.push(helpSection('commands (: prompt)', [
      [':inbox / :projects', 'jump to a view'],
      [':archive', 'toggle archive filter'],
      [':settings', 'open settings overlay'],
      [':cheat', 'this help'],
    ]));
    return sections;
  }

  function openHelpOverlay() {
    var existing = document.getElementById('help-overlay');
    if (existing) { existing.style.display = 'flex'; return; }
    var modal = el('div', { class: 'help-modal' }, [
      el('div', { class: 'settings-modal-head' }, [
        el('div', { class: 'settings-modal-title', text: 'harness-deck cheat sheet' }),
        el('button', { type: 'button', class: 'settings-modal-close', title: 'close (Esc)' }, ['✕'])
      ]),
      el('div', { class: 'help-body' }, helpContent())
    ]);
    var overlay = el('div', { id: 'help-overlay', class: 'settings-overlay help-overlay' }, [modal]);
    document.body.appendChild(overlay);
  }

  function closeHelpOverlay() {
    var o = document.getElementById('help-overlay');
    if (o) o.style.display = 'none';
  }

  window.HDHelp = { open: openHelpOverlay, close: closeHelpOverlay };
})();
