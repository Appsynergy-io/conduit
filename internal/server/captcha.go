package server

import (
	"net/http"
	"strings"

	"github.com/appsynergy-io/conduit/internal/apierror"
)

// handleCaptchaChallenge issues a proof-of-work CAPTCHA challenge bound to
// the requested endpoint path.
// GET /api/v1/auth/captcha/challenge?endpoint=/auth/recovery/verify
//
// Public endpoint (no auth — CAPTCHAs gate public flows).
// NIST SC-5: Denial-of-service protection.
// OWASP A04, API4, API6: Bot / business-flow abuse mitigation.
func (s *Server) handleCaptchaChallenge(w http.ResponseWriter, r *http.Request) {
	endpoint := r.URL.Query().Get("endpoint")
	if endpoint == "" {
		apierror.BadRequest(w, r, "endpoint query parameter is required.", nil)
		return
	}
	// Only allow challenges for endpoints that actually use CAPTCHAs.
	// Prevents attackers using our endpoint to forge challenges for other paths.
	if !isCaptchaGatedEndpoint(endpoint) {
		apierror.BadRequest(w, r, "endpoint is not captcha-gated.", nil)
		return
	}
	challenge, err := s.captchaVerifier.Issue(endpoint)
	if err != nil {
		apierror.BadRequest(w, r, "invalid endpoint.", err)
		return
	}
	writeJSON(w, http.StatusOK, challenge)
}

// isCaptchaGatedEndpoint returns true if the given path is an endpoint that
// requires a PoW CAPTCHA. Allowlist prevents attackers from requesting
// challenges bound to arbitrary (possibly non-existent) endpoints.
func isCaptchaGatedEndpoint(path string) bool {
	// Normalize: strip any trailing slash
	path = strings.TrimRight(path, "/")
	switch path {
	case "/auth/recovery/verify":
		return true
	}
	return false
}
