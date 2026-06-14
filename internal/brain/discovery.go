package brain

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoveredModel is a gguf file found under the models root.
type DiscoveredModel struct {
	Name string `json:"name"` // base filename
	Path string `json:"path"` // absolute path
	Size int64  `json:"size"` // bytes
}

// DiscoverModels walks root and returns every *.gguf file (case-insensitive),
// sorted by path. A missing/empty root yields an empty list and no error —
// discovery is best-effort and never fails the caller.
func DiscoverModels(root string) ([]DiscoveredModel, error) {
	out := []DiscoveredModel{}
	root = strings.TrimSpace(root)
	if root == "" {
		return out, nil
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // skip unreadable entries; never abort the walk
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".gguf") {
			return nil
		}
		var size int64
		if info, ierr := d.Info(); ierr == nil {
			size = info.Size()
		}
		out = append(out, DiscoveredModel{Name: d.Name(), Path: path, Size: size})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
