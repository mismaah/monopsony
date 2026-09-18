package cosmetics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Manifest tells the client how to render an item. Exactly what is required
// depends on the slot:
//
//   - token / buildings: a builtin shape or a glTF model
//   - board: a builtin skin or a palette (table, tile, tileEdge, text, centre,
//     house, hotel, mortgageTint); a model is optional set dressing
//   - dice: a builtin skin or a palette (body, pip), optionally a model
//   - cards: a builtin or a palette (back, face)
//
// The same struct is mirrored in packages/board-assets/manifest.schema.json.
type Manifest struct {
	Builtin  string            `json:"builtin,omitempty"`
	Color    string            `json:"color,omitempty"`
	Model    *ModelRef         `json:"model,omitempty"`
	Palette  map[string]string `json:"palette,omitempty"`
	Material *Material         `json:"material,omitempty"`
	Preview  string            `json:"preview,omitempty"`
}

// ModelRef points at a glTF asset. Scale/offset/rotation let an admin fit an
// arbitrary model to the token footprint (about 0.4 units wide).
type ModelRef struct {
	URL      string     `json:"url,omitempty"`
	Builtin  string     `json:"builtin,omitempty"` // model bundled with the client
	Scale    float64    `json:"scale,omitempty"`
	Offset   [3]float64 `json:"offset,omitempty"`
	Rotation [3]float64 `json:"rotation,omitempty"` // radians
	// Draco marks models compressed with the Draco extension so the client
	// loads the decoder.
	Draco bool `json:"draco,omitempty"`
}

// Material overrides the PBR parameters of a builtin shape or model.
type Material struct {
	Metalness *float64 `json:"metalness,omitempty"`
	Roughness *float64 `json:"roughness,omitempty"`
	Emissive  string   `json:"emissive,omitempty"`
}

// UploadPrefix is where uploaded assets are served when no store is configured.
const UploadPrefix = "/media/"

func uploadPrefix(assets *AssetStore) string {
	if assets != nil {
		return assets.BaseURL + "/"
	}
	return UploadPrefix
}

var colorRe = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

var paletteKeys = map[string][]string{
	"board": {"table", "tile", "tileEdge", "text", "centre", "house", "hotel", "mortgageTint"},
	"dice":  {"body", "pip"},
	"cards": {"back", "face"},
}

// ParseManifest decodes strictly (unknown fields are errors) so typos in
// the admin panel surface immediately.
func ParseManifest(raw json.RawMessage) (*Manifest, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return &Manifest{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	return &m, nil
}

// ValidateManifest checks a manifest for a slot. assets may be nil, in which
// case local asset URLs are only checked for shape.
func ValidateManifest(slot string, raw json.RawMessage, assets *AssetStore) error {
	m, err := ParseManifest(raw)
	if err != nil {
		return err
	}
	checkColor := func(field, v string) error {
		if v != "" && !colorRe.MatchString(v) {
			return fmt.Errorf("manifest: %s must be a #rgb or #rrggbb colour", field)
		}
		return nil
	}
	if err := checkColor("color", m.Color); err != nil {
		return err
	}
	if m.Material != nil {
		if err := checkColor("material.emissive", m.Material.Emissive); err != nil {
			return err
		}
		for name, v := range map[string]*float64{"metalness": m.Material.Metalness, "roughness": m.Material.Roughness} {
			if v != nil && (*v < 0 || *v > 1) {
				return fmt.Errorf("manifest: material.%s must be 0-1", name)
			}
		}
	}
	checkURL := func(field, u string) error {
		switch {
		case u == "":
			return nil
		case strings.HasPrefix(u, "https://"), strings.HasPrefix(u, "http://"):
			return nil
		case strings.HasPrefix(u, uploadPrefix(assets)):
			if assets != nil && !assets.Exists(u) {
				return fmt.Errorf("manifest: %s points at an asset that is not uploaded (%s)", field, u)
			}
			return nil
		}
		return fmt.Errorf("manifest: %s must be an https:// URL or an uploaded %s path", field, uploadPrefix(assets))
	}
	if err := checkURL("preview", m.Preview); err != nil {
		return err
	}
	if m.Model != nil {
		if m.Model.URL == "" && m.Model.Builtin == "" {
			return errors.New("manifest: model needs a url or a builtin name")
		}
		if err := checkURL("model.url", m.Model.URL); err != nil {
			return err
		}
		if m.Model.Scale < 0 || m.Model.Scale > 100 {
			return errors.New("manifest: model.scale must be 0-100")
		}
	}
	if keys, ok := paletteKeys[slot]; ok && len(m.Palette) > 0 {
		allowed := map[string]bool{}
		for _, k := range keys {
			allowed[k] = true
		}
		for k, v := range m.Palette {
			if !allowed[k] {
				return fmt.Errorf("manifest: palette key %q is not used by the %s slot (want %s)", k, slot, strings.Join(keys, ", "))
			}
			if err := checkColor("palette."+k, v); err != nil {
				return err
			}
		}
	}
	// Something must actually describe the look.
	switch slot {
	case "token", "buildings":
		if m.Builtin == "" && m.Model == nil {
			return fmt.Errorf("manifest: a %s item needs a builtin shape or a model", slot)
		}
	case "board", "dice", "cards":
		if m.Builtin == "" && len(m.Palette) == 0 && m.Model == nil {
			return fmt.Errorf("manifest: a %s item needs a builtin skin or a palette", slot)
		}
	}
	return nil
}
