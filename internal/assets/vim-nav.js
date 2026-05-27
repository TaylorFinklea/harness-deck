/* Vim-flavored page navigation. Medium scope:
 *   j / k          line down / up
 *   d / u          half-page down / up   (ctrl optional)
 *   gg / G         top / bottom
 *   ] / [          next / prev section   (data-vim-section)
 *   } / {          next / prev heading   (data-vim-heading)
 *   /              search (incremental)
 *   n / N          next / prev match
 *   :              command palette
 *   ?              toggle help
 *   Esc            cancel everything
 *
 * Usage:
 *   <script src="vim-nav.js"></script>
 *   <script>VimNav.init({
 *     statusEl: '#status',         // optional, will be updated with mode
 *     commands: {                  // optional, : commands
 *       'w':      () => alert('saved'),
 *       'theme':  (arg) => setTheme(arg),
 *     },
 *     onMode: (mode) => {},        // optional callback
 *     scope: document,             // root for sections/headings/search
 *   });
 *   </script>
 *
 * The script auto-injects a minimal command/search prompt bar at the bottom
 * of the viewport when active. It uses CSS vars from tokyo-night.css; if
 * they aren't defined, it falls back to sensible defaults.
 */
(function () {
  const NS = (window.VimNav = window.VimNav || {});

  let cfg = {
    statusEl: null,
    commands: {},
    onMode: () => {},
    scope: null,
  };

  let mode = 'NORMAL';        // NORMAL | INSERT | COMMAND | SEARCH
  let buffer = '';            // operator pending (g, etc.)
  let lastFind = '';
  let matches = [];
  let matchIdx = -1;

  const setMode = (m) => {
    mode = m;
    cfg.onMode(m);
    updateStatus();
  };

  const updateStatus = () => {
    if (!cfg.statusEl) return;
    const el = typeof cfg.statusEl === 'string' ? document.querySelector(cfg.statusEl) : cfg.statusEl;
    if (!el) return;
    el.setAttribute('data-vim-mode', mode);
  };

  /* ---------- prompt UI (incremental search / : commands) ---------- */
  let prompt;
  const ensurePrompt = () => {
    if (prompt) return prompt;
    prompt = document.createElement('div');
    prompt.id = 'vim-prompt';
    // Build the prompt DOM imperatively (no innerHTML) so a malicious
    // command name in cfg.commands could never inject markup.
    const list = document.createElement('div');
    list.className = 'vim-prompt-list';
    list.style.display = 'none';
    Object.assign(list.style, {
      maxHeight: '40vh', overflowY: 'auto',
      borderBottom: '1px solid var(--tn-rule, rgba(122,162,247,.18))',
      fontSize: '12px',
    });
    const row = document.createElement('div');
    row.className = 'vim-prompt-row';
    Object.assign(row.style, {
      display: 'flex', alignItems: 'center', gap: '8px',
      padding: '6px 12px',
    });
    const sigil = document.createElement('span');
    sigil.className = 'vim-prompt-sigil';
    Object.assign(sigil.style, { color: 'var(--tn-cyan, #7dcfff)', fontWeight: '600' });
    const inp = document.createElement('input');
    inp.type = 'text';
    inp.autocomplete = 'off';
    inp.autocorrect = 'off';
    inp.spellcheck = false;
    Object.assign(inp.style, {
      flex: 1, border: 'none', outline: 'none', background: 'transparent',
      color: 'inherit', font: 'inherit', padding: 0,
    });
    const hint = document.createElement('span');
    hint.className = 'vim-prompt-hint';
    Object.assign(hint.style, { color: 'var(--tn-comment, #565f89)', fontSize: '12px' });
    row.append(sigil, inp, hint);
    prompt.append(list, row);
    Object.assign(prompt.style, {
      position: 'fixed', left: 0, right: 0, bottom: 0,
      display: 'none', flexDirection: 'column',
      background: 'var(--tn-bg-darker, #13141c)',
      borderTop: '1px solid var(--tn-rule, rgba(122,162,247,.18))',
      fontFamily: 'inherit', fontSize: '13px',
      color: 'var(--tn-fg, #c0caf5)',
      zIndex: 99999,
    });
    document.body.appendChild(prompt);
    return prompt;
  };

  const showPrompt = (sigil, hint) => {
    ensurePrompt();
    prompt.querySelector('.vim-prompt-sigil').textContent = sigil;
    prompt.querySelector('.vim-prompt-hint').textContent = hint || '';
    prompt.style.display = 'flex';
    const inp = prompt.querySelector('input');
    inp.value = '';
    setTimeout(() => inp.focus(), 0);
  };

  const hidePrompt = () => {
    if (!prompt) return;
    prompt.style.display = 'none';
    // Always clear the autocomplete list so a stale palette can't show up
    // the next time : reuses this prompt element.
    const list = prompt.querySelector('.vim-prompt-list');
    if (list) { list.style.display = 'none'; list.replaceChildren(); }
    prompt.querySelector('input').blur();
  };

  /* ---------- scrolling ---------- */
  const lineHeight = () => parseFloat(getComputedStyle(document.body).lineHeight) || 24;
  const scroller = () => {
    // Use scrollingElement so j/k works inside iframes (where window.scrollY
    // can be on documentElement OR body depending on quirks mode).
    return document.scrollingElement || document.documentElement;
  };
  const scrollBy = (dy) => {
    scroller().scrollBy({ top: dy, behavior: 'auto' });
  };

  /* ---------- section/heading nav ---------- */
  const scopeRoot = () => cfg.scope || document;
  const sections = () => Array.from(scopeRoot().querySelectorAll('[data-vim-section]'));
  const headings = () => Array.from(scopeRoot().querySelectorAll('[data-vim-heading], h1, h2, h3, h4'));

  const jumpTo = (el) => {
    if (!el) return;
    const r = el.getBoundingClientRect();
    scroller().scrollTo({ top: scroller().scrollTop + r.top - 40, behavior: 'auto' });
    // ephemeral flash so the user sees what they jumped to
    el.classList.add('vim-flash');
    setTimeout(() => el.classList.remove('vim-flash'), 400);
  };

  const nextOf = (list, dir) => {
    const sy = scroller().scrollTop;
    const els = list.map((el) => ({ el, top: el.getBoundingClientRect().top + sy }));
    if (dir > 0) return (els.find((e) => e.top > sy + 5) || els[0]).el;
    const before = els.filter((e) => e.top < sy - 5);
    return (before.length ? before[before.length - 1] : els[els.length - 1]).el;
  };

  /* ---------- search ---------- */
  const clearMatches = () => {
    scopeRoot().querySelectorAll('.vim-match').forEach((el) => {
      const t = document.createTextNode(el.textContent);
      el.replaceWith(t);
      t.parentNode && t.parentNode.normalize();
    });
    matches = [];
    matchIdx = -1;
  };

  const doSearch = (q) => {
    clearMatches();
    if (!q) return;
    const re = new RegExp(q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');
    const walker = document.createTreeWalker(scopeRoot().body || scopeRoot(), NodeFilter.SHOW_TEXT, {
      acceptNode: (n) => {
        if (!n.nodeValue.trim()) return NodeFilter.FILTER_REJECT;
        if (!n.parentElement || n.parentElement.closest('#vim-prompt, .vim-no-search, script, style')) return NodeFilter.FILTER_REJECT;
        return NodeFilter.FILTER_ACCEPT;
      },
    });
    const nodes = [];
    let n;
    while ((n = walker.nextNode())) nodes.push(n);
    nodes.forEach((tn) => {
      const txt = tn.nodeValue;
      if (!re.test(txt)) return;
      re.lastIndex = 0;
      const frag = document.createDocumentFragment();
      let last = 0;
      let m;
      while ((m = re.exec(txt))) {
        if (m.index > last) frag.appendChild(document.createTextNode(txt.slice(last, m.index)));
        const span = document.createElement('span');
        span.className = 'vim-match';
        span.textContent = m[0];
        frag.appendChild(span);
        matches.push(span);
        last = m.index + m[0].length;
        if (m.index === re.lastIndex) re.lastIndex++;
      }
      if (last < txt.length) frag.appendChild(document.createTextNode(txt.slice(last)));
      tn.replaceWith(frag);
    });
    if (matches.length) {
      matchIdx = 0;
      focusMatch();
    }
  };

  const focusMatch = () => {
    matches.forEach((m, i) => m.classList.toggle('vim-match-current', i === matchIdx));
    const m = matches[matchIdx];
    if (m) {
      const r = m.getBoundingClientRect();
      scroller().scrollTo({ top: scroller().scrollTop + r.top - window.innerHeight / 3, behavior: 'auto' });
    }
  };

  const stepMatch = (d) => {
    if (!matches.length) return;
    matchIdx = (matchIdx + d + matches.length) % matches.length;
    focusMatch();
  };

  /* ---------- command palette ---------- */
  // Built-in names handled inline by runCommand. Included in autocomplete
  // so the user discovers them.
  const BUILTIN_COMMANDS = {
    q: 'close the prompt',
    quit: 'close the prompt',
    help: 'toggle the keymap overlay',
  };

  const runCommand = (line) => {
    const [name, ...args] = line.trim().split(/\s+/);
    if (!name) return;
    const fn = cfg.commands[name];
    if (fn) {
      try { fn(args.join(' '), args); } catch (e) { console.error(e); }
    } else if (name === 'q' || name === 'quit') {
      hidePrompt();
    } else if (name === 'help') {
      toggleHelp();
    } else {
      // Unknown — flash status briefly
      flash('E492: Not an editor command: ' + name);
    }
  };

  // openCommandPalette wires up the : prompt with a wildmenu-style
  // autocomplete: typing filters the suggestion list to prefix matches,
  // ↑/↓ moves the highlight, Tab fills the input, Enter runs the
  // current input (whether or not it matches a suggestion).
  const openCommandPalette = () => {
    const inp = prompt.querySelector('input');
    const list = prompt.querySelector('.vim-prompt-list');
    const hint = prompt.querySelector('.vim-prompt-hint');
    const allNames = Object.keys(cfg.commands)
      .concat(Object.keys(BUILTIN_COMMANDS))
      .sort();
    let visible = allNames.slice();
    let activeIdx = 0;

    const describe = (name) => {
      if (cfg.descriptions && cfg.descriptions[name]) return cfg.descriptions[name];
      if (BUILTIN_COMMANDS[name]) return BUILTIN_COMMANDS[name];
      return '';
    };

    const renderList = () => {
      list.replaceChildren();
      if (!visible.length) {
        list.style.display = 'none';
        hint.textContent = 'no match';
        return;
      }
      hint.textContent = '';
      list.style.display = '';
      visible.forEach((name, i) => {
        const row = document.createElement('div');
        row.className = 'vim-prompt-row-item';
        row.dataset.name = name;
        Object.assign(row.style, {
          display: 'flex', alignItems: 'baseline', gap: '12px',
          padding: '4px 12px',
          cursor: 'pointer',
          color: i === activeIdx ? 'var(--tn-fg, #c0caf5)' : 'var(--tn-fg-dark, #a9b1d6)',
          background: i === activeIdx ? 'var(--tn-bg-highlight, #292e42)' : 'transparent',
        });
        const n = document.createElement('span');
        n.textContent = name;
        n.style.color = i === activeIdx ? 'var(--tn-cyan, #7dcfff)' : 'inherit';
        n.style.fontWeight = '500';
        const d = document.createElement('span');
        d.textContent = describe(name);
        d.style.color = 'var(--tn-comment, #565f89)';
        d.style.flex = '1';
        row.append(n, d);
        list.appendChild(row);
      });
    };

    const filter = (q) => {
      q = q.trim();
      // Match the first whitespace-delimited word (typed command name),
      // so once the user has the name + args they no longer pick from list.
      const head = q.split(/\s+/)[0];
      if (!q || q.indexOf(' ') >= 0) {
        // After a space, the user is typing args — hide the list.
        visible = [];
      } else if (!head) {
        visible = allNames.slice();
      } else {
        visible = allNames.filter((n) => n.indexOf(head) === 0);
      }
      activeIdx = 0;
      renderList();
    };

    inp.oninput = () => filter(inp.value);
    inp.onkeydown = (ev) => {
      if (ev.key === 'Enter') {
        const v = inp.value;
        hidePrompt();
        list.style.display = 'none';
        setMode('NORMAL');
        runCommand(v);
        return;
      }
      if (ev.key === 'Escape') {
        hidePrompt();
        list.style.display = 'none';
        setMode('NORMAL');
        return;
      }
      if (ev.key === 'Tab' && visible.length) {
        ev.preventDefault();
        inp.value = visible[activeIdx] + ' ';
        // Refilter so the list reflects the now-completed name.
        filter(inp.value);
        return;
      }
      if (ev.key === 'ArrowDown' && visible.length) {
        ev.preventDefault();
        activeIdx = (activeIdx + 1) % visible.length;
        renderList();
        return;
      }
      if (ev.key === 'ArrowUp' && visible.length) {
        ev.preventDefault();
        activeIdx = (activeIdx - 1 + visible.length) % visible.length;
        renderList();
        return;
      }
    };
    list.onclick = (ev) => {
      const row = ev.target.closest && ev.target.closest('.vim-prompt-row-item');
      if (!row) return;
      inp.value = row.dataset.name + ' ';
      inp.focus();
      filter(inp.value);
    };

    filter('');
  };

  const flash = (msg) => {
    ensurePrompt();
    prompt.style.display = 'flex';
    prompt.querySelector('.vim-prompt-sigil').textContent = '';
    prompt.querySelector('input').value = '';
    prompt.querySelector('.vim-prompt-hint').textContent = msg;
    prompt.querySelector('.vim-prompt-hint').style.color = 'var(--tn-red, #f7768e)';
    setTimeout(() => {
      hidePrompt();
      prompt.querySelector('.vim-prompt-hint').style.color = '';
    }, 1500);
  };

  /* ---------- help overlay ---------- */
  let helpEl;
  const buildHelp = () => {
    helpEl = document.createElement('div');
    helpEl.id = 'vim-help';
    helpEl.innerHTML = `
      <div class="vim-help-card">
        <div class="vim-help-title">~ vim keys ~</div>
        <div class="vim-help-grid">
          <kbd>j</kbd><span>line down</span>
          <kbd>k</kbd><span>line up</span>
          <kbd>d</kbd><span>½ page down</span>
          <kbd>u</kbd><span>½ page up</span>
          <kbd>gg</kbd><span>top</span>
          <kbd>G</kbd><span>bottom</span>
          <kbd>]</kbd><span>next section</span>
          <kbd>[</kbd><span>prev section</span>
          <kbd>}</kbd><span>next heading</span>
          <kbd>{</kbd><span>prev heading</span>
          <kbd>/</kbd><span>search</span>
          <kbd>n / N</kbd><span>next / prev match</span>
          <kbd>:</kbd><span>command</span>
          <kbd>gt / gT</kbd><span>next / prev tab</span>
          <kbd>?</kbd><span>toggle this</span>
          <kbd>Esc</kbd><span>cancel</span>
        </div>
        <div class="vim-help-title" style="margin-top:14px">~ triage ~</div>
        <div class="vim-help-grid">
          <kbd>Tab</kbd><span>next unanswered ask</span>
          <kbd>Shift+Tab</kbd><span>prev unanswered ask</span>
          <kbd>1 – 9</kbd><span>pick option N</span>
          <kbd>y / n</kbd><span>yes / no shortcut</span>
          <kbd>Enter</kbd><span>submit highlighted</span>
          <kbd>i</kbd><span>focus text input</span>
        </div>
        <div class="vim-help-foot">press ? or Esc to close</div>
      </div>`;
    Object.assign(helpEl.style, {
      position: 'fixed', inset: 0,
      display: 'none', alignItems: 'center', justifyContent: 'center',
      background: 'rgba(19, 20, 28, 0.78)',
      backdropFilter: 'blur(4px)',
      zIndex: 99998,
      fontFamily: 'inherit',
    });
    const card = helpEl.querySelector('.vim-help-card');
    Object.assign(card.style, {
      background: 'var(--tn-bg-dark, #16161e)',
      border: '1px solid var(--tn-rule, rgba(122,162,247,.18))',
      padding: '20px 28px',
      minWidth: '340px',
      color: 'var(--tn-fg, #c0caf5)',
      fontSize: '13px',
      lineHeight: 1.6,
      boxShadow: '0 18px 60px rgba(0,0,0,.6)',
    });
    Object.assign(helpEl.querySelector('.vim-help-title').style, {
      color: 'var(--tn-cyan, #7dcfff)',
      marginBottom: '12px',
      fontWeight: '600',
    });
    const grid = helpEl.querySelector('.vim-help-grid');
    Object.assign(grid.style, {
      display: 'grid',
      gridTemplateColumns: 'auto 1fr',
      columnGap: '20px',
      rowGap: '4px',
    });
    grid.querySelectorAll('kbd').forEach((k) => Object.assign(k.style, {
      color: 'var(--tn-green, #9ece6a)',
      fontFamily: 'inherit',
    }));
    grid.querySelectorAll('span').forEach((s) => Object.assign(s.style, {
      color: 'var(--tn-fg-dark, #a9b1d6)',
    }));
    Object.assign(helpEl.querySelector('.vim-help-foot').style, {
      marginTop: '14px',
      color: 'var(--tn-comment, #565f89)',
      fontSize: '11px',
    });
    document.body.appendChild(helpEl);
    helpEl.addEventListener('click', () => toggleHelp(false));
  };
  const toggleHelp = (force) => {
    if (!helpEl) buildHelp();
    const want = force === undefined ? helpEl.style.display === 'none' : force;
    helpEl.style.display = want ? 'flex' : 'none';
  };

  /* ---------- key handler ---------- */
  const onKey = (e) => {
    // Ignore if user is typing in any input/textarea/contenteditable
    const t = e.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) {
      if (e.key === 'Escape') {
        if (t.id !== 'vim-prompt-input' && t.closest('#vim-prompt')) {
          hidePrompt();
          setMode('NORMAL');
          buffer = '';
        }
      }
      return;
    }

    if (e.key === 'Escape') {
      buffer = '';
      hidePrompt();
      clearMatches();
      toggleHelp(false);
      setMode('NORMAL');
      return;
    }

    if (mode !== 'NORMAL') return; // prompt owns keys

    // single-char dispatch — but track buffer for "gg"
    const k = e.key;
    if (k === 'g') {
      if (buffer === 'g') { scroller().scrollTo({ top: 0 }); buffer = ''; }
      else buffer = 'g';
      e.preventDefault(); return;
    }
    buffer = '';

    switch (k) {
      case 'j': scrollBy(lineHeight()); e.preventDefault(); break;
      case 'k': scrollBy(-lineHeight()); e.preventDefault(); break;
      case 'd': scrollBy(window.innerHeight / 2); e.preventDefault(); break;
      case 'u': scrollBy(-window.innerHeight / 2); e.preventDefault(); break;
      case 'G': scroller().scrollTo({ top: scroller().scrollHeight }); e.preventDefault(); break;
      case ']': jumpTo(nextOf(sections(), 1)); e.preventDefault(); break;
      case '[': jumpTo(nextOf(sections(), -1)); e.preventDefault(); break;
      case '}': jumpTo(nextOf(headings(), 1)); e.preventDefault(); break;
      case '{': jumpTo(nextOf(headings(), -1)); e.preventDefault(); break;
      case 'n': stepMatch(1); e.preventDefault(); break;
      case 'N': stepMatch(-1); e.preventDefault(); break;
      case '/': {
        setMode('SEARCH');
        showPrompt('/', 'incremental search · Enter to commit · Esc to cancel');
        const inp = prompt.querySelector('input');
        inp.oninput = () => doSearch(inp.value);
        inp.onkeydown = (ev) => {
          if (ev.key === 'Enter') { lastFind = inp.value; hidePrompt(); setMode('NORMAL'); }
          if (ev.key === 'Escape') { clearMatches(); hidePrompt(); setMode('NORMAL'); }
        };
        e.preventDefault(); break;
      }
      case ':': {
        setMode('COMMAND');
        showPrompt(':', '');
        openCommandPalette();
        e.preventDefault(); break;
      }
      case '?': toggleHelp(); e.preventDefault(); break;
    }
  };

  NS.init = (opts) => {
    cfg = Object.assign(cfg, opts || {});
    // Inject default highlight styles for matches + section flash
    if (!document.getElementById('vim-nav-style')) {
      const s = document.createElement('style');
      s.id = 'vim-nav-style';
      s.textContent = `
        .vim-match { background: rgba(224, 175, 104, 0.25); color: var(--tn-yellow, #e0af68); border-radius: 2px; }
        .vim-match-current { background: var(--tn-yellow, #e0af68); color: var(--tn-bg, #1a1b26); }
        .vim-flash { animation: vim-flash-anim 380ms ease-out; }
        @keyframes vim-flash-anim {
          0% { background-color: rgba(122, 162, 247, 0.25); }
          100% { background-color: transparent; }
        }
        kbd {
          font-family: inherit;
          display: inline-block;
          padding: 0 4px;
          background: var(--tn-bg-highlight, #292e42);
          color: var(--tn-cyan, #7dcfff);
          border-radius: 2px;
          font-size: 0.85em;
        }
      `;
      document.head.appendChild(s);
    }
    document.addEventListener('keydown', onKey);

    // INSERT mode tracking: any focused text input flips the statusline to
    // INSERT so the user can see when their keys are typing rather than
    // navigating. The vim-nav prompts (id starts with 'vim-prompt') manage
    // their own modes (SEARCH / COMMAND); we skip them here to avoid
    // overwriting their state.
    const isVimPromptInput = (el) =>
      !!el && (el.id === 'vim-prompt-input' || (el.closest && el.closest('#vim-prompt')));
    const isTextEditable = (el) =>
      !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable);
    document.addEventListener('focusin', (e) => {
      if (isVimPromptInput(e.target)) return;
      if (isTextEditable(e.target)) setMode('INSERT');
    });
    document.addEventListener('focusout', (e) => {
      if (isVimPromptInput(e.target)) return;
      if (!isTextEditable(e.target)) return;
      // Slight defer so a focus-shift between two inputs (tab between
      // fields) doesn't briefly flicker to NORMAL.
      setTimeout(() => {
        if (!isTextEditable(document.activeElement)) setMode('NORMAL');
      }, 0);
    });

    setMode('NORMAL');
    return NS;
  };

  NS.setMode = setMode;
  NS.flash = flash;
  NS.getMode = () => mode;
  // addCommand lets later-loaded modules (the aggregator, in particular)
  // wire `:foo`-style commands into the palette without having to
  // restart the init handshake. Names are case-sensitive and unique;
  // duplicates overwrite.
  NS.addCommand = (name, fn, desc) => {
    cfg.commands = cfg.commands || {};
    cfg.commands[name] = fn;
    cfg.descriptions = cfg.descriptions || {};
    if (desc) cfg.descriptions[name] = desc;
  };
})();
