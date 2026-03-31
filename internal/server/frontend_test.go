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
