// Saved searches manager. Persists the user's named JQL search queries to
// localStorage so they can be recalled from the SAVED sidebar section on the
// dashboard. Mirrors tabs.js in structure: IIFE, try/catch around every
// localStorage call, identical CustomEvent dispatch shape.
//
// State shape (`harness-deck:saved-searches` in localStorage):
//   [ { name, query } , ... ]
//
// The renderer of the SAVED section lives in aggregator.js (dashboard only);
// this module owns the persisted state. No keydown listener — pure storage.
(function () {
  var STORAGE_KEY = 'harness-deck:saved-searches';
  var MAX_SAVED = 30;

  function load() {
    try {
      var raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return [];
      var arr = JSON.parse(raw);
      return Array.isArray(arr) ? arr : [];
    } catch (_) { return []; }
  }

  function save(searches) {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(searches)); } catch (_) {}
    // Notify listeners (the aggregator repaints its tree) via a DOM event —
    // decoupled and multi-listener, no module dependency in this direction.
    if (typeof window !== 'undefined' && window.dispatchEvent) {
      window.dispatchEvent(new CustomEvent('hd:saved-changed'));
    }
  }

  // add saves a named search query. Returns false when the query is already
  // stored (dedup by exact query string) or when query is empty. Trims inputs.
  function add(name, query) {
    name = (name || '').trim();
    query = (query || '').trim();
    if (!query) return false;
    var searches = load();
    for (var i = 0; i < searches.length; i++) {
      if (searches[i].query === query) return false; // already present
    }
    searches.push({ name: name || query, query: query });
    while (searches.length > MAX_SAVED) searches.shift();
    save(searches);
    return true;
  }

  // remove deletes the entry whose query matches exactly. Returns true on
  // success, false when not found.
  function remove(query) {
    var searches = load();
    for (var i = 0; i < searches.length; i++) {
      if (searches[i].query === query) {
        searches.splice(i, 1);
        save(searches);
        return true;
      }
    }
    return false;
  }

  // isSaved returns true when the exact query string is already stored.
  function isSaved(query) {
    var searches = load();
    for (var i = 0; i < searches.length; i++) {
      if (searches[i].query === query) return true;
    }
    return false;
  }

  window.HDSaved = {
    load: load,
    list: load, // alias for read-only consumers
    add: add,
    remove: remove,
    isSaved: isSaved,
  };
})();
