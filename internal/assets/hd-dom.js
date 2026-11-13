// hd-dom.js — the shared no-innerHTML DOM helpers. Dashboard scripts bind these
// (var el = HDDom.el) instead of each redefining them, so "build with
// createElement + textContent, never innerHTML" is a reusable helper rather than
// a per-file convention — and separately-loaded IIFEs (e.g. future aggregator
// splits) can share one el() via this namespace. Loaded before its consumers.
window.HDDom = (function () {
  'use strict';

  /* el builds a DOM node. attrs: {class, text, data:{...}, <attr>:<val>}.
     kids: array of nodes or strings (strings become safe text nodes). */
  function el(tag, attrs, kids) {
    var n = document.createElement(tag);
    attrs = attrs || {};
    Object.keys(attrs).forEach(function (k) {
      if (k === 'text') n.textContent = attrs[k];
      else if (k === 'class') n.className = attrs[k];
      else if (k === 'data') Object.keys(attrs[k]).forEach(function (d) { n.dataset[d] = attrs[k][d]; });
      else n.setAttribute(k, attrs[k]);
    });
    (kids || []).forEach(function (c) {
      if (c == null) return;
      n.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    });
    return n;
  }

  /* htmlToNodes parses a server-rendered HTML string into detached nodes.
     A DOMParser document never executes scripts, and the only HTML passed
     here is roadmap markdown rendered (and escaped) server-side. */
  function htmlToNodes(htmlString) {
    var doc = new DOMParser().parseFromString(htmlString || '', 'text/html');
    return Array.prototype.slice.call(doc.body.childNodes);
  }

  // Native controls keep their browser keyboard behavior. Modal surfaces own
  // all keys, even if an async render temporarily moves focus to the body.
  function shortcutTarget(target, key) {
    key = key || '';
    return (window.HDDialog && HDDialog.isOpen()) || (target &&
      (target.isContentEditable || target.closest('input, textarea, select, [role="dialog"]') ||
        ((key === 'Enter' || key === ' ' || key === 'Tab' || key.indexOf('Arrow') === 0) && target.closest('button, a[href], summary'))));
  }

  function chromeHeight() {
    var chrome = document.querySelector('.term-chrome');
    return chrome ? chrome.getBoundingClientRect().height : 0;
  }

  return { el: el, htmlToNodes: htmlToNodes, shortcutTarget: shortcutTarget, chromeHeight: chromeHeight };
})();

// Shared modal lifecycle for settings, help, search and the mobile drawer.
// Keep the background inert, trap focus, and restore the invoking control.
// Register before the shortcut handlers so Escape dismisses only the top layer.
window.HDDialog = (function () {
  'use strict';
  var stack = [];
  var originalInert = new Map();
  var bodyOverflow = '';
  var focusable = 'a[href], button, input, select, textarea, [tabindex]';

  function top() { return stack[stack.length - 1]; }
  function controls(surface) {
    return Array.from(surface.querySelectorAll(focusable)).filter(function (node) {
      return node.tabIndex >= 0 && !node.disabled && !node.closest('[inert]') && node.getClientRects().length;
    });
  }
  function sync() {
    var current = top();
    originalInert.forEach(function (value, node) { node.inert = value; });
    // Walk the ancestor chain as well as body children: the mobile drawer
    // lives inside .layout, alongside the content it must make inert.
    if (current) {
      var branch = current.overlay;
      while (branch.parentElement) {
        Array.from(branch.parentElement.children).forEach(function (node) {
          if (node === branch || /^(SCRIPT|STYLE)$/.test(node.tagName)) return;
          if (!originalInert.has(node)) originalInert.set(node, node.inert);
          node.inert = true;
        });
        branch = branch.parentElement;
        if (branch === document.body) break;
      }
    }
    if (!current) {
      originalInert.forEach(function (value, node) { node.inert = value; });
      originalInert.clear();
      document.body.style.overflow = bodyOverflow;
    }
  }
  function open(overlay, options) {
    options = options || {};
    if (stack.some(function (entry) { return entry.overlay === overlay; })) return;
    if (!stack.length) {
      bodyOverflow = document.body.style.overflow;
      document.body.style.overflow = 'hidden';
    }
    var surface = options.surface || overlay.firstElementChild;
    var entry = { overlay: overlay, surface: surface, restore: options.restore || document.activeElement, close: options.close };
    stack.push(entry);
    overlay.style.display = 'flex';
    surface.setAttribute('role', 'dialog');
    surface.setAttribute('aria-modal', 'true');
    surface.setAttribute('aria-label', options.label || 'harness-deck');
    surface.tabIndex = -1;
    sync();
    (options.focus || controls(surface)[0] || surface).focus({ preventScroll: true });
  }
  function close(overlay) {
    var index = stack.findIndex(function (entry) { return entry.overlay === overlay; });
    if (index < 0) return;
    var entry = stack.splice(index, 1)[0];
    overlay.style.display = 'none';
    entry.surface.removeAttribute('aria-modal');
    sync();
    // A dynamically-rendered launcher may have been replaced while the
    // dialog was open; its owner can resolve the current equivalent here.
    var restore = typeof entry.restore === 'function' ? entry.restore() : entry.restore;
    var restored = false;
    if (restore && restore.isConnected && !restore.disabled && !restore.closest('[inert]') && restore.getClientRects().length) {
      restore.focus({ preventScroll: true });
      restored = document.activeElement === restore;
    }
    // BODY is not focusable in Safari, and a removed or hidden invoker cannot
    // take focus. Do not leave focus inside a dialog that was just hidden.
    if (!restored) {
      var fallback = top() ? (controls(top().surface)[0] || top().surface) : document.getElementById('content');
      if (fallback) fallback.focus({ preventScroll: true });
    }
  }
  document.addEventListener('keydown', function (event) {
    var current = top();
    if (!current) return;
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopImmediatePropagation();
      if (current.close) current.close();
      else close(current.overlay);
    } else if (event.key === 'Tab') {
      var nodes = controls(current.surface);
      var first = nodes[0] || current.surface;
      var last = nodes[nodes.length - 1] || current.surface;
      if (!nodes.length || (event.shiftKey && (document.activeElement === first || document.activeElement === current.surface)) ||
          (!event.shiftKey && (document.activeElement === last || !current.surface.contains(document.activeElement)))) {
        event.preventDefault();
        (event.shiftKey ? last : first).focus();
      }
    }
  }, true);
  return { open: open, close: close, isOpen: function () { return stack.length > 0; } };
})();
