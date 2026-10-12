// DOM morphing and listener scoping for the template-string views.
//
// morph() updates an existing element tree to match new HTML instead of
// replacing it with innerHTML, so nodes that did not change keep their focus,
// caret, scroll position and open state. Views keep producing HTML strings.
//
// Views attach listeners after every render (bindView and friends). Because
// morph keeps nodes alive, the same listener would be added again on each
// render. bindScope() records listeners added to elements during a bind pass
// and removes the ones from earlier passes, so every element carries exactly
// the listeners of the latest render.

const FORM_TAGS = new Set(["INPUT", "SELECT", "TEXTAREA"]);

function sameNode(from, to) {
  if (from.nodeType !== to.nodeType || from.nodeName !== to.nodeName) return false;
  if (from.nodeType !== Node.ELEMENT_NODE) return true;
  const fromId = from.getAttribute("id");
  const toId = to.getAttribute("id");
  if (fromId || toId) return fromId === toId;
  // Inputs of different types must not be morphed into each other.
  return from.nodeName !== "INPUT" || from.getAttribute("type") === to.getAttribute("type");
}

function syncAttributes(from, to) {
  for (const attr of [...from.attributes]) {
    if (!to.hasAttribute(attr.name)) from.removeAttribute(attr.name);
  }
  for (const attr of to.attributes) {
    if (from.getAttribute(attr.name) !== attr.value) from.setAttribute(attr.name, attr.value);
  }
}

function syncFormValue(from, to, options) {
  if (options.preserveFocused && from === document.activeElement) return;
  if (from.nodeName === "INPUT") {
    if (from.type === "checkbox" || from.type === "radio") {
      from.checked = to.hasAttribute("checked");
    } else if (from.type !== "file") {
      const value = to.getAttribute("value") ?? "";
      if (from.value !== value) from.value = value;
    }
  } else if (from.nodeName === "TEXTAREA") {
    const value = to.value;
    if (from.value !== value) from.value = value;
  } else if (from.nodeName === "SELECT") {
    const wanted = [...to.options].find((option) => option.hasAttribute("selected"));
    const value = wanted ? wanted.value : (to.options[0]?.value ?? "");
    if (from.value !== value) from.value = value;
  }
}

function morphNode(from, to, options) {
  if (from.nodeType !== Node.ELEMENT_NODE) {
    if (from.nodeValue !== to.nodeValue) from.nodeValue = to.nodeValue;
    return;
  }
  if (options.skip?.(from)) return;
  syncAttributes(from, to);
  if (from.nodeName !== "TEXTAREA") morphChildren(from, to, options);
  if (FORM_TAGS.has(from.nodeName)) syncFormValue(from, to, options);
}

function morphChildren(from, to, options) {
  let current = from.firstChild;
  for (const next of [...to.childNodes]) {
    if (current && sameNode(current, next)) {
      morphNode(current, next, options);
      current = current.nextSibling;
      continue;
    }
    const id = next.nodeType === Node.ELEMENT_NODE ? next.getAttribute("id") : null;
    const keyed = id ? from.querySelector(`:scope > [id="${CSS.escape(id)}"]`) : null;
    if (keyed && sameNode(keyed, next)) {
      from.insertBefore(keyed, current);
      morphNode(keyed, next, options);
      continue;
    }
    from.insertBefore(document.importNode(next, true), current);
  }
  while (current) {
    const stale = current;
    current = current.nextSibling;
    from.removeChild(stale);
  }
}

// morph replaces the children of target with html, reusing matching nodes.
// options.skip(element) leaves an element and its subtree untouched;
// options.preserveFocused keeps the value of the focused form field.
export function morph(target, html, options = {}) {
  const template = document.createElement("template");
  template.innerHTML = html;
  morphChildren(target, template.content, options);
}

const tracked = new Set();
const records = new WeakMap();
const nativeAdd = EventTarget.prototype.addEventListener;
let pass = 0;
let binding = false;

function recordingAdd(type, listener, options) {
  nativeAdd.call(this, type, listener, options);
  if (!binding || !(this instanceof Element) || !listener) return;
  let list = records.get(this);
  if (!list) {
    list = [];
    records.set(this, list);
  }
  if (list.length && list[0].pass !== pass) {
    for (const old of list) this.removeEventListener(old.type, old.listener, old.options);
    list.length = 0;
  }
  list.push({ pass, type, listener, options });
  tracked.add(this);
}

// bindScope runs fn (which attaches listeners for the current render) and
// removes listeners that earlier renders attached to elements still in use.
export function bindScope(fn) {
  if (binding) {
    // Re-entrant render from inside a bind pass: same pass.
    fn();
    return;
  }
  pass += 1;
  binding = true;
  EventTarget.prototype.addEventListener = recordingAdd;
  try {
    fn();
  } finally {
    EventTarget.prototype.addEventListener = nativeAdd;
    binding = false;
    for (const element of tracked) {
      const list = records.get(element);
      if (!element.isConnected || !list || list[0]?.pass !== pass) {
        for (const old of list || []) element.removeEventListener(old.type, old.listener, old.options);
        records.delete(element);
        tracked.delete(element);
      }
    }
  }
}
