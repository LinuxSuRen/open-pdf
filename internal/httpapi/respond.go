package httpapi

import (
	"archive/zip"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// serveFile streams a staged file back to the client as a download.
func serveFile(w http.ResponseWriter, path, downloadName string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}

	contentType := "application/octet-stream"
	switch strings.ToLower(filepath.Ext(downloadName)) {
	case ".pdf":
		contentType = "application/pdf"
	case ".zip":
		contentType = "application/zip"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", st.Size()))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": downloadName}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, f)
	return err
}

// zipFiles packs the given files into zipPath preserving their base names.
func zipFiles(zipPath string, files []string) error {
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	for _, f := range files {
		src, err := os.Open(f)
		if err != nil {
			return err
		}
		hdr := &zip.FileHeader{
			Name:   filepath.Base(f),
			Method: zip.Deflate,
		}
		entry, err := zw.CreateHeader(hdr)
		if err == nil {
			_, err = io.Copy(entry, src)
		}
		src.Close()
		if err != nil {
			return err
		}
	}
	return zw.Close()
}
