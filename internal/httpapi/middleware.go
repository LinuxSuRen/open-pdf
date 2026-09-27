package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/linuxsuren/open-pdf/internal/pdfops"
)

// statusRecorder captures the response status and size for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// withRecover converts panics into 500 responses instead of dropping
// connections.
func (s *Server) withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", fmt.Sprint(rec),
					"stack", string(debug.Stack()))
				writeError(w, r, http.StatusInternalServerError, "internal",
					"internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// withRequestLog logs one line per request, skipping health probes.
func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

// withAuth enforces bearer-token authentication when a token is configured.
// The health probe stays open so container healthchecks keep working.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.APIToken == "" || r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) ||
			subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, prefix)), []byte(s.cfg.APIToken)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="open-pdf"`)
			writeError(w, r, http.StatusUnauthorized, "unauthorized",
				"missing or invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withCORS adds permissive or restricted CORS headers. A restricted setup
// echoes a matching configured origin instead of "*".
func (s *Server) withCORS(next http.Handler) http.Handler {
	allowAll := false
	allowed := make(map[string]bool, len(s.cfg.CORSOrigins))
	for _, o := range s.cfg.CORSOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[strings.ToLower(o)] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		switch {
		case allowAll:
			w.Header().Set("Access-Control-Allow-Origin", "*")
		case origin != "" && allowed[strings.ToLower(origin)]:
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions &&
			r.Header.Get("Access-Control-Request-Method") != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// writeJSON marshals v as a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// writeError renders the uniform error envelope.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
	})
}

// mapOpError translates pdfops failures into HTTP error responses.
func (s *Server) mapOpError(w http.ResponseWriter, r *http.Request, err error) {
	var status int
	var code string
	msg := err.Error()
	switch {
	case errors.Is(err, pdfops.ErrNoInput):
		status, code = http.StatusBadRequest, "no_files"
	case errors.Is(err, pdfops.ErrUnsupported):
		status, code = http.StatusUnsupportedMediaType, "unsupported_file_type"
	case errors.Is(err, pdfops.ErrInvalidPDF):
		status, code = http.StatusUnprocessableEntity, "invalid_pdf"
	case errors.Is(err, pdfops.ErrPageRange):
		status, code = http.StatusUnprocessableEntity, "invalid_page_range"
	case errors.Is(err, pdfops.ErrMissingParam):
		status, code = http.StatusBadRequest, "missing_parameter"
	case errors.Is(err, pdfops.ErrInvalidParam):
		status, code = http.StatusBadRequest, "invalid_parameter"
	default:
		status, code = http.StatusUnprocessableEntity, "pdf_operation_failed"
	}
	writeError(w, r, status, code, msg)
}
