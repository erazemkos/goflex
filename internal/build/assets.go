package build

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type AssetOptions struct {
	Dir string
	Out string
}

type AssetManifest map[string]string

func CopyAssets(opts AssetOptions) (AssetManifest, error) {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	if opts.Out == "" {
		opts.Out = filepath.Join(opts.Dir, "dist")
	}
	src := filepath.Join(opts.Dir, "assets")
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return AssetManifest{}, nil
	}
	manifest := AssetManifest{}
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hashed := hashedAssetName(rel, b)
		destRel := filepath.Join("assets", hashed)
		dest := filepath.Join(opts.Out, destRel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, b, 0o644); err != nil {
			return err
		}
		manifest[filepath.ToSlash(filepath.Join("assets", rel))] = filepath.ToSlash(destRel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func (m AssetManifest) SortedKeys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func hashedAssetName(rel string, b []byte) string {
	dir := filepath.Dir(rel)
	base := filepath.Base(rel)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	sum := sha256.Sum256(b)
	name := stem + "-" + hex.EncodeToString(sum[:])[:8] + ext
	if dir == "." || dir == "" {
		return name
	}
	return filepath.Join(dir, name)
}
