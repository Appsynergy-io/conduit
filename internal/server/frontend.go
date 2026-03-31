package server

import (
	"io/fs"
	"net/http"
	"strings"
)

// setCacheHeaders sets Cache-Control based on the asset path.
// Hashed static assets (_next/) are immutable; HTML pages must revalidate.
func setCacheHeaders(w http.ResponseWriter, path string) {
	if strings.HasPrefix(path, "/_next/static/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
}

// newFrontendHandler creates an http.Handler that serves the embedded Next.js
// static export. It serves files from the embedded filesystem and falls back
// to index.html for client-side routing (SPA behavior).
func newFrontendHandler(embedded fs.FS) http.Handler {
	if embedded == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<!DOCTYPE html><html><body><h1>Conduit</h1><p>Frontend not built. Run <code>cd web && pnpm build</code></p></body></html>`))
		})
	}

	// Strip the "web/out" prefix from the embedded FS
	sub, err := fs.Sub(embedded, "web/out")
	if err != nil {
		// If the frontend wasn't built, serve a placeholder
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<!DOCTYPE html><html><body><h1>Conduit</h1><p>Frontend not built. Run <code>cd web && pnpm build</code></p></body></html>`))
		})
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Don't serve frontend for API or agent WebSocket routes
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/agent/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the exact file (CSS, JS, images, etc.)
		// Skip directories — Next.js creates dirs like login/ alongside login.html
		if path != "/" {
			cleanPath := strings.TrimPrefix(path, "/")
			if f, err := sub.Open(cleanPath); err == nil {
				stat, statErr := f.Stat()
				f.Close()
				if statErr == nil && !stat.IsDir() {
					setCacheHeaders(w, path)
					fileServer.ServeHTTP(w, r)
					return
				}
			}
		}

		// For HTML pages, try path.html (Next.js static export convention)
		if !strings.Contains(path, ".") && path != "/" {
			htmlPath := strings.TrimPrefix(path, "/") + ".html"
			if f, err := sub.Open(htmlPath); err == nil {
				f.Close()
				w.Header().Set("Cache-Control", "no-cache")
				r.URL.Path = path + ".html"
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// Fall back to index.html for SPA routing
		w.Header().Set("Cache-Control", "no-cache")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
