package admin

import "html/template"

// Templates of the weather, countdown, news and camera pages. Like the clock
// they are single files without external resources, sized in viewport units
// so they fit any screen, use inline SVG instead of emoji (Pi OS Lite has no
// emoji font) and pick German or English from the browser language.

const localPageHead = `<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<style>
:root{color-scheme:dark;--bg:#05070a;--fg:#f2f5f8;--muted:#9aa6b2;--card:rgba(255,255,255,.055);--line:rgba(255,255,255,.1);--accent:#38d39f;background:var(--bg);color:var(--fg);font-family:system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
html,body{height:100%;margin:0;overflow:hidden;cursor:none}
body{background:var(--bg)}
.msg{position:fixed;inset:0;display:grid;place-content:center;text-align:center;gap:1.5vmin;padding:6vmin;font-size:3.4vmin;color:var(--muted)}
.msg strong{color:var(--fg);font-size:4.4vmin;font-weight:500}
[hidden]{display:none!important}
`

var weatherTemplate = template.Must(template.New("weather").Parse(localPageHead + `
main{box-sizing:border-box;height:100%;padding:5vmin 6vmin;display:grid;grid-template-columns:1.25fr 1fr;grid-template-rows:auto 1fr auto;gap:4vmin 6vmin;transition:transform 2s ease}
header{grid-column:1/-1;display:flex;justify-content:space-between;align-items:baseline;gap:3vmin}
h1{margin:0;font-weight:500;font-size:4.2vmin}
#updated{color:var(--muted);font-size:2.4vmin}
.now{display:flex;align-items:center;gap:4vmin;min-width:0}
.now .icon{width:30vmin;height:30vmin;flex:none}
.temp{font-size:20vmin;font-weight:300;line-height:.9;letter-spacing:-.03em;font-variant-numeric:tabular-nums}
#desc{font-size:4.6vmin;margin-top:1.5vmin}
#meta{color:var(--muted);font-size:2.8vmin;margin-top:1.4vmin;display:flex;flex-wrap:wrap;gap:.6vmin 3vmin}
.days{display:grid;align-content:center;gap:1.2vmin}
.day{display:grid;grid-template-columns:7em 3.2em 1fr auto;align-items:center;gap:2vmin;font-size:3.2vmin;padding:1.1vmin 2vmin;border-radius:1.6vmin;background:var(--card)}
.day svg{width:5.6vmin;height:5.6vmin}
.day .rain{color:#7cb8ff;font-size:2.6vmin}
.day .range{font-variant-numeric:tabular-nums;white-space:nowrap}
.day .range span{color:var(--muted);margin-left:1.6vmin}
.hours{grid-column:1/-1;display:grid;grid-template-columns:repeat(8,1fr);gap:1.4vmin}
.hour{display:grid;justify-items:center;gap:.6vmin;padding:1.6vmin .5vmin;border-radius:1.6vmin;background:var(--card);font-size:2.8vmin}
.hour small{color:var(--muted);font-size:2.3vmin}
.hour svg{width:6.4vmin;height:6.4vmin}
.hour .rain{color:#7cb8ff;font-size:2.2vmin;min-height:1.2em}
@media (orientation:portrait){main{grid-template-columns:1fr;grid-template-rows:auto auto auto 1fr}.hours{grid-template-columns:repeat(4,1fr)}.days{align-content:start}}
</style></head>
<body><main><header><h1 id="title"></h1><span id="updated"></span></header>
<section class="now"><div class="icon" id="nowIcon"></div><div><div class="temp" id="temp"></div><div id="desc"></div><div id="meta"></div></div></section>
<section class="days" id="days"></section><section class="hours" id="hours"></section></main>
<div class="msg" id="msg" hidden><strong></strong><span></span></div>
<script>
const s={{.Settings}};
const de=navigator.language.toLowerCase().startsWith("de");
const T=de?{feels:"Gefühlt",wind:"Wind",humidity:"Feuchte",today:"Heute",updated:"Stand",noLocation:"Kein Standort festgelegt",noLocationHint:"In KioskMate unter Programm → Erweitert den Standort setzen oder in dieser Seite Koordinaten eintragen.",offline:"Wetterdienst nicht erreichbar",offlineHint:"Neuer Versuch in einer Minute.",title:"Wetter"}
:{feels:"Feels like",wind:"Wind",humidity:"Humidity",today:"Today",updated:"Updated",noLocation:"No location set",noLocationHint:"Set the location in KioskMate under Programme → Advanced, or enter coordinates in this page.",offline:"Weather service unreachable",offlineHint:"Trying again in a minute.",title:"Weather"};
const WMO={0:["Klar","Clear"],1:["Überwiegend klar","Mainly clear"],2:["Teilweise bewölkt","Partly cloudy"],3:["Bedeckt","Overcast"],45:["Nebel","Fog"],48:["Reifnebel","Rime fog"],51:["Leichter Nieselregen","Light drizzle"],53:["Nieselregen","Drizzle"],55:["Starker Nieselregen","Dense drizzle"],56:["Gefrierender Niesel","Freezing drizzle"],57:["Gefrierender Niesel","Freezing drizzle"],61:["Leichter Regen","Light rain"],63:["Regen","Rain"],65:["Starker Regen","Heavy rain"],66:["Gefrierender Regen","Freezing rain"],67:["Gefrierender Regen","Freezing rain"],71:["Leichter Schneefall","Light snow"],73:["Schneefall","Snow"],75:["Starker Schneefall","Heavy snow"],77:["Schneegriesel","Snow grains"],80:["Regenschauer","Rain showers"],81:["Regenschauer","Rain showers"],82:["Heftige Schauer","Violent showers"],85:["Schneeschauer","Snow showers"],86:["Schneeschauer","Snow showers"],95:["Gewitter","Thunderstorm"],96:["Gewitter mit Hagel","Thunderstorm with hail"],99:["Gewitter mit Hagel","Thunderstorm with hail"]};
function sun(cx,cy,r){let rays="";for(let i=0;i<8;i++){const a=i*Math.PI/4,x1=cx+Math.cos(a)*(r+4),y1=cy+Math.sin(a)*(r+4),x2=cx+Math.cos(a)*(r+9),y2=cy+Math.sin(a)*(r+9);rays+='<line x1="'+x1.toFixed(1)+'" y1="'+y1.toFixed(1)+'" x2="'+x2.toFixed(1)+'" y2="'+y2.toFixed(1)+'" stroke="#f6c344" stroke-width="3.2" stroke-linecap="round"/>'}return '<circle cx="'+cx+'" cy="'+cy+'" r="'+r+'" fill="#f6c344"/>'+rays}
const MOON='<path d="M38 12a20 20 0 1 0 14 32A17 17 0 0 1 38 12z" fill="#d9dcff"/>';
const CLOUD='<path d="M19 50h28a11 11 0 0 0 0-22 15 15 0 0 0-29-3A12.5 12.5 0 0 0 19 50z" fill="#cfd8e3"/>';
const DARK='<path d="M19 50h28a11 11 0 0 0 0-22 15 15 0 0 0-29-3A12.5 12.5 0 0 0 19 50z" fill="#8f9bab"/>';
function up(inner){return '<g transform="translate(0,-7)">'+inner+'</g>'}
function drops(n,color){let d="";for(let i=0;i<n;i++){const x=20+i*(24/Math.max(1,n-1));d+='<line x1="'+(x+2).toFixed(1)+'" y1="48" x2="'+(x-2).toFixed(1)+'" y2="58" stroke="'+color+'" stroke-width="3.2" stroke-linecap="round"/>'}return d}
function flakes(){return [18,32,46].map(x=>'<circle cx="'+x+'" cy="54" r="3" fill="#eef4ff"/>').join("")}
function icon(code,day){
  let inner;
  if(code===0)inner=day?sun(32,32,12):MOON;
  else if(code===1||code===2)inner=(day?sun(23,22,9):'<g transform="translate(-9,-9) scale(.75)">'+MOON+'</g>')+'<g transform="translate(5,4) scale(.92)">'+CLOUD+'</g>';
  else if(code===3)inner=CLOUD;
  else if(code===45||code===48)inner=up(CLOUD)+[48,55].map(y=>'<line x1="12" y1="'+y+'" x2="52" y2="'+y+'" stroke="#aab4c2" stroke-width="3.2" stroke-linecap="round"/>').join("");
  else if(code>=51&&code<=57)inner=up(CLOUD)+drops(3,"#7cb8ff");
  else if((code>=61&&code<=67)||(code>=80&&code<=82))inner=up(DARK)+drops(code===65||code===82?5:4,"#5aa9ff");
  else if((code>=71&&code<=77)||code===85||code===86)inner=up(CLOUD)+flakes();
  else if(code>=95)inner=up(DARK)+'<path d="M33 40l-8 12h7l-4 10 11-14h-7l5-8z" fill="#f6c344"/>';
  else inner=CLOUD;
  return '<svg viewBox="0 0 64 64" aria-hidden="true">'+inner+'</svg>';
}
const text=code=>(WMO[code]||["",""])[de?0:1];
const round=v=>Math.round(Number(v));
const el=id=>document.getElementById(id);
const localDate=v=>{const [y,m,d]=v.split("-").map(Number);return new Date(y,m-1,d)};
const weekday=new Intl.DateTimeFormat(undefined,{weekday:"short"});
const hourFmt=new Intl.DateTimeFormat(undefined,{hour:"2-digit",minute:"2-digit"});
el("title").textContent=s.title||s.location||T.title;
let data=null;
function message(title,hint){const m=el("msg");m.querySelector("strong").textContent=title;m.querySelector("span").textContent=hint;m.hidden=false;document.querySelector("main").hidden=true}
function render(){
  if(!data)return;
  el("msg").hidden=true;document.querySelector("main").hidden=false;
  const c=data.current,u=data.current_units||{},deg="°";
  el("nowIcon").innerHTML=icon(c.weather_code,c.is_day!==0);
  el("temp").textContent=round(c.temperature_2m)+deg;
  el("desc").textContent=text(c.weather_code);
  el("meta").innerHTML="";
  for(const part of [T.feels+" "+round(c.apparent_temperature)+deg,T.wind+" "+round(c.wind_speed_10m)+" "+(u.wind_speed_10m||""),T.humidity+" "+round(c.relative_humidity_2m)+"%"]){const span=document.createElement("span");span.textContent=part;el("meta").append(span)}
  el("updated").textContent=T.updated+" "+hourFmt.format(new Date());
  const h=data.hourly;let start=h.time.findIndex(t=>t>c.time);if(start<0)start=0;
  el("hours").innerHTML="";
  for(let i=start;i<Math.min(h.time.length,start+8);i++){
    const box=document.createElement("div");box.className="hour";
    const label=document.createElement("small");label.textContent=h.time[i].slice(11,16);
    const temp=document.createElement("strong");temp.textContent=round(h.temperature_2m[i])+deg;
    const rain=document.createElement("span");rain.className="rain";rain.textContent=h.precipitation_probability[i]>=20?h.precipitation_probability[i]+"%":"";
    box.append(label);box.insertAdjacentHTML("beforeend",icon(h.weather_code[i],h.is_day?h.is_day[i]!==0:true));box.append(temp,rain);el("hours").append(box);
  }
  const d=data.daily;el("days").innerHTML="";
  for(let i=0;i<Math.min(d.time.length,6);i++){
    const row=document.createElement("div");row.className="day";
    const name=document.createElement("strong");name.textContent=i===0?T.today:weekday.format(localDate(d.time[i]));
    const rain=document.createElement("span");rain.className="rain";rain.textContent=d.precipitation_probability_max[i]>=20?d.precipitation_probability_max[i]+"%":"";
    const range=document.createElement("span");range.className="range";range.textContent=round(d.temperature_2m_max[i])+deg;
    const low=document.createElement("span");low.textContent=round(d.temperature_2m_min[i])+deg;range.append(low);
    row.append(name);row.insertAdjacentHTML("beforeend",icon(d.weather_code[i],true));row.append(rain,range);el("days").append(row);
  }
  if(!s.reduceMotion)document.querySelector("main").style.transform="translate("+(Math.random()*2-1).toFixed(2)+"vw,"+(Math.random()*2-1).toFixed(2)+"vh)";
}
async function load(){
  let next=15*60000;
  try{
    const r=await fetch(location.pathname+"/data",{cache:"no-store"});const body=await r.json();
    if(r.ok&&body.current){data=body;render()}
    else if(body.error==="no_location"){message(T.noLocation,T.noLocationHint);next=60000}
    else throw new Error(body.error||r.status);
  }catch(e){if(!data)message(T.offline,T.offlineHint);next=60000}
  setTimeout(load,next);
}
load();
</script></body></html>`))

