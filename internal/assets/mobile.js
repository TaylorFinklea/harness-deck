// Mobile drawer toggle + service worker registration. Inlined into the
// aggregator shell; safe to load on desktop too (the menu button is
// hidden via CSS and the drawer-open class is harmless when no sidebar
// transform applies).
(function () {
  // 1) Drawer: tap the menu button, scrim, or any tree row to close it.
  document.addEventListener('click', function (e) {
    var btn = e.target.closest && e.target.closest('.menu-button');
    if (btn) {
      document.body.classList.toggle('drawer-open');
      e.preventDefault();
      return;
    }
    if (e.target.closest && e.target.closest('.drawer-scrim')) {
      document.body.classList.remove('drawer-open');
      return;
    }
    if (e.target.closest && e.target.closest('#tree .row')) {
      document.body.classList.remove('drawer-open');
    }
  });

  // 2) Service worker: register if supported. Required for push and for
  // installable home-screen behavior; harmless if push isn't configured.
  if ('serviceWorker' in navigator) {
    window.addEventListener('load', function () {
      navigator.serviceWorker.register('/service-worker.js').catch(function (err) {
        console.warn('service worker registration failed:', err);
      });
    });
  }

  // 3) Copy-button delegation for fenced code blocks. Lives here (rather
  // than in respond.js or aggregator.js) because mobile.js is the one
  // script loaded on every page — report pages, aggregator shell, and
  // any future page that inlines /assets MobileJSInline.
  document.addEventListener('click', function (e) {
    var btn = e.target.closest && e.target.closest('.copy-btn');
    if (!btn) return;
    var block = btn.closest('.code-block');
    var code = block && block.querySelector('code');
    if (!code) return;
    var text = code.textContent || '';
    var done = function () {
      var orig = btn.textContent;
      btn.textContent = 'copied';
      btn.classList.add('ok');
      setTimeout(function () { btn.textContent = orig; btn.classList.remove('ok'); }, 1200);
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(done).catch(function () { fb(text); done(); });
    } else {
      fb(text);
      done();
    }
  });
  // fb — textarea+execCommand fallback. Needed when the page is served
  // over plain HTTP (Clipboard API requires a secure context).
  function fb(text) {
    var ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand('copy'); } catch (_) {}
    document.body.removeChild(ta);
  }

  // 4) Pull-to-refresh on installed PWAs. Both iOS Safari and Chrome
  // disable native PTR when display-mode is standalone, so an installed
  // home-screen app has no way to refresh without our help. We gate
  // strictly on standalone so regular browser tabs keep their built-in
  // PTR (no double trigger).
  var isStandalone = (window.matchMedia && window.matchMedia('(display-mode: standalone)').matches) ||
    window.navigator.standalone === true;
  if (!isStandalone) return;

  // Indicator: a thin bar that grows in height as the user pulls. Uses
  // height + transition so we get a free animation on snap-back; the
  // pull math just sets the height.
  var ind = document.createElement('div');
  ind.id = 'pull-refresh';
  Object.assign(ind.style, {
    position: 'fixed',
    top: '0',
    left: '0',
    right: '0',
    height: '0px',
    overflow: 'hidden',
    display: 'flex',
    alignItems: 'flex-end',
    justifyContent: 'center',
    paddingBottom: '6px',
    background: 'var(--tn-bg-dark, #161723)',
    color: 'var(--tn-blue, #7aa2f7)',
    fontSize: '11px',
    fontFamily: 'inherit',
    letterSpacing: '0.04em',
    textTransform: 'uppercase',
    zIndex: 60,
    pointerEvents: 'none',
    borderBottom: '1px solid transparent',
  });
  document.body.appendChild(ind);

  var startY = 0;
  var pulling = false;
  var pullDistance = 0;
  var THRESHOLD = 70;
  var DAMPING = 0.5;

  function setIndicator(h, label) {
    ind.style.height = h + 'px';
    ind.textContent = label;
  }

  document.addEventListener('touchstart', function (e) {
    // Only arm when the document is scrolled to the top — otherwise the
    // user is just scrolling content upward, not pulling to refresh.
    if (window.scrollY > 0 || document.documentElement.scrollTop > 0) return;
    if (e.touches.length !== 1) return;
    var t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA')) return;
    startY = e.touches[0].clientY;
    pulling = true;
    pullDistance = 0;
    ind.style.transition = 'none';
  }, { passive: true });

  document.addEventListener('touchmove', function (e) {
    if (!pulling) return;
    var dy = e.touches[0].clientY - startY;
    if (dy <= 0) {
      pullDistance = 0;
      setIndicator(0, '');
      return;
    }
    // Damped pull — feels like rubber-band rather than a 1:1 drag.
    pullDistance = Math.min(dy * DAMPING, 120);
    setIndicator(
      pullDistance,
      pullDistance >= THRESHOLD ? '↻ release to refresh' : '↓ pull to refresh'
    );
  }, { passive: true });

  function endPull() {
    if (!pulling) return;
    pulling = false;
    ind.style.transition = 'height 200ms ease-out';
    if (pullDistance >= THRESHOLD) {
      setIndicator(40, '↻ refreshing…');
      // Tiny delay so the user sees the state change before the reload
      // wipes the page — feels less abrupt.
      setTimeout(function () { location.reload(); }, 120);
    } else {
      setIndicator(0, '');
    }
    pullDistance = 0;
  }
  document.addEventListener('touchend', endPull, { passive: true });
  document.addEventListener('touchcancel', endPull, { passive: true });
})();
