import fs from "node:fs";
import vm from "node:vm";

const assetsDir = "internal/admin/web/assets";
const app = fs.readdirSync(assetsDir)
  .filter((name) => name.endsWith(".js") && name !== "i18n.js")
  .sort()
  .map((name) => fs.readFileSync(`${assetsDir}/${name}`, "utf8"))
  .join("\n");
const source = fs.readFileSync("internal/admin/web/assets/i18n.js", "utf8");
const context = { window: {} };
vm.runInNewContext(source, context, { filename: "i18n.js" });

const translations = context.window.KIOSKMATE_I18N;
const languages = Object.keys(translations);
const referenced = new Set([...app.matchAll(/\bt\(["']([^"']+)["']\)/g)].map((match) => match[1]));
const allKeys = new Set(languages.flatMap((language) => Object.keys(translations[language])));
const failures = [];

for (const language of languages) {
  for (const key of referenced) {
    if (!(key in translations[language])) failures.push(`${language}: missing referenced key ${key}`);
  }
  for (const key of allKeys) {
    if (!(key in translations[language])) failures.push(`${language}: missing parity key ${key}`);
  }
}

if (failures.length) {
  console.error(failures.join("\n"));
  process.exit(1);
}

console.log(`i18n parity ok: ${languages.length} languages, ${referenced.size} referenced keys`);
