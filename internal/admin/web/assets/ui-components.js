"use strict";

window.KioskMateUI = (() => {
  const esc = (value) => String(value ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[c]);

  function field(id, label, type = "text", autocomplete = "", value = "", placeholder = "") {
    return `<div><label for="${esc(id)}">${esc(label)}</label><input id="${esc(id)}" type="${esc(type)}" ${autocomplete ? `autocomplete="${esc(autocomplete)}"` : ""} ${placeholder ? `placeholder="${esc(placeholder)}"` : ""} value="${esc(value)}" /></div>`;
  }

  function selectHtml(id, label, value, options) {
    return `<div><label for="${esc(id)}">${esc(label)}</label><select id="${esc(id)}">${(options || [])
      .map(([optionValue, text]) => `<option value="${esc(optionValue)}" ${String(optionValue) === String(value) ? "selected" : ""}>${esc(text)}</option>`)
      .join("")}</select></div>`;
  }

  function switchHtml(id, label, on) {
    return `<label class="switch" for="${esc(id)}"><span>${esc(label)}</span><input id="${esc(id)}" type="checkbox" ${on ? "checked" : ""} /></label>`;
  }

  function button(labelKey, action, cls, translate) {
    const hint = translate(action + "Hint");
    const title = hint === action + "Hint" ? translate(labelKey) : hint;
    return `<button class="${esc(cls || "")}" title="${esc(title)}" data-busy="${esc(action)}" data-action="${esc(action)}">${esc(translate(labelKey))}</button>`;
  }

  return { field, selectHtml, switchHtml, button };
})();
