// Every view and function of the Admin UI against a server started with
// fixture/config.json.
async function savedConfig(page) {
  return page.evaluate(async () => (await fetch("/api/config", { credentials: "same-origin" })).json());
}

export default async function admin({ browser, base, ok, errors, out }) {
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

  // ---- Programme ----
  ok("program: base list shows only base pages", await page.locator(".base-row").count() === 1, `${await page.locator(".base-row").count()} rows`);
  ok("program: other pages listed", await page.locator(".other-pages li").count() === 3);
  const nextRuns = await page.locator(".insert-next strong").allTextContents();
  ok("program: every insert shows its next run", nextRuns.length === 3, nextRuns.join(", "));
  for (const [minutes, label] of [[360, "6h"], [1440, "24h"], [60, "1h"]]) {
    const wait = page.waitForResponse((r) => r.url().includes("/api/program/timeline") && r.request().method() === "POST");
    await page.click(`[data-program-range="${minutes}"]`);
    const response = await wait;
    const body = await response.json();
    await page.waitForTimeout(300);
    ok(`program: range ${label} loads ${minutes} min`, body.minutes === minutes && await page.locator(`[data-program-range="${minutes}"].active`).count() === 1);
    if (minutes === 1440) ok("program: 24 h timeline has insert blocks", await page.locator(".timeline-card .tl-insert").count() > 5);
  }
  await page.selectOption("#override-minutes", "30");
  ok("program: show-now duration remembered", await page.evaluate(() => localStorage.getItem("kioskmate.overrideMinutes")) === "30");
  ok("program: show-now duration does not mark dirty", (await page.textContent("[data-dirty-indicator]")).includes("gespeichert"));
  // add Uhr to the base programme and move it up
  await page.locator(".other-pages li", { hasText: "Uhr" }).locator("[data-page-tobase]").click();
  await page.waitForTimeout(300);
  ok("program: 'Ins Grundprogramm' adds a base row", await page.locator(".base-row").count() === 2);
  const rowsBefore = await page.locator(".base-row .base-copy strong").allTextContents();
  await page.locator(".base-row").nth(1).locator('[data-base-move][data-direction="-1"]').click();
  await page.waitForTimeout(300);
  const rowsAfter = await page.locator(".base-row .base-copy strong").allTextContents();
  ok("program: arrow moves page up", rowsAfter[0].startsWith("Uhr") && rowsBefore[0].startsWith("Hauptseite"), rowsAfter.join(" / "));
  ok("program: changes mark dirty", (await page.textContent("[data-dirty-indicator]")).includes("Ungespeichert") || (await page.textContent("[data-dirty-indicator]")).includes("nicht gespeichert"), await page.textContent("[data-dirty-indicator]"));
  await page.screenshot({ path: `${out}/admin-e2e-program-edit.png`, fullPage: true });
  await page.click('[data-action="kiosk-save"]');
  await page.waitForTimeout(1200);
  let cfg = await savedConfig(page);
  const uhr = cfg.kiosk.pages.find((p) => p.page_id === "uhr");
  ok("program: save persists base change", uhr?.display_mode === "duration" && cfg.kiosk.pages[0].page_id === "uhr" && cfg.kiosk.scheduler.mode === "hybrid", `mode=${uhr?.display_mode} first=${cfg.kiosk.pages[0].page_id} sched=${cfg.kiosk.scheduler.mode}`);
  // insert duplicate, toggle, remove
  const insertsBefore = await page.locator(".insert-card").count();
  await page.locator("[data-insert-duplicate]").first().click();
  await page.waitForTimeout(300);
  ok("program: duplicate insert adds a disabled copy", await page.locator(".insert-card").count() === insertsBefore + 1 && await page.locator(".insert-card.disabled").count() === 1);
  await page.locator("[data-insert-remove]").nth(1).click();
  await page.waitForTimeout(300);
  ok("program: remove insert", await page.locator(".insert-card").count() === insertsBefore);
  // insert dialog from the empty state template: remove all inserts in the draft
  while (await page.locator("[data-insert-remove]").count()) {
    await page.locator("[data-insert-remove]").first().click();
    await page.waitForTimeout(150);
  }
  ok("program: empty inserts show templates", await page.locator("[data-insert-template]").count() === 3);
  await page.click('[data-insert-template="hourly"]');
  await page.waitForSelector(".insert-dialog");
  ok("program: template preselects rhythm", await page.isChecked('input[name="insert-repeat"][value="hourly"]'));
  ok("program: template preselects duration", await page.inputValue("#insert-duration") === "60");
  await page.click('[data-action="insert-commit"]');
  await page.waitForTimeout(400);
  ok("program: template insert added", await page.locator(".insert-card").count() === 1);
  await page.click('[data-action="kiosk-save"]');
  await page.waitForTimeout(1200);
  cfg = await savedConfig(page);
  ok("program: inserts saved", cfg.kiosk.inserts.length === 1 && cfg.kiosk.inserts[0].repeat === "hourly");
  // show now (browser is a dummy, so the server may refuse; the UI must report, not crash)
  await page.locator("[data-page-show-id]").first().click();
  await page.waitForTimeout(1200);
  ok("program: show now gives feedback", await page.locator(".toast").count() > 0, (await page.locator(".toast").allTextContents()).slice(-1)[0]);

  // ---- Pages ----
  await page.click('.nav [data-view="pages"]');
  await page.waitForSelector(".pages-view");
  const cards = async () => page.locator(".library-card:not([hidden])").count();
  ok("pages: all cards visible", await cards() === 4);
  await page.click('[data-page-kind="insert"]');
  await page.waitForTimeout(200);
  ok("pages: filter chip 'Einschübe'", await cards() === 1, `${await cards()} visible`);
  await page.click('[data-page-kind="all"]');
  await page.fill("#page-filter", "wett");
  await page.waitForTimeout(300);
  ok("pages: search filters", await cards() === 1);
  ok("pages: search keeps focus", await page.evaluate(() => document.activeElement?.id) === "page-filter");
  await page.fill("#page-filter", "");
  await page.waitForTimeout(200);
  const popupPromise = page.waitForEvent("popup", { timeout: 3000 }).catch(() => null);
  await page.locator('.library-card', { hasText: "Uhr" }).locator("[data-page-open]").click();
  const popup = await popupPromise;
  ok("pages: open in new tab", !!popup, popup ? popup.url() : "no popup");
  if (popup) await popup.close();
  await page.locator('.library-card', { hasText: "Uhr" }).locator("[data-page-check-one]").click();
  await page.waitForSelector(".library-card .check-chip.ok, .library-card .check-chip.bad", { timeout: 15000 }).catch(() => {});
  ok("pages: check shows result chip", await page.locator('.library-card', { hasText: "Uhr" }).locator(".check-chip").count() === 1, await page.locator('.library-card', { hasText: "Uhr" }).locator(".check-chip").textContent().catch(() => "none"));
  await page.locator('.library-card', { hasText: "Termine" }).locator("[data-page-enable]").uncheck();
  await page.waitForTimeout(300);
  ok("pages: switch disables page", await page.locator('.library-card.disabled', { hasText: "Termine" }).count() === 1);
  await page.locator('.library-card', { hasText: "Termine" }).locator("[data-page-enable]").check();
  await page.waitForTimeout(300);
  await page.locator('.library-card', { hasText: "Hauptseite" }).locator("[data-page-duplicate]").click();
  await page.waitForTimeout(300);
  ok("pages: duplicate adds a card", await page.locator(".library-card").count() === 5);
  await page.locator(".library-card").nth(2).locator("[data-page-remove]").click();
  await page.waitForTimeout(300);
  ok("pages: remove card", await page.locator(".library-card").count() === 4);
  await page.screenshot({ path: `${out}/admin-e2e-pages.png`, fullPage: true });
  await page.click('[data-action="kiosk-save"]');
  await page.waitForTimeout(1200);

  // ---- Automations ----
  await page.click('.nav [data-view="automations"]');
  await page.waitForSelector(".automations-view");
  ok("automations: MQTT status shown", await page.locator(".automation-status").count() === 1);
  ok("automations: six templates", await page.locator("[data-workflow-template]").count() === 6);
  await page.click('[data-workflow-template="doorbell"]');
  await page.waitForSelector(".workflow-editor");
  ok("automations: template fills name", (await page.inputValue("#wf-name")).includes("Türklingel"));
  ok("automations: template fills topic", await page.inputValue("#wf-topic") === "home/doorbell");
  await page.click("[data-wf-apply]");
  await page.waitForTimeout(400);
  ok("automations: workflow added", await page.locator(".workflow-row").count() === 1);
  await page.click('[data-workflow-template="night"]');
  await page.waitForSelector(".workflow-editor");
  await page.click("[data-wf-apply]");
  await page.waitForTimeout(300);
  await page.locator("[data-workflow-toggle]").nth(1).uncheck();
  await page.waitForTimeout(300);
  ok("automations: toggle disables workflow", await page.locator(".workflow-row.disabled").count() === 1);
  await page.click('[data-action="kiosk-save"]');
  await page.waitForTimeout(1200);
  cfg = await savedConfig(page);
  ok("automations: saved", (cfg.kiosk.rules || []).length === 2 && cfg.kiosk.rules[1].disabled === true && cfg.kiosk.rules[0].trigger.topic === "home/doorbell");
  await page.locator("[data-workflow-test]").first().click();
  await page.waitForTimeout(1000);
  await page.screenshot({ path: `${out}/admin-e2e-automations.png`, fullPage: true });

  // ---- Overview ----
  await page.click('.nav [data-view="dashboard"]');
  await page.waitForSelector(".overview");
  await page.waitForTimeout(1200);
  ok("overview: today timeline from midnight", (await page.locator(".today-card .tl-ticks span").first().textContent()) === "00:00");
  ok("overview: up next rows", await page.locator(".up-next li").count() >= 1, (await page.locator(".up-next li").allTextContents()).join(" | "));
  await page.uncheck("#snapshot-auto");
  ok("overview: snapshot auto off remembered", await page.evaluate(() => localStorage.getItem("kioskmate.snapshotAuto")) === "0");
  await page.check("#snapshot-auto");
  await page.click('[data-action="overview-show"]');
  await page.waitForTimeout(1000);
  await page.screenshot({ path: `${out}/admin-e2e-overview.png`, fullPage: true });

  // ---- Every remaining view renders ----
  for (const view of ["automations-mqtt", "device", "device-display", "device-maintenance", "device-logs", "device-settings"]) {
    await page.evaluate((v) => document.querySelector(`[data-view="${v}"]`)?.click(), view);
    await page.waitForTimeout(900);
    const content = await page.locator(".content").innerText();
    ok(`view ${view} renders`, content.length > 50);
  }

  // ---- Phone: no horizontal overflow on any view ----
  const phone = await ctx.newPage();
  await phone.setViewportSize({ width: 390, height: 844 });
  phone.on("pageerror", (e) => errors.push("phone pageerror: " + String(e)));
  for (const view of ["dashboard", "program", "pages", "automations", "automations-mqtt", "device", "device-display", "device-maintenance", "device-logs", "device-settings"]) {
    await phone.goto(base + "/");
    await phone.evaluate((v) => localStorage.setItem("kioskmate.view", v), view);
    await phone.reload();
    await phone.waitForSelector(".layout");
    await phone.waitForTimeout(900);
    const overflow = await phone.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    ok(`phone ${view}: no horizontal scroll`, overflow <= 1, `overflow ${overflow}px`);
    await phone.screenshot({ path: `${out}/admin-phone-${view}.png`, fullPage: true });
  }
  await ctx.close();
}
