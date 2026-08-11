package derphttp

import (
	"net/url"
	"testing"

	"scaletail.com/tailcfg"
)

func TestRegionTransportHonorsAdvertisedHTTP(t *testing.T) {
	client := new(Client)
	httpNode := &tailcfg.DERPNode{
		HostName:         "derp.example.test",
		InsecureForTests: true,
	}
	if client.useHTTPS(httpNode) {
		t.Fatal("HTTP DERP node unexpectedly selected TLS")
	}
	if got, want := client.urlString(httpNode), "http://derp.example.test/derp"; got != want {
		t.Fatalf("DERP URL = %q, want %q", got, want)
	}

	httpsNode := &tailcfg.DERPNode{HostName: "derp.example.test"}
	if !client.useHTTPS(httpsNode) {
		t.Fatal("HTTPS DERP node unexpectedly selected plaintext HTTP")
	}
	if got, want := client.urlString(httpsNode), "https://derp.example.test/derp"; got != want {
		t.Fatalf("DERP URL = %q, want %q", got, want)
	}
}

func TestExplicitDERPURLControlsTransport(t *testing.T) {
	httpURL, err := url.Parse("http://derp.example.test:3340/derp")
	if err != nil {
		t.Fatal(err)
	}
	if (&Client{url: httpURL}).useHTTPS(nil) {
		t.Fatal("explicit HTTP DERP URL unexpectedly selected TLS")
	}

	httpsURL, err := url.Parse("https://derp.example.test/derp")
	if err != nil {
		t.Fatal(err)
	}
	if !(&Client{url: httpsURL}).useHTTPS(nil) {
		t.Fatal("explicit HTTPS DERP URL unexpectedly selected plaintext HTTP")
	}
}
