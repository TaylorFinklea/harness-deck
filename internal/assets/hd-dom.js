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

  return { el: el, htmlToNodes: htmlToNodes };
})();
