// Package api serves the read-only JSON API, health endpoints and the
// embedded single-page application.
package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aurimasniekis/go-pwr-tracker/internal/analytics"
	"github.com/aurimasniekis/go-pwr-tracker/internal/store"
)

type Options struct {
	Timezone         *time.Location
	PollInterval     time.Duration
	StaleAfter       time.Duration
	ContinuityGap    time.Duration
	FetchMode        string
	CollectorEnabled bool
	// NextAttemptAt reports the collector's next scheduled poll; may be nil.
	NextAttemptAt func() time.Time
	// Now is the clock; defaults to time.Now.
	Now func() time.Time
	// UI is the built frontend (index.html at its root); nil disables it.
	UI  fs.FS
	Log *slog.Logger
}

type Server struct {
	store *store.Store
	opts  Options
}

func New(s *store.Store, opts Options) *Server {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	return &Server{store: s, opts: opts}
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)

	mux.HandleFunc("GET /api/v1/now-playing", s.nowPlaying)
	mux.HandleFunc("GET /api/v1/status", s.status)
	mux.HandleFunc("GET /api/v1/plays", s.listPlays)
	mux.HandleFunc("GET /api/v1/tracks", s.listTracks)
	mux.HandleFunc("GET /api/v1/tracks/{id}", s.getTrack)
	mux.HandleFunc("GET /api/v1/artists", s.listArtists)
	mux.HandleFunc("GET /api/v1/artists/{id}", s.getArtist)
	mux.HandleFunc("GET /api/v1/stats/summary", s.statsSummary)
	mux.HandleFunc("GET /api/v1/stats/top-tracks", s.topTracks)
	mux.HandleFunc("GET /api/v1/stats/top-artists", s.topArtists)
	mux.HandleFunc("GET /api/v1/stats/airplay", s.airplay)
	mux.HandleFunc("GET /api/v1/stats/hourly", s.hourly)
	// Unknown API paths and methods must never fall through to the SPA.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "unknown API endpoint")
	})

	if s.opts.UI != nil {
		mux.Handle("/", spaHandler(s.opts.UI))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "pwr-tracker API is running. The web UI is not embedded in this build (build with -tags embedui).")
		})
	}
	return s.middleware(mux)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if v := recover(); v != nil {
				s.opts.Log.Error("panic serving request", "path", r.URL.Path, "panic", fmt.Sprint(v))
				writeError(rec, http.StatusInternalServerError, "internal", "internal server error")
			}
			level := slog.LevelDebug
			if rec.status >= 500 {
				level = slog.LevelWarn
			}
			s.opts.Log.Log(r.Context(), level, "http request", "method", r.Method, "path", r.URL.Path,
				"status", rec.status, "duration_ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(rec, r)
	})
}

// --- responses ---

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	var e apiError
	e.Error.Code, e.Error.Message = code, msg
	writeJSON(w, status, e)
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	s.opts.Log.Error("request failed", "path", r.URL.Path, "error", err.Error())
	writeError(w, http.StatusInternalServerError, "internal", "internal server error")
}

// badRequest is returned by parameter helpers.
type badRequest struct{ msg string }

func (e badRequest) Error() string { return e.msg }

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var br badRequest
	switch {
	case errors.As(err, &br):
		writeError(w, http.StatusBadRequest, "invalid_parameter", br.msg)
	case errors.Is(err, analytics.ErrInvalidRange):
		writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	default:
		s.internalError(w, r, err)
	}
}

// --- parameters ---

func intParam(r *http.Request, name string, def, lo, hi int) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, badRequest{fmt.Sprintf("%s must be an integer between %d and %d", name, lo, hi)}
	}
	return n, nil
}

func idParam(r *http.Request, name string) (int64, error) {
	v := r.URL.Query().Get(name)
	if r.PathValue(name) != "" {
		v = r.PathValue(name)
	}
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, badRequest{name + " must be a positive integer"}
	}
	return n, nil
}

func boolParam(r *http.Request, name string) (bool, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, badRequest{name + " must be true or false"}
	}
	return b, nil
}

func queryParam(r *http.Request) (string, error) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 100 {
		return "", badRequest{"q must be at most 100 bytes"}
	}
	return q, nil
}

// optionalTime parses an optional from/to bound for the activity feed.
func (s *Server) optionalTime(r *http.Request, name string) (*time.Time, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, nil
	}
	t, err := analytics.ParseTime(v, s.opts.Timezone)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func encodeCursor(c store.Cursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.StartAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(c.ID, 10)))
}

func decodeCursor(s string) (*store.Cursor, error) {
	if s == "" {
		return nil, nil
	}
	bad := badRequest{"cursor is invalid"}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, bad
	}
	tsPart, idPart, ok := strings.Cut(string(b), "|")
	if !ok {
		return nil, bad
	}
	t, err := time.Parse(time.RFC3339Nano, tsPart)
	if err != nil {
		return nil, bad
	}
	id, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil || id <= 0 {
		return nil, bad
	}
	return &store.Cursor{StartAt: t, ID: id}, nil
}

// --- health ---

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		s.opts.Log.Warn("readiness check failed", "error", err.Error())
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "reason": "database not usable"})
		return
	}
	// Upstream freshness is deliberately not part of readiness: an
	// upstream outage must not take historical data offline.
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
