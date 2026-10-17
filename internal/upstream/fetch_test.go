package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDirectFetcherSuccess(t *testing.T) {
	fixture, _ := os.ReadFile("testdata/music.json")
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.UserAgent()
		w.Header().Set("Content-Type", "text/html; charset=UTF-8") // as observed upstream
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	f := NewDirectFetcher(srv.URL, "pwr-tracker/test", time.Second, 64*1024)
	body, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(fixture) || gotUA != "pwr-tracker/test" {
		t.Fatalf("body/UA mismatch: %q", gotUA)
	}
}

func TestDirectFetcherStatusAndRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := NewDirectFetcher(srv.URL, "ua", time.Second, 1024).Fetch(context.Background())
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != 429 || se.RetryAfter != 2*time.Minute {
		t.Fatalf("got %#v", err)
	}
}

func TestDirectFetcherTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 2048))
	}))
	defer srv.Close()

	_, err := NewDirectFetcher(srv.URL, "ua", time.Second, 1024).Fetch(context.Background())
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
}

func TestDirectFetcherTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	start := time.Now()
	_, err := NewDirectFetcher(srv.URL, "ua", 50*time.Millisecond, 1024).Fetch(context.Background())
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("expected prompt timeout, got %v after %s", err, time.Since(start))
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":                              0,
		"0":                             0,
		"-5":                            0,
		"abc":                           0,
		"30":                            30 * time.Second,
		"Thu, 01 Jan 2026 12:05:00 GMT": 5 * time.Minute,
		"Thu, 01 Jan 2026 11:00:00 GMT": 0,
	}
	for in, want := range cases {
		if got := ParseRetryAfter(in, now); got != want {
			t.Errorf("ParseRetryAfter(%q) = %s, want %s", in, got, want)
		}
	}
}

// fakeSolver emulates FlareSolverr's POST /v1 API.
func fakeSolver(t *testing.T, handle func(req solverRequest) (int, any)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		var req solverRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		status, body := handle(req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func okSolution(status int, response string, headers map[string]string) map[string]any {
	return map[string]any{
		"status":         "ok",
		"message":        "Challenge not detected!",
		"startTimestamp": 1, "endTimestamp": 2, "version": "3.4.0",
		"solution": map[string]any{
			"url": "https://example.test/", "status": status, "headers": headers,
			"response": response, "cookies": []any{}, "userAgent": "Mozilla/5.0",
		},
	}
}

func TestFlareSolverrRequestShapeAndHTMLWrappedJSON(t *testing.T) {
	fixture, _ := os.ReadFile("testdata/music.json")
	// Browsers render JSON inside <pre> with HTML escaping.
	wrapped := `<html><head><meta name="color-scheme" content="light dark"></head><body><pre style="word-wrap: break-word;">` +
		strings.ReplaceAll(strings.ReplaceAll(string(fixture), "&", "&amp;"), "'", "&#39;") +
		`</pre><div class="json-formatter-container"></div></body></html>`
	srv := fakeSolver(t, func(req solverRequest) (int, any) {
		if req.Cmd != "request.get" || req.URL != "https://upstream.test/Pwr/lastSong/" || req.MaxTimeout != 30000 {
			t.Errorf("unexpected request %+v", req)
		}
		return 200, okSolution(200, wrapped, map[string]string{})
	})
	defer srv.Close()

	f := NewFlareSolverrFetcher(srv.URL+"/", "https://upstream.test/Pwr/lastSong/", 30*time.Second, 64*1024)
	body, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != strings.TrimSpace(string(fixture)) {
		t.Fatalf("extracted body mismatch:\n%s", body)
	}
	if _, err := Parse(body, time.UTC, time.Date(2026, 10, 17, 19, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("extracted body does not parse: %v", err)
	}
}

func TestFlareSolverrBareJSON(t *testing.T) {
	srv := fakeSolver(t, func(solverRequest) (int, any) {
		return 200, okSolution(200, ` {"songTitle":"x"} `, nil)
	})
	defer srv.Close()
	body, err := NewFlareSolverrFetcher(srv.URL, "https://u.test/", time.Second, 1024).Fetch(context.Background())
	if err != nil || string(body) != `{"songTitle":"x"}` {
		t.Fatalf("%q %v", body, err)
	}
}

func TestFlareSolverrSolverFailure(t *testing.T) {
	srv := fakeSolver(t, func(solverRequest) (int, any) {
		return 500, map[string]any{"status": "error", "message": "Error: Error solving the challenge. Timeout after 60.0 seconds."}
	})
	defer srv.Close()
	_, err := NewFlareSolverrFetcher(srv.URL, "https://u.test/", time.Second, 1024).Fetch(context.Background())
	var se *SolverError
	if !errors.As(err, &se) || se.HTTPStatus != 500 || !strings.Contains(se.Message, "Timeout") {
		t.Fatalf("got %#v", err)
	}
	var st *StatusError
	if errors.As(err, &st) {
		t.Fatal("solver failure must not be reported as an upstream status error")
	}
}

func TestFlareSolverrUpstreamFailure(t *testing.T) {
	srv := fakeSolver(t, func(solverRequest) (int, any) {
		return 200, okSolution(503, "<html>down</html>", map[string]string{"retry-after": "90"})
	})
	defer srv.Close()
	_, err := NewFlareSolverrFetcher(srv.URL, "https://u.test/", time.Second, 1024).Fetch(context.Background())
	var st *StatusError
	if !errors.As(err, &st) || st.StatusCode != 503 || st.RetryAfter != 90*time.Second {
		t.Fatalf("got %#v", err)
	}
}

func TestFlareSolverrMalformedAndUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "not json")
	}))
	_, err := NewFlareSolverrFetcher(srv.URL, "https://u.test/", time.Second, 1024).Fetch(context.Background())
	var se *SolverError
	if !errors.As(err, &se) {
		t.Fatalf("malformed: got %#v", err)
	}
	srv.Close()

	_, err = NewFlareSolverrFetcher(srv.URL, "https://u.test/", time.Second, 1024).Fetch(context.Background())
	if !errors.As(err, &se) || se.Err == nil {
		t.Fatalf("unreachable: got %#v", err)
	}
}

func TestFlareSolverrAppliesBodyLimit(t *testing.T) {
	srv := fakeSolver(t, func(solverRequest) (int, any) {
		return 200, okSolution(200, `{"songTitle":"`+strings.Repeat("x", 4000)+`"}`, nil)
	})
	defer srv.Close()
	_, err := NewFlareSolverrFetcher(srv.URL, "https://u.test/", time.Second, 1024).Fetch(context.Background())
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
}
