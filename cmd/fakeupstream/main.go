// Command fakeupstream serves a deterministic imitation of the Power Hit
// Radio /Pwr/lastSong/ endpoint for local development and end-to-end tests.
// The response shape matches the real endpoint (see docs/upstream.md).
//
// Each playlist entry is served for -repeat consecutive requests, so a
// collector polling it sees repeated observations of the same item followed
// by transitions. POST /__control?fail=1 makes it return HTTP 503 until
// POST /__control?fail=0.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type entry struct{ artist, title string }

var playlist = []entry{
	{"Fake Artist Alpha", "First Fixture"},
	{"Fake Artist Beta", "Second Fixture"},
	{"", "Eteryje - Informacija"},
	{"Fake Artist Alpha", "First Fixture"},
	{"Fake Artist Gamma & Delta", "Ąžuolų Fixture (Remix)"},
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18081", "listen address")
	repeat := flag.Int("repeat", 2, "requests served per playlist entry")
	loop := flag.Bool("loop", false, "restart the playlist at the end instead of holding the last entry")
	flag.Parse()

	var mu sync.Mutex
	requests := 0
	failing := false
	uidBase := 3086800
	loc, err := time.LoadLocation("Europe/Vilnius")
	if err != nil {
		log.Fatal(err)
	}
	starts := map[int]time.Time{}

	http.HandleFunc("GET /Pwr/lastSong/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if failing {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "fake outage", http.StatusServiceUnavailable)
			return
		}
		i := requests / *repeat
		if *loop {
			i %= len(playlist)
		} else if i >= len(playlist) {
			i = len(playlist) - 1
		}
		requests++
		if _, ok := starts[i]; !ok {
			starts[i] = time.Now()
		}
		e := playlist[i]
		song := e.title
		if e.artist != "" {
			song = e.artist + " - " + e.title
		}
		w.Header().Set("Content-Type", "text/html; charset=UTF-8") // as the real endpoint does
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uid":        strconv.Itoa(uidBase + i),
			"artistName": e.artist,
			"songTitle":  e.title,
			"song":       song,
			"date":       starts[i].In(loc).Format("2006-01-02 15:04:05"),
			"dj":         map[string]string{"djname": "FAKE DJ", "program": "FAKE PROGRAM"},
		})
	})
	http.HandleFunc("POST /__control", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		failing = r.URL.Query().Get("fail") == "1"
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	log.Printf("fake upstream listening on http://%s/Pwr/lastSong/", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
