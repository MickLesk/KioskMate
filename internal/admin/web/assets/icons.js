// Line icons for navigation and actions (24px grid, stroke follows the text
// colour). Decorative: callers always put a text label next to them.
const PATHS = {
  overview: '<rect x="3" y="3" width="7" height="9" rx="1"/><rect x="14" y="3" width="7" height="5" rx="1"/><rect x="14" y="12" width="7" height="9" rx="1"/><rect x="3" y="16" width="7" height="5" rx="1"/>',
  program: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  pages: '<rect x="3" y="4" width="18" height="14" rx="2"/><path d="M8 21h8"/>',
  automations: '<path d="M13 2 4 14h7l-1 8 9-12h-7z"/>',
  device: '<rect x="5" y="2" width="14" height="20" rx="2"/><path d="M11 18h2"/>',
  more: '<path d="M4 6h16M4 12h16M4 18h16"/>',
  warn: '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4M12 17h.01"/>',
  close: '<path d="M6 6l12 12M18 6 6 18"/>',
  reload: '<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
};

export function icon(name, size = 20) {
  return `<svg class="icon" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">${PATHS[name] || ""}</svg>`;
}
