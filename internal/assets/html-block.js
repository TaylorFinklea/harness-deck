// Report html-block isolation.
//
// Each `html` block is rendered as
//   <hd-html><template>…raw author HTML…</template></hd-html>
// and this script moves the (inert) <template> content into a shadow root.
// That gives the block a true canvas: its <style> rules and selectors can't
// leak into the dashboard chrome, and the page's stylesheet can't bleed into
// the block. Theme custom properties (--tn-*) are *inherited*, so they pierce
// the shadow boundary — var(--tn-blue) etc. still resolve inside a block and
// live light/dark theme switches propagate automatically.
//
// Tradeoff, by design: cloning inert <template> content never executes
// scripts, so a <script> inside an html block does NOT run. html blocks are
// for layout / rendered mock-ups / inline SVG, not interactive JS widgets.
//
// Putting the author HTML inside a <template> also means nothing renders,
// no style leaks, and no resource loads before this script upgrades the
// element — the content is parser-inert until we clone it in.
(function () {
  // Non-inherited niceties scoped to the shadow root. Color and font are
  // inherited through the host, so we only set what doesn't inherit:
  // overflow containment, media max-width, and the anchor color (anchors
  // carry a UA color that isn't inherited).
  var BASE =
    ':host{display:block;overflow-x:auto}' +
    'img,svg,video,canvas{max-width:100%;height:auto}' +
    'table{max-width:100%}' +
    'a{color:var(--tn-blue,#7aa2f7)}';

  function hydrate(el) {
    if (el.shadowRoot) return; // already upgraded
    var tpl = el.querySelector('template');
    var root = el.attachShadow({ mode: 'open' });
    var style = document.createElement('style');
    style.textContent = BASE;
    root.appendChild(style);
    if (tpl) root.appendChild(tpl.content.cloneNode(true));
  }

  if (window.customElements && window.ShadowRoot) {
    customElements.define('hd-html', class extends HTMLElement {
      connectedCallback() { hydrate(this); }
    });
    return;
  }

  // Fallback: no custom elements / shadow DOM (very old browsers). Inline
  // the template content unisolated so the block is at least visible.
  function inlineAll() {
    var els = document.querySelectorAll('hd-html');
    for (var i = 0; i < els.length; i++) {
      var tpl = els[i].querySelector('template');
      if (tpl) els[i].replaceChildren(tpl.content.cloneNode(true));
    }
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', inlineAll);
  } else {
    inlineAll();
  }
})();
