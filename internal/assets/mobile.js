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
})();
