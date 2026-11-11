// The 1.0.0-pre.4 round: CPU per page, profile explanation, stopped browser
// tile and the other Admin UI, robustness and security additions.
import fs from "fs";
import path from "path";

export default async function pre4({ browser, base, ok, errors, out, api }) {
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

  await ctx.close();
}

// Seed a runtime state with per-page telemetry before the server starts.
pre4.prepare = (dir) => {
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
