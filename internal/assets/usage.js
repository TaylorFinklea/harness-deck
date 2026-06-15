// usage.js — footer usage monitors. Polls /api/usage and renders one segment
// per provider into the statusline #usage slot (CodexBar-style). Window-kind
// providers show "LABEL NN%" tinted by severity; budget-kind show "LABEL TEXT".
// No-innerHTML discipline: built with createElement/textContent only.
(function () {
  'use strict';

  var el = window.HDDom.el; // shared no-innerHTML DOM helper (hd-dom.js)

  function colorFor(pct) {
    if (pct == null) return '';
    if (pct >= 90) return 'usage-crit';
    if (pct >= 70) return 'usage-warn';
    return ''; // ok: default dark-on-blue, no class
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
      var pctView = s.kind === 'window' && s.percent != null;
      var val = el('span', pctView
        ? { text: Math.round(s.percent) + '%', class: colorFor(s.percent) }
        : { text: s.text || '' });
      var seg = el('span', { class: 'usage-seg' }, [el('b', { text: s.label }), ' ', val]);

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
    // First fetch decides whether to keep polling: an empty array means no
    // providers are configured, so don't poll a permanently-empty endpoint.
    fetch('/api/usage', { cache: 'no-store' })
      .then(function (r) { return r.ok ? r.json() : []; })
      .then(function (samples) {
        render(samples);
        if (samples && samples.length) setInterval(poll, 30000);
      })
      .catch(function () {});
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
  } else {
    start();
  }
})();
