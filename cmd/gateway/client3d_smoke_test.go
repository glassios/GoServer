package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestClient3dServing verifies the embedded WebGL client (built into webclient/dist)
// is mounted and served at /client3d/, including its prefab JSON assets. This is the
// Slice 0 acceptance check: build/serve pipeline works without disturbing port 8080.
func TestClient3dServing(t *testing.T) {
	sub, err := fs.Sub(client3dFS, "webclient/dist")
	if err != nil {
		t.Fatalf("fs.Sub failed (did you run `npm run build` in cmd/gateway/webclient?): %v", err)
	}
	srv := httptest.NewServer(http.StripPrefix("/client3d/", http.FileServer(http.FS(sub))))
	defer srv.Close()

	cases := []struct {
		path     string
		wantBody string
	}{
		{"/client3d/", "<div id=\"root\">"},
		{"/client3d/index.html", "/client3d/assets/"},
		{"/client3d/prefubs/Ships/default-fighter.json", "\"id\": \"default-fighter\""},
	}
	for _, c := range cases {
		resp, err := http.Get(srv.URL + c.path)
		if err != nil {
			t.Fatalf("GET %s: %v", c.path, err)
		}
		body := make([]byte, 4096)
		n, _ := resp.Body.Read(body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", c.path, resp.StatusCode)
			continue
		}
		if !strings.Contains(string(body[:n]), c.wantBody) {
			t.Errorf("GET %s: body missing %q", c.path, c.wantBody)
		}
	}
}
