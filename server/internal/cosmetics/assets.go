package cosmetics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AssetStore keeps admin-uploaded files (glTF models, textures, preview
// images) on local disk under content-addressed names and serves them with
// immutable caching. A CDN or object store can front the same URL space in
// production by syncing Dir.
type AssetStore struct {
	Dir string
	// BaseURL is the public path prefix the files are served under.
	BaseURL string
}

// Asset describes one stored file.
type Asset struct {
	ID          string    `json:"id"` // <hash>.<ext>, also the file name
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	ContentType string    `json:"contentType"`
	URL         string    `json:"url"`
	UploadedAt  time.Time `json:"uploadedAt"`
}

// MaxAssetSize bounds a single upload.
const MaxAssetSize = 25 << 20

var assetTypes = map[string]string{
	".glb":  "model/gltf-binary",
	".gltf": "model/gltf+json",
	".bin":  "application/octet-stream",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".ktx2": "image/ktx2",
	".hdr":  "image/vnd.radiance",
}

// ErrUnsupportedAsset is returned for file types the client cannot use.
var ErrUnsupportedAsset = errors.New("unsupported file type (use .glb, .gltf, .bin, .png, .jpg, .webp, .ktx2 or .hdr)")

// NewAssetStore creates dir if needed.
func NewAssetStore(dir, baseURL string) (*AssetStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if baseURL == "" {
		baseURL = "/media"
	}
	return &AssetStore{Dir: dir, BaseURL: strings.TrimRight(baseURL, "/")}, nil
}

// Put stores the file, naming it by content hash so re-uploads dedupe and
// URLs can be cached forever.
func (s *AssetStore) Put(name string, r io.Reader) (*Asset, error) {
	ext := strings.ToLower(filepath.Ext(name))
	ctype, ok := assetTypes[ext]
	if !ok {
		return nil, ErrUnsupportedAsset
	}
	tmp, err := os.CreateTemp(s.Dir, "upload-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, MaxAssetSize+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	if size > MaxAssetSize {
		return nil, fmt.Errorf("file exceeds %d MB", MaxAssetSize>>20)
	}
	if size == 0 {
		return nil, errors.New("empty file")
	}
	id := hex.EncodeToString(h.Sum(nil))[:20] + ext
	final := filepath.Join(s.Dir, id)
	if _, err := os.Stat(final); err != nil {
		if err := os.Rename(tmp.Name(), final); err != nil {
			return nil, err
		}
	}
	a := &Asset{ID: id, Name: filepath.Base(name), Size: size, ContentType: ctype, URL: s.URL(id), UploadedAt: time.Now()}
	meta, _ := json.Marshal(a)
	if err := os.WriteFile(final+".json", meta, 0o644); err != nil {
		return nil, err
	}
	return a, nil
}

// URL returns the public URL for an asset id.
func (s *AssetStore) URL(id string) string { return s.BaseURL + "/" + id }

// Exists reports whether a URL points at a stored asset.
func (s *AssetStore) Exists(url string) bool {
	if !strings.HasPrefix(url, s.BaseURL+"/") {
		return false
	}
	id := strings.TrimPrefix(url, s.BaseURL+"/")
	if id == "" || strings.ContainsAny(id, "/\\") {
		return false
	}
	_, err := os.Stat(filepath.Join(s.Dir, id))
	return err == nil
}

// List returns every stored asset, newest first.
func (s *AssetStore) List() ([]Asset, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	var out []Asset
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			continue
		}
		var a Asset
		if json.Unmarshal(b, &a) == nil && a.ID != "" {
			a.URL = s.URL(a.ID)
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UploadedAt.After(out[j].UploadedAt) })
	if out == nil {
		out = []Asset{}
	}
	return out, nil
}

// Delete removes an asset and its metadata.
func (s *AssetStore) Delete(id string) error {
	if id == "" || strings.ContainsAny(id, "/\\") || strings.HasPrefix(id, ".") {
		return errors.New("invalid asset id")
	}
	if err := os.Remove(filepath.Join(s.Dir, id)); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(s.Dir, id+".json"))
	return nil
}

// Handler serves GET <BaseURL>/{id}. Names are content hashes, so the
// response is cacheable forever.
func (s *AssetStore) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := path.Base(r.URL.Path)
		ext := strings.ToLower(filepath.Ext(id))
		ctype, ok := assetTypes[ext]
		if !ok || strings.HasPrefix(id, ".") || strings.HasSuffix(id, ".json") {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(s.Dir, id))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		http.ServeContent(w, r, id, st.ModTime(), f)
	})
}
