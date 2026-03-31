package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"web/out/index.html":        {Data: []byte("<html>index</html>")},
		"web/out/_next/static/a.js": {Data: []byte("console.log('a')")},
		"web/out/login.html":        {Data: []byte("<html>login</html>")},
		"web/out/dashboard.html":    {Data: []byte("<html>dashboard</html>")},
		"web/out/favicon.ico":       {Data: []byte("icon")},
	}
}

// testFSWithDirs mirrors real Next.js static export output where both
// login.html AND login/ directory exist (Next.js creates login/index.html
// inside the directory). Without the IsDir check in the handler, Open("login")
// matches the directory and the file server tries to serve it instead of
// falling through to login.html.
func testFSWithDirs() fstest.MapFS {
	return fstest.MapFS{
		"web/out/index.html":              {Data: []byte("<html>index</html>")},
		"web/out/_next/static/a.js":       {Data: []byte("console.log('a')")},
		"web/out/login.html":              {Data: []byte("<html>login page</html>")},
		"web/out/login/index.html":        {Data: []byte("<html>login dir index</html>")},
		"web/out/setup.html":              {Data: []byte("<html>setup page</html>")},
		"web/out/setup/index.html":        {Data: []byte("<html>setup dir index</html>")},
		"web/out/dashboard.html":          {Data: []byte("<html>dashboard</html>")},
		"web/out/dashboard/terminal.html": {Data: []byte("<html>terminal</html>")},
		"web/out/favicon.ico":             {Data: []byte("icon")},
	}
}

func TestFrontendHandler_IndexPage(t *testing.T) {
	h := newFrontendHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "index")
}

func TestFrontendHandler_StaticAsset(t *testing.T) {
	h := newFrontendHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/_next/static/a.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "console.log")
}

func TestFrontendHandler_HTMLPage(t *testing.T) {
	h := newFrontendHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "login")
}

func TestFrontendHandler_SPAFallback(t *testing.T) {
	h := newFrontendHandler(testFS())
	// Unknown route should fall back to index.html
	req := httptest.NewRequest(http.MethodGet, "/some/unknown/route", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "index")
}

func TestFrontendHandler_APIRouteExcluded(t *testing.T) {
	h := newFrontendHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestFrontendHandler_AgentRouteExcluded(t *testing.T) {
	h := newFrontendHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/agent/v1/connect", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestFrontendHandler_FaviconServed(t *testing.T) {
	h := newFrontendHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "icon", rec.Body.String())
}

func TestFrontendHandler_HTMLPageWithDirectory(t *testing.T) {
	// Next.js creates both login.html and login/ directory. The handler must
	// serve login.html (not the directory) when the browser requests /login.
	h := newFrontendHandler(testFSWithDirs())

	tests := []struct {
		path     string
		contains string
	}{
		{"/login", "login page"},
		{"/setup", "setup page"},
		{"/dashboard", "dashboard"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code, "path %s", tt.path)
		assert.Contains(t, rec.Body.String(), tt.contains, "path %s", tt.path)
	}
}

func TestFrontendHandler_SubpathWithDirectory(t *testing.T) {
	// /dashboard/terminal should resolve to dashboard/terminal.html
	h := newFrontendHandler(testFSWithDirs())
	req := httptest.NewRequest(http.MethodGet, "/dashboard/terminal", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "terminal")
}

func TestFrontendHandler_MissingBuild(t *testing.T) {
	// When embedded FS has no web/out directory, fs.Sub fails and placeholder is served.
	// fstest.MapFS treats any prefix as valid via Sub, so use a nil FS wrapper instead.
	h := newFrontendHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Frontend not built")
}
