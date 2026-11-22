package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/linuxsuren/open-pdf/internal/pdfops"
)

// formFloat parses an optional float form value; ok is false when unset.
func formFloat(values url.Values, key string) (v float64, ok bool, err error) {
	s := values.Get(key)
	if s == "" {
		return 0, false, nil
	}
	v, perr := strconv.ParseFloat(s, 64)
	if perr != nil {
		return 0, true, fmt.Errorf("%w: %s must be a number, got %q", pdfops.ErrInvalidParam, key, s)
	}
	return v, true, nil
}

// handleMerge concatenates any mix of uploaded PDFs and images.
//
//	POST /api/v1/merge
//	  files: file parts (pdf/png/jpg/jpeg/webp/tif/tiff), order preserved
//	  output: optional output file name
func (s *Server) handleMerge(w http.ResponseWriter, r *http.Request) {
	s.withRequestScope(w, r, func(dir string, files []uploadedFile, values url.Values) {
		if len(files) == 0 {
			s.mapOpError(w, r, pdfops.ErrNoInput)
			return
		}
		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = f.Path
		}
		out := filepath.Join(dir, outputName(values.Get("output"), "merged", ".pdf"))
		if err := pdfops.Merge(paths, out); err != nil {
			s.mapOpError(w, r, err)
			return
		}
		s.serveResult(w, r, out, outputName(values.Get("output"), "merged", ".pdf"))
	})
}

// handleSplit splits one PDF by page.
//
//	POST /api/v1/split
//	  file: the PDF
//	  mode: burst | span | ranges  (default burst)
//	  span: chunk size for mode=span
//	  ranges: e.g. "1-3,5,8-" for mode=ranges
func (s *Server) handleSplit(w http.ResponseWriter, r *http.Request) {
	s.withRequestScope(w, r, func(dir string, files []uploadedFile, values url.Values) {
		in, ok := s.pdfOf(w, r, files)
		if !ok {
			return
		}
		opts := pdfops.SplitOptions{
			Mode:   values.Get("mode"),
			Ranges: values.Get("ranges"),
			Name:   strings.TrimSuffix(in.Name, filepath.Ext(in.Name)),
		}
		if v := values.Get("span"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				s.mapOpError(w, r, fmt.Errorf("%w: span must be a positive integer, got %q",
					pdfops.ErrInvalidParam, v))
				return
			}
			opts.Span = n
		}
		outDir := filepath.Join(dir, "split")
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			s.log.Error("create split dir", "err", err)
			writeError(w, r, http.StatusInternalServerError, "internal", "cannot create scratch directory")
			return
		}
		outs, err := pdfops.Split(in.Path, outDir, opts)
		if err != nil {
			s.mapOpError(w, r, err)
			return
		}
		base := strings.TrimSuffix(in.Name, filepath.Ext(in.Name))
		if len(outs) == 1 {
			name := base + ".pdf"
			s.serveResult(w, r, outs[0], name)
			return
		}
		zipPath := filepath.Join(dir, base+"_split.zip")
		if err := zipFiles(zipPath, outs); err != nil {
			s.log.Error("zip split output", "err", err)
			writeError(w, r, http.StatusInternalServerError, "internal", "cannot pack split output")
			return
		}
		s.serveResult(w, r, zipPath, base+"_split.zip")
	})
}

// handleCompress losslessly optimizes one PDF.
//
//	POST /api/v1/compress
//	  file: the PDF
func (s *Server) handleCompress(w http.ResponseWriter, r *http.Request) {
	s.withRequestScope(w, r, func(dir string, files []uploadedFile, values url.Values) {
		in, ok := s.pdfOf(w, r, files)
		if !ok {
			return
		}
		out := filepath.Join(dir, "compressed.pdf")
		if err := pdfops.Compress(in.Path, out); err != nil {
			s.mapOpError(w, r, err)
			return
		}
		s.serveResult(w, r, out, in.Name)
	})
}

