// Search palette. Triggered by Cmd+K (Mac) / Ctrl+K (other), or the
// `/api/search` route directly. Live-queries the server as the user
// types — every keystroke after a 120ms debounce hits the endpoint and
// renders the top matches. ↑/↓ to navigate, Enter to open, Esc closes.
//
// The query language is a JQL-like filter (status = …, project IN (…),
// created >= -7d, mixed with free text). As the user types, a dropdown
// suggests valid fields/operators/values from /api/search/schema. The
// RAW query string is always what gets sent to /api/search — the client
// tokenizer only drives suggestions; the server parser is authoritative.
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
  var suggestBox = null;
  var saveBtn = null;
  var hits = [];
  var activeIdx = 0;
  var debounceTimer = null;
  var lastQuery = "";
  var inflight = false;
  var queryValid = false; // true when the last runQuery succeeded without a parse error
  var el = window.HDDom.el; // shared no-innerHTML DOM helper (hd-dom.js)

  // Autocomplete vocabulary, fetched once per session from the server. null
  // until the first successful fetch; a failed fetch leaves it null and the
  // suggestion engine simply stays quiet.
  var schema = null;
  var schemaFetched = false; // we attempted a fetch (success or failure)
  // Operators recognized by the tokenizer, longest-first so ">=" wins over ">".
  var OPERATORS = [">=", "<=", "!=", "!~", "=", "~", ">", "<"];
  // Boolean keywords that reset the caret to a field position (case-insensitive).
  var FIELD_BOUNDARY_KEYWORDS = { AND: true, OR: true, NOT: true };

  // Current suggestion state.
  var suggestions = []; // array of {text, insert} — text shown, insert applied
  var suggestActive = -1; // highlighted suggestion index, -1 = none/closed
  // The slice of the input that the active suggestion replaces on accept.
  var suggestReplaceStart = 0;
  var suggestReplaceEnd = 0;

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
    saveBtn = el("button", { class: "search-save", type: "button", title: "save this search" }, ["☆"]);
    list = el("div", { class: "search-results" });
    suggestBox = el("div", { class: "search-suggest" });
    suggestBox.style.display = "none";
    overlay.appendChild(
      el("div", { class: "search-card" }, [
        el("div", { class: "search-input-wrap" }, [
          el("span", { class: "search-sigil", text: "?" }),
          input,
          status,
          saveBtn,
        ]),
        suggestBox,
        list,
      ])
    );
    saveBtn.addEventListener("click", function () {
      var q = input.value.trim();
      if (!q || !queryValid) return;
      var name = prompt("Name this search", q);
      if (name == null) return;
      name = name.trim() || q;
      if (window.HDSaved) HDSaved.add(name, q);
      status.textContent = "saved ✓";
    });

    document.body.appendChild(overlay);

    input.addEventListener("input", onInput);
    input.addEventListener("keydown", onKeydown);
    // Caret moves (click, arrow within text) can change suggestions without
    // an input event; keyup catches those so the dropdown tracks the caret.
    input.addEventListener("keyup", onCaretMaybeMoved);
    input.addEventListener("click", onCaretMaybeMoved);
    overlay.addEventListener("click", function (e) {
      if (e.target === overlay) close();
    });
    list.addEventListener("click", function (e) {
      var row = e.target.closest && e.target.closest(".search-row");
      if (!row) return;
      openRow(parseInt(row.dataset.idx, 10));
    });
    suggestBox.addEventListener("click", function (e) {
      var item = e.target.closest && e.target.closest(".search-suggest-item");
      if (!item) return;
      acceptSuggestion(parseInt(item.dataset.sidx, 10));
    });
    return overlay;
  }

  function open(initialQuery) {
    ensureOverlay();
    overlay.style.display = "flex";
    if (initialQuery) {
      input.value = initialQuery;
      lastQuery = ""; // force a fresh fetch even if the same query was last run
      hits = [];
      activeIdx = 0;
      queryValid = false;
      // Hide the save button + clear stale status up front: runQuery() below
      // early-returns when a prior fetch is still inflight, so without this the
      // button/status from the last open could linger until that fetch chains.
      updateSaveBtn();
      status.textContent = "";
      renderList();
      fetchSchema();
      closeSuggest();
      updateSuggestions();
      runQuery(initialQuery);
    } else {
      input.value = "";
      lastQuery = "";
      hits = [];
      activeIdx = 0;
      queryValid = false;
      renderList();
      status.textContent = "";
      fetchSchema();
      closeSuggest();
      updateSaveBtn();
    }
    setTimeout(function () { input.focus(); }, 0);
  }

  function close() {
    if (!overlay) return;
    closeSuggest();
    overlay.style.display = "none";
    input.blur();
  }

  // fetchSchema loads the autocomplete vocabulary once per session. A failure
  // is swallowed: schema stays null and the suggestion engine goes quiet, but
  // search itself keeps working (the raw query is always what's sent).
  function fetchSchema() {
    if (schemaFetched) return;
    schemaFetched = true;
    fetch("/api/search/schema", { cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (j) {
        if (j && Array.isArray(j.fields)) schema = j;
      })
      .catch(function () { /* stay quiet; autocomplete just won't appear */ });
  }

  function onInput() {
    var q = input.value.trim();
    if (debounceTimer) clearTimeout(debounceTimer);
    updateSuggestions();
    if (q === "") {
      hits = [];
      lastQuery = "";
      queryValid = false;
      updateSaveBtn();
      renderList();
      status.textContent = "";
      return;
    }
    debounceTimer = setTimeout(function () { runQuery(q); }, 120);
  }

  function onCaretMaybeMoved(e) {
    // Don't re-derive suggestions on keys the keydown handler already owns
    // (navigation/accept/escape); only plain caret motion should refresh them.
    if (e && e.type === "keyup") {
      var k = e.key;
      if (k === "ArrowUp" || k === "ArrowDown" || k === "Enter" ||
          k === "Tab" || k === "Escape") return;
    }
    updateSuggestions();
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
        // Parse-error UX: a partially-typed / invalid query comes back with an
        // `error` field. Keep the last-good results on screen (no clear, no
        // flash) and surface the message as a low-key hint instead.
        if (j && typeof j.error === "string" && j.error !== "") {
          queryValid = false;
          updateSaveBtn();
          status.textContent = j.error;
          status.classList.add("search-status-hint");
          return;
        }
        queryValid = true;
        updateSaveBtn();
        status.classList.remove("search-status-hint");
        hits = (j && j.matches) || [];
        activeIdx = 0;
        renderList();
        status.textContent = hits.length === 0 ? "no matches" : hits.length + " match" + (hits.length === 1 ? "" : "es");
      })
      .catch(function () {
        status.classList.remove("search-status-hint");
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

  // === Suggestion engine ===
  // Tokenize the input up to the caret, classify the caret position, and offer
  // the matching vocabulary. The tokenizer is intentionally lenient: it only
  // needs to be good enough to suggest; the server parser is the source of
  // truth for what actually matches.

  // tokenizeUpToCaret splits the prefix into tokens with [start,end) offsets.
  // A token is a quoted string, a parenthesis, a comma, an operator, or a run
  // of bare chars. Whitespace separates tokens and is not itself a token.
  function tokenizeUpToCaret(text, caret) {
    var prefix = text.slice(0, caret);
    var toks = [];
    var i = 0;
    var n = prefix.length;
    while (i < n) {
      var c = prefix[i];
      if (c === " " || c === "\t") { i++; continue; }
      if (c === '"') {
        var start = i;
        i++;
        while (i < n) {
          if (prefix[i] === "\\" && i + 1 < n) { i += 2; continue; }
          if (prefix[i] === '"') { i++; break; }
          i++;
        }
        toks.push({ kind: "quoted", text: prefix.slice(start, i), start: start, end: i });
        continue;
      }
      if (c === "(" || c === ")" || c === ",") {
        toks.push({ kind: c === "," ? "comma" : "paren", text: c, start: i, end: i + 1 });
        i++;
        continue;
      }
      // Operator? Match the longest operator that starts here.
      var op = matchOperator(prefix, i);
      if (op) {
        toks.push({ kind: "op", text: op, start: i, end: i + op.length });
        i += op.length;
        continue;
      }
      // Bare run: up to the next space, paren, comma, quote, or operator start.
      var bareStart = i;
      while (i < n) {
        var ch = prefix[i];
        if (ch === " " || ch === "\t" || ch === "(" || ch === ")" ||
            ch === "," || ch === '"') break;
        if (matchOperator(prefix, i)) break;
        i++;
      }
      toks.push({ kind: "bare", text: prefix.slice(bareStart, i), start: bareStart, end: i });
    }
    return toks;
  }

  function matchOperator(s, i) {
    for (var k = 0; k < OPERATORS.length; k++) {
      var op = OPERATORS[k];
      if (s.startsWith(op, i)) return op;
    }
    return null;
  }

  function fieldSchema(name) {
    if (!schema || !schema.fields) return null;
    var lower = name.toLowerCase();
    for (var i = 0; i < schema.fields.length; i++) {
      if (schema.fields[i].name === lower) return schema.fields[i];
    }
    return null;
  }

  // valuesFor returns the value vocabulary for a field: schema.values for the
  // enumerable fields, created_hints for `created`, else null (no suggestions).
  function valuesFor(name) {
    if (!schema) return null;
    var lower = name.toLowerCase();
    if (lower === "created") return schema.created_hints || null;
    if (schema.values && schema.values[lower]) return schema.values[lower];
    return null;
  }

  function isKeyword(tok) {
    if (!tok || tok.kind !== "bare") return false;
    return !!FIELD_BOUNDARY_KEYWORDS[tok.text.toUpperCase()];
  }

  // classify inspects the tokens before the caret and the partial token the
  // caret sits in, returning {mode, field, prefix, replaceStart}. mode is one
  // of "field", "op", "value", or "none". `prefix` is the partial word being
  // typed (filtered against the vocabulary); replaceStart marks where an
  // accepted suggestion begins replacing.
  function classify(toks, caret) {
    // Determine the partial token at the caret: the last token whose end is at
    // the caret AND that isn't separated from it by whitespace. If the caret
    // follows whitespace, there's no partial token (we're starting fresh).
    var partial = null;
    if (toks.length > 0) {
      var last = toks[toks.length - 1];
      if (last.end === caret) partial = last;
    }

    // Tokens that precede the partial (the committed context).
    var committed = partial ? toks.slice(0, toks.length - 1) : toks;
    var prev = committed.length > 0 ? committed[committed.length - 1] : null;
    var prev2 = committed.length > 1 ? committed[committed.length - 2] : null;

    // Inside an open IN-list: walk back to find an unmatched "(" and, before
    // it, an "IN" operator preceded by a field. Suggest that field's values.
    var inList = openInListField(committed);
    if (inList) {
      // A partial that's a comma/paren shouldn't be treated as a value prefix.
      var listPrefix = partial && (partial.kind === "bare" || partial.kind === "quoted") ? partial.text : "";
      var lstart = partial && (partial.kind === "bare" || partial.kind === "quoted") ? partial.start : caret;
      return { mode: "value", field: inList, prefix: stripQuote(listPrefix), replaceStart: lstart };
    }

    // partial is an operator being typed → suggest operators for the field.
    if (partial && partial.kind === "op" && prev && prev.kind === "bare" &&
        !isKeyword(prev) && fieldSchema(prev.text)) {
      return { mode: "op", field: prev.text, prefix: partial.text, replaceStart: partial.start };
    }

    // partial is a bare word.
    if (partial && partial.kind === "bare") {
      // After an operator → value position.
      if (prev && prev.kind === "op" && prev2 && prev2.kind === "bare" &&
          !isKeyword(prev2) && fieldSchema(prev2.text)) {
        return { mode: "value", field: prev2.text, prefix: stripQuote(partial.text), replaceStart: partial.start };
      }
      // After a field name (no op yet) → still field position UNLESS what we're
      // typing already names a field — treat the partial as a field prefix.
      if (atFieldStart(prev)) {
        return { mode: "field", field: null, prefix: partial.text, replaceStart: partial.start };
      }
      // Otherwise it's bare text (a text term) — no suggestions.
      return { mode: "none" };
    }

    // partial is a quoted string in value position.
    if (partial && partial.kind === "quoted") {
      if (prev && prev.kind === "op" && prev2 && prev2.kind === "bare" &&
          !isKeyword(prev2) && fieldSchema(prev2.text)) {
        return { mode: "value", field: prev2.text, prefix: stripQuote(partial.text), replaceStart: partial.start };
      }
      return { mode: "none" };
    }

    // No partial token (caret after whitespace or at start). Classify by prev.
    if (!prev) {
      return { mode: "field", field: null, prefix: "", replaceStart: caret };
    }
    // After a known field with no operator yet → operator position.
    if (prev.kind === "bare" && !isKeyword(prev) && fieldSchema(prev.text)) {
      return { mode: "op", field: prev.text, prefix: "", replaceStart: caret };
    }
    // After an operator → value position (field is prev2).
    if (prev.kind === "op" && prev2 && prev2.kind === "bare" &&
        !isKeyword(prev2) && fieldSchema(prev2.text)) {
      return { mode: "value", field: prev2.text, prefix: "", replaceStart: caret };
    }
    // After a boolean keyword or "(" → field position.
    if (atFieldStart(prev)) {
      return { mode: "field", field: null, prefix: "", replaceStart: caret };
    }
    return { mode: "none" };
  }

  // atFieldStart reports whether a fresh field name is expected right after
  // `tok`: at the start (tok null), after a boolean keyword, or after "(".
  function atFieldStart(tok) {
    if (!tok) return true;
    if (tok.kind === "paren" && tok.text === "(") return true;
    if (isKeyword(tok)) return true;
    return false;
  }

  // openInListField returns the field whose value-list the caret is inside, or
  // null. It scans for an unmatched "(" whose preceding tokens are FIELD IN.
  function openInListField(committed) {
    var depth = 0;
    for (var i = committed.length - 1; i >= 0; i--) {
      var t = committed[i];
      if (t.kind === "paren" && t.text === ")") { depth++; continue; }
      if (t.kind === "paren" && t.text === "(") {
        if (depth > 0) { depth--; continue; }
        // Unmatched "(" — is it an IN/NOT IN list opener?
        var op = committed[i - 1];
        var field = committed[i - 2];
        // `IN (` → op is the "IN" bare keyword.
        if (op && op.kind === "bare" && op.text.toUpperCase() === "IN" &&
            field && field.kind === "bare" && !isKeyword(field) && fieldSchema(field.text)) {
          return field.text;
        }
        // `NOT IN (` → op="IN", before it "NOT", before that the field.
        if (op && op.kind === "bare" && op.text.toUpperCase() === "IN") {
          var notTok = committed[i - 2];
          var fld = committed[i - 3];
          if (notTok && notTok.kind === "bare" && notTok.text.toUpperCase() === "NOT" &&
              fld && fld.kind === "bare" && !isKeyword(fld) && fieldSchema(fld.text)) {
            return fld.text;
          }
        }
        return null; // an open paren that isn't an IN-list
      }
    }
    return null;
  }

  function stripQuote(s) {
    if (s && s.length > 0 && s[0] === '"') return s.slice(1);
    return s;
  }

  // updateSuggestions recomputes the dropdown contents from the input + caret.
  function updateSuggestions() {
    if (!input) return;
    var caret = input.selectionStart;
    if (caret == null) caret = input.value.length;
    var toks = tokenizeUpToCaret(input.value, caret);
    var cls = classify(toks, caret);
    var vocab = null;

    if (cls.mode === "field") {
      vocab = (schema && schema.fields) ? schema.fields.map(function (f) { return f.name; }) : null;
    } else if (cls.mode === "op") {
      var fs = fieldSchema(cls.field);
      vocab = fs ? fs.ops : null;
    } else if (cls.mode === "value") {
      vocab = valuesFor(cls.field);
    }

    if (!vocab) { closeSuggest(); return; }

    var prefixLower = (cls.prefix || "").toLowerCase();
    var matched = [];
    for (var i = 0; i < vocab.length; i++) {
      var v = vocab[i];
      if (prefixLower === "" || v.toLowerCase().indexOf(prefixLower) >= 0) {
        matched.push(v);
      }
      if (matched.length >= 12) break;
    }
    if (matched.length === 0) { closeSuggest(); return; }

    // Build insert payloads. Values that contain spaces are quoted on accept;
    // operators/fields insert verbatim. replaceStart..caret is what's replaced.
    suggestReplaceStart = cls.replaceStart;
    suggestReplaceEnd = caret;
    suggestions = matched.map(function (v) {
      var insert = v;
      if (cls.mode === "value" && /\s/.test(v) && v.indexOf('"') !== 0) {
        insert = '"' + v + '"';
      }
      return { text: v, insert: insert };
    });
    suggestActive = 0;
    renderSuggest();
  }

  function renderSuggest() {
    suggestBox.replaceChildren();
    if (suggestions.length === 0) { suggestBox.style.display = "none"; return; }
    suggestions.forEach(function (s, i) {
      suggestBox.appendChild(el("div", {
        class: "search-suggest-item" + (i === suggestActive ? " active" : ""),
        data: { sidx: String(i) },
        text: s.text,
      }));
    });
    suggestBox.style.display = "block";
  }

  function closeSuggest() {
    suggestions = [];
    suggestActive = -1;
    if (suggestBox) {
      suggestBox.replaceChildren();
      suggestBox.style.display = "none";
    }
  }

  function updateSaveBtn() {
    if (!saveBtn) return;
    var show = queryValid && input && input.value.trim() !== "";
    // Explicit "inline-block" (not "") on show: the .search-save CSS rule
    // defaults to display:none, so clearing the inline style would fall back
    // to that and the button would never appear.
    saveBtn.style.display = show ? "inline-block" : "none";
  }

  function suggestOpen() {
    return suggestActive >= 0 && suggestions.length > 0;
  }

  // acceptSuggestion splices the active suggestion's insert text into the input
  // at [suggestReplaceStart, suggestReplaceEnd), then re-derives suggestions so
  // the next position (e.g. operators after a field) opens immediately.
  function acceptSuggestion(idx) {
    if (idx < 0 || idx >= suggestions.length) return;
    var s = suggestions[idx];
    var before = input.value.slice(0, suggestReplaceStart);
    var after = input.value.slice(suggestReplaceEnd);
    // A space after fields/operators/values keeps typing flowing.
    var insert = s.insert + " ";
    input.value = before + insert + after;
    var newCaret = before.length + insert.length;
    input.setSelectionRange(newCaret, newCaret);
    closeSuggest();
    updateSuggestions();
    // Re-run the live query so results track the accepted token.
    var q = input.value.trim();
    if (q !== "") {
      if (debounceTimer) clearTimeout(debounceTimer);
      debounceTimer = setTimeout(function () { runQuery(q); }, 120);
    }
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
    // Esc closes the dropdown first; a second Esc (dropdown already closed)
    // closes the whole palette.
    if (e.key === "Escape") {
      if (suggestOpen()) { closeSuggest(); e.preventDefault(); return; }
      close();
      e.preventDefault();
      return;
    }
    // Tab accepts the active suggestion (never the default focus traversal).
    if (e.key === "Tab" && suggestOpen()) {
      acceptSuggestion(suggestActive);
      e.preventDefault();
      return;
    }
    // Enter always opens the active RESULT — never hijacked by the dropdown.
    if (e.key === "Enter") {
      if (hits.length > 0) openRow(activeIdx);
      e.preventDefault();
      return;
    }
    // Arrows move the dropdown when it's open, else the results list.
    if (e.key === "ArrowDown") {
      if (suggestOpen()) {
        suggestActive = (suggestActive + 1) % suggestions.length;
        renderSuggest();
        ensureActiveSuggestVisible();
        e.preventDefault();
        return;
      }
      if (hits.length > 0) {
        activeIdx = (activeIdx + 1) % hits.length;
        renderList();
        ensureActiveVisible();
        e.preventDefault();
        return;
      }
      return;
    }
    if (e.key === "ArrowUp") {
      if (suggestOpen()) {
        suggestActive = (suggestActive - 1 + suggestions.length) % suggestions.length;
        renderSuggest();
        ensureActiveSuggestVisible();
        e.preventDefault();
        return;
      }
      if (hits.length > 0) {
        activeIdx = (activeIdx - 1 + hits.length) % hits.length;
        renderList();
        ensureActiveVisible();
        e.preventDefault();
        return;
      }
      return;
    }
  }

  function ensureActiveVisible() {
    var row = list.querySelector(".search-row.active");
    if (row && row.scrollIntoView) {
      row.scrollIntoView({ block: "nearest" });
    }
  }

  function ensureActiveSuggestVisible() {
    var item = suggestBox.querySelector(".search-suggest-item.active");
    if (item && item.scrollIntoView) {
      item.scrollIntoView({ block: "nearest" });
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