var countdownTemplate = template.Must(template.New("countdown").Parse(localPageHead + `
main{height:100%;display:grid;place-content:center;justify-items:center;text-align:center;gap:4vmin;padding:4vmin;box-sizing:border-box;transition:transform 2s ease}
h1{margin:0;font-weight:500;font-size:6vmin}
.units{display:flex;gap:3vmin}
.unit{min-width:17vmin;padding:3vmin 2vmin;border-radius:2.4vmin;background:var(--card);display:grid;gap:1vmin}
.unit strong{font-size:14vmin;font-weight:300;line-height:1;font-variant-numeric:tabular-nums}
.unit span{color:var(--muted);font-size:2.8vmin;text-transform:uppercase;letter-spacing:.08em}
#when{color:var(--muted);font-size:3.4vmin}
#done{font-size:11vmin;font-weight:500;color:var(--accent);max-width:90vw}
</style></head>
<body><main><h1 id="title"></h1><div id="since" hidden></div><div class="units" id="units"></div><div id="done" hidden></div><div id="when"></div></main>
<div class="msg" id="msg" hidden><strong></strong></div>
<script>
const s={{.Settings}};
const de=navigator.language.toLowerCase().startsWith("de");
const T=de?{units:["Tage","Stunden","Minuten","Sekunden"],unit1:["Tag","Stunde","Minute","Sekunde"],done:"Es ist so weit!",since:"seit",noTarget:"Kein Zieldatum festgelegt"}:{units:["Days","Hours","Minutes","Seconds"],unit1:["Day","Hour","Minute","Second"],done:"It's time!",since:"since",noTarget:"No target date set"};
const el=id=>document.getElementById(id);
const m=/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(s.target||"");
if(!m){el("msg").querySelector("strong").textContent=T.noTarget;el("msg").hidden=false;document.querySelector("main").hidden=true}
else{
  const target=new Date(+m[1],+m[2]-1,+m[3],+m[4],+m[5]);
  el("title").textContent=s.title;el("title").hidden=!s.title;
  el("when").textContent=new Intl.DateTimeFormat(undefined,{weekday:"long",day:"numeric",month:"long",year:"numeric",hour:"2-digit",minute:"2-digit"}).format(target);
  const boxes=T.units.map(()=>{const box=document.createElement("div");box.className="unit";box.innerHTML="<strong></strong><span></span>";el("units").append(box);return box});
  let minute=-1;
  function tick(){
    const now=Date.now();let diff=Math.floor((target-now)/1000);
    const past=diff<=0;
    if(past&&!s.countUp){el("units").hidden=true;el("done").hidden=false;el("done").textContent=s.done||T.done;setTimeout(tick,60000);return}
    if(past){diff=-diff;el("when").textContent=T.since+" "+new Intl.DateTimeFormat(undefined,{day:"numeric",month:"long",year:"numeric",hour:"2-digit",minute:"2-digit"}).format(target)}
    const parts=[Math.floor(diff/86400),Math.floor(diff%86400/3600),Math.floor(diff%3600/60),diff%60];
    // Hide leading zero units, but always keep minutes and seconds.
    let lead=true;
    parts.forEach((value,i)=>{const box=boxes[i];if(lead&&value===0&&i<2){box.hidden=true;return}lead=false;box.hidden=false;box.querySelector("strong").textContent=i>0?String(value).padStart(2,"0"):String(value);box.querySelector("span").textContent=(value===1?T.unit1:T.units)[i]});
    const nowMinute=Math.floor(now/60000);
    if(nowMinute!==minute){minute=nowMinute;if(!s.reduceMotion)document.querySelector("main").style.transform="translate("+(Math.random()*3-1.5).toFixed(2)+"vw,"+(Math.random()*3-1.5).toFixed(2)+"vh)"}
    setTimeout(tick,1000-(Date.now()%1000)+15);
  }
  tick();
}
</script></body></html>`))

