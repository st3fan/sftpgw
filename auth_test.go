package main

import (
	"net"
	"testing"
)

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name     string
		addr     net.Addr
		expected string
	}{
		{
			name:     "TCP address with IPv4",
			addr:     &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345},
			expected: "192.168.1.1",
		},
		{
			name:     "TCP address with IPv6", 
			addr:     &net.TCPAddr{IP: net.ParseIP("::1"), Port: 12345},
			expected: "::1",
		},
		{
			name:     "TCP address with localhost",
			addr:     &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22},
			expected: "127.0.0.1",
		},
		{
			name:     "non-TCP address fallback",
			addr:     &testAddr{addr: "unix:/tmp/socket"},
			expected: "unix:/tmp/socket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getClientIP(tt.addr)
			if result != tt.expected {
				t.Errorf("getClientIP() = %v, want %v", result, tt.expected)
			}
		})
	}
}

type testAddr struct {
	addr string
}

func (t *testAddr) Network() string {
	return "test"
}

func (t *testAddr) String() string {
	return t.addr
}