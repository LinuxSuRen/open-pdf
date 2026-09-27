package httpapi

import (
	"io/fs"
	"strings"

	"github.com/linuxsuren/open-pdf/internal/pdfops"
)

// fsWalkAssets walks the embedded asset directory and calls fn with the
// asset name relative to /assets/.
func fsWalkAssets(fn func(name string)) error {
	return fs.WalkDir(webui, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fn(strings.TrimPrefix(p, "assets/"))
		return nil
	})
}

// pageCountFor validates and counts pages of a written file.
func pageCountFor(path string) (int, error) {
	if err := pdfops.Validate(path); err != nil {
		return 0, err
	}
	return pdfops.PageCount(path)
}
