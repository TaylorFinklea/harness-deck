// Pin manager. Replaces the v0.1.x in-app tab strip with a manual-pin
// model: you press `p` on a report (or row, or tree-active item) to
// keep it on the sidebar's PINNED section; press `p` again to unpin.
// Nothing auto-pins anymore — opening a report is a transient act.
//
// State shape (`harness-deck:pins` in localStorage):
//   [ { project, run, title } , ... ]
//
// Same fields as the old tabs state so existing consumers (search,
// digit shortcuts) keep working with minimal rename. The renderer of
// the PINNED section itself lives in aggregator.js (dashboard only);
// this module owns the persisted state, the toggle verb, and the
// report-page navigation chords (q / g d / g i / g p / g a / g t / g T).
(function () {
  var STORAGE_KEY = 'harness-deck:pins';
  var MAX_PINS = 9; // matches the digit-shortcut range (1=dashboard, 2-9=pins)

  function load() {
    try {
      var raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return [];
      var arr = JSON.parse(raw);
      return Array.isArray(arr) ? arr : [];
    } catch (_) { return []; }
  }
  function save(pins) {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(pins)); } catch (_) {}
    // Tell the dashboard's tree renderer to repaint if it's listening
    // (the aggregator IIFE wires this up). Cheap synchronous notify;
    // no module dependency in this direction.
    if (typeof window !== 'undefined' && window.HDPinsChanged) window.HDPinsChanged();
  }

  function reportURL(p) {
    return '/r/' + encodeURIComponent(p.project) + '/' + encodeURIComponent(p.run);
  }

  function indexOf(pins, project, run) {
    for (var i = 0; i < pins.length; i++) {
      if (pins[i].project === project && pins[i].run === run) return i;
    }
    return -1;
  }

  function currentRoute() {
    var m = /^\/r\/([^\/]+)\/([^\/?#]+)/.exec(location.pathname);
    if (!m) return null;
    return { project: decodeURIComponent(m[1]), run: decodeURIComponent(m[2]) };
  }

  function pin(project, run, title) {
    var pins = load();
    if (indexOf(pins, project, run) >= 0) return false; // already pinned
    pins.push({ project: project, run: run, title: title || run });
    while (pins.length > MAX_PINS) pins.shift();
    save(pins);
    return true;
  }

  function unpin(project, run) {
    var pins = load();
    var i = indexOf(pins, project, run);
    if (i < 0) return false;
    pins.splice(i, 1);
    save(pins);
    return true;
  }

  function toggle(project, run, title) {
    if (isPinned(project, run)) { unpin(project, run); return false; }
    pin(project, run, title);
    return true;
  }

  function isPinned(project, run) {
    return indexOf(load(), project, run) >= 0;
  }

  // open navigates to a report without pinning it. Mouse + Cmd+K
  // results call this so opening doesn't add noise to the pin list.
  function openReport(project, run) {
    location.href = reportURL({ project: project, run: run });
  }

  // Migration: a v0.1.x install will have `harness-deck:tabs` in
  // localStorage. The shape is identical, just renamed; move the
  // value across once, then forget the old key.
  (function migrate() {
    try {
      if (localStorage.getItem(STORAGE_KEY)) return; // already migrated
      var legacy = localStorage.getItem('harness-deck:tabs');
      if (!legacy) return;
      localStorage.setItem(STORAGE_KEY, legacy);
      localStorage.removeItem('harness-deck:tabs');
    } catch (_) {}
  })();

  /* g-prefix navigation. Lives here (not aggregator.js) so both the
     dashboard AND every rendered report page get the same vocabulary.
     The 1500ms window matches the aggregator's chord timeout. */
  var pendingG = false;
  var pendingGTimer = 0;
  function armG() {
    pendingG = true;
    if (pendingGTimer) clearTimeout(pendingGTimer);
    pendingGTimer = setTimeout(function () { pendingG = false; }, 1500);
  }
  function disarmG() { pendingG = false; if (pendingGTimer) clearTimeout(pendingGTimer); }

  document.addEventListener('keydown', function (e) {
    var t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
    if (window.VimNav && VimNav.getMode && VimNav.getMode() !== 'NORMAL') return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    var pins = load();
    var here = currentRoute();

    // q quits the current report back to the dashboard. Vim convention.
    if (e.key === 'q' && here && !pendingG) {
      location.href = '/';
      e.preventDefault();
      return;
    }

    // `p` toggles pin on the current report. Aggregator handles `p`
    // for inbox + tree contexts (different active-item sources); on a
    // report page the current route IS the target.
    if (e.key === 'p' && here && !pendingG) {
      var title = (window.HD_REPORT && HD_REPORT.title) || '';
      toggle(here.project, here.run, title);
      e.preventDefault();
      return;
    }

    // Digits 1-9 jump to the Nth pinned report; 1 is reserved for the
    // dashboard so 2-9 pick pins[0..7]. Report pages own digits 1-9
    // for option-picking inside an ask, so check the focused-ask
    // class first; the aggregator's own dashboard handler is in a
    // separate file and stops the event before we see it there.
    if (e.key >= '1' && e.key <= '9' && !pendingG) {
      // Don't fight triage when an ask is focused — it owns digits.
      if (document.querySelector('.panel.ask-focused')) return;
      var n = parseInt(e.key, 10);
      if (n === 1) { location.href = '/'; e.preventDefault(); return; }
      var hit = pins[n - 2];
      if (hit) { location.href = reportURL(hit); e.preventDefault(); return; }
      return;
    }

    if (e.key === 'g') { armG(); return; }
    if (!pendingG) return;

    switch (e.key) {
      case 't':
        // Cycle to the next pin. With no pins, this is a no-op
        // (instead of bouncing to dashboard like the old tabs did,
        // which felt magical).
        disarmG();
        if (!pins.length) { e.preventDefault(); return; }
        var iN = here ? indexOf(pins, here.project, here.run) : -1;
        location.href = reportURL(pins[(iN + 1 + pins.length) % pins.length]);
        e.preventDefault();
        return;
      case 'T':
        disarmG();
        if (!pins.length) { e.preventDefault(); return; }
        var iP = here ? indexOf(pins, here.project, here.run) : -1;
        location.href = reportURL(pins[(iP - 1 + pins.length) % pins.length]);
        e.preventDefault();
        return;
      case 'd':
      case 'h':
        disarmG();
        location.href = '/';
        e.preventDefault();
        return;
      case 'i':
        disarmG();
        location.href = '/?v=inbox';
        e.preventDefault();
        return;
      case 'p':
        disarmG();
        location.href = '/?v=projects';
        e.preventDefault();
        return;
      case 'a':
        disarmG();
        location.href = '/?archive=1';
        e.preventDefault();
        return;
      case 'x':
        // g x — unpin the current report (the new "close tab"). The
        // page itself stays; only the sidebar entry goes.
        disarmG();
        if (here) unpin(here.project, here.run);
        e.preventDefault();
        return;
    }
  });

  // Public API. HDTabs name retained so external callers (search.js)
  // keep working; the verb is documented as pin/unpin going forward.
  window.HDPins = {
    load: load,
    pin: pin,
    unpin: unpin,
    toggle: toggle,
    isPinned: isPinned,
    open: openReport,
  };
  // Backward-compat shim. search.js calls HDTabs.open() to navigate to
  // a result — same semantics as openReport(). Drop after the next
  // tag if no one else picks it up.
  window.HDTabs = {
    open: function (project, run) { openReport(project, run); },
  };
})();
