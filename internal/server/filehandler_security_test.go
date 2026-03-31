package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- Server-Side Path Traversal Tests (OWASP A03, ASVS V12) ---

func TestSanitizeFilePath_TraversalAttacks(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple traversal", "/var/../etc/passwd"},
		{"double traversal", "/var/../../etc/shadow"},
		{"relative traversal", "../../../etc/passwd"},
		{"dot-dot chain", "/a/b/c/../../../../etc/passwd"},
		{"mixed slashes", "/var/log//..//../etc/passwd"},
		{"empty to root", ""},
		{"current dir", "."},
		{"parent dir", ".."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFilePath(tt.input)
			assert.True(t, len(result) > 0, "result must not be empty")
			assert.Equal(t, byte('/'), result[0], "result must be absolute: %s", result)
			assert.NotContains(t, result, "..", "result must not contain ..: %s", result)
		})
	}
}

func TestSanitizeFilePath_PreservesValidPaths(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/var/log/syslog", "/var/log/syslog"},
		{"/home/user/file.txt", "/home/user/file.txt"},
		{"/tmp/upload.bin", "/tmp/upload.bin"},
		{"/", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeFilePath(tt.input))
		})
	}
}

// --- Resize Bounds Tests (OWASP API4) ---

func TestParseBrowserResize_MaxBounds(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantNil bool
	}{
		{"normal", `{"type":"resize","cols":120,"rows":40}`, false},
		{"max allowed", `{"type":"resize","cols":500,"rows":500}`, false},
		{"cols exceeds max", `{"type":"resize","cols":501,"rows":40}`, true},
		{"rows exceeds max", `{"type":"resize","cols":120,"rows":501}`, true},
		{"both exceed max", `{"type":"resize","cols":1000,"rows":1000}`, true},
		{"extreme cols", `{"type":"resize","cols":99999,"rows":40}`, true},
		{"extreme rows", `{"type":"resize","cols":120,"rows":99999}`, true},
		{"zero cols", `{"type":"resize","cols":0,"rows":40}`, true},
		{"zero rows", `{"type":"resize","cols":120,"rows":0}`, true},
		{"negative cols", `{"type":"resize","cols":-1,"rows":40}`, true},
		{"negative rows", `{"type":"resize","cols":120,"rows":-1}`, true},
		{"minimum valid", `{"type":"resize","cols":1,"rows":1}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBrowserResize([]byte(tt.json), 1)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
			}
		})
	}
}

func TestParseBrowserResize_MalformedInput(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"truncated JSON", `{"type":"resize","cols":10`},
		{"extra fields", `{"type":"resize","cols":10,"rows":10,"__proto__":{"admin":true}}`},
		{"string cols", `{"type":"resize","cols":"10","rows":"10"}`},
		{"null values", `{"type":"resize","cols":null,"rows":null}`},
		{"array values", `{"type":"resize","cols":[10],"rows":[10]}`},
		{"boolean values", `{"type":"resize","cols":true,"rows":true}`},
		{"wrong type", `{"type":"exec","cols":10,"rows":10}`},
		{"empty type", `{"type":"","cols":10,"rows":10}`},
		{"no type", `{"cols":10,"rows":10}`},
		{"binary data", "\x00\x01\x02\x03"},
		{"huge payload", `{"type":"resize","cols":10,"rows":10,"padding":"` + string(make([]byte, 10000)) + `"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Must not panic on any input
			result := parseBrowserResize([]byte(tt.data), 1)
			_ = result
		})
	}
}