// handleWatermark stamps a text or image watermark onto one PDF.
//
//	POST /api/v1/watermark
//	  file: the PDF
//	  type: text (default) | image
//	  text: watermark text (type=text)
//	  image: watermark image file part (type=image)
//	  pages, rotation, opacity, fontsize, color, position, fontname,
//	  diagonal, on_top: optional tuning
func (s *Server) handleWatermark(w http.ResponseWriter, r *http.Request) {
	s.withRequestScope(w, r, func(dir string, files []uploadedFile, values url.Values) {
		in, ok := s.pdfOf(w, r, files)
		if !ok {
			return
		}
		opts := pdfops.WatermarkOptions{
			Type:     values.Get("type"),
			Text:     values.Get("text"),
			Pages:    values.Get("pages"),
			Color:    values.Get("color"),
			Position: values.Get("position"),
			FontName: values.Get("fontname"),
			OnTop:    isTruthy(values.Get("on_top")),
		}
		if opts.Type == "" {
			opts.Type = "text"
		}
		if v, ok, err := formFloat(values, "rotation"); err != nil {
			s.mapOpError(w, r, err)
			return
		} else if ok {
			opts.Rotation = &v
		}
		if v, ok, err := formFloat(values, "opacity"); err != nil {
			s.mapOpError(w, r, err)
			return
		} else if ok {
			opts.Opacity = &v
		}
		if v, ok, err := formFloat(values, "fontsize"); err != nil {
			s.mapOpError(w, r, err)
			return
		} else if ok {
			opts.FontSize = &v
		}
		if v := values.Get("diagonal"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				s.mapOpError(w, r, fmt.Errorf("%w: diagonal must be 1, 2 or 3", pdfops.ErrInvalidParam))
				return
			}
			opts.Diagonal = n
		}

		if opts.Type == "image" {
			var img uploadedFile
			found := false
			for _, f := range files {
				if f.Field == "image" && pdfops.IsImageFile(f.Name) {
					img, found = f, true
					break
				}
			}
			if !found {
				s.mapOpError(w, r, fmt.Errorf("%w: image watermark requires an image part named \"image\"",
					pdfops.ErrMissingParam))
				return
			}
			opts.ImageFile = img.Path
		}

		out := filepath.Join(dir, "watermarked.pdf")
		if err := pdfops.AddWatermark(in.Path, out, opts); err != nil {
			s.mapOpError(w, r, err)
			return
		}
		s.serveResult(w, r, out, in.Name)
	})
}

// handleRotate rotates pages of one PDF.
//
//	POST /api/v1/rotate
//	  file: the PDF
//	  angle: ±90, ±180, ±270 (counterclockwise, required)
//	  pages: optional selection
func (s *Server) handleRotate(w http.ResponseWriter, r *http.Request) {
	s.withRequestScope(w, r, func(dir string, files []uploadedFile, values url.Values) {
		in, ok := s.pdfOf(w, r, files)
		if !ok {
			return
		}
		if values.Get("angle") == "" {
			s.mapOpError(w, r, fmt.Errorf("%w: angle is required (±90, ±180, ±270)", pdfops.ErrMissingParam))
			return
		}
		angle, err := strconv.Atoi(values.Get("angle"))
		if err != nil {
			s.mapOpError(w, r, fmt.Errorf("%w: angle must be an integer, got %q",
				pdfops.ErrInvalidParam, values.Get("angle")))
			return
		}
		out := filepath.Join(dir, "rotated.pdf")
		if err := pdfops.Rotate(in.Path, out, angle, values.Get("pages")); err != nil {
			s.mapOpError(w, r, err)
			return
		}
		s.serveResult(w, r, out, in.Name)
	})
}

// handleInfo reports document information for one PDF.
//
//	POST /api/v1/info
//	  file: the PDF
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	s.withRequestScope(w, r, func(dir string, files []uploadedFile, values url.Values) {
		in, ok := s.pdfOf(w, r, files)
		if !ok {
			return
		}
		info, err := pdfops.Info(in.Path, in.Name)
		if err != nil {
			s.mapOpError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, info)
	})
}

// handleEncrypt password-protects one PDF with AES.
//
//	POST /api/v1/encrypt
//	  file: the PDF
//	  password: user password (required)
//	  owner: owner password (defaults to password)
//	  key_length: 128 | 256 (default 256)
func (s *Server) handleEncrypt(w http.ResponseWriter, r *http.Request) {
	s.withRequestScope(w, r, func(dir string, files []uploadedFile, values url.Values) {
		in, ok := s.pdfOf(w, r, files)
		if !ok {
			return
		}
		pw := values.Get("password")
		if pw == "" {
			s.mapOpError(w, r, fmt.Errorf("%w: password is required", pdfops.ErrMissingParam))
			return
		}
		keyLength := 0
		if v := values.Get("key_length"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				s.mapOpError(w, r, fmt.Errorf("%w: key_length must be 128 or 256", pdfops.ErrInvalidParam))
				return
			}
			keyLength = n
		}
		out := filepath.Join(dir, "encrypted.pdf")
		if err := pdfops.Encrypt(in.Path, out, pw, values.Get("owner"), keyLength); err != nil {
			s.mapOpError(w, r, err)
			return
		}
		s.serveResult(w, r, out, in.Name)
	})
}

