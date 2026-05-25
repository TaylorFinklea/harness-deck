// Search palette. Triggered by Cmd+P (Mac) / Ctrl+P (other), or the
// `/api/search` route directly. Live-queries the server as the user
// types — every keystroke after a 120ms debounce hits the endpoint and
// renders the top matches. ↑/↓ to navigate, Enter to open, Esc closes.
//
// Loaded on every page (aggregator shell + report pages) so the user
// can jump between reports from anywhere without going back to the
// dashboard first. Tab-strip flow handles the actual navigation: a
// click on a result calls HDTabs.open when available, falls back to
// plain navigation otherwise.
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
    var card = document.createElement("div");
    card.className = "search-card";
    overlay.appendChild(card);

    var inputWrap = document.createElement("div");
    inputWrap.className = "search-input-wrap";
    var sigil = document.createElement("span");
    sigil.className = "search-sigil";
    sigil.textContent = "?";
    inputWrap.appendChild(sigil);
    input = document.createElement("input");
    input.type = "text";
    input.placeholder = "search reports… (title, project, status, body content)";
    input.autocomplete = "off";
    input.spellcheck = false;
    inputWrap.appendChild(input);
    status = document.createElement("span");
    status.className = "search-status";
    inputWrap.appendChild(status);
    card.appendChild(inputWrap);

    list = document.createElement("div");
    list.className = "search-results";
    card.appendChild(list);

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
      var row = document.createElement("div");
      row.className = "search-row" + (i === activeIdx ? " active" : "");
      row.dataset.idx = String(i);

      var head = document.createElement("div");
      head.className = "search-row-head";
      var proj = document.createElement("span");
      proj.className = "search-row-proj";
      proj.textContent = hit.project;
      head.appendChild(proj);
      head.appendChild(document.createTextNode(" · "));
      var title = document.createElement("span");
      title.className = "search-row-title";
      title.textContent = hit.title || hit.run;
      head.appendChild(title);
      if (hit.kind) {
        head.appendChild(document.createTextNode(" "));
        var kind = document.createElement("span");
        kind.className = "search-row-kind";
        kind.textContent = hit.kind;
        head.appendChild(kind);
      }
      if (hit.status) {
        head.appendChild(document.createTextNode(" "));
        var st = document.createElement("span");
        st.className = "search-row-status " + hit.status;
        st.textContent = hit.status;
        head.appendChild(st);
      }
      row.appendChild(head);

      if (hit.snippet) {
        var snip = document.createElement("div");
        snip.className = "search-row-snippet";
        // [[match]] markers were emitted by the server's snippet helper.
        // Split + rebuild with <mark> spans rather than running innerHTML.
        renderSnippet(snip, hit.snippet);
        row.appendChild(snip);
      }
      list.appendChild(row);
    });
  }

  // renderSnippet inserts text nodes + <mark> spans based on the [[match]]
  // brackets the server emitted. No innerHTML so a snippet containing raw
  // markup never injects.
  function renderSnippet(parent, snippet) {
    var i = 0;
    while (i < snippet.length) {
      var open = snippet.indexOf("[[", i);
      if (open < 0) {
        parent.appendChild(document.createTextNode(snippet.slice(i)));
        return;
      }
      if (open > i) parent.appendChild(document.createTextNode(snippet.slice(i, open)));
      var endMark = snippet.indexOf("]]", open + 2);
      if (endMark < 0) {
        parent.appendChild(document.createTextNode(snippet.slice(open)));
        return;
      }
      var mark = document.createElement("mark");
      mark.textContent = snippet.slice(open + 2, endMark);
      parent.appendChild(mark);
      i = endMark + 2;
    }
  }

  function openRow(idx) {
    var hit = hits[idx];
    if (!hit) return;
    close();
    if (window.HDTabs && HDTabs.open) {
      HDTabs.open(hit.project, hit.run, hit.title || hit.run);
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

  // Global trigger: Cmd+P (Mac) / Ctrl+P (other). Use lowercase 'p'
  // alongside 'P' so Shift doesn't accidentally disqualify the match.
  document.addEventListener("keydown", function (e) {
    var trigger = (e.metaKey || e.ctrlKey) && (e.key === "p" || e.key === "P");
    if (!trigger) return;
    // Don't hijack the print dialog if user is in a text field — though
    // most users will accept Cmd+P → search, the polite default is to
    // let inputs keep their default. Vim-nav prompt input excluded too.
    var t = e.target;
    if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable)) return;
    e.preventDefault();
    open();
  });

  // Expose for command palette integration etc.
  window.HDSearch = { open: open, close: close };
})();

