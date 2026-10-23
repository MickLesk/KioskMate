// The 1.0.0-pre.2 features: sun times, conditional inserts, inline editing,
// undo, presence, location, conflict detection, doctor, thumbnails.
import fs from "fs";
import path from "path";

const TINY_PNG = "iVBORw0KGgoAAAANSUhEUgAAAAgAAAAFCAYAAAB4ka1VAAAAFklEQVR4nGNkYPj/n4EIwESMolGF1FMIAD2cAglSyBGlAAAAAElFTkSuQmCC";

export default async function features({ browser, base, ok, errors, out, api, dir }) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: "de-DE" });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => errors.push("pageerror: " + String(e)));
  page.on("console", (m) => { if (m.type() === "error" && !/Failed to load resource/.test(m.text())) errors.push("console: " + m.text()); });
  page.on("dialog", (d) => d.accept());
  await page.goto(base + "/");
  await page.evaluate(() => { localStorage.clear(); localStorage.setItem("kioskmate.lang", "de"); localStorage.setItem("kioskmate.view", "program"); });
  await page.reload();
  await page.fill("#auth-password", "testtest1234");
  await page.click(".login-submit");
  await page.waitForSelector(".program-view");
  await page.waitForTimeout(800);
  ok("setup wizard stays closed for a configured kiosk", await page.locator(".setup-wizard").count() === 0);

  // ---- Insert with sun window and condition ----
  await page.locator(".insert-card").first().locator("[data-insert-edit]").click();
  await page.waitForSelector(".insert-dialog");
  await page.selectOption("#insert-start-kind", "sunset");
  ok("clock field: sun choice shows offset", await page.locator("#insert-start-offset").isVisible() && !(await page.locator("#insert-start").isVisible()));
  await page.fill("#insert-start-offset", "-30");
  await page.check("#insert-condition");
  ok("condition box appears", await page.locator("[data-condition]").isVisible());
  await page.fill("#insert-condition-topic", "test/motion");
  await page.fill("#insert-condition-value", "ON");
  const summary = await page.textContent("#insert-summary");
  ok("summary mentions sunset and condition", /Sonnenuntergang −30 min/.test(summary) && /test\/motion = ON/.test(summary), summary);
  await page.click('[data-action="insert-commit"]');
  await page.waitForTimeout(500);
  await page.click('[data-action="kiosk-save"]');
  await page.waitForTimeout(1200);
  let cfg = (await api("/api/config")).body;
  const insert = cfg.kiosk.inserts[0];
  ok("insert saved with sun window", insert.start === "sunset-30", insert.start);
  ok("insert saved with condition", insert.condition?.topic === "test/motion" && insert.condition?.value === "ON");
  await page.click('[data-program-range="1440"]');
  await page.waitForTimeout(800);
  ok("timeline marks conditional insert", await page.locator(".tl-insert.conditional.unmet").count() > 0, `${await page.locator(".tl-insert.conditional").count()} conditional`);
  ok("insert card shows condition state", (await page.locator(".insert-card").first().textContent()).includes("Bedingung gerade nicht erfüllt"));

  // ---- Inline duration and window ----
  const firstRow = page.locator(".base-row").first();
  ok("base row has inline window inputs", await firstRow.locator("[data-inline-start]").count() === 1);
  await firstRow.locator("[data-inline-end]").fill("21:30");
  await firstRow.locator("[data-inline-end]").dispatchEvent("change");
  await page.waitForTimeout(400);
  ok("inline edit marks dirty", (await page.textContent("[data-dirty-indicator]")).includes("Ungespeichert"));
  await page.click('[data-action="kiosk-save"]');
  await page.waitForTimeout(1200);
  cfg = (await api("/api/config")).body;
  ok("inline window end saved", cfg.kiosk.pages[0].schedule.end === "21:30" && cfg.kiosk.time_rules[0].end === "21:30", cfg.kiosk.pages[0].schedule.end);

  // ---- Undo toast ----
  const insertCount = await page.locator(".insert-card").count();
  await page.locator(".insert-card").last().locator("[data-insert-remove]").click();
  await page.waitForTimeout(300);
  ok("remove insert without confirm", await page.locator(".insert-card").count() === insertCount - 1);
  await page.click(".toast-action");
  await page.waitForTimeout(400);
  ok("undo restores the insert", await page.locator(".insert-card").count() === insertCount);

  // ---- Presence and location ----
  await page.locator(".presence-card summary").click();
  await page.check("#presence-enabled");
  await page.selectOption("#presence-mode", "wake");
  await page.fill("#presence-topic", "test/presence");
  await page.locator("#presence-topic").dispatchEvent("change");
  await page.check("#presence-touch");
  await page.locator(".advanced-settings summary").click();
  await page.fill("#location-lat", "52.52");
  await page.locator("#location-lat").dispatchEvent("change");
  await page.fill("#location-lon", "13.405");
  await page.locator("#location-lon").dispatchEvent("change");
  await page.waitForTimeout(800);
  await page.click('[data-action="kiosk-save"]');
  await page.waitForTimeout(1200);
  cfg = (await api("/api/config")).body;
  ok("presence saved", cfg.kiosk.presence?.enabled && cfg.kiosk.presence.topic === "test/presence" && cfg.kiosk.presence.touch === true, JSON.stringify(cfg.kiosk.presence));
  ok("location saved", cfg.kiosk.scheduler.latitude === 52.52 && cfg.kiosk.scheduler.longitude === 13.405);
  const timeline = (await api("/api/program/timeline?minutes=60")).body;
  ok("timeline returns sun times for the location", timeline.location === true && /^\d\d:\d\d$/.test(timeline.sun?.[1]?.sunset || ""), JSON.stringify(timeline.sun?.[1]));
  // advanced stays open across re-renders
  await page.waitForTimeout(300);
  ok("sun today shown", (await page.locator(".location-box").textContent()).includes("Sonnenuntergang"));
  const presence = await api("/api/presence", { method: "POST" });
  ok("presence API records detection", presence.body.present === true && presence.body.source === "api", JSON.stringify(presence.body));
  await page.waitForTimeout(2000);
  const status = (await api("/api/status")).body;
  ok("scheduler reports presence", status.browser?.scheduler?.presence === "present", status.browser?.scheduler?.presence);

  // ---- Conflict detection ----
  await page.click('[data-program-range="60"]');
  await page.waitForTimeout(500);
  const other = (await api("/api/config")).body;
  other.kiosk.zoom_percent = 105;
  delete other.revision;
  const otherSave = await api("/api/config", { method: "POST", body: JSON.stringify(other) });
  ok("save from elsewhere succeeds", otherSave.status === 200, String(otherSave.status));
  await page.locator(".base-row").first().locator("[data-inline-end]").fill("21:45");
  await page.locator(".base-row").first().locator("[data-inline-end]").dispatchEvent("change");
  await page.waitForTimeout(300);
  await page.click('[data-action="kiosk-save"]');
  await page.waitForSelector(".conflict-dialog", { timeout: 5000 }).catch(() => {});
  ok("conflict dialog appears", await page.locator(".conflict-dialog").count() === 1);
  await page.screenshot({ path: `${out}/features-pre2-conflict.png` });
  await page.click('[data-conflict="overwrite"]');
  await page.waitForTimeout(1500);
  cfg = (await api("/api/config")).body;
  ok("overwrite saves the change", cfg.kiosk.pages[0].schedule.end === "21:45");

  // ---- Doctor ----
  await page.goto(base + "/");
  await page.evaluate(() => localStorage.setItem("kioskmate.view", "device"));
  await page.reload();
  await page.waitForSelector(".device-view");
  await page.click('.health-card [data-action="browser-doctor"]');
  await page.waitForSelector(".doctor-dialog", { timeout: 40000 });
  ok("doctor shows a readable check list", await page.locator(".doctor-dialog .health-check").count() > 3);
  ok("doctor uses translated titles", (await page.locator(".doctor-dialog").textContent()).includes("Browser-Profil"));
  await page.screenshot({ path: `${out}/features-pre2-doctor.png` });
  await page.click(".doctor-dialog [data-modal-close]");

  // ---- Thumbnails ----
  fs.mkdirSync(path.join(dir, "thumbnails"), { recursive: true });
  fs.writeFileSync(path.join(dir, "thumbnails", "ha.jpg"), Buffer.from(TINY_PNG, "base64"));
  await page.evaluate(() => localStorage.setItem("kioskmate.view", "pages"));
  await page.reload();
  await page.waitForSelector(".library-card");
  await page.waitForTimeout(800);
  ok("pages show the thumbnail", await page.locator('.library-thumb.has-image img.page-thumb').count() === 1);
  const loaded = await page.locator("img.page-thumb").evaluate((img) => img.complete && img.naturalWidth > 0);
  ok("thumbnail image loads", loaded);
  await page.screenshot({ path: `${out}/features-pre2-pages.png` });
  await page.evaluate(() => localStorage.setItem("kioskmate.view", "program"));
  await page.reload();
  await page.waitForSelector(".program-view");
  await page.waitForTimeout(800);
  ok("base row shows the thumbnail", await page.locator(".base-row img.base-thumb").count() === 1);
  await page.screenshot({ path: `${out}/features-pre2-program.png`, fullPage: true });

  // ---- Page wizard with a sun time ----
  await page.locator(".base-row").first().locator("[data-page-edit]").click();
  await page.waitForSelector(".modal");
  await page.click('[data-wizard-next], [data-action="wizard-next"]').catch(() => {});
  await page.waitForTimeout(400);
  const hasClock = await page.locator("#wizard-schedule-start-kind").count();
  ok("page wizard offers sun times", hasClock === 1);
  await page.keyboard.press("Escape");

  // ---- Phone width ----
  await page.setViewportSize({ width: 390, height: 844 });
  for (const view of ["program", "pages", "device"]) {
    await page.evaluate((v) => localStorage.setItem("kioskmate.view", v), view);
    await page.reload();
    await page.waitForTimeout(900);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    ok(`phone ${view}: no horizontal scroll`, overflow <= 0, `${overflow}px`);
  }
  await page.evaluate(() => localStorage.setItem("kioskmate.view", "program"));
  await page.reload();
  await page.waitForTimeout(900);
  await page.screenshot({ path: `${out}/features-pre2-program-phone.png`, fullPage: true });
  await ctx.close();
}
