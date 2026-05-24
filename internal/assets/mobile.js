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
})();
