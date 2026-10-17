package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Fetcher retrieves the raw upstream body. Implementations must honor ctx,
// bound the body size, and report failures using the error types below.
type Fetcher interface {
	Fetch(ctx context.Context) ([]byte, error)
	// Mode identifies the transport ("direct" or "flaresolverr") in logs and status.
	Mode() string
}

// StatusError is a non-2xx response from the upstream itself (whether
// received directly or relayed by FlareSolverr).
type StatusError struct {
	StatusCode int
	RetryAfter time.Duration // zero when absent or unparsable
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("upstream returned HTTP %d", e.StatusCode)
}

// SolverError is a failure of the FlareSolverr service itself, as opposed to
// the upstream site: unreachable, timed out, a challenge it could not solve,
// or a malformed solver response.
type SolverError struct {
	HTTPStatus int    // FlareSolverr HTTP status, 0 if no response
	Message    string // solver-provided message, may be empty
	Err        error  // underlying transport error, may be nil
}

func (e *SolverError) Error() string {
	var b strings.Builder
	b.WriteString("flaresolverr")
	if e.HTTPStatus != 0 {
		fmt.Fprintf(&b, " HTTP %d", e.HTTPStatus)
	}
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

func (e *SolverError) Unwrap() error { return e.Err }

// ErrTooLarge is returned when a body exceeds the configured limit.
var ErrTooLarge = errors.New("upstream response exceeds size limit")

// DirectFetcher performs a plain HTTPS GET against the upstream URL.
type DirectFetcher struct {
	URL       string
	UserAgent string
	MaxBytes  int64
	Client    *http.Client
}

// NewDirectFetcher builds a fetcher with its own client and timeout. Cookies
// are not retained between polls.
func NewDirectFetcher(url, userAgent string, timeout time.Duration, maxBytes int64) *DirectFetcher {
	return &DirectFetcher{
		URL:       url,
		UserAgent: userAgent,
		MaxBytes:  maxBytes,
		Client:    &http.Client{Timeout: timeout},
	}
}

func (f *DirectFetcher) Mode() string { return "direct" }

func (f *DirectFetcher) Fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.1")
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, &StatusError{StatusCode: resp.StatusCode, RetryAfter: ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())}
	}
	return readLimited(resp.Body, f.MaxBytes)
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, ErrTooLarge
	}
	return b, nil
}

// ParseRetryAfter accepts delta-seconds or an HTTP-date. It returns zero for
// empty, invalid, or past values.
func ParseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(min(secs, int64((24*time.Hour)/time.Second))) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

// FlareSolverrFetcher relays the upstream request through a FlareSolverr
// instance using its documented v1 API (POST /v1, cmd "request.get").
type FlareSolverrFetcher struct {
	BaseURL     string
	UpstreamURL string
	// MaxTimeout is passed to FlareSolverr as maxTimeout and also bounds the
	// HTTP call to FlareSolverr (plus a small margin).
	MaxTimeout time.Duration
	MaxBytes   int64
	Client     *http.Client
}

// Solver envelopes include rendered HTML, headers and cookies, so they get a
// fixed, larger bound than the extracted upstream body.
const maxSolverEnvelope = 4 << 20

func NewFlareSolverrFetcher(baseURL, upstreamURL string, maxTimeout time.Duration, maxBytes int64) *FlareSolverrFetcher {
	return &FlareSolverrFetcher{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		UpstreamURL: upstreamURL,
		MaxTimeout:  maxTimeout,
		MaxBytes:    maxBytes,
		Client:      &http.Client{Timeout: maxTimeout + 10*time.Second},
	}
}

func (f *FlareSolverrFetcher) Mode() string { return "flaresolverr" }

type solverRequest struct {
	Cmd        string `json:"cmd"`
	URL        string `json:"url"`
	MaxTimeout int64  `json:"maxTimeout"`
}

type solverResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution *struct {
		URL      string          `json:"url"`
		Status   int             `json:"status"`
		Headers  json.RawMessage `json:"headers"`
		Response string          `json:"response"`
	} `json:"solution"`
}

func (f *FlareSolverrFetcher) Fetch(ctx context.Context) ([]byte, error) {
	reqBody, _ := json.Marshal(solverRequest{
		Cmd:        "request.get",
		URL:        f.UpstreamURL,
		MaxTimeout: f.MaxTimeout.Milliseconds(),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.BaseURL+"/v1", bytes.NewReader(reqBody))
	if err != nil {
		return nil, &SolverError{Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, &SolverError{Err: err}
	}
	defer resp.Body.Close()
	raw, err := readLimited(resp.Body, maxSolverEnvelope)
	if err != nil {
		return nil, &SolverError{HTTPStatus: resp.StatusCode, Err: err}
	}
	var sr solverResponse
	if err := json.Unmarshal(raw, &sr); err != nil {
		return nil, &SolverError{HTTPStatus: resp.StatusCode, Message: "malformed solver response", Err: err}
	}
	if resp.StatusCode != http.StatusOK || sr.Status != "ok" {
		msg := sr.Message
		if msg == "" {
			msg = "status " + strconv.Quote(sr.Status)
		}
		return nil, &SolverError{HTTPStatus: resp.StatusCode, Message: msg}
	}
	if sr.Solution == nil {
		return nil, &SolverError{HTTPStatus: resp.StatusCode, Message: "solver response has no solution"}
	}
	if s := sr.Solution.Status; s < 200 || s > 299 {
		return nil, &StatusError{StatusCode: s, RetryAfter: ParseRetryAfter(headerValue(sr.Solution.Headers, "Retry-After"), time.Now())}
	}
	body := extractBody(sr.Solution.Response)
	if int64(len(body)) > f.MaxBytes {
		return nil, ErrTooLarge
	}
	return body, nil
}

var preRe = regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`)

// extractBody returns the upstream payload from a FlareSolverr solution.
// FlareSolverr drives a real browser and returns the rendered document, so a
// JSON response usually arrives wrapped as <html>…<pre>{json}</pre>…</html>
// with HTML escaping applied. A bare JSON body is returned unchanged.
func extractBody(resp string) []byte {
	t := strings.TrimSpace(resp)
	if strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
		return []byte(t)
	}
	if m := preRe.FindStringSubmatch(t); m != nil {
		return []byte(strings.TrimSpace(html.UnescapeString(m[1])))
	}
	return []byte(t)
}

// headerValue looks up a header in FlareSolverr's "headers" field, which is
// an object in practice (and often empty for browser-rendered responses).
func headerValue(raw json.RawMessage, name string) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for k, v := range m {
		if strings.EqualFold(k, name) {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}
