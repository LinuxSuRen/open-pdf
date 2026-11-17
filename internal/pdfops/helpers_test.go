package pdfops

import (
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// hasWatermarks reports whether the PDF carries any watermark.
func hasWatermarks(path string) (bool, error) {
	return api.HasWatermarksFile(path, newConf())
}

// validateWithPassword validates an encrypted PDF after authenticating.
func validateWithPassword(path, pw string) error {
	conf := newConf()
	conf.UserPW = pw
	return api.ValidateFile(path, conf)
}

// pageDimsOf returns the page dimensions of the PDF at path.
func pageDimsOf(t *testing.T, path string) []types.Dim {
	t.Helper()
	dims, err := api.PageDimsFile(path)
	if err != nil {
		t.Fatalf("page dims of %s: %v", path, err)
	}
	return dims
}
