// Report-page live reload. The aggregator already refreshes on SSE
// change events; this script gives report pages the same behavior.
//
// The page bakes window.HD_REPORT.sig at render time. On every SSE
// 'change' broadcast we fetch /r/{p}/{r}/sig and reload if the
// server-side fingerprint diverges. exists=false redirects to /.
//
// Two debounces guard against unwanted reloads:
//   * 2s grace after page load — respond.js calls location.reload()
//     when an answer is submitted, and the watcher's next tick would
//     otherwise trigger a second reload immediately after.
//   * single in-flight check at a time — multiple SSE events arriving
//     in quick succession collapse to one /sig fetch.
(function () {
  var R = window.HD_REPORT || {};
  if (!R.project || !R.run || !R.sig) return;     // standalone render, no live state
  if (typeof EventSource !== 'function') return;  // no SSE support

  var loadedAt = Date.now();
  var inflight = false;

  function check() {
    if (inflight) return;
    if (Date.now() - loadedAt < 2000) return;     // post-respond cooldown
    inflight = true;
    fetch('/r/' + encodeURIComponent(R.project) + '/' + encodeURIComponent(R.run) + '/sig', {
      cache: 'no-store',
    }).then(function (r) { return r.json(); }).then(function (j) {
      if (!j.exists) {
        location.href = '/';
        return;
      }
      if (j.sig && j.sig !== R.sig) {
        location.reload();
      }
    }).catch(function (err) {
      console.warn('harness-deck: live-reload sig check failed:', err);
    }).then(function () { inflight = false; });
  }

  var es = new EventSource('/events');
  es.addEventListener('change', check);
})();
