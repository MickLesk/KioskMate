// Browser tests for the Admin UI. Starts the kioskmate binary on a copy of a
// fixture config (browser command /bin/true, nothing is displayed) and runs
// the suites against it with Playwright.
//
//   go build -o kioskmate ./cmd/kioskmate
//   npm install --prefix tests/e2e && npx --prefix tests/e2e playwright install chromium
//   node tests/e2e/run.mjs [path/to/kioskmate] [screenshot dir]
import { spawn } from "child_process";
import { createRequire } from "module";
import fs from "fs";
import os from "os";
import path from "path";
import { fileURLToPath } from "url";
import admin from "./admin.mjs";
import features from "./features.mjs";
import setup from "./setup.mjs";
import templates from "./templates.mjs";
import { startFakes } from "./fakes.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const binary = path.resolve(process.argv[2] || "kioskmate");
const out = path.resolve(process.argv[3] || fs.mkdtempSync(path.join(os.tmpdir(), "kioskmate-shots-")));
fs.mkdirSync(out, { recursive: true });

function loadPlaywright() {
  const places = [path.join(here, "package.json"), process.env.PLAYWRIGHT_MODULES && path.join(process.env.PLAYWRIGHT_MODULES, "x")].filter(Boolean);
  for (const place of places) {
    try { return createRequire(place)("playwright"); } catch {}
  }
  throw new Error("playwright not found: run npm install --prefix tests/e2e or set PLAYWRIGHT_MODULES");
}

const results = [];
const errors = [];
const ok = (name, cond, detail = "") => results.push(`${cond ? "PASS" : "FAIL"}  ${name}${detail ? "  — " + detail : ""}`);

async function withServer(fixture, port, run) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "kioskmate-e2e-"));
  const cfg = JSON.parse(fs.readFileSync(path.join(here, "fixture", fixture), "utf8"));
  cfg.admin.bind = "127.0.0.1";
  cfg.admin.port = port;
  cfg.kiosk.browser_command = "/bin/true";
  cfg.kiosk.user_data_dir = path.join(dir, "profile");
  const configPath = path.join(dir, "config.json");
  fs.writeFileSync(configPath, JSON.stringify(cfg, null, 2));
  const log = fs.openSync(path.join(out, `server-${port}.log`), "w");
  const env = { ...process.env, KIOSKMATE_OPEN_METEO_URL: `${fakes.url}/forecast` };
  const server = spawn(binary, ["-config", configPath], { cwd: dir, env, stdio: ["ignore", log, log] });
  const base = `http://127.0.0.1:${port}`;
  try {
    let up = false;
    for (let i = 0; i < 100 && !up; i++) {
      up = await fetch(base + "/healthz").then((r) => r.ok, () => false);
      if (!up) await new Promise((resolve) => setTimeout(resolve, 100));
    }
    if (!up) throw new Error(`server on ${base} did not start, see ${out}/server-${port}.log`);
    const token = cfg.admin.token;
    const api = async (p, options = {}) => {
      const response = await fetch(base + p, { ...options, headers: { "X-KioskMate-Token": token, "Content-Type": "application/json", ...(options.headers || {}) } });
      return { status: response.status, body: await response.json().catch(() => ({})) };
    };
    await run({ base, api, dir, fakes });
  } finally {
    server.kill();
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

const fakes = await startFakes();
const { chromium } = loadPlaywright();
const browser = await chromium.launch();
const suites = [["config.json", 33481, admin], ["config.json", 33482, features], ["config-fresh.json", 33483, setup], ["config.json", 33484, templates]];
try {
  for (const [fixture, port, suite] of suites) {
    const before = results.length;
    try {
      await withServer(fixture, port, (env) => suite({ browser, ok, errors, out, ...env }));
    } catch (error) {
      ok(`${suite.name}: suite finished`, false, String(error?.stack || error));
    }
    console.log(`== ${suite.name}\n${results.slice(before).join("\n")}`);
  }
} finally {
  await browser.close();
  await fakes.close();
}

const failed = results.filter((r) => r.startsWith("FAIL"));
console.log(`\n${results.length - failed.length}/${results.length} passed, screenshots in ${out}`);
if (errors.length) console.log("browser errors:\n" + errors.join("\n"));
process.exit(failed.length || errors.length ? 1 : 0);