var newsTemplate = template.Must(template.New("news").Parse(localPageHead + `
main{box-sizing:border-box;height:100%;padding:6vmin 7vmin;display:grid;grid-template-rows:auto 1fr auto;gap:4vmin}
header{display:flex;justify-content:space-between;align-items:baseline;color:var(--muted);font-size:2.8vmin}
header h1{margin:0;color:var(--fg);font-weight:500;font-size:3.6vmin}
article{align-self:center;display:grid;gap:3vmin;transition:opacity .6s ease}
article.fade{opacity:0}
.source{color:var(--accent);font-size:2.8vmin;text-transform:uppercase;letter-spacing:.08em}
article h2{margin:0;font-size:6.4vmin;line-height:1.15;font-weight:600;display:-webkit-box;-webkit-line-clamp:4;-webkit-box-orient:vertical;overflow:hidden}
article p{margin:0;color:#c8d1da;font-size:3.4vmin;line-height:1.4;display:-webkit-box;-webkit-line-clamp:3;-webkit-box-orient:vertical;overflow:hidden}
.bar{height:.6vmin;border-radius:1vmin;background:var(--line);overflow:hidden}
.bar i{display:block;height:100%;width:0;background:var(--accent)}
footer{display:grid;gap:1.2vmin}
footer small{color:var(--muted);font-size:2.4vmin;text-transform:uppercase;letter-spacing:.08em}
footer ol{margin:0;padding:0;list-style:none;display:grid;gap:1vmin}
footer li{font-size:2.8vmin;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:#c8d1da}
body.still article{transition:none}
</style></head>
<body><main><header><h1 id="title"></h1><span id="count"></span></header>
<article id="card"><span class="source" id="source"></span><h2 id="headline"></h2><p id="summary"></p></article>
<footer><div class="bar"><i id="bar"></i></div><small id="nextLabel"></small><ol id="next"></ol></footer></main>
<div class="msg" id="msg" hidden><strong></strong><span></span></div>
<script>
const s={{.Settings}};
const de=navigator.language.toLowerCase().startsWith("de");
const T=de?{title:"Nachrichten",next:"Als Nächstes",empty:"Keine Meldungen",emptyHint:"Die Feeds sind gerade nicht erreichbar oder leer. Neuer Versuch in einer Minute."}:{title:"News",next:"Up next",empty:"No headlines",emptyHint:"The feeds are unreachable or empty right now. Trying again in a minute."};
const el=id=>document.getElementById(id);
if(s.reduceMotion)document.body.classList.add("still");
el("title").textContent=s.title||T.title;el("nextLabel").textContent=T.next;
const rel=new Intl.RelativeTimeFormat(undefined,{numeric:"auto"});
function ago(value){if(!value)return "";const diff=(new Date(value)-Date.now())/1000;const abs=Math.abs(diff);if(abs<3600)return rel.format(Math.round(diff/60),"minute");if(abs<86400)return rel.format(Math.round(diff/3600),"hour");return rel.format(Math.round(diff/86400),"day")}
let items=[],index=0,loaded=0,timer=null;
const interval=Math.max(4,s.interval||12)*1000;
async function load(){
  try{const r=await fetch(location.pathname+"/data",{cache:"no-store"});const body=await r.json();if(r.ok&&Array.isArray(body)){items=body;loaded=Date.now()}}catch(e){}
}
function animateBar(){const bar=el("bar");bar.style.transition="none";bar.style.width="0";if(s.reduceMotion)return;void bar.offsetWidth;bar.style.transition="width "+interval+"ms linear";bar.style.width="100%"}
async function show(){
  if(!items.length||Date.now()-loaded>15*60000)await load();
  if(!items.length){const m=el("msg");m.querySelector("strong").textContent=T.empty;m.querySelector("span").textContent=T.emptyHint;m.hidden=false;document.querySelector("main").hidden=true;setTimeout(show,60000);return}
  el("msg").hidden=true;document.querySelector("main").hidden=false;
  if(index>=items.length)index=0;
  const item=items[index],card=el("card");
  card.classList.add("fade");
  setTimeout(()=>{
    el("source").textContent=[item.source,ago(item.published)].filter(Boolean).join(" · ");
    el("headline").textContent=item.title;el("summary").textContent=item.summary||"";el("summary").hidden=!item.summary;
    el("count").textContent=(index+1)+" / "+items.length;
    el("next").innerHTML="";
    for(let i=1;i<=3&&i<items.length;i++){const li=document.createElement("li");li.textContent=items[(index+i)%items.length].title;el("next").append(li)}
    card.classList.remove("fade");animateBar();index++;
  },s.reduceMotion?0:600);
  setTimeout(show,interval);
}
show();
</script></body></html>`))

