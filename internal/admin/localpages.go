package admin

import (
	"encoding/json"
	"html/template"
	"io/fs"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/MickLesk/KioskMate/internal/config"
)

// Local pages (clock, slideshow, custom HTML) are rendered here and opened by
// the kiosk browser at /local/<page_id>. They need no login, so they are only
// served to the device itself (loopback or the address the server listens
// on) or to a signed-in admin for previews. Every page runs in a CSP sandbox:
// its scripts get an opaque origin and cannot use the Admin API.

const maxSlideshowImages = 5000

var slideshowExtensions = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".gif":  "image/gif",
	".avif": "image/avif",
	".bmp":  "image/bmp",
}

func (s *Server) localPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !localRequest(r) && !s.authenticated(r) {
		http.Error(w, "local pages are only served to this device", http.StatusForbidden)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/local/")
	pageID, sub, _ := strings.Cut(rest, "/")
	cfg := s.cfg.Snapshot()
	var page *config.KioskPage
	for index := range cfg.Kiosk.Pages {
		if cfg.Kiosk.Pages[index].PageID == pageID && cfg.Kiosk.Pages[index].IsLocal() && cfg.Kiosk.Pages[index].Local != nil {
			page = &cfg.Kiosk.Pages[index]
			break
		}
	}
	if page == nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case sub == "":
		s.renderLocalPage(w, cfg, *page)
	case sub == "images" && page.SourceType == config.SourceSlideshow:
		w.Header().Set("Access-Control-Allow-Origin", "*")
		writeJSON(w, http.StatusOK, map[string]any{"images": slideshowImages(*page)})
	case strings.HasPrefix(sub, "image/") && page.SourceType == config.SourceSlideshow:
		serveSlideshowImage(w, r, page.Local.Folder, strings.TrimPrefix(sub, "image/"))
	case sub == "images" && page.SourceType == config.SourceImmich:
		w.Header().Set("Access-Control-Allow-Origin", "*")
		images, err := s.immichImages(r, *page)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"images": []string{}, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"images": images})
	case strings.HasPrefix(sub, "image/") && page.SourceType == config.SourceImmich:
		s.serveImmichAsset(w, r, *page, strings.TrimPrefix(sub, "image/"))
	case sub == "data" && (page.SourceType == config.SourceWeather || page.SourceType == config.SourceNews):
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-store")
		var data any
		var err error
		if page.SourceType == config.SourceWeather {
			data, err = s.weatherData(r.Context(), cfg, *page)
		} else {
			data, err = s.newsData(r.Context(), *page)
		}
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, data)
	default:
		http.NotFound(w, r)
	}
}

// localRequest reports whether the request comes from the device itself.
func localRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	remote := net.ParseIP(host)
	if remote == nil {
		return false
	}
	if remote.IsLoopback() {
		return true
	}
	local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return false
	}
	localHost, _, err := net.SplitHostPort(local.String())
	return err == nil && remote.Equal(net.ParseIP(localHost))
}

func (s *Server) renderLocalPage(w http.ResponseWriter, cfg *config.Config, page config.KioskPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if page.SourceType == config.SourceHTML {
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups allow-modals")
		_, _ = w.Write([]byte(page.Local.HTML))
		return
	}
	w.Header().Set("Content-Security-Policy", "sandbox allow-scripts; default-src 'none'; img-src * data: blob:; media-src * data: blob:; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src *")
	settings := map[string]any{
		"title":        page.Local.Title,
		"reduceMotion": cfg.Performance.ReduceMotion,
	}
	tmpl := clockTemplate
	switch page.SourceType {
	case config.SourceClock:
		settings["hour12"] = page.Local.Hour12
		settings["seconds"] = page.Local.ShowSeconds
		settings["date"] = !page.Local.HideDate
	case config.SourceSlideshow:
		tmpl = slideshowTemplate
		settings["interval"] = page.Local.IntervalSeconds
		settings["fit"] = page.Local.Fit
		settings["shuffle"] = page.Local.Shuffle
		settings["folder"] = page.Local.Folder
		settings["images"] = slideshowImages(page)
	case config.SourceImmich:
		tmpl = slideshowTemplate
		settings["interval"] = page.Local.IntervalSeconds
		settings["fit"] = page.Local.Fit
		settings["folder"] = page.Local.ImmichURL
		settings["images"] = []string{}
	case config.SourceWeather:
		tmpl = weatherTemplate
		settings["units"] = page.Local.Units
		settings["location"] = page.Local.Location
	case config.SourceCountdown:
		tmpl = countdownTemplate
		settings["target"] = page.Local.Target
		settings["done"] = page.Local.DoneText
		settings["countUp"] = page.Local.CountUp
	case config.SourceNews:
		tmpl = newsTemplate
		settings["interval"] = page.Local.IntervalSeconds
	case config.SourceCamera:
		tmpl = cameraTemplate
		settings["streams"] = page.Local.Streams
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	title := page.Local.Title
	if title == "" {
		title = page.Name
	}
	_ = tmpl.Execute(w, map[string]any{"Title": title, "Settings": template.JS(encoded)})
}

// immichImages lists the photos of an Immich page as URLs relative to the page.
func (s *Server) immichImages(r *http.Request, page config.KioskPage) ([]string, error) {
	ids, err := s.immichAssets(r.Context(), page)
	if err != nil {
		return nil, err
	}
	urls := make([]string, len(ids))
	for index, id := range ids {
		urls[index] = page.PageID + "/image/" + id
	}
	if page.Local.Shuffle {
		rand.Shuffle(len(urls), func(i, j int) { urls[i], urls[j] = urls[j], urls[i] })
	}
	return urls, nil
}

// slideshowImages lists the image files in the slideshow folder as URLs
// relative to the page.
func slideshowImages(page config.KioskPage) []string {
	entries, err := os.ReadDir(page.Local.Folder)
	if err != nil {
		return []string{}
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || slideshowExtensions[strings.ToLower(path.Ext(name))] == "" {
			continue
		}
		if entry.Type()&fs.ModeType != 0 && entry.Type()&fs.ModeSymlink == 0 {
			continue
		}
		names = append(names, name)
		if len(names) >= maxSlideshowImages {
			break
		}
	}
	sort.Strings(names)
	if page.Local.Shuffle {
		rand.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
	}
	urls := make([]string, len(names))
	for index, name := range names {
		urls[index] = page.PageID + "/image/" + url.PathEscape(name)
	}
	return urls
}

func serveSlideshowImage(w http.ResponseWriter, r *http.Request, folder, name string) {
	contentType := slideshowExtensions[strings.ToLower(path.Ext(name))]
	if name == "" || contentType == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, "/\\\x00") {
		http.NotFound(w, r)
		return
	}
	// os.Root refuses names and symlinks that lead outside the folder.
	root, err := os.OpenRoot(folder)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, name, info.ModTime(), file)
}

