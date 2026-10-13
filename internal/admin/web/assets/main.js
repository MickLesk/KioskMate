// Admin UI entry point: load the active language, then start the app.
import { boot, state } from "./app-core.js";
import { loadLanguage } from "./i18n.js";

state.lang = await loadLanguage(state.lang).catch(() => loadLanguage("en"));
document.documentElement.lang = state.lang;
boot();
