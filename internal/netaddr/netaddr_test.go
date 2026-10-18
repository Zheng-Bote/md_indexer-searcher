// SPDX-License-Identifier: Apache-2.0

package netaddr

import "testing"

func TestListen(t *testing.T) {
	tests := []struct {
		host, wantAddr string
		wantExposed    bool
	}{
		{"127.0.0.1", "127.0.0.1:8081", false},
		{"127.1.2.3", "127.1.2.3:8081", false},
		{"::1", "[::1]:8081", false},
		{"localhost", "localhost:8081", false},
		{"", ":8081", true},
		{"0.0.0.0", "0.0.0.0:8081", true},
		{"::", "[::]:8081", true},
		{"192.168.1.10", "192.168.1.10:8081", true},
		{"example.org", "example.org:8081", true},
	}
	for _, tt := range tests {
		addr, exposed := Listen(tt.host, "8081")
		if addr != tt.wantAddr || exposed != tt.wantExposed {
			t.Errorf("Listen(%q) = %q, %v; want %q, %v", tt.host, addr, exposed, tt.wantAddr, tt.wantExposed)
		}
	}
}
