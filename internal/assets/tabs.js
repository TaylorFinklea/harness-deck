// In-app tab strip. Open report URLs are persisted in localStorage so
// they survive reloads and appear on every page (the aggregator shell
// and every rendered report). Switching tabs is real navigation —
// avoids the iframe/state-isolation mess and lets each report keep its
// own vim-nav, scroll position, and SSE connection.
//
// State shape (`harness-deck:tabs` in localStorage):
//   [ { project, run, title } , ... ]
//
// The dashboard pseudo-tab is rendered first and is never in storage;
// it is implicit.
(function () {
  var STORAGE_KEY = 'harness-deck:tabs';
  var MAX_TABS = 12; // soft cap so the strip stays readable

  function load() {
    try {
      var raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return [];
      var arr = JSON.parse(raw);
      return Array.isArray(arr) ? arr : [];
    } catch (_) { return []; }
  }
  function save(tabs) {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(tabs)); } catch (_) {}
  }

  function reportURL(t) {
    return '/r/' + encodeURIComponent(t.project) + '/' + encodeURIComponent(t.run);
  }

  function indexOf(tabs, project, run) {
    for (var i = 0; i < tabs.length; i++) {
      if (tabs[i].project === project && tabs[i].run === run) return i;
    }
    return -1;
  }

  function currentRoute() {
    var m = /^\/r\/([^\/]+)\/([^\/?#]+)/.exec(location.pathname);
    if (!m) return null;
    return { project: decodeURIComponent(m[1]), run: decodeURIComponent(m[2]) };
  }

  function openTab(project, run, title) {
    var tabs = load();
    var i = indexOf(tabs, project, run);
    var entry = { project: project, run: run, title: title || run };
    if (i >= 0) {
      tabs[i] = entry;
    } else {
      tabs.push(entry);
      while (tabs.length > MAX_TABS) tabs.shift();
    }
    save(tabs);
    location.href = reportURL(entry);
  }

  function closeTab(project, run) {
    var tabs = load();
    var i = indexOf(tabs, project, run);
    if (i < 0) return;
    tabs.splice(i, 1);
    save(tabs);
    var here = currentRoute();
    if (here && here.project === project && here.run === run) {
      var prev = tabs[Math.max(0, i - 1)];
      location.href = prev ? reportURL(prev) : '/';
    } else {
      renderTabs();
    }
  }

  function ensureCurrentInTabs(titleHint) {
    var here = currentRoute();
    if (!here) return;
    var tabs = load();
    if (indexOf(tabs, here.project, here.run) >= 0) return;
    tabs.push({ project: here.project, run: here.run, title: titleHint || here.run });
    while (tabs.length > MAX_TABS) tabs.shift();
    save(tabs);
  }

  function renderTabs() {
    var bar = document.querySelector('.tabbar');
    if (!bar) return;
    var here = currentRoute();
    var tabs = load();

    var keep = bar.querySelector('.report-actions');
    bar.replaceChildren();

    var dash = document.createElement('a');
    dash.className = 'tab' + (here ? '' : ' active');
    dash.href = '/';
    var dashNum = document.createElement('span'); dashNum.className = 'num'; dashNum.textContent = '~';
    dash.append(dashNum, document.createTextNode(' dashboard'));
    bar.appendChild(dash);

    tabs.forEach(function (t, i) {
      var a = document.createElement('a');
      a.href = reportURL(t);
      a.className = 'tab';
      if (here && here.project === t.project && here.run === t.run) {
        a.className += ' active';
      }
      var n = document.createElement('span'); n.className = 'num'; n.textContent = String(i + 1);
      var label = document.createElement('span'); label.className = 'label'; label.textContent = t.title || t.run;
      var x = document.createElement('button');
      x.type = 'button';
      x.className = 'tab-close';
      x.setAttribute('aria-label', 'Close tab');
      x.textContent = '×';
      x.addEventListener('click', function (e) {
        e.preventDefault();
        e.stopPropagation();
        closeTab(t.project, t.run);
      });
      a.append(n, document.createTextNode(' '), label, x);
      bar.appendChild(a);
    });

    if (keep) bar.appendChild(keep);
  }

  document.addEventListener('click', function (e) {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button) return;
    var row = e.target.closest && e.target.closest('[data-url]');
    if (!row) return;
    var url = row.dataset.url || '';
    var m = /^\/r\/([^\/]+)\/([^\/?#]+)/.exec(url);
    if (!m) return;
    e.preventDefault();
    var title = '';
    var lbl = row.querySelector('.title, .label');
    if (lbl) title = lbl.textContent.trim();
    openTab(decodeURIComponent(m[1]), decodeURIComponent(m[2]), title);
  });

  function activeIndex(tabs) {
    var here = currentRoute();
    if (!here) return -1;
    return indexOf(tabs, here.project, here.run);
  }
  /* g-prefix navigation. Lives in tabs.js (not aggregator.js) so both
     the dashboard AND every rendered report page get the same vocabulary
     — aggregator.js only ships with the dashboard shell. The 1500ms
     window matches the aggregator's chord timeout for consistency. */
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
    var tabs = load();
    var here = currentRoute();

    /* `q` quits the current report and returns to the dashboard. Vim
       convention; cheap to reach with the home row. Tab stays open in
       the strip so the user can come back via `g t` or a digit. Only
       active on report pages (here !== null). */
    if (e.key === 'q' && here && !pendingG) {
      location.href = '/';
      e.preventDefault();
      return;
    }

    if (e.key === 'g') { armG(); return; }
    if (!pendingG) return;

    // Chord completions — every one navigates somewhere, so we always
    // disarm + preventDefault after dispatch.
    switch (e.key) {
      case 't':
        disarmG();
        if (!tabs.length) { location.href = '/'; e.preventDefault(); return; }
        var iN = activeIndex(tabs);
        location.href = reportURL(tabs[(iN + 1) % tabs.length]);
        e.preventDefault();
        return;
      case 'T':
        disarmG();
        if (!tabs.length) { location.href = '/'; e.preventDefault(); return; }
        var iP = activeIndex(tabs);
        location.href = reportURL(tabs[(iP - 1 + tabs.length) % tabs.length]);
        e.preventDefault();
        return;
      case 'd':
      case 'h':
        // g d / g h — go home (dashboard). `gd` is vim-ish "go-to
        // definition" reuse; `gh` is the obvious mnemonic.
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
        // g x — close the current in-app tab and navigate to the
        // previous one (or the dashboard if this was the only tab).
        disarmG();
        if (here) closeTab(here.project, here.run);
        else location.href = '/';
        e.preventDefault();
        return;
    }
  });

  window.HDTabs = {
    open: openTab,
    close: closeTab,
    ensureCurrent: ensureCurrentInTabs,
    render: renderTabs,
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', renderTabs);
  } else {
    renderTabs();
  }
})();
