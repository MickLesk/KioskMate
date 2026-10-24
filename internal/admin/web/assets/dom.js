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

// Form fields the user changed since their last foreground render. Background
// renders (live status updates) keep their values, so a half-edited form can
// stay on screen while everything around it updates.
const edited = new WeakSet();
const noteEdit = (event) => {
  if (FORM_TAGS.has(event.target?.nodeName)) edited.add(event.target);
};
document.addEventListener("input", noteEdit, true);
document.addEventListener("change", noteEdit, true);

// Elements whose content an action wrote directly (check results, test
// output, log text). Background renders leave them alone, otherwise a live
// status update would wipe a result the user is reading. Writes are found
// with a MutationObserver; mutations made by morph itself are discarded.
const external = new WeakSet();
const observed = new WeakSet();
const observer = typeof MutationObserver === "undefined" ? null : new MutationObserver(collectExternal);

function collectExternal(records) {
  for (const record of records) {
    const element = record.target.nodeType === Node.ELEMENT_NODE ? record.target : record.target.parentElement;
    if (element) external.add(element);
  }
}

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
  if (options.preserveEdited && edited.has(from)) return;
  edited.delete(from);
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
  if (options.preserveExternal && external.has(from)) return;
  if (!options.preserveExternal) external.delete(from);
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
// options.preserveFocused keeps the value of the focused form field;
// options.preserveEdited keeps the values of fields the user changed.
// options.preserveExternal keeps elements an action wrote to directly.
export function morph(target, html, options = {}) {
  if (observer && !observed.has(target)) {
    observer.observe(target, { childList: true, characterData: true, subtree: true });
    observed.add(target);
  }
  if (observer) collectExternal(observer.takeRecords());
  const template = document.createElement("template");
  template.innerHTML = html;
  morphChildren(target, template.content, options);
  observer?.takeRecords();
}

const tracked = new Set();
const records = new WeakMap();
const nativeAdd = EventTarget.prototype.addEventListener;
let pass = 0;
let binding = false;

function recordingAdd(type, listener, options) {
  if (!binding || !(this instanceof Element) || !listener) {
    nativeAdd.call(this, type, listener, options);
    return;
  }
  let list = records.get(this);
  if (!list) {
    list = [];
    records.set(this, list);
  }
  // Drop the previous render's listeners before adding: a named function
  // bound again (same reference) would otherwise be added as a no-op and
  // then removed with the old ones, leaving the element without it.
  if (list.length && list[0].pass !== pass) {
    for (const old of list) this.removeEventListener(old.type, old.listener, old.options);
    list.length = 0;
  }
  nativeAdd.call(this, type, listener, options);
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
