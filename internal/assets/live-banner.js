// Live in-flight telemetry banner — keeps the "X seconds ago" timer,
// elapsed counter, and live/stale state honest while the report page
// stays open. Server emits the banner with a `data-updated` ISO
// timestamp; we re-evaluate freshness every 1s here.
//
// Live window is 60 seconds. Past that the pulse stops, the state
// label switches to "stale," and the banner mutes — the data stays
// visible so the user can see *what* the last update said, not just
// that there hasn't been one.
(function () {
  const LIVE_WINDOW_MS = 60 * 1000;
  const banner = document.getElementById('live-banner');
  if (!banner) return;

  const updatedStr = banner.dataset.updated;
  const updatedAt = updatedStr ? Date.parse(updatedStr) : NaN;
  if (!isFinite(updatedAt)) return;

  const stateEl = banner.querySelector('[data-live-state]');
  const sinceEl = banner.querySelector('[data-live-since]');
  const elapsedEl = banner.querySelector('[data-live-elapsed]');
  const progressEl = banner.querySelector('[data-live-progress]');
  const baselineElapsed = elapsedEl ? Number(elapsedEl.dataset.liveElapsed) : 0;

  function fmtDuration(ms) {
    if (ms < 1000) return ms + 'ms';
    const s = Math.floor(ms / 1000);
    if (s < 60) return s + 's';
    const m = Math.floor(s / 60);
    if (m < 60) return m + 'm ' + (s % 60) + 's';
    const h = Math.floor(m / 60);
    return h + 'h ' + (m % 60) + 'm';
  }

  function fmtAgo(ms) {
    if (ms < 5000) return 'just now';
    if (ms < 60000) return Math.floor(ms / 1000) + 's ago';
    if (ms < 3600000) return Math.floor(ms / 60000) + 'm ago';
    return Math.floor(ms / 3600000) + 'h ago';
  }

  if (progressEl) {
    const p = Math.max(0, Math.min(1, Number(progressEl.dataset.liveProgress)));
    const fill = progressEl.querySelector('.fill');
    if (fill) fill.style.width = (p * 100).toFixed(1) + '%';
  }

  function tick() {
    const now = Date.now();
    const ageMs = now - updatedAt;
    const live = ageMs < LIVE_WINDOW_MS;
    banner.classList.toggle('stale', !live);
    if (stateEl) stateEl.textContent = live ? 'working' : 'stale';
    if (sinceEl) sinceEl.textContent = fmtAgo(ageMs);
    // While live, the elapsed counter ticks forward from the
    // server-provided baseline. Once stale we freeze at the last value
    // so the user isn't watching a counter race past the last actual
    // update.
    if (elapsedEl) {
      const e = live ? baselineElapsed + ageMs : baselineElapsed;
      elapsedEl.textContent = 'elapsed ' + fmtDuration(e);
    }
  }

  tick();
  setInterval(tick, 1000);
})();
