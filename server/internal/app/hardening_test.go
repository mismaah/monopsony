package app

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"monopsony/server/internal/cosmetics"
	"monopsony/server/internal/httpapi"
	"monopsony/server/internal/ratelimit"
	"monopsony/server/internal/store"
)

func TestRateLimitsAndMetrics(t *testing.T) {
	a, err := New(context.Background(), store.NewMem(), Config{
		JWTSecret: "test", Origins: []string{"http://example.test"},
		RateLimits: RateLimits{Auth: ratelimit.Rate{PerMinute: 60, Burst: 2}, API: ratelimit.Rate{PerMinute: 600, Burst: 100}},
		Ads:        httpapi.AdsConfig{Provider: "adsense", Client: "ca-pub-1", Slot: "42"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()

	c := &client{t: t, base: srv.URL}
	c.call("POST", "/api/auth/guest", map[string]string{"name": "one"}, 200)
	c.call("POST", "/api/auth/guest", map[string]string{"name": "two"}, 200)
	res := c.call("POST", "/api/auth/guest", map[string]string{"name": "three"}, 429)
	if res["error"].(map[string]any)["code"] != "rate_limited" {
		t.Fatalf("429 body: %v", res)
	}
	// Other routes are on a separate budget.
	cfg := c.call("GET", "/api/config", nil, 200)
	ads := cfg["ads"].(map[string]any)
	if ads["provider"] != "adsense" || ads["client"] != "ca-pub-1" || ads["slot"] != "42" {
		t.Fatalf("config: %v", cfg)
	}

	rec := httptest.NewRecorder()
	a.Metrics.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{
		`monopsony_http_requests_total{method="POST",route="POST /api/auth/guest",status="200"} 2`,
		`monopsony_rate_limited_total{scope="auth"} 1`,
		`monopsony_rooms_lobby 0`,
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
}

func TestAdsConfigFallsBackToHouse(t *testing.T) {
	srv, _ := newServer(t)
	c := &client{t: t, base: srv.URL}
	cfg := c.call("GET", "/api/config", nil, 200)
	if cfg["ads"].(map[string]any)["provider"] != "house" {
		t.Fatalf("unset ads provider should fall back to house: %v", cfg)
	}
}

func TestManifestsAndAssetUpload(t *testing.T) {
	assets, err := cosmetics.NewAssetStore(t.TempDir(), "/media")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), store.NewMem(), Config{JWTSecret: "test", Origins: []string{"http://example.test"}, AdminEmails: []string{"root@example.com"}, Assets: assets})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()

	player := guest(t, srv.URL, "Player")
	m := player.call("GET", "/api/cosmetics/manifests", nil, 200)["items"].(map[string]any)
	if m["token.tophat"].(map[string]any)["model"].(map[string]any)["builtin"] != "tophat" {
		t.Fatalf("manifests: %v", m["token.tophat"])
	}

	admin := &client{t: t, base: srv.URL}
	out := admin.call("POST", "/api/auth/register", map[string]string{"email": "root@example.com", "password": "hunter22!", "name": "Root"}, 200)
	admin.token = out["accessToken"].(string)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "crown.glb")
	_, _ = io.WriteString(fw, "fake-glb")
	_ = mw.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/admin/api/assets", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+admin.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 201 {
		t.Fatalf("upload status %d", res.StatusCode)
	}
	res.Body.Close()
	list := admin.call("GET", "/admin/api/assets", nil, 200)["assets"].([]any)
	if len(list) != 1 {
		t.Fatalf("assets: %v", list)
	}
	url := list[0].(map[string]any)["url"].(string)

	// The uploaded file is public and cacheable.
	get, _ := http.Get(srv.URL + url)
	if get.StatusCode != 200 || get.Header.Get("Content-Type") != "model/gltf-binary" {
		t.Fatalf("serve asset: %d %s", get.StatusCode, get.Header.Get("Content-Type"))
	}
	get.Body.Close()

	// Manifests referencing it validate; dangling ones do not.
	admin.call("PUT", "/admin/api/cosmetics/token.crown", map[string]any{"slot": "token", "name": "Crown", "manifest": map[string]any{"model": map[string]any{"url": url, "scale": 0.8}}, "enabled": true}, 200)
	admin.call("PUT", "/admin/api/cosmetics/token.ghost", map[string]any{"slot": "token", "name": "Ghost", "manifest": map[string]any{"model": map[string]any{"url": "/media/nope.glb"}}, "enabled": true}, 400)
	v := admin.call("POST", "/admin/api/cosmetics/validate", map[string]any{"slot": "board", "manifest": map[string]any{"palette": map[string]any{"table": "#123456"}}}, 200)
	if v["ok"] != true {
		t.Fatalf("validate: %v", v)
	}
	// Players now see the new item's manifest.
	m = player.call("GET", "/api/cosmetics/manifests", nil, 200)["items"].(map[string]any)
	if _, ok := m["token.crown"]; !ok {
		t.Fatal("new manifest not exposed")
	}
	admin.call("DELETE", "/admin/api/assets/"+list[0].(map[string]any)["id"].(string), nil, 204)
}
