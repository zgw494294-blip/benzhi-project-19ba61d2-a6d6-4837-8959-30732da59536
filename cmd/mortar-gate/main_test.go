package main

import "testing"

func TestValidateAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:19081", "localhost:19082", "[::1]:19083"} {
		if err := validateAddress(address); err != nil {
			t.Fatalf("%s: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:19081", "127.0.0.1:80", "example.com:19081"} {
		if err := validateAddress(address); err == nil {
			t.Fatalf("应拒绝 %s", address)
		}
	}
}
