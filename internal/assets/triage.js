// Keyboard triage. Lets a power user clear an inbox of asks/decisions/
// approvals without touching the mouse: the first unanswered block is
// auto-focused on load, digit keys pick options, y/n are shortcuts,
// Enter submits the highlighted choice, Tab skips ahead. The respond
// handler reloads the page after each submission, which naturally moves
// focus to the next unanswered block — so the inner loop is just
// "press digit, page reloads, press digit, …" until everything is
// answered.
//
// Coexists with vim-nav: only acts when (a) no input is focused and
// (b) VimNav reports NORMAL mode. Triage does not register its own
// vim-nav mode; it is just a contextual interpretation of normal keys
// when an unanswered ask is in focus.
(function () {
  // unansweredBlocks returns the .panel elements that still carry
  // active .ask-controls (i.e. response buttons haven't been replaced
  // by .ask-answered yet). Order matches document order.
  function unansweredBlocks() {
    var ctrls = document.querySelectorAll('.panel .ask-controls');
    var out = [];
    ctrls.forEach(function (c) {
      var panel = c.closest('.panel');
      if (panel) out.push(panel);
    });
    return out;
  }

  function buttons(panel) {
    return Array.prototype.slice.call(panel.querySelectorAll('.hd-respond'));
  }

  function input(panel) {
    return panel.querySelector('.hd-input');
  }

  /* activeItemIdx — which navigable slot on the focused panel is the
     "current" choice. For most asks this walks the response buttons.
     For text-mode asks the slot list is [input, submitButton] so h/l
     bounces between "type" and "send." Enter resolves to "focus input"
     or "click button" depending on which slot is active.

     Tracked in closure (not DOM) so re-renders don't lose it. */
  var activeItemIdx = 0;

  // items returns the navigable slots for a panel. For a text-mode ask
  // (input + single submit button) the input is slot 0 and the button
  // is slot 1, so the natural arrival point is the input itself.
  function items(panel) {
    if (!panel) return [];
    var inp = input(panel);
    var btns = buttons(panel);
    if (inp && btns.length === 1 && btns[0].dataset.input) {
      return [inp, btns[0]];
    }
    return btns;
  }

  function setActiveItem(panel, idx) {
    if (!panel) return;
    var els = items(panel);
    if (!els.length) return;
    // Clamp without wrap — wrap on a 2-option ask feels disorienting;
    // explicit bounds make h/l visibly stop at the ends.
    idx = Math.max(0, Math.min(els.length - 1, idx));
    activeItemIdx = idx;
    els.forEach(function (e, i) {
      var on = i === idx;
      if (e.tagName === 'INPUT') e.classList.toggle('hd-item-active', on);
      else e.classList.toggle('hd-btn-active', on);
    });
  }

  function setFocus(panel) {
    document.querySelectorAll('.panel.ask-focused').forEach(function (p) {
      p.classList.remove('ask-focused');
    });
    document.querySelectorAll('.hd-btn-active').forEach(function (b) {
      b.classList.remove('hd-btn-active');
    });
    if (!panel) return;
    panel.classList.add('ask-focused');
    // Reset the highlighted slot to the first whenever the focused
    // panel changes. For a text-mode ask the first slot is the input
    // itself, so the natural next action is `i` (or `Enter`) to start
    // typing. For other modes it's option 1.
    activeItemIdx = 0;
    setActiveItem(panel, 0);
    // Scroll the focused panel into view without yanking the page if
    // it's already on-screen. The 40px top inset matches vim-nav's
    // jumpTo offset so headings/sections feel consistent.
    var r = panel.getBoundingClientRect();
    if (r.top < 60 || r.bottom > window.innerHeight - 40) {
      window.scrollTo({ top: window.scrollY + r.top - 60, behavior: 'auto' });
    }
  }

  function focusedPanel() {
    return document.querySelector('.panel.ask-focused');
  }

  function moveFocus(dir) {
    var asks = unansweredBlocks();
    if (!asks.length) return;
    var cur = focusedPanel();
    var i = cur ? asks.indexOf(cur) : -1;
    var next = asks[((i + dir) + asks.length) % asks.length];
    if (!cur && dir < 0) next = asks[asks.length - 1];
    setFocus(next);
  }

  function clickButton(_panel, btn) {
    if (!btn) return;
    // Brief visual confirmation. The respond handler reloads on
    // success, so this flash is mostly seen on slow networks or when
    // the action errors out.
    btn.classList.add('hd-btn-firing');
    setTimeout(function () { btn.classList.remove('hd-btn-firing'); }, 200);
    btn.click();
  }

  // pickByIndex selects the Nth response button (1-indexed) of the
  // focused panel and clicks it.
  function pickByIndex(n) {
    var p = focusedPanel();
    if (!p) return false;
    var btns = buttons(p);
    if (n < 1 || n > btns.length) return false;
    clickButton(p, btns[n - 1]);
    return true;
  }

  // pickByMatch finds the first response button whose data-value or
  // text content matches one of `vals` (case-insensitive substring
  // match). Used by the y/n shortcuts.
  function pickByMatch(vals) {
    var p = focusedPanel();
    if (!p) return false;
    var btns = buttons(p);
    for (var v = 0; v < vals.length; v++) {
      var needle = vals[v].toLowerCase();
      for (var i = 0; i < btns.length; i++) {
        var dv = (btns[i].dataset.value || '').toLowerCase();
        var tx = (btns[i].textContent || '').toLowerCase();
        if (dv === needle || dv.indexOf(needle) === 0 || tx.indexOf(needle) >= 0) {
          clickButton(p, btns[i]);
          return true;
        }
      }
    }
    return false;
  }

  // submitFocused — Enter behavior. Three paths:
  //   1. Input is currently focused (INSERT mode):  submit the value.
  //   2. Highlighted slot is the input (NORMAL):    focus the input,
  //      enter INSERT — symmetric to `i`.
  //   3. Highlighted slot is a button:              click it.
  function submitFocused() {
    var p = focusedPanel();
    if (!p) return;
    var inp = input(p);
    if (inp && document.activeElement === inp) {
      var submitBtn = p.querySelector('.hd-respond[data-input]');
      if (submitBtn) clickButton(p, submitBtn);
      return;
    }
    var els = items(p);
    if (!els.length) return;
    var el = els[activeItemIdx] || els[0];
    if (el.tagName === 'INPUT') {
      el.focus();
      el.select();
      return;
    }
    clickButton(p, el);
  }

  function focusInput() {
    var p = focusedPanel();
    if (!p) return false;
    var inp = input(p);
    if (!inp) return false;
    inp.focus();
    inp.select();
    return true;
  }

  function inIgnoredTarget(e) {
    var t = e.target;
    if (!t) return false;
    var tag = t.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || t.isContentEditable) {
      // Inputs swallow keys except Esc, which we still want to handle
      // to defocus the input and the ask.
      return e.key !== 'Escape';
    }
    return false;
  }

  document.addEventListener('keydown', function (e) {
    if (inIgnoredTarget(e)) return;
    // Esc always passes through the mode-check so it can clear the
    // ask-focused highlight (and blur a focused input as a fallback).
    // Without this, INSERT mode would lock triage out of its own Esc
    // handler.
    if (window.VimNav && VimNav.getMode && VimNav.getMode() !== 'NORMAL' && e.key !== 'Escape') return;

    // Allow modifier-only keystrokes (Cmd+R, etc.) to pass through.
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    switch (e.key) {
      case 'Enter':
        if (focusedPanel()) { submitFocused(); e.preventDefault(); }
        return;
      case 'Tab': {
        if (!unansweredBlocks().length) return;
        moveFocus(e.shiftKey ? -1 : 1);
        e.preventDefault();
        return;
      }
      case 'h': {
        var p = focusedPanel();
        if (p && items(p).length > 1) {
          setActiveItem(p, activeItemIdx - 1);
          e.preventDefault();
        }
        return;
      }
      case 'l': {
        var p2 = focusedPanel();
        if (p2 && items(p2).length > 1) {
          setActiveItem(p2, activeItemIdx + 1);
          e.preventDefault();
        }
        return;
      }
      case 'Escape': {
        // Two-stage Esc:
        //   1. INSERT (input focused)  → blur the input, leave the ask
        //      focused. The input remains the "active" slot so the
        //      next l/Enter walks naturally to the submit button.
        //      Mode flips back to NORMAL via the focusout listener.
        //   2. NORMAL (no input focused) → defocus the ask entirely.
        var inFocused = document.activeElement && document.activeElement.tagName === 'INPUT';
        if (inFocused) {
          document.activeElement.blur();
          // Don't clear ask-focused — first Esc just exits INSERT.
          return;
        }
        var cur = focusedPanel();
        if (cur) cur.classList.remove('ask-focused');
        document.querySelectorAll('.hd-btn-active').forEach(function (b) {
          b.classList.remove('hd-btn-active');
        });
        document.querySelectorAll('.hd-item-active').forEach(function (b) {
          b.classList.remove('hd-item-active');
        });
        return;
      }
      case 'i':
      case 'a':
        if (focusInput()) e.preventDefault();
        return;
      case 'y':
        if (pickByMatch(['yes', 'approve'])) e.preventDefault();
        return;
      case 'n':
        if (pickByMatch(['no', 'changes'])) e.preventDefault();
        return;
    }

    // Digit keys 1-9 pick the Nth response option. Skip 0 so it stays
    // available for vim's column-start convention.
    if (e.key >= '1' && e.key <= '9') {
      if (pickByIndex(parseInt(e.key, 10))) e.preventDefault();
    }
  });

  /* Swap the text-input's placeholder based on focus so it tells the
     user what to do in each mode:
       blurred + active slot:  "hit Enter to start typing"
       focused (INSERT):       "Enter to submit · Esc to exit"
     Falls back to whatever the server-rendered placeholder was if the
     data-placeholder-* attributes are missing (defensive). */
  document.addEventListener('focusin', function (e) {
    var t = e.target;
    if (!t || !t.classList || !t.classList.contains('ask-text-input')) return;
    if (t.dataset.placeholderFocus) t.placeholder = t.dataset.placeholderFocus;
  });
  document.addEventListener('focusout', function (e) {
    var t = e.target;
    if (!t || !t.classList || !t.classList.contains('ask-text-input')) return;
    if (t.dataset.placeholderBlur) t.placeholder = t.dataset.placeholderBlur;
  });

  // Clicking an open-asks banner link also sets focus on the linked
  // ask (in addition to scrolling to it via the # anchor).
  document.addEventListener('click', function (e) {
    var a = e.target.closest && e.target.closest('.open-asks-list a');
    if (!a) return;
    var anchor = (a.getAttribute('href') || '').replace(/^#/, '');
    if (!anchor) return;
    var panel = document.getElementById(anchor);
    if (panel) setFocus(panel);
  });

  // Initial focus: first unanswered block, if any. Skip when the user
  // landed on a deep #anchor (they're targeting something specific).
  if (!location.hash) {
    var asks = unansweredBlocks();
    if (asks.length) setFocus(asks[0]);
  }
})();
