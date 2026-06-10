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
  var banner = null;

  // typingInProgress guards the one user-hostile reload: an in-flight
  // agent heartbeating `live` telemetry rewrites report.json every few
  // seconds, and reloading mid-answer wipes whatever the user has typed
  // into an ask. Focused input OR any non-empty draft defers the reload.
  function typingInProgress() {
    var ae = document.activeElement;
    if (ae && (ae.tagName === 'INPUT' || ae.tagName === 'TEXTAREA')) return true;
    var inputs = document.querySelectorAll('.hd-input');
    for (var i = 0; i < inputs.length; i++) {
      if (inputs[i].value && inputs[i].value.trim() !== '') return true;
    }
    return false;
  }

  // showPending surfaces a passive "tap to reload" chip instead of
  // yanking the page out from under the user. Clicking runs the action
  // the guard deferred (reload, or home when the report is gone).
  function showPending(label, action) {
    if (banner) return;
    banner = document.createElement('div');
    banner.id = 'hd-reload-pending';
    banner.setAttribute('role', 'status');
    banner.textContent = label;
    banner.style.cssText =
      'position:fixed;bottom:14px;right:14px;z-index:1000;' +
      'padding:6px 12px;cursor:pointer;font-size:12px;' +
      'background:var(--tn-bg-highlight,#292e42);color:var(--tn-fg,#c0caf5);' +
      'border:1px solid var(--tn-blue,#7aa2f7);border-radius:4px;';
    banner.addEventListener('click', action);
    document.body.appendChild(banner);
  }

  function check() {
    if (inflight) return;
    if (Date.now() - loadedAt < 2000) return;     // post-respond cooldown
    inflight = true;
    fetch('/r/' + encodeURIComponent(R.project) + '/' + encodeURIComponent(R.run) + '/sig', {
      cache: 'no-store',
    }).then(function (r) { return r.json(); }).then(function (j) {
      if (!j.exists) {
        if (typingInProgress()) {
          showPending('report removed — tap to go home', function () { location.href = '/'; });
          return;
        }
        location.href = '/';
        return;
      }
      if (j.sig && j.sig !== R.sig) {
        if (typingInProgress()) {
          showPending('report updated — tap to reload', function () { location.reload(); });
          return;
        }
        location.reload();
      }
    }).catch(function (err) {
      console.warn('harness-deck: live-reload sig check failed:', err);
    }).then(function () { inflight = false; });
  }

  var es = new EventSource('/events');
  es.addEventListener('change', check);
})();
