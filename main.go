package main

import (
	"embed"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/starfederation/datastar-go/datastar"
)

//go:embed static
var staticFS embed.FS

var room = NewRoom()

// sig holds the signals the browser sends with each request. Pid is the player's private token.
type sig struct {
	Pid  string `json:"pid"`
	Name string `json:"name"`
	Pass string `json:"pass"`
}

func readSig(r *http.Request) sig {
	var s sig
	datastar.ReadSignals(r, &s)
	return s
}

func logf(f string, a ...any) {
	log.Printf(f, a...)
	room.addLog(fmt.Sprintf(f, a...))
}

func index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	Index().Render(r.Context(), w)
}

func validToken(t string) bool { return len(t) >= 16 && len(t) <= 64 }

func stream(w http.ResponseWriter, r *http.Request) {
	tok := readSig(r).Pid
	if !validToken(tok) {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	sse := datastar.NewSSE(w, r)
	rc := http.NewResponseController(w)
	ch := make(chan struct{}, 1)
	room.connect(tok, ch)
	defer room.disconnect(tok, ch)
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	if sse.PatchElements(room.View(tok)) != nil {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			if sse.PatchElements(room.View(tok)) != nil {
				return
			}
		case <-tick.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			rc.Flush()
		}
	}
}

func act(fn func(tok string, r *http.Request, s sig)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := readSig(r)
		if !validToken(s.Pid) {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		fn(s.Pid, r, s)
		datastar.NewSSE(w, r)
	}
}

func qint(r *http.Request, k string) int {
	n, err := strconv.Atoi(r.URL.Query().Get(k))
	if err != nil {
		return -1
	}
	return n
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	static := http.FileServerFS(staticFS)
	mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		static.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /{$}", index)
	mux.HandleFunc("GET /stream", stream)
	post := func(path string, fn func(tok string, r *http.Request, s sig)) { mux.HandleFunc("POST "+path, act(fn)) }
	post("/join", func(t string, r *http.Request, s sig) { room.Join(t, s.Name, s.Pass) })
	post("/level", func(t string, r *http.Request, s sig) { room.SetLevel(t, qint(r, "lv")) })
	post("/solo", func(t string, r *http.Request, s sig) { room.SetSolo(t, r.URL.Query().Get("on") == "1") })
	post("/ready", func(t string, r *http.Request, s sig) { room.ToggleReady(t) })
	post("/leave", func(t string, r *http.Request, s sig) { room.Leave(t) })
	post("/abort", func(t string, r *http.Request, s sig) { room.Abort(t) })
	post("/kick", func(t string, r *http.Request, s sig) { room.Kick(t, r.URL.Query().Get("id")) })
	post("/brief", func(t string, r *http.Request, s sig) { room.SetStep(t, qint(r, "d")) })
	post("/toggle", func(t string, r *http.Request, s sig) { room.Toggle(t, r.URL.Query().Get("k")) })
	post("/furi", func(t string, r *http.Request, s sig) { room.ToggleFuri(t) })
	post("/startnow", func(t string, r *http.Request, s sig) { room.StartNow(t) })
	post("/skip", func(t string, r *http.Request, s sig) { room.Skip(t) })
	post("/begin", func(t string, r *http.Request, s sig) { room.Begin(t) })
	post("/place", func(t string, r *http.Request, s sig) { room.Place(t, qint(r, "c")) })
	post("/unplace", func(t string, r *http.Request, s sig) { room.Unplace(t, qint(r, "c")) })
	post("/confirm", func(t string, r *http.Request, s sig) { room.Confirm(t) })
	post("/readdone", func(t string, r *http.Request, s sig) { room.ReadDone(t) })
	post("/answer", func(t string, r *http.Request, s sig) { room.Answer(t, qint(r, "o")) })
	post("/next", func(t string, r *http.Request, s sig) { room.Next(t) })
	post("/reset", func(t string, r *http.Request, s sig) { room.Reset(t) })
	return mux
}

func main() {
	room.Pass = os.Getenv("ROOM_PASSPHRASE")
	if n, err := strconv.Atoi(os.Getenv("MAX_GAMES_PER_HOUR")); err == nil && n > 0 {
		room.MaxGames = n
	}
	port := envOr("PORT", "8080")
	if apiKey == "" {
		log.Println("OPENROUTER_API_KEY not set: using the offline mock story")
	} else {
		log.Printf("models: %v, provider: %q, reasoning: %q, judge: %v", models, provider, reasoning, judgeOn)
	}
	if room.Pass == "" {
		log.Println("ROOM_PASSPHRASE not set: anyone with the link can join")
	}
	log.Fatal(http.ListenAndServe(":"+port, newMux()))
}
