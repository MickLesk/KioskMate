package admin

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
)

// The offline page replaces Chromium's error page when a kiosk page cannot
// be reached (the Pi booted before Wi-Fi, Home Assistant restarts). It shows
// a calm dark screen with the clock and the address, checks the address with
// growing pauses and opens it as soon as it answers.

// OfflinePageID is the reserved local page id of the offline page.
const OfflinePageID = "offline"

var offlineTemplate = template.Must(template.New("offline").Parse(localPageHead + `
main{position:fixed;inset:0;display:grid;place-content:center;text-align:center;gap:2vmin;padding:6vmin}
#time{font-size:16vmin;font-weight:300;font-variant-numeric:tabular-nums;line-height:1}
h1{margin:2vmin 0 0;font-size:4.2vmin;font-weight:500}
p{margin:0;color:var(--muted);font-size:2.8vmin}
code{color:var(--muted);font-size:2.2vmin;overflow-wrap:anywhere}
</style></head>
<body><main><div id="time"></div><h1 id="title"></h1><p id="state"></p><code id="target"></code></main>
<script>
const s={{.Settings}};
const de=navigator.language.toLowerCase().startsWith("de");
const T=de?{title:"Keine Verbindung",wait:"Nächster Versuch in {s} s",trying:"Verbinde …"}:{title:"No connection",wait:"Next attempt in {s} s",trying:"Connecting …"};
document.getElementById("title").textContent=T.title;
let host=s.target;try{host=new URL(s.target).host}catch(e){}
document.getElementById("target").textContent=host;
const timeEl=document.getElementById("time"),stateEl=document.getElementById("state");
const fmt=new Intl.DateTimeFormat(undefined,{hour:"2-digit",minute:"2-digit"});
function clock(){timeEl.textContent=fmt.format(new Date());setTimeout(clock,60000-Date.now()%60000+20)}
clock();
const pauses=[5,10,15,30,60];
let attempt=0;
// A no-cors request resolves for any answer and fails only when the host is
// unreachable. Mixed content (https page, http target) cannot be checked, so
// then the target is simply opened again.
async function reachable(){
  if(location.protocol==="https:"&&s.target.startsWith("http:"))return true;
  try{await fetch(s.target,{mode:"no-cors",cache:"no-store",signal:AbortSignal.timeout(8000)});return true}catch(e){return false}
}
async function check(){
  stateEl.textContent=T.trying;
  if(await reachable()){location.replace(s.target);return}
  let left=pauses[Math.min(attempt++,pauses.length-1)];
  const tick=()=>{stateEl.textContent=T.wait.replace("{s}",left);if(left--<=0){check();return}setTimeout(tick,1000)};
  tick();
}
setTimeout(check,2000);
</script></body></html>`))

// OfflineTarget validates the address the offline page should retry.
func offlineTarget(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", false
	}
	return parsed.String(), true
}

func (s *Server) renderOfflinePage(w http.ResponseWriter, r *http.Request) {
	target, ok := offlineTarget(r.URL.Query().Get("u"))
	if !ok {
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}
	encoded, err := json.Marshal(map[string]any{"target": target})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src *")
	_ = offlineTemplate.Execute(w, map[string]any{"Title": "KioskMate", "Settings": template.JS(encoded)})
}
