// Package httpapi exposes the PDF operations as a REST API.
package httpapi

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/linuxsuren/open-pdf/internal/buildinfo"
	"github.com/linuxsuren/open-pdf/internal/config"
	webpkg "github.com/linuxsuren/open-pdf/web"
)

// webui holds the embedded Vue build rooted at its dist directory.
var webui fs.FS = webpkg.Dist()

// Server wires the HTTP routes and holds shared dependencies.
type Server struct {
	cfg *config.Config
	log *slog.Logger
	mux *http.ServeMux
}

// New creates the server and registers all routes.
func New(cfg *config.Config, log *slog.Logger) (*Server, error) {
	if err := os.MkdirAll(cfg.TempDir, 0o755); err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, log: log, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

// Handler returns the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = s.withRequestLog(h)
	h = s.withAuth(h)
	h = s.withCORS(h)
	h = s.withRecover(h)
	return h
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /healthz", s.handleHealth)
	m.HandleFunc("GET /api/v1/version", s.handleVersion)
	m.HandleFunc("POST /api/v1/merge", s.requireJSON(s.handleMerge))
	m.HandleFunc("POST /api/v1/split", s.requireJSON(s.handleSplit))
	m.HandleFunc("POST /api/v1/compress", s.requireJSON(s.handleCompress))
	m.HandleFunc("POST /api/v1/watermark", s.requireJSON(s.handleWatermark))
	m.HandleFunc("POST /api/v1/rotate", s.requireJSON(s.handleRotate))
	m.HandleFunc("POST /api/v1/info", s.requireJSON(s.handleInfo))
	m.HandleFunc("POST /api/v1/encrypt", s.requireJSON(s.handleEncrypt))
	m.HandleFunc("POST /api/v1/decrypt", s.requireJSON(s.handleDecrypt))
	m.HandleFunc("GET /api", s.handleAPIIndex)
	m.HandleFunc("GET /assets/{path...}", s.handleAsset)
	m.HandleFunc("GET /{$}", s.handleIndex)
	m.HandleFunc("GET /", s.handleNotFound)
}

// requireJSON ensures error responses are JSON even when a handler panics
// early or a route is hit with the wrong method.
func (s *Server) requireJSON(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		h(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": buildinfo.Version,
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "open-pdf",
		"version": buildinfo.Version,
		"commit":  buildinfo.Commit,
		"date":    buildinfo.Date,
	})
}

// handleAPIIndex lists the API endpoints as JSON (the browser UI lives at /).
func (s *Server) handleAPIIndex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":        "open-pdf",
		"version":     buildinfo.Version,
		"description": "REST API for common PDF operations",
		"endpoints": []string{
			"GET  /healthz",
			"GET  /api/v1/version",
			"POST /api/v1/merge",
			"POST /api/v1/split",
			"POST /api/v1/compress",
			"POST /api/v1/watermark",
			"POST /api/v1/rotate",
			"POST /api/v1/info",
			"POST /api/v1/encrypt",
			"POST /api/v1/decrypt",
		},
	})
}

// handleIndex serves the embedded Vue application.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(webui, "index.html")
	if err != nil {
		writeError(w, r, http.StatusNotFound, "ui_missing",
			"web UI not bundled; run `make web` and rebuild")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleNotFound renders a JSON 404 for unmatched GET paths and a JSON 405
// when the path exists under a different method.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	if methods, ok := knownRoutes[r.URL.Path]; ok {
		allowed := false
		for _, m := range methods {
			if m == r.Method {
				allowed = true
				break
			}
		}
		if !allowed {
			w.Header().Set("Allow", strings.Join(methods, ", "))
			writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed",
				r.Method+" is not allowed for "+r.URL.Path)
			return
		}
	}
	writeError(w, r, http.StatusNotFound, "not_found",
		"unknown path "+r.URL.Path+"; see GET /api for the endpoint list")
}

// knownRoutes documents the method map used for 405 detection. It mirrors
// the patterns registered in routes().
var knownRoutes = map[string][]string{
	"/healthz":            {"GET"},
	"/api":                {"GET"},
	"/api/v1/version":     {"GET"},
	"/api/v1/merge":       {"POST"},
	"/api/v1/split":       {"POST"},
	"/api/v1/compress":    {"POST"},
	"/api/v1/watermark":   {"POST"},
	"/api/v1/rotate":      {"POST"},
	"/api/v1/info":        {"POST"},
	"/api/v1/encrypt":     {"POST"},
	"/api/v1/decrypt":     {"POST"},
}

// handleAsset serves the hashed Vite build artifacts immutably.
func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("path")
	if strings.Contains(name, "..") || strings.ContainsAny(name, "\\") {
		http.NotFound(w, r)
		return
	}
	f, err := webui.Open("assets/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, st.Name(), time.Time{}, rs)
		return
	}
	data, err := io.ReadAll(f)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, st.Name(), time.Time{}, strings.NewReader(string(data)))
}

// newRequestDir creates a scratch directory that the handler must remove.
func (s *Server) newRequestDir() (string, error) {
	return os.MkdirTemp(s.cfg.TempDir, "req-")
}

// safeBaseName sanitizes a client supplied file name to its harmless base.
func safeBaseName(name string) string {
	name = filepath.Base(name)
	if name == "." || name == ".." || name == "/" || name == `\` {
		return ""
	}
	return name
}