// handleDecrypt removes protection from one PDF.
//
//	POST /api/v1/decrypt
//	  file: the PDF
//	  password: the user or owner password
func (s *Server) handleDecrypt(w http.ResponseWriter, r *http.Request) {
	s.withRequestScope(w, r, func(dir string, files []uploadedFile, values url.Values) {
		in, ok := s.pdfOf(w, r, files)
		if !ok {
			return
		}
		out := filepath.Join(dir, "decrypted.pdf")
		if err := pdfops.Decrypt(in.Path, out, values.Get("password")); err != nil {
			s.mapOpError(w, r, err)
			return
		}
		s.serveResult(w, r, out, in.Name)
	})
}

// handleMarkdown converts a markdown document to PDF.
//
//	POST /api/v1/markdown
//	  file: the .md document (wins over text and url)
//	  text: markdown source text (wins over url)
//	  url: http(s) link to a markdown document, fetched server-side
//	  file parts (any name): image assets referenced by relative name
//	  page_size: a4|a3|a5|letter|legal (default a4)
//	  orientation: portrait|landscape
//	  margin: page margin in mm (default 18)
//	  font_size: body font size in pt (default 11)
//	  output: download file name (default derived from the source name)
func (s *Server) handleMarkdown(w http.ResponseWriter, r *http.Request) {
	s.withRequestScope(w, r, func(dir string, files []uploadedFile, values url.Values) {
		var source []byte
		var mdName string
		assets := map[string]string{}
		for _, f := range files {
			switch {
			case pdfops.IsMarkdownFile(f.Name):
				if source != nil {
					writeError(w, r, http.StatusBadRequest, "too_many_files",
						"只能提供一个 Markdown 文件")
					return
				}
				b, err := os.ReadFile(f.Path)
				if err != nil {
					s.log.Error("read markdown upload", "err", err)
					writeError(w, r, http.StatusInternalServerError, "internal", "无法读取上传的文件")
					return
				}
				source, mdName = b, f.Name
			case pdfops.IsImageFile(f.Name):
				assets[f.Name] = f.Path
			default:
				writeError(w, r, http.StatusUnsupportedMediaType, "unsupported_file_type",
					fmt.Sprintf("文件 %q 的类型不受支持", f.Name))
				return
			}
		}
		if len(source) == 0 && values.Get("text") != "" {
			source = []byte(values.Get("text"))
		}
		if len(source) == 0 {
			if raw := values.Get("url"); raw != "" {
				b, name, err := fetchMarkdown(r.Context(), raw)
				var fe *mdFetchError
				if errors.As(err, &fe) {
					if fe.cause != nil {
						s.log.Error("fetch markdown url", "err", fe.cause)
					}
					writeError(w, r, fe.status, fe.code, fe.msg)
					return
				}
				if err != nil {
					s.log.Error("fetch markdown url", "err", err)
					writeError(w, r, http.StatusInternalServerError, "internal", "无法下载链接指向的文件")
					return
				}
				source, mdName = b, name
			}
		}
		if len(source) == 0 {
			s.mapOpError(w, r, fmt.Errorf("%w: 需要上传 Markdown 文件、填写 text 内容或提供 url 链接",
				pdfops.ErrMissingParam))
			return
		}

		opts := pdfops.MarkdownOptions{
			Source:      source,
			PageSize:    values.Get("page_size"),
			Orientation: values.Get("orientation"),
			Assets:      assets,
		}
		if v, ok, err := formFloat(values, "margin"); err != nil {
			s.mapOpError(w, r, err)
			return
		} else if ok {
			opts.MarginMM = v
		}
		if v, ok, err := formFloat(values, "font_size"); err != nil {
			s.mapOpError(w, r, err)
			return
		} else if ok {
			opts.FontSize = v
		}

		fallback := "document"
		if mdName != "" {
			fallback = strings.TrimSuffix(mdName, filepath.Ext(mdName))
		}
		name := outputName(values.Get("output"), fallback, ".pdf")
		out := filepath.Join(dir, name)
		if err := pdfops.RenderMarkdown(out, opts); err != nil {
			if errors.Is(err, pdfops.ErrRenderFailed) {
				s.log.Error("markdown render failed", "err", err)
				writeError(w, r, http.StatusInternalServerError, "render_failed",
					"Markdown 转换失败，请检查文档内容后重试")
				return
			}
			s.mapOpError(w, r, err)
			return
		}
		s.serveResult(w, r, out, name)
	})
}

// serveResult streams the staged output file and logs streaming failures.
func (s *Server) serveResult(w http.ResponseWriter, r *http.Request, path, name string) {
	if err := serveFile(w, path, name); err != nil {
		// Headers may already be sent; nothing left to do but log.
		s.log.Warn("stream result", "path", path, "err", err)
	}
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
