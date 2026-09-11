import { expect, test } from "@playwright/test";

async function signIn(page) {
  await page.goto("/");
  await expect(page.locator("#auth-form")).toBeVisible();
  await page.locator("#auth-password").fill("KioskMate-E2E");
  await page.locator("#auth-form").evaluate((form) => form.requestSubmit());
  await expect(page.locator(".layout")).toBeVisible();
  await expect(page.locator(".fatal-shell")).toHaveCount(0);
  await expect(page.locator('button.chip[data-view="dashboard"]')).toContainText(/Running|Läuft/);
}

async function openView(page, view) {
  await page.locator(`[data-view="${view}"]`).first().evaluate((button) => button.click());
  await expect(page.locator(".page-stack").first()).toBeVisible();
}

async function expectNoDocumentOverflow(page) {
  const dimensions = await page.locator("html").evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
  }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth + 1);
}

test("login and dashboard remain usable", async ({ page }) => {
  await signIn(page);
  await expect(page.locator("[data-action=\"browser-reload\"]").first()).toBeVisible();
  await expect(page.locator("[data-action=\"action-center\"]")).toBeVisible();
  await expectNoDocumentOverflow(page);
});

test("page wizard validates and completes all steps", async ({ page }) => {
  await signIn(page);
  await openView(page, "kiosk-pages");
  await page.locator('[data-action="page-wizard-new"]').first().click();
  await expect(page.locator("#page-wizard-title")).toBeVisible();
  await page.locator("#wizard-page-name").fill("Weather");
  await page.locator("#wizard-page-url").fill("https://example.com/weather");
  await page.locator("[data-wizard-next]").click();
  await expect(page.locator("#wizard-display-mode")).toBeVisible();
  await page.locator("[data-wizard-next]").click();
  await expect(page.locator(".wizard-review")).toContainText("Weather");
  await page.locator("[data-wizard-finish]").click();
  await expect(page.locator(".sequence-card-copy", { hasText: "Weather" })).toBeVisible();
  await expectNoDocumentOverflow(page);
});

test("MQTT and update workspaces render without hidden fatal errors", async ({ page }) => {
  await signIn(page);
  await openView(page, "mqtt");
  await expect(page.locator('[data-action="mqtt-test"]').first()).toBeVisible();
  await openView(page, "settings-updates");
  await expect(page.locator('[data-action="update-preflight"]')).toBeVisible();
  await expectNoDocumentOverflow(page);
});
