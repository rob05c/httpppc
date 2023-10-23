package httpppc

import (
	"net"
	"net/http"
	"testing"
)

func TestClient(t *testing.T) {
	tr := New(net.ParseIP("192.0.2.42"), 31337, nil)
	cl := &http.Client{Transport: tr}
	resp, err := cl.Get("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	t.Log("ran")
}
