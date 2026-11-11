package show

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web
var webFS embed.FS

// Handler serves the panel, its SSE feed and the narration audio:
//
//	/, /full, /column, /lower   the panel (layouts; ?demo=1 fills the game hole, ?audio=0|1)
//	/about                      the "about this AI" card alone (its own OBS source)
//	/events                     the feed as server-sent events (state first)
//	/state                      the panel state as JSON
//	/v1/ingest                  POST events from other producers
//	/audio/<file>               narration WAVs from audioDir ("" disables)
func Handler(h *Hub, audioDir string) http.Handler {
	static, _ := fs.Sub(webFS, "web")
	files := http.FileServerFS(static)
	mux := http.NewServeMux()
	page := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFileFS(w, r, static, "index.html")
	}
	mux.HandleFunc("GET /{$}", page)
	mux.HandleFunc("GET /full", page)
	mux.HandleFunc("GET /column", page)
	mux.HandleFunc("GET /lower", page)
	mux.HandleFunc("GET /about", page)
	mux.Handle("GET /panel.js", noStore(files))
	mux.Handle("GET /panel.css", noStore(files))
	mux.HandleFunc("GET /events", h.ServeSSE)
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(h.State())
	})
	mux.HandleFunc("POST /v1/ingest", h.ServeIngest)
	if audioDir != "" {
		audio := http.StripPrefix("/audio/", http.FileServer(http.Dir(audioDir)))
		mux.Handle("GET /audio/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "..") || !strings.HasSuffix(r.URL.Path, ".wav") {
				http.NotFound(w, r)
				return
			}
			audio.ServeHTTP(w, r)
		}))
	}
	return mux
}

func noStore(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}
