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
	if !publicFederationIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public IP rejected")
	}
	for _, raw := range []string{"0.0.0.0", "127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.1.1", "224.0.0.1"} {
		if publicFederationIP(net.ParseIP(raw)) {
			t.Fatalf("non-public IP %s accepted", raw)
		}
	}
}
