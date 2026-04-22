package presets

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// embeddedFS holds every preset YAML shipped with the binary.
//
//go:embed all:data
var embeddedFS embed.FS

// index is the parsed in-memory catalog. Populated lazily by load().
//
// Presets are keyed by "category/name" because the same short name (e.g.
// "oracle-19c") is legitimately reused across categories (packages + sysctl +
// limits). Bundles reference child presets by name + category from the
// bundle spec, so the registry resolves them unambiguously.
type index struct {
	byKey   map[string]*Preset // "category/name" → preset
	byName  map[string][]*Preset // "name" → all presets with that name (across categories)
	bundles map[string]*Bundle
	meta    []PresetMeta
}

func catKey(category, name string) string { return category + "/" + name }

var (
	loadOnce sync.Once
	loaded   *index
	loadErr  error
)

// load reads every *.yaml under data/ exactly once and caches the result.
func load() (*index, error) {
	loadOnce.Do(func() {
		idx := &index{
			byKey:   map[string]*Preset{},
			byName:  map[string][]*Preset{},
			bundles: map[string]*Bundle{},
		}
		err := fs.WalkDir(embeddedFS, "data", func(p string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return werr
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") {
				return nil
			}
			b, rerr := embeddedFS.ReadFile(p)
			if rerr != nil {
				return fmt.Errorf("read %s: %w", p, rerr)
			}
			var pr Preset
			if derr := yaml.Unmarshal(b, &pr); derr != nil {
				return fmt.Errorf("parse %s: %w", p, derr)
			}
			if pr.Metadata.Name == "" {
				return fmt.Errorf("%s: metadata.name is required", p)
			}
			// Infer category from parent dir if not set (defensive).
			if pr.Metadata.Category == "" {
				pr.Metadata.Category = path.Base(path.Dir(p))
			}
			// Default tier.
			if pr.Metadata.Tier == "" {
				pr.Metadata.Tier = TierCommunity
			}
			// Filename must match metadata.name (bisect-friendly).
			fn := strings.TrimSuffix(path.Base(p), path.Ext(p))
			if fn != pr.Metadata.Name {
				return fmt.Errorf("%s: filename %q != metadata.name %q", p, fn, pr.Metadata.Name)
			}
			key := catKey(pr.Metadata.Category, pr.Metadata.Name)
			if _, exists := idx.byKey[key]; exists {
				return fmt.Errorf("duplicate preset %q (file %s)", key, p)
			}
			idx.byKey[key] = &pr
			idx.byName[pr.Metadata.Name] = append(idx.byName[pr.Metadata.Name], &pr)
			idx.meta = append(idx.meta, pr.Metadata)
			if pr.Kind == "Bundle" {
				bundle, berr := decodeBundle(&pr)
				if berr != nil {
					return fmt.Errorf("%s: %w", p, berr)
				}
				idx.bundles[pr.Metadata.Name] = bundle
			}
			return nil
		})
		if err != nil {
			loadErr = err
			return
		}
		loaded = idx
	})
	return loaded, loadErr
}

// decodeBundle extracts the bundle child-preset names from a Preset with
// Kind == "Bundle".
func decodeBundle(pr *Preset) (*Bundle, error) {
	raw, ok := pr.RawSpec["bundle"]
	if !ok {
		return nil, fmt.Errorf("bundle %q: spec.bundle missing", pr.Metadata.Name)
	}
	// Round-trip through YAML for strict typed decoding.
	b, err := yaml.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var out Bundle
	if err := yaml.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
