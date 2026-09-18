package cosmetics

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssetStoreRoundTrip(t *testing.T) {
	st, err := NewAssetStore(t.TempDir(), "/media")
	if err != nil {
		t.Fatal(err)
	}
	a, err := st.Put("Top Hat.glb", strings.NewReader("glTF-binary-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(a.ID, ".glb") || a.URL != "/media/"+a.ID || a.ContentType != "model/gltf-binary" || a.Size != 17 {
		t.Fatalf("asset: %+v", a)
	}
	// Same bytes -> same id (content addressed).
	b, _ := st.Put("copy.glb", strings.NewReader("glTF-binary-bytes"))
	if b.ID != a.ID {
		t.Fatalf("dedupe: %s vs %s", a.ID, b.ID)
	}
	if _, err := st.Put("virus.exe", strings.NewReader("x")); err == nil {
		t.Fatal("unsupported extension must be rejected")
	}
	if _, err := st.Put("empty.png", strings.NewReader("")); err == nil {
		t.Fatal("empty file must be rejected")
	}
	list, _ := st.List()
	if len(list) != 1 || list[0].Name != "copy.glb" {
		t.Fatalf("list: %+v", list)
	}
	if !st.Exists(a.URL) || st.Exists("/media/../secret") || st.Exists("/media/nope.glb") {
		t.Fatal("exists")
	}

	rec := httptest.NewRecorder()
	st.Handler().ServeHTTP(rec, httptest.NewRequest("GET", a.URL, nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "model/gltf-binary" || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("serve: %d %v", rec.Code, rec.Header())
	}
	rec = httptest.NewRecorder()
	st.Handler().ServeHTTP(rec, httptest.NewRequest("GET", a.URL+".json", nil))
	if rec.Code != 404 {
		t.Fatal("metadata sidecars are not public")
	}
	if err := st.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := st.List(); len(list) != 0 {
		t.Fatal("delete")
	}
	if err := st.Delete("../../etc/passwd"); err == nil {
		t.Fatal("path traversal")
	}
}

func TestValidateManifest(t *testing.T) {
	st, _ := NewAssetStore(t.TempDir(), "/media")
	a, _ := st.Put("hat.glb", strings.NewReader("bytes"))
	cases := []struct {
		slot     string
		manifest string
		wantErr  string
	}{
		{"token", `{"builtin":"token.pawn"}`, ""},
		{"token", `{"builtin":"token.pawn","color":"#0ff"}`, ""},
		{"token", `{"model":{"url":"` + a.URL + `","scale":1.2}}`, ""},
		{"token", `{"model":{"builtin":"tophat"}}`, ""},
		{"token", `{"model":{"url":"https://cdn.example.com/x.glb","draco":true}}`, ""},
		{"token", `{}`, "needs a builtin shape or a model"},
		{"token", `{"builtin":"token.pawn","colour":"#fff"}`, "unknown field"},
		{"token", `{"builtin":"token.pawn","color":"red"}`, "colour"},
		{"token", `{"model":{"url":"/media/missing.glb"}}`, "not uploaded"},
		{"token", `{"model":{"url":"ftp://x/y.glb"}}`, "https:// URL"},
		{"token", `{"model":{"scale":2}}`, "needs a url or a builtin"},
		{"token", `{"builtin":"x","material":{"metalness":1.5}}`, "material.metalness"},
		{"board", `{"palette":{"table":"#112233","tile":"#ffffff"}}`, ""},
		{"board", `{"palette":{"body":"#112233"}}`, `palette key "body"`},
		{"board", `{}`, "needs a builtin skin or a palette"},
		{"dice", `{"palette":{"body":"#000000","pip":"#ffffff"}}`, ""},
		{"cards", `{"builtin":"cards.classic"}`, ""},
	}
	for _, c := range cases {
		err := ValidateManifest(c.slot, json.RawMessage(c.manifest), st)
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s %s: unexpected error %v", c.slot, c.manifest, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s %s: want error containing %q, got %v", c.slot, c.manifest, c.wantErr, err)
		}
	}
}
