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

  function post(block, value, note, values) {
    var body = { block: block, value: value, note: note || '' };
    if (values && values.length) body.values = values;
    return fetch('/r/' + encodeURIComponent(R.project) + '/' +
        encodeURIComponent(R.run) + '/respond', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    }).then(function (resp) {
      if (!resp.ok) throw new Error('HTTP ' + resp.status);
    });
  }

  document.addEventListener('click', function (e) {
    var btn = e.target.closest('.hd-respond');
    if (!btn || btn.disabled) return;

    var panel = btn.closest('.panel');
    var noteEl = panel ? panel.querySelector('.hd-note') : null;
    var note = noteEl ? noteEl.value.trim() : '';

    // Multi-select path: collect checked .hd-check inputs within the panel.
    if (btn.dataset.multi) {
      var checked = panel ? panel.querySelectorAll('.hd-check:checked') : [];
      var values = [];
      checked.forEach(function (cb) { values.push(cb.value); });
      if (!values.length) {
        // Nothing checked — focus the first checkbox as a hint.
        var first = panel ? panel.querySelector('.hd-check') : null;
        if (first) first.focus();
        return;
      }
      var original = btn.textContent;
      panel.querySelectorAll('.hd-respond').forEach(function (b) { b.disabled = true; });
      btn.textContent = 'recording…';
      post(btn.dataset.block, '', note, values).then(function () {
        window.location.reload();
      }).catch(function (err) {
        btn.textContent = original + ' — retry';
        panel.querySelectorAll('.hd-respond').forEach(function (b) { b.disabled = false; });
        console.error('harness-deck: respond failed', err);
      });
      return;
    }

    // Single-value path (choice / yesno / text / decision / approval).
    var value = btn.dataset.value || '';
    if (btn.dataset.input) {
      var field = document.getElementById(btn.dataset.input);
      value = field ? field.value.trim() : '';
      if (!value) { if (field) field.focus(); return; }
    }

    var original2 = btn.textContent;
    if (panel) {
      panel.querySelectorAll('.hd-respond').forEach(function (b) { b.disabled = true; });
    }
    btn.textContent = 'recording…';

    post(btn.dataset.block, value, note).then(function () {
      window.location.reload();
    }).catch(function (err) {
      btn.textContent = original2 + ' — retry';
      if (panel) {
        panel.querySelectorAll('.hd-respond').forEach(function (b) { b.disabled = false; });
      }
      console.error('harness-deck: respond failed', err);
    });
  });
})();
