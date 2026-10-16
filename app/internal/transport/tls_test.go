package transport

import "testing"

func TestPlaintextRequiresExplicitLocalOptIn(t *testing.T) {
	if _, err := ClientCredentials("", "", false); err == nil {
		t.Fatal("client silently used plaintext")
	}
	if _, err := ServerCredentials("", "", false); err == nil {
		t.Fatal("server silently used plaintext")
	}
	if _, err := ClientCredentials("", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := ServerCredentials("", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := ClientCredentials("ca.crt", "catalog", true); err == nil {
		t.Fatal("mixed insecure/TLS config must fail")
	}
}
