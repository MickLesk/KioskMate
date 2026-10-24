// Local page templates (weather, countdown, news, camera, Immich): created in
// the page wizard or over the API, then opened like the kiosk browser does,
// with fake services from fakes.mjs behind them.
import { IMMICH_ALBUM, IMMICH_KEY } from "./fakes.mjs";

export default async function templates({ browser, base, ok, errors, out, api, fakes }) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: "de-DE" });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => errors.push("pageerror: " + String(e)));
  page.on("dialog", (d) => d.accept());
  await page.goto(base + "/");
  await page.evaluate(() => { localStorage.clear(); localStorage.setItem("kioskmate.lang", "de"); localStorage.setItem("kioskmate.view", "pages"); });
  await page.reload();
  await page.fill("#auth-password", "testtest1234");
  await page.click(".login-submit");
  await page.waitForSelector(".library-card");

  for (const type of ["weather", "countdown", "news", "camera", "immich"]) {
    ok(`templates: tile for ${type}`, await page.locator(`[data-page-new="${type}"]`).count() === 1);
  }

  await page.locator(".new-page-type").first().scrollIntoViewIfNeeded();
  await page.screenshot({ path: `${out}/template-tiles.png` });

  // News in the wizard: a suggestion chip fills the feed list.
  await page.click('[data-page-new="news"]');
  await page.waitForSelector("#wizard-local-feeds");
  ok("templates: news page named after its type", await page.inputValue("#wizard-page-name") === "Nachrichten");
  await page.click('[data-feed-add*="tagesschau"]');
  ok("templates: suggestion adds the feed", (await page.inputValue("#wizard-local-feeds")).includes("tagesschau.de"));
  await page.screenshot({ path: `${out}/template-wizard-news.png` });
  const overlap = await page.evaluate(() => {
    const bar = document.querySelector(".wizard-progress").getBoundingClientRect();
    const body = document.querySelector(".wizard-body").getBoundingClientRect();
    return body.top < bar.bottom - 1;
  });
  ok("templates: wizard body stays below the step bar", !overlap);
  await page.fill("#wizard-local-feeds", `${fakes.url}/rss`);
  await page.fill("#wizard-local-interval", "6");
  for (let step = 0; step < 2; step++) await page.click("[data-wizard-next]");
  await page.click("[data-wizard-finish]");

  // Immich in the wizard: the album address is reduced to its ID and the key
  // is required.
  await page.click('[data-page-new="immich"]');
  await page.waitForSelector("#wizard-local-immich-url");
  await page.fill("#wizard-local-immich-url", `${fakes.url}/immich/`);
  await page.fill("#wizard-local-immich-album", `${fakes.url}/immich/albums/${IMMICH_ALBUM}`);
  await page.click("[data-wizard-next]");
  ok("templates: Immich needs a key", await page.locator("#wizard-local-immich-key").count() === 1);
  await page.fill("#wizard-local-immich-key", IMMICH_KEY);
  await page.fill("#wizard-local-interval", "3");
  for (let step = 0; step < 2; step++) await page.click("[data-wizard-next]");
  await page.click("[data-wizard-finish]");
  await page.click('[data-action="kiosk-save"]');
  await page.waitForTimeout(1200);

  let cfg = (await api("/api/config")).body;
  const news = cfg.kiosk.pages.find((p) => p.source_type === "news");
  const immich = cfg.kiosk.pages.find((p) => p.source_type === "immich");
  ok("templates: news saved", news?.local?.feeds?.[0] === `${fakes.url}/rss` && news.local.interval_seconds === 6, JSON.stringify(news?.local));
  ok("templates: Immich album ID extracted", immich?.local?.immich_album === IMMICH_ALBUM && immich.local.immich_url === `${fakes.url}/immich`, JSON.stringify(immich?.local));
  ok("templates: Immich key hidden from the Admin API", immich?.local?.immich_api_key === undefined && immich?.local?.immich_key_configured === true);
  ok("templates: library cards summarise the pages", (await page.locator(".library-card", { hasText: "Immich-Fotos" }).textContent()).includes("Album"));

  // Editing keeps the stored key without retyping it.
  const immichIndex = cfg.kiosk.pages.indexOf(immich);
  await page.locator(".library-card", { hasText: "Immich-Fotos" }).locator("[data-page-edit]").click();
  await page.waitForSelector("#wizard-local-immich-key");
  ok("templates: saved key shown as saved", (await page.getAttribute("#wizard-local-immich-key", "placeholder")).includes("Gespeichert"));
  await page.keyboard.press("Escape");

  // The other templates over the API.
  cfg.kiosk.pages.push(
    { page_id: "wx", name: "Wetter", source_type: "weather", display_mode: "insert", local: { location: "Berlin", latitude: 52.52, longitude: 13.405 } },
    { page_id: "cd", name: "Urlaub", source_type: "countdown", display_mode: "insert", local: { title: "Bis zum Urlaub", target: "2031-07-01T08:00" } },
    { page_id: "cdp", name: "Vorbei", source_type: "countdown", display_mode: "insert", local: { target: "2020-01-01T00:00", done_text: "Geschafft" } },
    { page_id: "cam", name: "Kameras", source_type: "camera", display_mode: "insert", local: { title: "Haustür", streams: [`${fakes.url}/camera.png`, `${fakes.url}/camera.png?second`] } },
  );
  const saved = await api("/api/config", { method: "POST", body: JSON.stringify(cfg) });
  ok("templates: API save with the Immich key left out", saved.status === 200, JSON.stringify(saved.body).slice(0, 200));
  cfg = (await api("/api/config")).body;
  ok("templates: key still stored after saving", cfg.kiosk.pages[immichIndex]?.local?.immich_key_configured === true);

  // Open the pages like the kiosk browser does.
  const kiosk = await browser.newContext({ viewport: { width: 1280, height: 800 }, locale: "de-DE" });
  const view = await kiosk.newPage();
  view.on("pageerror", (e) => errors.push("local page: " + String(e)));
  const open = async (id, name) => {
    await view.goto(`${base}/local/${id}`);
    await view.waitForTimeout(1500);
    await view.screenshot({ path: `${out}/template-${name}.png` });
  };

  await open("wx", "weather");
  ok("weather: current temperature", (await view.textContent("#temp")) === "11°", await view.textContent("#temp"));
  ok("weather: description in German", (await view.textContent("#desc")) === "Teilweise bewölkt");
  ok("weather: eight hours and six days", await view.locator(".hour").count() === 8 && await view.locator(".day").count() === 6);
  ok("weather: icons drawn as SVG", await view.locator(".day svg").count() === 6);

  await open("cd", "countdown");
  const units = await view.locator(".unit:not([hidden]) span").allTextContents();
  ok("countdown: days to seconds", units.join(",") === "Tage,Stunden,Minuten,Sekunden", units.join(","));
  ok("countdown: title", (await view.textContent("#title")) === "Bis zum Urlaub");
  await open("cdp", "countdown-done");
  ok("countdown: done text after the target", (await view.textContent("#done")) === "Geschafft" && await view.locator("#done").isVisible());

  await open(news.page_id, "news");
  ok("news: newest headline first", (await view.textContent("#headline")).startsWith("Neue Straßenbahnlinie"), await view.textContent("#headline"));
  ok("news: source and age", /Lokalnachrichten · vor \d+ Minuten/.test(await view.textContent("#source")), await view.textContent("#source"));
  ok("news: up next list", await view.locator("#next li").count() === 3);
  await view.waitForTimeout(6500);
  ok("news: rotates to the next headline", (await view.textContent("#headline")).startsWith("Wetterdienst"), await view.textContent("#headline"));

  await open("cam", "camera");
  ok("camera: two cells", await view.locator(".cell").count() === 2 && await view.locator("main.n2").count() === 1);
  ok("camera: snapshot loads", await view.locator(".cell img").first().evaluate((img) => img.complete && img.naturalWidth === 320));

  await open(immich.page_id, "immich");
  const photo = view.locator("img.shown");
  ok("immich: photo from the album", await photo.count() === 1 && await photo.evaluate((img) => img.naturalWidth === 480 && img.src.includes("/image/")), await photo.getAttribute("src").catch(() => "none"));
  ok("immich: page does not contain the key", !(await view.content()).includes(IMMICH_KEY));

  await view.setViewportSize({ width: 600, height: 1000 });
  await open("wx", "weather-portrait");
  ok("weather: portrait fits", await view.evaluate(() => document.documentElement.scrollHeight <= window.innerHeight + 1));
  await kiosk.close();
  await ctx.close();
}
