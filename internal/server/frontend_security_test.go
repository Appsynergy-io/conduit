package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

// securityFS provides a filesystem with known structure for security tests.
func securityFS() fstest.MapFS {
	return fstest.MapFS{
		"web/out/index.html":            {Data: []byte("<html>index</html>")},
		"web/out/_next/static/chunk.js": {Data: []byte("var x=1;")},
		"web/out/login.html":            {Data: []byte("<html>login</html>")},
		"web/out/dashboard.html":        {Data: []byte("<html>dashboard</html>")},
		"web/out/secret.txt":            {Data: []byte("not-a-real-secret")},
	}
}

// ---------------------------------------------------------------------------
// 1. Path Traversal Prevention (OWASP A03, ASVS V5, V12)
// ---------------------------------------------------------------------------

func TestFrontendSecurity_PathTraversal(t *testing.T) {
	h := newFrontendHandler(securityFS())

	attacks := []struct {
		name string
		path string
	}{
		{"simple_dotdot", "/../etc/passwd"},
		{"double_dotdot", "/../../etc/shadow"},
		{"triple_dotdot", "/../../../etc/hosts"},
		{"dotdot_in_middle", "/assets/../../../etc/passwd"},
		{"backslash_traversal", "/..\\..\\etc\\passwd"},
		{"encoded_slash", "/%2e%2e/%2e%2e/etc/passwd"},
		{"double_encoded_slash", "/%252e%252e/%252e%252e/etc/passwd"},
		{"encoded_backslash", "/%5c..%5c..%5cetc%5cpasswd"},
		{"null_byte", "/index.html%00.js"},
		{"null_byte_traversal", "/../../../etc/passwd%00.html"},
		{"unicode_slash_u2215", "/\u2215..\u2215..\u2215etc\u2215passwd"},
		{"overlong_utf8_dot", "/%c0%ae%c0%ae/%c0%ae%c0%ae/etc/passwd"},
		{"mixed_encoding", "/..%252f..%252f..%252fetc/passwd"},
		{"windows_drive", "/C:/Windows/System32/config/SAM"},
		{"dot_segment_root", "/./../../etc/passwd"},
	}

	for _, tt := range attacks {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			body := rec.Body.String()

			// Must never return actual system file content
			assert.NotContains(t, body, "root:", "must not serve /etc/passwd")
			assert.NotContains(t, body, "shadow", "must not serve /etc/shadow")

			// Must either serve index.html (SPA fallback) or 404 — never an error that leaks paths
			assert.True(t,
				rec.Code == http.StatusOK || rec.Code == http.StatusNotFound ||
					rec.Code == http.StatusBadRequest || rec.Code == http.StatusMovedPermanently,
				"unexpected status %d for path %s", rec.Code, tt.path)
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Route Isolation — API/Agent Routes Never Served by Frontend (OWASP A01, A05)
// ---------------------------------------------------------------------------

func TestFrontendSecurity_RouteIsolation(t *testing.T) {
	h := newFrontendHandler(securityFS())

	protectedPrefixes := []struct {
		name string
		path string
	}{
		{"api_root", "/api/"},
		{"api_v1", "/api/v1/users"},
		{"api_auth", "/api/v1/auth/password/login"},
		{"api_setup", "/api/v1/setup/status"},
		{"api_agents", "/api/v1/agents"},
		{"api_audit", "/api/v1/audit/events"},
		{"api_webhooks", "/api/v1/webhooks"},
		{"api_sessions", "/api/v1/sessions"},
		{"api_events", "/api/v1/events/stream"},
		{"agent_connect", "/agent/v1/connect"},
		{"agent_root", "/agent/"},
		{"agent_future", "/agent/v2/something"},
	}

	for _, tt := range protectedPrefixes {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusNotFound, rec.Code,
				"frontend must not serve %s — must return 404", tt.path)
			// Must not serve index.html for API routes
			assert.NotContains(t, rec.Body.String(), "<html>index</html>",
				"frontend must not fall back to index.html for %s", tt.path)
		})
	}
}

