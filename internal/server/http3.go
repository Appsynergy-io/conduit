package server

import "crypto/tls"

// configureHTTP2 ensures the HTTP/2 ALPN protocol is negotiated on the TLS config.
// Go's net/http automatically supports HTTP/2 over TLS, but we explicitly set
// NextProtos to ensure proper ALPN negotiation.
func configureHTTP2(cfg *tls.Config) {
	hasH2 := false
	hasHTTP11 := false
	for _, p := range cfg.NextProtos {
		if p == "h2" {
			hasH2 = true
		}
		if p == "http/1.1" {
			hasHTTP11 = true
		}
	}
	if !hasH2 {
		cfg.NextProtos = append(cfg.NextProtos, "h2")
	}
	if !hasHTTP11 {
		cfg.NextProtos = append(cfg.NextProtos, "http/1.1")
	}
}
