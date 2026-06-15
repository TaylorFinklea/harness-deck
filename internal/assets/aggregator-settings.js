  /*
     phone push notifications. The whole view renders synchronously with
     placeholder copy, then async fetches (server status, browser
     subscription state) overwrite the relevant cells via id refs. */
  function viewSettings() {
    var statusValue = el('span', { id: 'set-push-status', text: 'checking…' });
    var subValue = el('span', { id: 'set-push-sub', text: 'checking…' });
    var actions = el('div', { class: 'respond-actions', id: 'set-push-actions' }, [
      el('div', { class: 'sub', text: 'loading…' })
    ]);

    var body = [
      el('div', { class: 'kv' }, [
        el('div', null, [el('b', { text: 'push status: ' }), statusValue]),
        el('div', null, [el('b', { text: 'this browser: ' }), subValue])
      ]),
      el('div', { style: 'margin-top: 14px;' }, [actions]),
      el('div', { class: 'sub', style: 'margin-top: 18px;',
        text: 'Push needs a VAPID keypair (run `harness-deck vapid` once) and HTTPS — iOS will not show notifications over plain http.' })
    ];

    // Kick off the async refresh after render returns.
    setTimeout(refreshSettings, 0);

    return [
      panel('phone notifications', null, body),
      panel('notification destinations', null, [destinationsBody()]),
      panel('theme', null, [themePicker()]),
      panel('about', null, [
        el('div', null, [el('b', { text: 'tip: ' }),
          'open this page on your phone (over Tailscale) and tap Add to Home Screen to install harness-deck as an app.'])
      ])
    ];
  }

  /* destinationsBody — renders the "notification destinations" settings
     panel: a list of configured Slack / Discord / webhook endpoints with
     per-row test + remove buttons, and an add form below. The list is
     async-fetched after render so the panel itself is synchronous. */
  function destinationsBody() {
    var listEl = el('div', { id: 'dest-list' }, [
      el('div', { class: 'sub', text: 'loading…' })
    ]);
    setTimeout(refreshDestinations, 0);
    return el('div', null, [
      listEl,
      el('div', { class: 'sub', style: 'margin: 14px 0 6px; font-size: 10px; text-transform: uppercase; letter-spacing: 0.12em; color: var(--tn-fg-gutter);', text: 'add destination' }),
      destinationForm(),
      el('div', { class: 'sub', style: 'margin-top: 12px;', text:
        'Fan-out fires for every new ask alongside Web Push. Set public_url in config.json so the link in Slack/Discord resolves externally (defaults to bind+port, which is "0.0.0.0:7420" on open binds).'
      })
    ]);
  }

  /* destinationForm — the add-destination form. POSTs the four fields
     to /api/notifications; on success, refreshes the list and resets. */
  function destinationForm() {
    return el('form', { id: 'dest-form', class: 'dest-form' }, [
      el('div', { class: 'dest-form-row' }, [
        el('select', { name: 'type', class: 'hd-input' }, [
          el('option', { value: 'slack', text: 'slack' }),
          el('option', { value: 'discord', text: 'discord' }),
          el('option', { value: 'webhook', text: 'webhook' })
        ]),
        el('input', { name: 'name', class: 'hd-input', placeholder: 'name (e.g. team-alerts)', required: 'required' })
      ]),
      el('input', { name: 'url', class: 'hd-input', placeholder: 'https://hooks.slack.com/services/...', required: 'required' }),
      el('input', { name: 'projects', class: 'hd-input', placeholder: 'projects allowlist (comma-separated, empty = all)' }),
      el('div', { class: 'dest-form-actions' }, [
        el('button', { type: 'submit', class: 'hd-btn' }, ['add destination'])
      ])
    ]);
  }

  /* refreshDestinations — pull the current list, render rows. Called
     after add / delete so the UI matches server state. */
  function refreshDestinations() {
    var list = document.getElementById('dest-list');
    if (!list) return;
    fetch('/api/notifications').then(function (r) { return r.json(); }).then(function (data) {
      var dests = data.destinations || [];
      if (!dests.length) {
        list.replaceChildren(el('div', { class: 'sub', text: 'no destinations configured.' }));
        return;
      }
      list.replaceChildren.apply(list, dests.map(destinationRow));
    }).catch(function (e) {
      list.replaceChildren(el('div', { class: 'sub', text: 'failed to load: ' + e }));
    });
  }

  /* destinationRow — one entry in the configured list. URL host only
     (the server redacts secrets); test + remove buttons wire via data-
     attributes to the delegated click handler. */
  function destinationRow(d) {
    var kids = [
      el('div', { class: 'dest-main' }, [
        el('div', null, [
          el('b', { text: d.name }),
          el('span', { class: 'sub', text: ' · ' + d.type + (d.url_host ? ' · ' + d.url_host : '') })
        ])
      ]),
      el('div', { class: 'dest-actions' }, [
        el('button', { type: 'button', class: 'hd-btn dest-test', data: { name: d.name } }, ['test']),
        el('button', { type: 'button', class: 'hd-btn danger dest-remove', data: { name: d.name } }, ['remove'])
      ])
    ];
    if (d.projects && d.projects.length) {
      kids[0].appendChild(el('div', { class: 'sub', text: 'projects: ' + d.projects.join(', ') }));
    }
    return el('div', { class: 'dest-row' }, kids);
  }

  function addDestination(formData) {
    var body = {
      name: (formData.get('name') || '').trim(),
      type: formData.get('type'),
      url: (formData.get('url') || '').trim(),
    };
    var projects = (formData.get('projects') || '').trim();
    if (projects) {
      body.projects = projects.split(',').map(function (s) { return s.trim(); }).filter(Boolean);
    }
    return fetch('/api/notifications', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t || ('http ' + r.status)); });
      return r.json();
    });
  }

  function testDestination(name) {
    var btn = document.querySelector('.dest-test[data-name="' + cssEscape(name) + '"]');
    if (btn) { btn.disabled = true; btn.textContent = 'testing…'; }
    fetch('/api/notifications/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: name })
    }).then(function (r) { return r.json(); }).then(function (data) {
      if (btn) {
        btn.disabled = false;
        btn.textContent = data.ok ? 'sent ✓' : 'failed';
        btn.classList.toggle('ok', !!data.ok);
        setTimeout(function () {
          btn.textContent = 'test';
          btn.classList.remove('ok');
        }, 2500);
      }
      if (!data.ok && data.error) alert('test failed: ' + data.error);
    }).catch(function (e) {
      if (btn) { btn.disabled = false; btn.textContent = 'test'; }
      alert('test failed: ' + e);
    });
  }

  function removeDestination(name) {
    if (!confirm('Remove destination "' + name + '"? This rewrites config.json.')) return;
    fetch('/api/notifications/' + encodeURIComponent(name), { method: 'DELETE' })
      .then(function (r) {
        if (!r.ok) throw new Error('http ' + r.status);
        refreshDestinations();
      })
      .catch(function (e) { alert('remove failed: ' + e); });
  }

  /* cssEscape — minimal substitute for CSS.escape (Safari has it but
     older WebViews may not). Only needs to handle what dest names can
     contain (safePathComponent: alphanumeric + . _ -). */
  function cssEscape(s) { return s.replace(/["\\]/g, '\\$&'); }

  /* themePicker — three-way segmented control (system / dark / light)
     persisted to localStorage. "system" follows prefers-color-scheme
     via the @media query in tokyo-night.css. The dark/light buttons
     set data-theme on <html> immediately so the page repaints. */
  function themePicker() {
    var current = 'system';
    try {
      var t = localStorage.getItem('harness-deck:theme');
      if (t === 'light' || t === 'dark') current = t;
    } catch (_) {}
    function btn(key, label) {
      return el('button', {
        type: 'button',
        class: 'theme-btn' + (current === key ? ' active' : ''),
        data: { theme: key },
        text: label,
      });
    }
    var bar = el('div', { class: 'theme-bar' }, [
      btn('system', 'system'),
      btn('dark', 'dark'),
      btn('light', 'light'),
    ]);
    return el('div', null, [
      bar,
      el('div', { class: 'sub', style: 'margin-top: 10px;',
        text: 'system follows your OS preference (prefers-color-scheme). dark / light override it for this browser; choice persists across sessions.' }),
    ]);
  }

  function setTheme(choice) {
    try {
      if (choice === 'system') localStorage.removeItem('harness-deck:theme');
      else localStorage.setItem('harness-deck:theme', choice);
    } catch (_) {}
    if (choice === 'dark' || choice === 'light') {
      document.documentElement.setAttribute('data-theme', choice);
    } else {
      document.documentElement.removeAttribute('data-theme');
    }
    // Update active states without re-rendering the whole view.
    document.querySelectorAll('.theme-btn').forEach(function (b) {
      b.classList.toggle('active', b.dataset.theme === choice);
    });
  }

  /* refreshSettings — pulls /api/push/status and the in-browser
     PushSubscription, then re-renders the settings cells. Safe to call
     repeatedly; each call replaces the action button(s). */
  function refreshSettings() {
    var statusEl = document.getElementById('set-push-status');
    var subEl = document.getElementById('set-push-sub');
    var actEl = document.getElementById('set-push-actions');
    if (!statusEl || !subEl || !actEl) return; // not on the settings view

    fetch('/api/push/status').then(function (r) { return r.json(); }).then(function (s) {
      if (!s.enabled) {
        statusEl.textContent = 'disabled';
        statusEl.className = 'pill warn';
        actEl.replaceChildren(el('div', { class: 'sub',
          text: 'No VAPID key found. Run `harness-deck vapid` in a terminal, then restart the server.' }));
        return;
      }
      statusEl.textContent = 'ready · ' + s.subscription_count + ' device(s)';
      statusEl.className = 'pill ok';
      if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
        subEl.textContent = 'unsupported in this browser';
        actEl.replaceChildren();
        return;
      }
      navigator.serviceWorker.ready.then(function (reg) {
        return reg.pushManager.getSubscription().then(function (sub) {
          if (sub) {
            subEl.textContent = 'subscribed';
            subEl.className = 'pill ok';
            actEl.replaceChildren(
              el('button', { type: 'button', id: 'push-off', text: '⌦ unsubscribe this browser' })
            );
          } else {
            subEl.textContent = 'not subscribed';
            subEl.className = 'pill';
            actEl.replaceChildren(
              el('button', { type: 'button', id: 'push-on', text: '🔔 enable notifications on this browser' })
            );
          }
        });
      }).catch(function (err) {
        subEl.textContent = 'error: ' + err.message;
      });
    }).catch(function (err) {
      statusEl.textContent = 'error: ' + err.message;
    });
  }

  /* enablePushHere — prompt for notification permission, ask the SW for
     a PushManager subscription using the server's VAPID public key,
     and POST it. Surfaces the user-facing error inline rather than via
     alert() so the settings view stays usable. */
  function enablePushHere() {
    fetch('/api/push/vapid-key').then(function (r) {
      if (!r.ok) throw new Error('vapid-key HTTP ' + r.status);
      return r.json();
    }).then(function (j) {
      return navigator.serviceWorker.ready.then(function (reg) {
        return reg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: urlBase64ToUint8Array(j.key)
        });
      });
    }).then(function (sub) {
      return fetch('/api/push/subscribe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(sub)
      });
    }).then(function (r) {
      if (!r.ok) throw new Error('subscribe HTTP ' + r.status);
      refreshSettings();
    }).catch(function (err) {
      var sub = document.getElementById('set-push-sub');
      if (sub) { sub.textContent = 'enable failed: ' + err.message; }
    });
  }

  /* disablePushHere — unsubscribe in the browser, then tell the server
     to drop the stored Subscription. */
  function disablePushHere() {
    navigator.serviceWorker.ready.then(function (reg) {
      return reg.pushManager.getSubscription();
    }).then(function (sub) {
      if (!sub) return null;
      var endpoint = sub.endpoint;
      return sub.unsubscribe().then(function () {
        return fetch('/api/push/unsubscribe', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ endpoint: endpoint })
        });
      });
    }).then(refreshSettings).catch(function (err) {
      var sub = document.getElementById('set-push-sub');
      if (sub) { sub.textContent = 'disable failed: ' + err.message; }
    });
  }

  /* urlBase64ToUint8Array — converts the base64url VAPID key the server
     returns into the Uint8Array format PushManager.subscribe wants. */
  function urlBase64ToUint8Array(s) {
    var pad = '='.repeat((4 - (s.length % 4)) % 4);
    var b64 = (s + pad).replace(/-/g, '+').replace(/_/g, '/');
    var raw = atob(b64);
    var out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out;
  }
