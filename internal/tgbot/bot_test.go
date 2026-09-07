package tgbot

import (
	"net/http"
	"testing"
)

func TestNewHTTPClientProxy(t *testing.T) {
	client, err := newHTTPClient("")
	if err != nil || client != http.DefaultClient {
		t.Fatalf("no proxy must give the default client: %v", err)
	}
	client, err = newHTTPClient("socks5://user:pass@127.0.0.1:1080")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.telegram.org/bot/x", nil)
	u, err := client.Transport.(*http.Transport).Proxy(req)
	if err != nil || u == nil || u.Scheme != "socks5" || u.Host != "127.0.0.1:1080" {
		t.Fatalf("proxy not applied: %v %v", u, err)
	}
	if _, err := newHTTPClient("://bad"); err == nil {
		t.Fatal("bad url must fail")
	}
}
