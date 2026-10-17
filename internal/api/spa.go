package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
	"connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// spaHandler serves the built frontend. Existing files are served directly
// (hashed assets under /assets/ are cached immutably); extension-less paths
// fall back to index.html so client-side routes work as deep links. /api/
// is routed elsewhere and never reaches this handler.
func spaHandler(ui fs.FS) http.Handler {
	gz := &gzipCache{entries: map[string][]byte{}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		data, err := fs.ReadFile(ui, name)
		if err != nil {
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
			if data, err = fs.ReadFile(ui, name); err != nil {
				http.Error(w, "UI not available", http.StatusNotFound)
				return
			}
		}
		h := w.Header()
		if strings.HasPrefix(name, "assets/") {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "no-cache")
		}
		ctype := mime.TypeByExtension(path.Ext(name))
		if ctype == "" {
			ctype = http.DetectContentType(data)
		}
		h.Set("Content-Type", ctype)
		if name == "index.html" {
			h.Set("Content-Security-Policy", csp)
		}
		if compressible(ctype) && len(data) > 1024 && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h.Set("Content-Encoding", "gzip")
			h.Add("Vary", "Accept-Encoding")
			data = gz.get(name, data)
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	})
}

func compressible(ctype string) bool {
	return strings.HasPrefix(ctype, "text/") || strings.Contains(ctype, "javascript") ||
		strings.Contains(ctype, "json") || strings.Contains(ctype, "svg")
}

// gzipCache memoizes compressed copies of embedded (immutable) files.
type gzipCache struct {
	mu      sync.Mutex
	entries map[string][]byte
}

func (c *gzipCache) get(name string, data []byte) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if b, ok := c.entries[name]; ok {
		return b
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, _ = io.Copy(zw, bytes.NewReader(data))
	_ = zw.Close()
	c.entries[name] = buf.Bytes()
	return c.entries[name]
}
