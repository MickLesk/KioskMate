// First-run setup assistant against a server started with
// fixture/config-fresh.json (only the demo page).
export default async function setup({ browser, base, ok, errors, out }) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: "de-DE" });
  const page = await context.newPage();
  page.on("pageerror", (e) => errors.push(String(e)));
  await page.goto(base + "/");
  await page.evaluate(() => { localStorage.clear(); localStorage.setItem("kioskmate.lang", "de"); localStorage.setItem("kioskmate.view", "dashboard"); });
  await page.reload();
  await page.fill("#auth-password", "testtest1234");
  await page.click(".login-submit");
  await page.waitForSelector(".setup-wizard", { timeout: 10000 }).catch(() => {});
  ok("wizard opens on a fresh install", await page.locator(".setup-wizard").count() === 1);
  await page.screenshot({ path: `${out}/setup-pre2-setup-1.png` });
  await page.click('[data-setup="next"]');
  await page.waitForTimeout(200);
  ok("wizard validates the URL", await page.locator("#setup-url").count() === 1 && (await page.locator(".toast").allTextContents()).join(" ").length > 0);
  await page.fill("#setup-name", "Wohnzimmer");
  await page.fill("#setup-url", "http://homeassistant.local:8123/lovelace/kiosk");
  await page.click('[data-setup="next"]');
  await page.check('input[name="setup-screen"][value="window"]');
  await page.selectOption("#setup-end-kind", "sunset");
  await page.fill("#setup-end-offset", "60");
  await page.screenshot({ path: `${out}/setup-pre2-setup-2.png` });
  await page.click('[data-setup="next"]');
  ok("mqtt step offers skip", (await page.textContent('[data-setup="next"]')).includes("Überspringen"));
  await page.click('[data-setup="back"]');
  ok("back keeps choices", await page.locator('input[name="setup-screen"][value="window"]').isChecked());
  await page.click('[data-setup="next"]');
  await page.click('[data-setup="next"]');
  const summary = await page.textContent(".setup-summary");
  ok("summary lists the choices", summary.includes("Wohnzimmer") && summary.includes("Sonnenuntergang +60 min"), summary);
  await page.screenshot({ path: `${out}/setup-pre2-setup-4.png` });
  await page.click('[data-setup="finish"]');
  await page.waitForTimeout(2500);
  const cfg = await page.evaluate(async () => (await fetch("/api/config")).json());
  const p = cfg.kiosk.pages[0];
  ok("page saved", cfg.kiosk.pages.length === 1 && p.name === "Wohnzimmer" && p.url.endsWith("/lovelace/kiosk") && p.source_type === "home_assistant");
  ok("screen window saved as time rule", p.display_mode === "schedule" && cfg.kiosk.time_rules[0]?.end === "sunset+60" && cfg.kiosk.scheduler.enabled, JSON.stringify(cfg.kiosk.time_rules));
  ok("lands on programme", await page.locator(".program-view").count() === 1);
  ok("wizard closed", await page.locator(".setup-wizard").count() === 0);
  await page.reload();
  await page.waitForTimeout(1500);
  ok("wizard does not come back", await page.locator(".setup-wizard").count() === 0);
  await context.close();
}
