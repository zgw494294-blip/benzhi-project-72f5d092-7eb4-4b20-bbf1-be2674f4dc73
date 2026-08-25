package main

import "testing"

func TestResolveAddress(t *testing.T) {
	tests := []struct {
		name, flag, port, expected string
		invalid                    bool
	}{
		{name: "default", expected: "127.0.0.1:19081"},
		{name: "environment", port: "19123", expected: "127.0.0.1:19123"},
		{name: "flag priority", flag: "127.0.0.1:19234", port: "19123", expected: "127.0.0.1:19234"},
		{name: "wildcard", flag: "0.0.0.0:19234", invalid: true},
		{name: "bare port", flag: "19234", invalid: true},
		{name: "common port", flag: "127.0.0.1:8080", invalid: true},
		{name: "bad environment", port: "not-a-port", invalid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := resolveAddress(test.flag, test.port)
			if test.invalid && err == nil {
				t.Fatalf("期望拒绝，得到 %s", actual)
			}
			if !test.invalid && (err != nil || actual != test.expected) {
				t.Fatalf("得到 %q, %v，期望 %q", actual, err, test.expected)
			}
		})
	}
}