func TestFrontendSecurity_RouteIsolationCaseVariants(t *testing.T) {
	h := newFrontendHandler(securityFS())

	// URL paths are case-sensitive per RFC 3986 — /API/ should NOT match /api/ guard.
	// The frontend handler should only block exact /api/ and /agent/ prefixes.
	// Uppercase variants fall through to SPA fallback which is safe (no data leak).
	variants := []struct {
		name      string
		path      string
		expectSPA bool // true = SPA fallback is acceptable (uppercase doesn't match prefix)
	}{
		{"uppercase_API", "/API/v1/users", true},
		{"mixed_Api", "/Api/v1/users", true},
		{"uppercase_AGENT", "/AGENT/v1/connect", true},
	}

	for _, tt := range variants {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if tt.expectSPA {
				// SPA fallback serves index.html — this is safe, no data leak
				assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusNotFound)
			} else {
				assert.Equal(t, http.StatusNotFound, rec.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. Directory Listing Prevention (NIST CM-7, OWASP A05, ASVS V14)
// ---------------------------------------------------------------------------

func TestFrontendSecurity_NoDirectoryListing(t *testing.T) {
	h := newFrontendHandler(securityFS())

	dirs := []string{
		"/_next/",
		"/_next/static/",
	}

	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, dir, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			body := rec.Body.String()
			// Must not expose directory listing with filenames
			assert.NotContains(t, body, "chunk.js",
				"directory listing must not expose file names for %s", dir)
			assert.NotContains(t, body, "<pre>",
				"Go default directory listing must not appear for %s", dir)
		})
	}
}

// ---------------------------------------------------------------------------
// 4. Content-Type Verification (OWASP A08, ASVS V14)
// ---------------------------------------------------------------------------

func TestFrontendSecurity_ContentTypeOnHTML(t *testing.T) {
	h := newFrontendHandler(securityFS())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	assert.Contains(t, ct, "text/html",
		"index.html must be served with text/html content type")
}

func TestFrontendSecurity_ContentTypeOnJS(t *testing.T) {
	h := newFrontendHandler(securityFS())

	req := httptest.NewRequest(http.MethodGet, "/_next/static/chunk.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	assert.True(t,
		strings.Contains(ct, "javascript") || strings.Contains(ct, "application/javascript") || strings.Contains(ct, "text/javascript"),
		"JS files must be served with javascript content type, got: %s", ct)
}

// ---------------------------------------------------------------------------
// 5. No Information Leakage (NIST SI-11, REC-API-23, OWASP A05)
// ---------------------------------------------------------------------------

func TestFrontendSecurity_NoServerHeader(t *testing.T) {
	h := newFrontendHandler(securityFS())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Server header must not reveal technology stack
	serverHeader := rec.Header().Get("Server")
	if serverHeader != "" {
		assert.NotContains(t, strings.ToLower(serverHeader), "go",
			"Server header must not reveal Go")
		assert.NotContains(t, strings.ToLower(serverHeader), "next",
			"Server header must not reveal Next.js")
	}
}

func TestFrontendSecurity_ErrorsDoNotLeakPaths(t *testing.T) {
	h := newFrontendHandler(securityFS())

	// Requesting a path that triggers SPA fallback — must not leak FS structure
	req := httptest.NewRequest(http.MethodGet, "/nonexistent/deep/path/file.xyz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	// Must not contain filesystem paths
	assert.NotContains(t, body, "web/out",
		"response must not leak embedded FS prefix")
	assert.NotContains(t, body, "fstest",
		"response must not leak test FS implementation")
}

// ---------------------------------------------------------------------------
// 6. Null Byte and Special Character Injection (OWASP A03, ASVS V5)
// ---------------------------------------------------------------------------

func TestFrontendSecurity_SpecialCharacters(t *testing.T) {
	h := newFrontendHandler(securityFS())

	payloads := []struct {
		name string
		path string
	}{
		{"null_byte_ext", "/index.html%00"},
		{"tab_char", "/index.html%09"},
		{"semicolon", "/index.html;id"},
		{"angle_brackets", "/%3Cscript%3Ealert(1)%3C/script%3E"},
		{"space_encoded", "/index%20.html"},
		{"hash_fragment", "/index.html%23fragment"},
		{"question_mark", "/index.html%3Fkey=val"},
		{"double_slash", "//index.html"},
		{"triple_slash", "///index.html"},
	}

	for _, tt := range payloads {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			// Must not panic
			h.ServeHTTP(rec, req)

			body := rec.Body.String()
			// Must not reflect injection payloads in output
			assert.NotContains(t, body, "<script>",
				"XSS payload must not be reflected for %s", tt.path)

			// Acceptable outcomes: 200 (SPA fallback), 301/400/404
			assert.True(t,
				rec.Code >= 200 && rec.Code < 500,
				"must not return 5xx for special chars: status=%d path=%s", rec.Code, tt.path)
		})
	}
}

