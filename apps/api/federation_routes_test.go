package main

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestFederationDialerRejectsNonPublicAddresses(t *testing.T) {
	for _, address := range []string{"127.0.0.1:443", "10.1.2.3:443", "169.254.1.1:443", "[::1]:443"} {
		_, err := safeFederationDialer(false)(context.Background(), "tcp", address)
		if err == nil || !strings.Contains(err.Error(), "not public") {
			t.Fatalf("dial %s = %v", address, err)
		}
	}
}
func TestPublicFederationIPClassification(t *testing.T) {
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicFederationIP(net.ParseIP(raw)) {
			t.Fatalf("public IP %s rejected", raw)
		}
	}
	for _, raw := range []string{"0.0.0.0", "127.0.0.1", "10.0.0.1", "100.64.0.1", "172.16.0.1", "192.0.2.1", "192.31.196.1", "192.52.193.1", "192.168.0.1", "192.175.48.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "169.254.1.1", "224.0.0.1", "240.0.0.1", "::1", "2001:db8::1", "fc00::1", "fe80::1"} {
		if publicFederationIP(net.ParseIP(raw)) {
			t.Fatalf("non-public IP %s accepted", raw)
		}
	}
}

func TestFederationDialerRejectsSpecialPurposeAddresses(t *testing.T) {
	for _, address := range []string{"100.64.0.1:443", "192.31.196.1:443", "192.52.193.1:443", "192.175.48.1:443", "198.18.0.1:443", "[2001:db8::1]:443"} {
		_, err := safeFederationDialer(false)(context.Background(), "tcp", address)
		if err == nil || !strings.Contains(err.Error(), "not public") {
			t.Fatalf("dial %s = %v", address, err)
		}
	}
}
