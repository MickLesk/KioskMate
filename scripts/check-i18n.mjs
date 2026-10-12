import fs from "node:fs";

// Translations are one JSON file per language (i18n-<lang>.json). This check
// keeps them canonical, in key parity, and covering every statically
// referenced t("key") in the Admin UI modules.
const assetsDir = "internal/admin/web/assets";
const app = fs.readdirSync(assetsDir)
  .filter((name) => name.endsWith(".js") && name !== "i18n.js")
  .sort()
  .map((name) => fs.readFileSync(`${assetsDir}/${name}`, "utf8"))
  .join("\n");
const files = fs.readdirSync(assetsDir).filter((name) => /^i18n-[a-z]{2}\.json$/.test(name)).sort();
const translations = {};
const failures = [];
for (const file of files) {
  const language = file.slice(5, 7);
  const source = fs.readFileSync(`${assetsDir}/${file}`, "utf8");
  translations[language] = JSON.parse(source);
  const canonical = JSON.stringify(translations[language], null, 2) + "\n";
  if (process.argv.includes("--write")) {
    fs.writeFileSync(`${assetsDir}/${file}`, canonical);
  } else if (source !== canonical) {
    failures.push(`${file} is not canonical; run \`node scripts/check-i18n.mjs --write\``);
  }
}
const languages = Object.keys(translations);
const declared = fs.readFileSync(`${assetsDir}/i18n.js`, "utf8").match(/LANGUAGES = \[([^\]]*)\]/)?.[1] || "";
for (const language of languages) {
  if (!declared.includes(`"${language}"`)) failures.push(`i18n.js LANGUAGES is missing ${language}`);
}
const referenced = new Set([...app.matchAll(/\bt\(["']([^"']+)["']\)/g)].map((match) => match[1]));
const allKeys = new Set(languages.flatMap((language) => Object.keys(translations[language])));

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

console.log(`i18n parity ok: ${languages.length} languages, ${allKeys.size} total keys, ${referenced.size} statically referenced keys`);