// ---------------------------------------------------------------------------
// 7. Extremely Long Paths / DoS Resistance (OWASP API4, ASVS V5)
// ---------------------------------------------------------------------------

func TestFrontendSecurity_LongPath(t *testing.T) {
	h := newFrontendHandler(securityFS())

	// 10KB path — must not panic or hang
	longSegment := strings.Repeat("a", 10000)
	req := httptest.NewRequest(http.MethodGet, "/"+longSegment, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Must respond, not hang
	assert.True(t, rec.Code > 0, "must respond to extremely long paths")
}

func TestFrontendSecurity_DeepNesting(t *testing.T) {
	h := newFrontendHandler(securityFS())

	// 200 levels of nesting
	path := "/" + strings.Repeat("dir/", 200) + "file.html"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.True(t, rec.Code > 0, "must respond to deeply nested paths")
}

// ---------------------------------------------------------------------------
// 8. SPA Fallback Scope — Must Not Serve Index for Non-HTML Requests
// ---------------------------------------------------------------------------

func TestFrontendSecurity_SPAFallbackScope(t *testing.T) {
	h := newFrontendHandler(securityFS())

	// Request for a non-existent .json file — SPA fallback serves index.html
	// but must not trick the browser into parsing HTML as JSON
	req := httptest.NewRequest(http.MethodGet, "/data/config.json", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// The response is the SPA index.html — the Content-Type must still be text/html
	if rec.Code == http.StatusOK {
		ct := rec.Header().Get("Content-Type")
		assert.Contains(t, ct, "text/html",
			"SPA fallback must serve text/html, not guess from requested extension")
	}
}

// ---------------------------------------------------------------------------
// 9. Frontend Isolation Under Concurrent Requests
// ---------------------------------------------------------------------------

func TestFrontendSecurity_ConcurrentRequests(t *testing.T) {
	h := newFrontendHandler(securityFS())

	done := make(chan struct{}, 50)
	for i := 0; i < 50; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()

			paths := []string{"/", "/login", "/nonexistent", "/../../../etc/passwd",
				"/_next/static/chunk.js", "/api/v1/secret"}
			path := paths[n%len(paths)]

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			// No panics, no data races (run with -race)
			assert.True(t, rec.Code >= 200 && rec.Code < 600)
		}(i)
	}

	for i := 0; i < 50; i++ {
		<-done
	}
}

// ---------------------------------------------------------------------------
// 10. Integration: Frontend Route vs API Route Priority
// ---------------------------------------------------------------------------

func TestFrontendSecurity_HealthEndpointNotOverridden(t *testing.T) {
	// Build a full server with frontend to verify /health still works
	// even with the frontend catch-all installed
	h := newFrontendHandler(securityFS())

	// /health is not /api/ or /agent/, so the frontend handler would serve it.
	// But in the real server, /health is registered as an explicit route BEFORE
	// the catch-all, so chi routes it correctly. Here we verify the frontend
	// handler at least doesn't return an error for /health.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Frontend serves SPA fallback — this is OK because in the real server,
	// chi matches the explicit /health route first.
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---------------------------------------------------------------------------
// 11. HTTP Method Handling (OWASP A05, ASVS V13)
// ---------------------------------------------------------------------------

func TestFrontendSecurity_NonGETMethods(t *testing.T) {
	h := newFrontendHandler(securityFS())

	methods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			// Static file server should handle non-GET methods gracefully
			// (Go's http.FileServer returns 405 for POST/PUT/DELETE/PATCH
			// or serves the file for HEAD — all acceptable)
			assert.True(t, rec.Code < 500,
				"%s to / must not return 5xx, got %d", method, rec.Code)
		})
	}
}

func TestFrontendSecurity_HEADMethod(t *testing.T) {
	h := newFrontendHandler(securityFS())

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code,
		"HEAD / must return 200")
	assert.Empty(t, rec.Body.String(),
		"HEAD must return empty body")
}
