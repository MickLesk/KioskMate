// Local stand-ins for the services the weather, news, camera and Immich pages
// use, so the browser tests need no internet: an Open-Meteo forecast, an RSS
// feed, a camera snapshot and a tiny Immich API.
import http from "http";
import zlib from "zlib";

export const IMMICH_KEY = "e2e-immich-key";
export const IMMICH_ALBUM = "a1b2c3d4-0000-4000-8000-000000000001";
const PHOTOS = ["11111111-0000-4000-8000-000000000001", "22222222-0000-4000-8000-000000000002"];

// A gradient PNG, so pages show a real picture.
function png(width, height, [r1, g1, b1], [r2, g2, b2]) {
  const rows = Buffer.alloc((width * 3 + 1) * height);
  for (let y = 0; y < height; y++) {
    rows[y * (width * 3 + 1)] = 0;
    for (let x = 0; x < width; x++) {
      const f = (x / width + y / height) / 2;
      const at = y * (width * 3 + 1) + 1 + x * 3;
      rows[at] = r1 + (r2 - r1) * f;
      rows[at + 1] = g1 + (g2 - g1) * f;
      rows[at + 2] = b1 + (b2 - b1) * f;
    }
  }
  const chunk = (type, data) => {
    const length = Buffer.alloc(4);
    length.writeUInt32BE(data.length);
    const body = Buffer.concat([Buffer.from(type), data]);
    const crc = Buffer.alloc(4);
    crc.writeUInt32BE(zlib.crc32(body));
    return Buffer.concat([length, body, crc]);
  };
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header.set([8, 2, 0, 0, 0], 8);
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk("IHDR", header), chunk("IDAT", zlib.deflateSync(rows)), chunk("IEND", Buffer.alloc(0))]);
}

const images = {
  camera: png(320, 180, [30, 40, 60], [90, 120, 110]),
  [PHOTOS[0]]: png(480, 320, [240, 150, 60], [80, 40, 120]),
  [PHOTOS[1]]: png(480, 320, [40, 120, 200], [20, 200, 140]),
};

function localISO(date) {
  const pad = (n) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function forecast() {
  const now = new Date();
  now.setMinutes(0, 0, 0);
  const codes = [0, 2, 3, 61, 80, 71, 95, 45, 1];
  const hourly = { time: [], temperature_2m: [], precipitation_probability: [], weather_code: [], is_day: [] };
  for (let i = -2; i < 30; i++) {
    const at = new Date(now.getTime() + i * 3600000);
    hourly.time.push(localISO(at));
    hourly.temperature_2m.push(Math.round((9 + 5 * Math.sin(i / 4)) * 10) / 10);
    hourly.precipitation_probability.push((i * 17) % 90);
    hourly.weather_code.push(codes[(i + 9) % codes.length]);
    hourly.is_day.push(at.getHours() >= 7 && at.getHours() < 18 ? 1 : 0);
  }
  const daily = { time: [], weather_code: [], temperature_2m_max: [], temperature_2m_min: [], precipitation_probability_max: [], sunrise: [], sunset: [] };
  for (let d = 0; d < 6; d++) {
    const day = new Date(now.getTime() + d * 86400000);
    daily.time.push(localISO(day).slice(0, 10));
    daily.weather_code.push(codes[d * 2 % codes.length]);
    daily.temperature_2m_max.push(12 + d);
    daily.temperature_2m_min.push(3 + d / 2);
    daily.precipitation_probability_max.push(d * 15);
    daily.sunrise.push(localISO(day).slice(0, 10) + "T07:45");
    daily.sunset.push(localISO(day).slice(0, 10) + "T17:50");
  }
  return {
    current: { time: localISO(new Date()), temperature_2m: 11.4, apparent_temperature: 9.8, relative_humidity_2m: 72, weather_code: 2, wind_speed_10m: 14.2, is_day: 1 },
    current_units: { temperature_2m: "°C", wind_speed_10m: "km/h" },
    hourly,
    daily,
  };
}

function rss() {
  const items = [
    ["Neue Straßenbahnlinie verbindet Hauptbahnhof und Messe", "Ab Montag fahren die Bahnen alle zehn Minuten, auch am Wochenende.", 20],
    ["Wetterdienst erwartet ersten Frost der Saison", "In der Nacht zum Sonntag kann es in Bodennähe bis zu minus zwei Grad kalt werden.", 75],
    ["Stadtbibliothek verlängert Öffnungszeiten", "", 160],
    ["Regionale Erzeuger öffnen Hofläden zum Herbstfest", "Mit Kürbissen, Äpfeln und frischem Most.", 400],
  ];
  return `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>Lokalnachrichten</title>${items.map(([title, text, minutes]) => `<item><title>${title}</title><description>${text}</description><pubDate>${new Date(Date.now() - minutes * 60000).toUTCString()}</pubDate></item>`).join("")}</channel></rss>`;
}

export async function startFakes() {
  const server = http.createServer((req, res) => {
    const url = new URL(req.url, "http://fake");
    const send = (status, type, body) => { res.writeHead(status, { "Content-Type": type }); res.end(body); };
    if (url.pathname === "/forecast") return send(200, "application/json", JSON.stringify(forecast()));
    if (url.pathname === "/rss") return send(200, "application/rss+xml", rss());
    if (url.pathname === "/camera.png") return send(200, "image/png", images.camera);
    if (url.pathname.startsWith("/immich/api/")) {
      if (req.headers["x-api-key"] !== IMMICH_KEY) return send(401, "application/json", "{}");
      const path = url.pathname.slice("/immich/api/".length);
      if (path === `albums/${IMMICH_ALBUM}`) return send(200, "application/json", JSON.stringify({ assets: PHOTOS.map((id) => ({ id, type: "IMAGE" })) }));
      if (path === "search/random") return send(200, "application/json", JSON.stringify(PHOTOS.map((id) => ({ id, type: "IMAGE" }))));
      const photo = /^assets\/([^/]+)\/thumbnail$/.exec(path)?.[1];
      if (photo && images[photo]) return send(200, "image/png", images[photo]);
    }
    send(404, "text/plain", "not found");
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const url = `http://127.0.0.1:${server.address().port}`;
  return { url, close: () => new Promise((resolve) => server.close(resolve)) };
}