var clockTemplate = template.Must(template.New("clock").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<style>
:root{color-scheme:dark;background:#05070a;color:#f2f5f8;font-family:system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
@media (prefers-color-scheme:light){:root{color-scheme:light;background:#f5f6f8;color:#11161c}}
html,body{height:100%;margin:0;overflow:hidden;cursor:none}
main{height:100%;display:grid;place-content:center;text-align:center;gap:2vh;transition:transform 2s ease}
h1{margin:0;font-weight:500;font-size:4vmin;opacity:.7}
#time{font-size:22vmin;font-weight:300;font-variant-numeric:tabular-nums;letter-spacing:-.02em;line-height:1}
#date{font-size:5vmin;opacity:.75}
</style></head>
<body><main><h1 id="title" hidden></h1><div id="time"></div><div id="date"></div></main>
<script>
const s={{.Settings}};
const main=document.querySelector("main"),timeEl=document.getElementById("time"),dateEl=document.getElementById("date"),titleEl=document.getElementById("title");
if(s.title){titleEl.textContent=s.title;titleEl.hidden=false}
if(!s.date)dateEl.hidden=true;
const timeFormat=new Intl.DateTimeFormat(undefined,{hour:"2-digit",minute:"2-digit",second:s.seconds?"2-digit":undefined,hour12:!!s.hour12});
const dateFormat=new Intl.DateTimeFormat(undefined,{weekday:"long",day:"numeric",month:"long",year:"numeric"});
let minute=-1;
function tick(){
  const now=new Date();
  timeEl.textContent=timeFormat.format(now);
  if(now.getMinutes()!==minute){
    minute=now.getMinutes();
    if(s.date)dateEl.textContent=dateFormat.format(now);
    // Move the clock a little every minute against burn-in.
    if(!s.reduceMotion)main.style.transform="translate("+(Math.random()*4-2).toFixed(2)+"vw,"+(Math.random()*4-2).toFixed(2)+"vh)";
  }
  const step=s.seconds?1000:60000;
  setTimeout(tick,step-(now.getTime()%step)+20);
}
tick();
</script></body></html>`))

var slideshowTemplate = template.Must(template.New("slideshow").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<style>
html,body{height:100%;margin:0;overflow:hidden;background:#000;color:#ccd3da;font-family:system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;cursor:none}
img{position:absolute;inset:0;width:100%;height:100%;opacity:0;transition:opacity 1.2s ease}
img.shown{opacity:1}
body.still img{transition:none}
#empty{height:100%;display:grid;place-content:center;text-align:center;gap:1vh;font-size:3vmin}
#empty code{opacity:.7}
#empty[hidden]{display:none}
</style></head>
<body><div id="empty" hidden><strong>No images found</strong><code></code></div><img alt=""><img alt="">
<script>
const s={{.Settings}};
const layers=[...document.querySelectorAll("img")],empty=document.getElementById("empty");
if(s.reduceMotion)document.body.classList.add("still");
for(const layer of layers)layer.style.objectFit=s.fit==="cover"?"cover":"contain";
empty.querySelector("code").textContent=s.folder;
let images=s.images||[],index=0,front=0;
const base=location.pathname.replace(/[^/]*$/,"");
async function refresh(){
  try{const r=await fetch(location.pathname+"/images",{cache:"no-store"});if(r.ok)images=(await r.json()).images||[]}catch(e){}
}
async function show(){
  if(!images.length){await refresh();if(images.length)return show();empty.hidden=false;for(const l of layers)l.classList.remove("shown");setTimeout(show,30000);return}
  empty.hidden=true;
  if(index>=images.length){index=0;await refresh();if(!images.length)return show()}
  const next=layers[1-front],src=base+images[index++];
  next.src=src;
  try{await next.decode()}catch(e){setTimeout(show,1000);return}
  next.classList.add("shown");layers[front].classList.remove("shown");front=1-front;
  // Free the decoded previous image once the fade is over.
  const old=layers[1-front];setTimeout(()=>{if(!old.classList.contains("shown"))old.removeAttribute("src")},1500);
  setTimeout(show,Math.max(2,s.interval||10)*1000);
}
show();
</script></body></html>`))
