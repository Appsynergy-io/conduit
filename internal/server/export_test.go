package server

// SetSetupTokenForTesting exposes the package-level setupToken for external tests.
// Only compiled during test runs (_test.go suffix).
func SetSetupTokenForTesting(token string) {
	setupToken = token
}
