// Translations live in one JSON file per language (i18n-<lang>.json) and only
// the active language is fetched. scripts/check-i18n.mjs keeps them in parity.
export const LANGUAGES = ["en", "de"];
export const I18N = {};

export async function loadLanguage(lang) {
  const code = LANGUAGES.includes(lang) ? lang : "en";
  if (I18N[code]) return code;
  const response = await fetch(`/assets/i18n-${code}.json`, { credentials: "same-origin" });
  if (!response.ok) throw new Error(`Translations for ${code} could not be loaded (HTTP ${response.status})`);
  I18N[code] = await response.json();
  return code;
}
