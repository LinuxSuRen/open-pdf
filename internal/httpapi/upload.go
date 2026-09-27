package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/linuxsuren/open-pdf/internal/pdfops"
)

// uploadedFile is one staged file part of a request.
type uploadedFile struct {
	// Field is the multipart form field name.
	Field string
	// Name is the sanitized original file name.
	Name string
	// Path is the on-disk staging path.
	Path string
}

// uploadError carries an HTTP status for multipart parsing failures.
type uploadError struct {
	status int
	code   string
	err    error
}

func (e *uploadError) Error() string { return e.err.Error() }
func (e *uploadError) Unwrap() error { return e.err }

func uploadErr(status int, code, msg string) error {
	return &uploadError{status: status, code: code, err: errors.New(msg)}
}

// parseUpload streams the multipart body into dir and returns the file
// parts in arrival order plus the non-file form values. The request body
// must already be wrapped in http.MaxBytesReader.
func (s *Server) parseUpload(r *http.Request, dir string) ([]uploadedFile, url.Values, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, nil, &uploadError{
			status: http.StatusBadRequest, code: "bad_request",
			err: fmt.Errorf("multipart/form-data body required: %w", err),
		}
	}

	var files []uploadedFile
	values := url.Values{}

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				return nil, nil, &uploadError{
					status: http.StatusRequestEntityTooLarge, code: "payload_too_large",
					err: fmt.Errorf("request exceeds %d MB upload limit", s.cfg.MaxUploadBytes>>20),
				}
			}
			return nil, nil, &uploadError{
				status: http.StatusBadRequest, code: "bad_multipart",
				err: fmt.Errorf("reading multipart body: %w", err),
			}
		}

		field := part.FormName()
		if part.FileName() == "" {
			// Plain form value.
			b, err := io.ReadAll(io.LimitReader(part, 1<<20))
			if err != nil {
				return nil, nil, &uploadError{
					status: http.StatusBadRequest, code: "bad_multipart",
					err: fmt.Errorf("reading form field %q: %w", field, err),
				}
			}
			values.Add(field, strings.TrimSpace(string(b)))
			continue
		}

		name := safeBaseName(part.FileName())
		if name == "" {
			return nil, nil, uploadErr(http.StatusBadRequest, "bad_filename",
				"file part has an unusable name")
		}
		if !pdfops.IsPDFFile(name) && !pdfops.IsImageFile(name) {
			return nil, nil, &uploadError{
				status: http.StatusUnsupportedMediaType, code: "unsupported_file_type",
				err: fmt.Errorf("file %q: unsupported type (supported: pdf, png, jpg, jpeg, webp, tif, tiff)",
					name),
			}
		}
		if len(files) >= s.cfg.MaxFiles {
			return nil, nil, &uploadError{
				status: http.StatusRequestEntityTooLarge, code: "too_many_files",
				err: fmt.Errorf("more than %d files in one request", s.cfg.MaxFiles),
			}
		}

		staged := filepath.Join(dir, fmt.Sprintf("%03d-%s", len(files), name))
		if err := copyTo(staged, part); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				return nil, nil, &uploadError{
					status: http.StatusRequestEntityTooLarge, code: "payload_too_large",
					err: fmt.Errorf("request exceeds %d MB upload limit", s.cfg.MaxUploadBytes>>20),
				}
			}
			return nil, nil, &uploadError{
				status: http.StatusBadRequest, code: "bad_multipart",
				err: fmt.Errorf("staging file %q: %w", name, err),
			}
		}
		files = append(files, uploadedFile{Field: field, Name: name, Path: staged})
	}

	return files, values, nil
}

func copyTo(path string, src io.Reader) error {
	dst, err := os.Create(path)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	return err
}

// withRequestScope creates the per-request scratch directory, applies the
// upload size limit and hands the parsed upload to fn. The scratch directory
// is always removed afterwards.
func (s *Server) withRequestScope(w http.ResponseWriter, r *http.Request, fn func(dir string, files []uploadedFile, values url.Values)) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)

	dir, err := s.newRequestDir()
	if err != nil {
		s.log.Error("create request dir", "err", err)
		writeError(w, r, http.StatusInternalServerError, "internal", "cannot create scratch directory")
		return
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			s.log.Warn("cleanup request dir", "dir", dir, "err", err)
		}
	}()

	files, values, err := s.parseUpload(r, dir)
	if err != nil {
		var ue *uploadError
		if errors.As(err, &ue) {
			writeError(w, r, ue.status, ue.code, ue.Error())
			return
		}
		writeError(w, r, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	fn(dir, files, values)
}

// pdfOf returns the single PDF expected by an endpoint: the parts sent
// under the "file" field (or, for lenient clients, all parts) must contain
// exactly one PDF.
func (s *Server) pdfOf(w http.ResponseWriter, r *http.Request, files []uploadedFile) (uploadedFile, bool) {
	candidates := files
	fielded := make([]uploadedFile, 0, len(files))
	for _, f := range files {
		if f.Field == "file" {
			fielded = append(fielded, f)
		}
	}
	if len(fielded) > 0 {
		candidates = fielded
	}
	if len(candidates) == 0 {
		s.mapOpError(w, r, pdfops.ErrNoInput)
		return uploadedFile{}, false
	}
	if len(candidates) > 1 {
		writeError(w, r, http.StatusBadRequest, "too_many_files",
			"this endpoint accepts exactly one PDF file")
		return uploadedFile{}, false
	}
	f := candidates[0]
	if !pdfops.IsPDFFile(f.Name) {
		writeError(w, r, http.StatusUnsupportedMediaType, "unsupported_file_type",
			fmt.Sprintf("file %q is not a PDF", f.Name))
		return uploadedFile{}, false
	}
	return f, true
}

// outputName derives a safe download name from the requested value or the
// fallback, forcing the given extension.
func outputName(requested, fallback, ext string) string {
	name := safeBaseName(requested)
	if name == "" {
		name = fallback
	}
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" {
		name = "output"
	}
	return name + ext
}
