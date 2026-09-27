package pdfops

import (
	"github.com/pdfcpu/pdfcpu/pkg/api"
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