var cameraTemplate = template.Must(template.New("camera").Parse(localPageHead + `
body{background:#000}
main{height:100%;display:grid;gap:.4vmin;background:#000}
main.n2{grid-template-columns:1fr 1fr}
main.n3,main.n4{grid-template-columns:1fr 1fr;grid-template-rows:1fr 1fr}
@media (orientation:portrait){main.n2{grid-template-columns:1fr;grid-template-rows:1fr 1fr}}
.cell{position:relative;overflow:hidden;background:#000;min-height:0}
.cell video,.cell img{position:absolute;inset:0;width:100%;height:100%;object-fit:contain}
.cell .state{position:absolute;left:2vmin;bottom:2vmin;padding:.6vmin 1.4vmin;border-radius:1vmin;background:rgba(0,0,0,.6);color:#c8d1da;font-size:2.2vmin}
#title{position:fixed;left:2vmin;top:2vmin;margin:0;padding:.6vmin 1.6vmin;border-radius:1vmin;background:rgba(0,0,0,.55);font-size:2.6vmin;font-weight:500}
</style></head>
<body><main id="grid"></main><h1 id="title" hidden></h1>
<div class="msg" id="msg" hidden><strong></strong></div>
<script>
const s={{.Settings}};
const de=navigator.language.toLowerCase().startsWith("de");
const T=de?{none:"Keine Kamera eingetragen",reconnect:"Verbinde neu …"}:{none:"No camera configured",reconnect:"Reconnecting …"};
const streams=(s.streams||[]).slice(0,4);
const grid=document.getElementById("grid");
if(s.title){const h=document.getElementById("title");h.textContent=s.title;h.hidden=false}
if(!streams.length){const m=document.getElementById("msg");m.querySelector("strong").textContent=T.none;m.hidden=false}
grid.className="n"+streams.length;
// go2rtc player pages become its MP4 stream, which Chromium plays with the
// hardware decoder; MJPEG and snapshots use an image, everything else video.
function source(raw){
  let url;try{url=new URL(raw)}catch(e){return {kind:"video",src:raw}}
  const lower=url.pathname.toLowerCase(),src=url.searchParams.get("src");
  if(src&&/(stream|webrtc|links)\.html$|\/api\/ws$|\/api\/webrtc$/.test(lower)){
    const base=url.pathname.replace(/[^/]*$/,"").replace(/api\/$/,"");
    return {kind:"video",src:url.origin+base+"api/stream.mp4?src="+encodeURIComponent(src)};
  }
  if(/mjpe?g|\.cgi$/.test(lower)||/mjpe?g/.test(url.search.toLowerCase()))return {kind:"mjpeg",src:raw};
  if(/\.(jpe?g|png|webp)$|snapshot|\/api\/frame/.test(lower))return {kind:"snapshot",src:raw};
  return {kind:"video",src:raw};
}
function bust(src){return src+(src.includes("?")?"&":"?")+"_="+Date.now()}
function camera(raw){
  const cell=document.createElement("div");cell.className="cell";grid.append(cell);
  const state=document.createElement("span");state.className="state";state.textContent=T.reconnect;state.hidden=true;
  const info=source(raw);
  if(info.kind==="video"){
    const video=document.createElement("video");video.muted=true;video.autoplay=true;video.playsInline=true;video.setAttribute("muted","");
    cell.append(video,state);
    let last=-1,still=0;
    const start=()=>{state.hidden=true;video.src=info.src;video.play().catch(()=>{})};
    video.addEventListener("error",()=>{state.hidden=false;setTimeout(start,5000)});
    // A stream that stops moving for 15 s is reconnected.
    setInterval(()=>{if(video.currentTime===last){still+=5;if(still>=15){still=0;state.hidden=false;start()}}else{still=0;last=video.currentTime;state.hidden=true}},5000);
    start();
  }else{
    const img=document.createElement("img");img.alt="";cell.append(img,state);
    if(info.kind==="mjpeg"){
      img.addEventListener("error",()=>{state.hidden=false;setTimeout(()=>{img.src=bust(info.src)},5000)});
      img.addEventListener("load",()=>{state.hidden=true});
      img.src=info.src;
    }else{
      const next=()=>{const pre=new Image();pre.onload=()=>{img.src=pre.src;state.hidden=true;setTimeout(next,2000)};pre.onerror=()=>{state.hidden=false;setTimeout(next,5000)};pre.src=bust(info.src)};
      next();
    }
  }
}
streams.forEach(camera);
</script></body></html>`))
