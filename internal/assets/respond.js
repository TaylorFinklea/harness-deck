/* harness-deck report page — posts the user's answers to interactive blocks
   (ask / decision / approval) back to the server, which records them in
   responses.json beside the report. window.HD_REPORT carries {project, run}.

   A control is a button with class "hd-respond" and data-block; it carries
   either data-value (choice/yes-no/approval) or data-input (the id of a text
   field to read). On success the page reloads so the block re-renders in its
   recorded-answer state. */
(function () {
  'use strict';

  var R = window.HD_REPORT || {};
  if (!R.project || !R.run) return; // standalone render — no server to post to

  function post(block, value, note) {
    return fetch('/r/' + encodeURIComponent(R.project) + '/' +
        encodeURIComponent(R.run) + '/respond', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ block: block, value: value, note: note || '' })
    }).then(function (resp) {
      if (!resp.ok) throw new Error('HTTP ' + resp.status);
    });
  }

  document.addEventListener('click', function (e) {
    var btn = e.target.closest('.hd-respond');
    if (!btn || btn.disabled) return;

    var value = btn.dataset.value || '';
    if (btn.dataset.input) {
      var field = document.getElementById(btn.dataset.input);
      value = field ? field.value.trim() : '';
      if (!value) { if (field) field.focus(); return; }
    }

    var panel = btn.closest('.panel');
    var noteEl = panel ? panel.querySelector('.hd-note') : null;
    var note = noteEl ? noteEl.value.trim() : '';
    var original = btn.textContent;
    if (panel) {
      panel.querySelectorAll('.hd-respond').forEach(function (b) { b.disabled = true; });
    }
    btn.textContent = 'recording…';

    post(btn.dataset.block, value, note).then(function () {
      window.location.reload();
    }).catch(function (err) {
      btn.textContent = original + ' — retry';
      if (panel) {
        panel.querySelectorAll('.hd-respond').forEach(function (b) { b.disabled = false; });
      }
      console.error('harness-deck: respond failed', err);
    });
  });
})();
