// usage.js — footer usage monitors. Polls /api/usage and renders one segment
// per provider into the statusline #usage slot (CodexBar-style). Window-kind
// providers show "LABEL NN%" tinted by severity; budget-kind show "LABEL TEXT".
// No-innerHTML discipline: built with createElement/textContent only.
(function () {
  'use strict';

  function colorFor(pct) {
    if (pct == null) return '';
    if (pct >= 90) return 'usage-crit';
    if (pct >= 70) return 'usage-warn';
    return 'usage-ok';
  }

  // fmtReset turns an ISO8601 reset into a compact countdown ("3h12m", "2d").
  function fmtReset(iso) {
    if (!iso) return '';
    var t = new Date(iso);
    if (isNaN(t.getTime())) return '';
    var ms = t.getTime() - Date.now();
    if (ms <= 0) return 'now';
    var mins = Math.round(ms / 60000);
    if (mins < 60) return mins + 'm';
    var hrs = Math.floor(mins / 60), rem = mins % 60;
    if (hrs < 24) return hrs + 'h' + (rem ? rem + 'm' : '');
    return Math.floor(hrs / 24) + 'd';
  }

  function render(samples) {
    var host = document.getElementById('usage');
    if (!host) return;
    host.textContent = '';
    (samples || []).forEach(function (s) {
      if (!s || !s.ok) return;
      var seg = document.createElement('span');
      seg.className = 'usage-seg';

      var label = document.createElement('b');
      label.textContent = s.label;
      seg.appendChild(label);
      seg.appendChild(document.createTextNode(' '));

      var val = document.createElement('span');
      if (s.kind === 'window' && s.percent != null) {
        val.textContent = Math.round(s.percent) + '%';
        val.className = colorFor(s.percent);
      } else {
        val.textContent = s.text || '';
      }
      seg.appendChild(val);

      var tip = [];
      if (s.kind === 'window' && s.reset_at) {
        var r = fmtReset(s.reset_at);
        if (r) tip.push('resets ' + r);
      }
      if (s.detail) tip.push(s.detail);
      if (tip.length) seg.title = s.label + ' — ' + tip.join(' · ');

      host.appendChild(seg);
    });
  }

  function poll() {
    fetch('/api/usage', { cache: 'no-store' })
      .then(function (r) { return r.ok ? r.json() : []; })
      .then(render)
      .catch(function () { /* leave the last good render in place */ });
  }

  function start() {
    poll();
    setInterval(poll, 30000);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
  } else {
    start();
  }
})();
