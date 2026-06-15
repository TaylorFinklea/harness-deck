// Search palette. Triggered by Cmd+K (Mac) / Ctrl+K (other), or the
// `/api/search` route directly. Live-queries the server as the user
// types — every keystroke after a 120ms debounce hits the endpoint and
// renders the top matches. ↑/↓ to navigate, Enter to open, Esc closes.
//
// Cmd+P is reserved by macOS Safari (and every other browser) for the
// print dialog, so we use Cmd+K — the de-facto standard for global
// search across modern apps (Linear, Notion, GitHub, Slack, Stripe).
//
// Loaded on every page (aggregator shell + report pages) so the user
// can jump between reports from anywhere without going back to the
// dashboard first. A click on a result calls HDPins.open when available,
// falling back to plain navigation otherwise.
(function () {
  var overlay = null;
  var input = null;
  var list = null;
  var status = null;
  var hits = [];
  var activeIdx = 0;
  var debounceTimer = null;
  var lastQuery = "";
  var inflight = false;
  var el = window.HDDom.el; // shared no-innerHTML DOM helper (hd-dom.js)

  function ensureOverlay() {
    if (overlay) return overlay;
    overlay = document.createElement("div");
    overlay.id = "search-palette";
    Object.assign(overlay.style, {
      position: "fixed",
      inset: "0",
      display: "none",
      zIndex: "99997",
      background: "rgba(19, 20, 28, 0.55)",
      backdropFilter: "blur(4px)",
    });
    input = el("input", {
      type: "text",
      placeholder: "search reports… (title, project, status, body content)",
      autocomplete: "off",
      spellcheck: "false",
    });
    status = el("span", { class: "search-status" });
    list = el("div", { class: "search-results" });
    overlay.appendChild(
      el("div", { class: "search-card" }, [
        el("div", { class: "search-input-wrap" }, [
          el("span", { class: "search-sigil", text: "?" }),
          input,
          status,
        ]),
        list,
      ])
    );

    document.body.appendChild(overlay);

    input.addEventListener("input", onInput);
    input.addEventListener("keydown", onKeydown);
    overlay.addEventListener("click", function (e) {
      if (e.target === overlay) close();
    });
    list.addEventListener("click", function (e) {
      var row = e.target.closest && e.target.closest(".search-row");
      if (!row) return;
      openRow(parseInt(row.dataset.idx, 10));
    });
    return overlay;
  }

  function open() {
    ensureOverlay();
    overlay.style.display = "flex";
    input.value = "";
    lastQuery = "";
    hits = [];
    activeIdx = 0;
    renderList();
    status.textContent = "";
    setTimeout(function () { input.focus(); }, 0);
  }

  function close() {
    if (!overlay) return;
    overlay.style.display = "none";
    input.blur();
  }

  function onInput() {
    var q = input.value.trim();
    if (debounceTimer) clearTimeout(debounceTimer);
    if (q === "") {
      hits = [];
      lastQuery = "";
      renderList();
      status.textContent = "";
      return;
    }
    debounceTimer = setTimeout(function () { runQuery(q); }, 120);
  }

  function runQuery(q) {
    if (inflight) return;
    if (q === lastQuery) return;
    inflight = true;
    status.textContent = "…";
    fetch("/api/search?q=" + encodeURIComponent(q), { cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (j) {
        lastQuery = q;
        hits = (j && j.matches) || [];
        activeIdx = 0;
        renderList();
        status.textContent = hits.length === 0 ? "no matches" : hits.length + " match" + (hits.length === 1 ? "" : "es");
      })
      .catch(function () {
        status.textContent = "search failed";
      })
      .then(function () {
        inflight = false;
        // If the user kept typing while we were fetching, run the latest.
        var cur = input.value.trim();
        if (cur !== "" && cur !== lastQuery) runQuery(cur);
      });
  }

  function renderList() {
    list.replaceChildren();
    if (hits.length === 0) return;
    hits.forEach(function (hit, i) {
      var headKids = [
        el("span", { class: "search-row-proj", text: hit.project }),
        " · ",
        el("span", { class: "search-row-title", text: hit.title || hit.run }),
      ];
      if (hit.kind) headKids.push(" ", el("span", { class: "search-row-kind", text: hit.kind }));
      if (hit.status) headKids.push(" ", el("span", { class: "search-row-status " + hit.status, text: hit.status }));

      var rowKids = [el("div", { class: "search-row-head" }, headKids)];
      if (hit.snippet) {
        // [[match]] markers were emitted by the server's snippet helper.
        rowKids.push(el("div", { class: "search-row-snippet" }, snippetNodes(hit.snippet)));
      }
      list.appendChild(el("div", {
        class: "search-row" + (i === activeIdx ? " active" : ""),
        data: { idx: String(i) },
      }, rowKids));
    });
  }

  // snippetNodes turns the server's [[match]]-bracketed snippet into an array
  // of text nodes + <mark> elements (passed as el() kids — no innerHTML, so a
  // snippet containing raw markup never injects).
  function snippetNodes(snippet) {
    var nodes = [];
    var i = 0;
    while (i < snippet.length) {
      var open = snippet.indexOf("[[", i);
      if (open < 0) {
        nodes.push(snippet.slice(i));
        break;
      }
      if (open > i) nodes.push(snippet.slice(i, open));
      var endMark = snippet.indexOf("]]", open + 2);
      if (endMark < 0) {
        nodes.push(snippet.slice(open));
        break;
      }
      nodes.push(el("mark", { text: snippet.slice(open + 2, endMark) }));
      i = endMark + 2;
    }
    return nodes;
  }

  function openRow(idx) {
    var hit = hits[idx];
    if (!hit) return;
    close();
    if (window.HDPins && HDPins.open) {
      HDPins.open(hit.project, hit.run, hit.title || hit.run);
    } else {
      location.href = "/r/" + encodeURIComponent(hit.project) + "/" + encodeURIComponent(hit.run);
    }
  }

  function onKeydown(e) {
    if (e.key === "Escape") { close(); e.preventDefault(); return; }
    if (e.key === "Enter") {
      if (hits.length > 0) openRow(activeIdx);
      e.preventDefault();
      return;
    }
    if (e.key === "ArrowDown" && hits.length > 0) {
      activeIdx = (activeIdx + 1) % hits.length;
      renderList();
      ensureActiveVisible();
      e.preventDefault();
      return;
    }
    if (e.key === "ArrowUp" && hits.length > 0) {
      activeIdx = (activeIdx - 1 + hits.length) % hits.length;
      renderList();
      ensureActiveVisible();
      e.preventDefault();
      return;
    }
  }

  function ensureActiveVisible() {
    var row = list.querySelector(".search-row.active");
    if (row && row.scrollIntoView) {
      row.scrollIntoView({ block: "nearest" });
    }
  }

  // Global trigger: Cmd+K (Mac) / Ctrl+K (other). Lowercase + capital
  // covered so Shift doesn't disqualify the match.
  document.addEventListener("keydown", function (e) {
    var trigger = (e.metaKey || e.ctrlKey) && (e.key === "k" || e.key === "K");
    if (!trigger) return;
    var t = e.target;
    if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable)) return;
    e.preventDefault();
    open();
  });

  // Expose for command palette integration etc.
  window.HDSearch = { open: open, close: close };
})();

