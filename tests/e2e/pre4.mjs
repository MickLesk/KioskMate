// The 1.0.0-pre.4 round: CPU per page, profile explanation, stopped browser
// tile and the other Admin UI, robustness and security additions.
import fs from "fs";
import path from "path";

export default async function pre4({ browser, base, ok, errors, out, api, fakes }) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: "de-DE" });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => errors.push("pre4 pageerror: " + String(e)));
  page.on("console", (m) => { if (m.type() === "error" && !/Failed to load resource/.test(m.text())) errors.push("pre4 console: " + m.text()); });
  page.on("dialog", (d) => d.accept());
  await page.goto(base + "/");
  await page.evaluate(() => { localStorage.clear(); localStorage.setItem("kioskmate.lang", "de"); localStorage.setItem("kioskmate.view", "dashboard"); });
  await page.reload();
  await page.fill("#auth-password", "testtest1234");
  await page.click(".login-submit");
  await page.waitForSelector(".overview");
  await page.waitForTimeout(800);

  // ---- Overview: no fake 0 % while the browser is stopped ----
  const tile = page.locator(".status-strip .status-tile").first();
  ok("overview: stopped browser load shows a dash", (await tile.locator("strong").textContent()) === "–", await tile.innerText());

  // ---- Telemetry: CPU per page ----
  const telemetry = (await api("/api/browser/telemetry")).body;
  ok("telemetry: pages most expensive first", telemetry.pages?.[0]?.page_id === "wetter" && telemetry.pages?.length === 2, JSON.stringify(telemetry.pages?.map((p) => p.page_id)));
  await page.click('.nav [data-view="device"]');
  await page.waitForTimeout(600);
  await page.evaluate(() => document.querySelector('[data-view="device-display"]')?.click());
  await page.waitForSelector(".page-cpu", { timeout: 8000 }).catch(async () => { await page.screenshot({ path: `${out}/pre4-fail.png`, fullPage: true }); });
  const rows = await page.locator(".page-cpu tbody tr").allInnerTexts();
  ok("telemetry: page table lists names", rows.length === 2 && rows[0].includes("Wetter & Kameras") && rows[1].includes("Hauptseite"), rows.join(" | "));
  ok("telemetry: expensive page marked", await page.locator(".page-cpu tbody tr.warn .tag.warn").count() === 1);
  ok("telemetry: bars sized by script", await page.locator(".page-cpu-bar i").first().evaluate((el) => el.style.width === "100%"));
  ok("profile: effect explained", (await page.textContent("#perf-profile-hint")).startsWith("Ausgewogen"), await page.textContent("#perf-profile-hint"));
  await page.locator(".page-cpu").screenshot({ path: `${out}/pre4-page-cpu.png` });

  // ---- Toasts: capped, repeats folded, API errors translated ----
  await page.evaluate(() => document.querySelectorAll(".toast").forEach((t) => t.remove()));
  await page.evaluate(async () => {
    const core = await import("/assets/app-core.js");
    for (let i = 0; i < 3; i++) core.toast("Fehlgeschlagen", "1 display: display power control unsupported", "error");
    for (const name of ["a", "b", "c", "d"]) core.toast(name);
  });
  const toastTexts = await page.locator(".toast").allInnerTexts();
  ok("toasts: at most three on screen", toastTexts.length === 3, toastTexts.join(" | "));
  await page.evaluate(() => document.querySelectorAll(".toast").forEach((t) => t.remove()));
  await page.evaluate(async () => {
    const core = await import("/assets/app-core.js");
    for (let i = 0; i < 3; i++) core.toast("Fehlgeschlagen", "1 display: display power control unsupported", "error");
  });
  const folded = await page.locator(".toast").allInnerTexts();
  ok("toasts: repeats folded with a counter", folded.length === 1 && folded[0].includes("×3"), folded.join(" | "));
  ok("toasts: backend error translated", folded[0]?.includes("Dieses Gerät kann den Bildschirm nicht ein- und ausschalten"), folded[0]);
  await page.evaluate(() => document.querySelectorAll(".toast").forEach((t) => t.remove()));

  // ---- Device health: missing touchscreen, processor per core ----
  await page.click('.nav [data-view="device"]');
  await page.waitForSelector(".health-checks");
  await page.click('[data-action="health-refresh"]');
  await page.waitForTimeout(6000);
  await page.click('[data-action="health-refresh"]');
  await page.waitForTimeout(1500);
  const health = await page.locator(".health-check").allInnerTexts();
  ok("health: touchscreen seen before is reported missing", health.some((text) => text.includes("Testpanel Touch") && text.includes("fehlt jetzt")), health.find((text) => text.includes("Touch")) || "none");
  await page.locator(".status-strip").first().screenshot({ path: `${out}/pre4-processor.png` });
  const cores = await page.locator(".processor-tile .core-bars i").count();
  ok("device: processor tile with one bar per core", cores >= 1, `${cores} bars`);

  // ---- Doctor: readable details, focus trap, Escape, copy ----
  const doctorButton = page.locator('[data-action="browser-doctor"]').first();
  await doctorButton.focus();
  await doctorButton.click();
  await page.waitForSelector(".doctor-dialog", { timeout: 30000 });
  await page.locator(".doctor-dialog details.tech-details").first().evaluate((d) => { d.open = true; });
  ok("doctor: technical details as a list", await page.locator(".doctor-dialog .detail-list dt").count() > 0 && await page.locator(".doctor-dialog pre.logbox").count() === 0);
  ok("doctor: touch details translated", (await page.locator(".doctor-dialog").innerText()).includes("Der Touchscreen war schon erkannt"));
  for (let i = 0; i < 40; i++) await page.keyboard.press("Tab");
  ok("dialog: focus stays inside", await page.evaluate(() => !!document.activeElement?.closest(".modal")));
  await page.click("[data-doctor-copy]");
  await page.waitForTimeout(200);
  ok("doctor: copy gives feedback", (await page.locator(".toast").allInnerTexts()).some((text) => /kopiert|Kopieren/i.test(text)));
  await page.keyboard.press("Escape");
  await page.waitForTimeout(200);
  ok("dialog: Escape closes", await page.locator(".doctor-dialog").count() === 0);
  ok("dialog: focus returns to the opener", await page.evaluate(() => document.activeElement?.dataset?.action === "browser-doctor"));

  // ---- Shell: skip link and tab title ----
  ok("shell: skip link targets the content", await page.locator('.skip-link[href="#main-content"]').count() === 1 && await page.locator("#main-content").count() === 1);
  ok("shell: tab title warns while the display is stopped", (await page.title()).startsWith("⚠ Gerät · KioskMate"), await page.title());

  // ---- Offline page instead of Chromium's error page ----
  const kiosk = await ctx.newPage();
  await kiosk.goto(`${base}/local/offline?u=${encodeURIComponent("http://127.0.0.1:9/dashboard")}`);
  await kiosk.waitForTimeout(3500);
  ok("offline: calm page with the host", (await kiosk.textContent("#title")) === "Keine Verbindung" && (await kiosk.textContent("#target")) === "127.0.0.1:9");
  ok("offline: counts down to the next attempt", /Nächster Versuch in \d+ s/.test(await kiosk.textContent("#state")), await kiosk.textContent("#state"));
  await kiosk.screenshot({ path: `${out}/pre4-offline.png` });
  await kiosk.goto(`${base}/local/offline?u=${encodeURIComponent(fakes.url + "/camera.png")}`);
  await kiosk.waitForURL((url) => url.href.startsWith(fakes.url), { timeout: 10000 }).catch(() => {});
  ok("offline: opens the page once it answers", kiosk.url().startsWith(fakes.url), kiosk.url());
  const bad = await fetch(`${base}/local/offline?u=${encodeURIComponent("javascript:alert(1)")}`);
  ok("offline: only http(s) targets", bad.status === 400);

  // ---- Request size limit ----
  const big = await fetch(`${base}/api/auth/login`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password: "x".repeat(200000) }) });
  ok("limits: oversized login body refused", big.status >= 400 && big.status < 500, String(big.status));

  // ---- Accessibility basics on every view ----
  const views = ["dashboard", "program", "pages", "automations", "automations-mqtt", "device", "device-display", "device-maintenance", "device-logs", "device-settings"];
  const a11yProblems = [];
  let a11yChecked = 0;
  for (const view of views) {
    await page.evaluate((v) => { localStorage.setItem("kioskmate.view", v); }, view);
    await page.reload();
    await page.waitForSelector("#main-content", { timeout: 8000 }).catch(() => {});
    await page.waitForTimeout(700);
    const found = await page.evaluate(() => {
      const visible = (el) => !!(el.offsetWidth || el.offsetHeight || el.getClientRects().length);
      const name = (el) => (el.getAttribute("aria-label") || el.getAttribute("title") || el.textContent || "").trim() || (el.getAttribute("aria-labelledby") && document.getElementById(el.getAttribute("aria-labelledby"))?.textContent.trim());
      const out = [];
      for (const el of document.querySelectorAll("button, a[href], [role=button]")) {
        if (visible(el) && !name(el)) out.push(`unnamed ${el.tagName.toLowerCase()} ${el.outerHTML.slice(0, 90)}`);
      }
      for (const el of document.querySelectorAll("input:not([type=hidden]), select, textarea")) {
        if (!visible(el)) continue;
        const labelled = el.labels?.length || el.getAttribute("aria-label") || el.getAttribute("aria-labelledby") || el.getAttribute("title") || el.closest("label");
        if (!labelled) out.push(`unlabelled ${el.tagName.toLowerCase()} ${el.outerHTML.slice(0, 90)}`);
      }
      for (const el of document.querySelectorAll("img")) {
        if (!el.hasAttribute("alt")) out.push(`img without alt ${el.outerHTML.slice(0, 90)}`);
      }
      return { out, count: document.querySelectorAll("button, a[href], input, select, textarea, img").length };
    });
    a11yChecked += found.count;
    for (const problem of found.out) a11yProblems.push(`${view}: ${problem}`);
  }
  ok("a11y: buttons named, fields labelled, images with alt on all views", a11yProblems.length === 0 && a11yChecked > 200, a11yProblems.slice(0, 6).join(" | ") || `${a11yChecked} elements`);

  // ---- Session expired: back to the login instead of error toasts ----
  await ctx.clearCookies();
  await page.click('[data-action="refresh"]');
  await page.waitForSelector("#auth-password", { timeout: 6000 }).catch(() => {});
  ok("session: expired session returns to the login", await page.locator("#auth-password").isVisible().catch(() => false));
  const toastText = await page.locator("#toasts").innerText();
  ok("session: explains why without error toasts", /Sitzung abgelaufen/.test(toastText) && !/Unauthorized/.test(toastText), toastText);

  await ctx.close();
}

// Seed a runtime state with per-page telemetry before the server starts.
pre4.prepare = (dir) => {
  fs.writeFileSync(path.join(dir, "touch-devices.json"), JSON.stringify({ devices: ["Testpanel Touch"], last_seen: new Date(Date.now() - 3 * 3600e3).toISOString() }));
  const now = new Date().toISOString();
  const entry = (id, name, samples, pages) => ({ page_id: id, name, samples, cpu_sum: samples * (pages + 15), pages_cpu_sum: samples * pages, rss_sum: samples * 420, last_seen: now });
  fs.writeFileSync(path.join(dir, "runtime-state.json"), JSON.stringify({
    start_count: 1,
    restart_count: 0,
    recovery: { state: "idle", attempts: 0 },
    override: { active: false, page: 0 },
    telemetry: [],
    telemetry_pages: [entry("ha", "Hauptseite", 400, 25), entry("wetter", "Wetter & Kameras", 120, 210), entry("deleted", "Gelöscht", 50, 500)],
  }));
};
