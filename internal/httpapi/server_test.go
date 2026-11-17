package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linuxsuren/open-pdf/internal/config"
	"github.com/linuxsuren/open-pdf/internal/testutil"
)

// newTestServer builds a server with test-friendly defaults and returns the
// httptest server plus the scratch directory root.
func newTestServer(t *testing.T, mutate func(*config.Config)) *httptest.Server {
	t.Helper()
	tmp := t.TempDir()
	cfg := &config.Config{
		Addr:            ":0",
		MaxUploadBytes:  64 << 20,
		MaxFiles:        20,
		TempDir:         tmp,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		CORSOrigins:     []string{"*"},
	}
	if mutate != nil {
		mutate(cfg)
	}
	srv, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// upload builds a multipart body from files (name+content) and fields.
type filePart struct {
	field, name string
	content     []byte
}

func upload(t *testing.T, files []filePart, fields map[string]string) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		pw, err := mw.CreateFormFile(f.field, f.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pw.Write(f.content); err != nil {
			t.Fatal(err)
		}
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String(), mw.FormDataContentType()
}

func doPost(ts *httptest.Server, path, body, contentType string) *http.Response {
	resp, err := http.Post(ts.URL+path, contentType, strings.NewReader(body))
	if err != nil {
		panic(err)
	}
	return resp
}

func pdfFixture(t *testing.T, pages int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := testutil.WritePDF(&buf, pages); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pngFixture(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := testutil.WritePNG(&buf, 32, 24); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegFixture(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := testutil.WriteJPEG(&buf, 32, 24); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// errBody extracts the error envelope from a response.
func errBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var data struct {
		Error map[string]any `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if data.Error == nil {
		t.Fatal("no error envelope in response")
	}
	return data.Error
}

// countPages validates the downloaded PDF and returns its page count.
func countPages(t *testing.T, resp *http.Response) int {
	t.Helper()
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("content type = %q, want application/pdf", ct)
	}
	tmp := t.TempDir()
	p := filepath.Join(tmp, "out.pdf")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return validatePDFPages(t, p)
}

func validatePDFPages(t *testing.T, path string) int {
	t.Helper()
	out, err := runPageCount(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return out
}

// runPageCount shells out to the pdfops package via a tiny helper to avoid
// an import cycle in tests.
func runPageCount(path string) (int, error) {
	return pageCountFor(path)
}

func TestHealthAndMeta(t *testing.T) {
	ts := newTestServer(t, nil)

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}
	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data["status"] != "ok" {
		t.Fatalf("healthz status field = %v", data["status"])
	}

	resp2, err := http.Get(ts.URL + "/api/v1/version")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("version status = %d", resp2.StatusCode)
	}
	var v map[string]any
	_ = json.NewDecoder(resp2.Body).Decode(&v)
	if v["name"] != "open-pdf" {
		t.Fatalf("version name = %v", v["name"])
	}

	resp3, err := http.Get(ts.URL + "/api")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("api index status = %d", resp3.StatusCode)
	}
}

func TestWebUI(t *testing.T) {
	ts := newTestServer(t, nil)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index status = %d", resp.StatusCode)
	}
	if !bytes.Contains(body, []byte(`<div id="app">`)) {
		t.Fatalf("index does not contain the Vue mount point: %.200s", body)
	}

	// Serve one real asset from the embedded build.
	var asset string
	err = fsWalkAssets(func(name string) {
		if asset == "" {
			asset = name
		}
	})
	if err != nil || asset == "" {
		t.Fatalf("no embedded assets found: %v", err)
	}
	resp2, err := http.Get(ts.URL + "/assets/" + asset)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("asset status = %d", resp2.StatusCode)
	}
	if cc := resp2.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("asset cache control = %q", cc)
	}

	resp3, err := http.Get(ts.URL + "/definitely-not-here")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown path status = %d", resp3.StatusCode)
	}
	if e := errBody(t, resp3); e["code"] != "not_found" {
		t.Fatalf("unknown path code = %v", e["code"])
	}
}

func TestMethodNotAllowed(t *testing.T) {
	ts := newTestServer(t, nil)
	resp, err := http.Get(ts.URL + "/api/v1/merge")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET merge status = %d, want 405", resp.StatusCode)
	}
}

func TestMergeEndpoint(t *testing.T) {
	ts := newTestServer(t, nil)

	body, ct := upload(t, []filePart{
		{"files", "a.pdf", pdfFixture(t, 2)},
		{"files", "b.pdf", pdfFixture(t, 3)},
		{"files", "img.png", pngFixture(t)},
		{"files", "img.jpg", jpegFixture(t)},
	}, map[string]string{})
	resp := doPost(ts, "/api/v1/merge", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("merge status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	if n := countPages(t, resp); n != 7 {
		t.Fatalf("merged pages = %d, want 7", n)
	}
}

func TestMergeSingleImage(t *testing.T) {
	ts := newTestServer(t, nil)
	body, ct := upload(t, []filePart{{"files", "x.png", pngFixture(t)}}, nil)
	resp := doPost(ts, "/api/v1/merge", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("merge status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	if n := countPages(t, resp); n != 1 {
		t.Fatalf("pages = %d, want 1", n)
	}
}

func TestMergeErrors(t *testing.T) {
	ts := newTestServer(t, nil)

	cases := []struct {
		name   string
		files  []filePart
		status int
		code   string
	}{
		{"no files", nil, http.StatusBadRequest, "no_files"},
		{"unsupported type", []filePart{{"files", "a.gif", []byte("GIF89a")}}, http.StatusUnsupportedMediaType, "unsupported_file_type"},
		{"corrupt pdf", []filePart{{"files", "a.pdf", []byte("%PDF-1.4 junk")}}, http.StatusUnprocessableEntity, "invalid_pdf"},
	}
	for _, tc := range cases {
		body, ct := upload(t, tc.files, nil)
		resp := doPost(ts, "/api/v1/merge", body, ct)
		if resp.StatusCode != tc.status {
			t.Fatalf("%s: status = %d, want %d (%v)", tc.name, resp.StatusCode, tc.status, errBody(t, resp))
		}
		if e := errBody(t, resp); e["code"] != tc.code {
			t.Fatalf("%s: code = %v, want %s", tc.name, e["code"], tc.code)
		}
	}
}

func TestMergeTooManyFiles(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.MaxFiles = 2 })
	files := []filePart{
		{"files", "a.pdf", pdfFixture(t, 1)},
		{"files", "b.pdf", pdfFixture(t, 1)},
		{"files", "c.pdf", pdfFixture(t, 1)},
	}
	body, ct := upload(t, files, nil)
	resp := doPost(ts, "/api/v1/merge", body, ct)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %v", resp.StatusCode, errBody(t, resp))
	}
}

func TestMergeUploadTooLarge(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.MaxUploadBytes = 4 << 10 })
	big := make([]byte, 32<<10)
	for i := range big {
		big[i] = '%'
	}
	body, ct := upload(t, []filePart{{"files", "big.pdf", big}}, nil)
	resp := doPost(ts, "/api/v1/merge", body, ct)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
	if e := errBody(t, resp); e["code"] != "payload_too_large" {
		t.Fatalf("code = %v", e["code"])
	}
}

func TestSplitBurstZip(t *testing.T) {
	ts := newTestServer(t, nil)
	body, ct := upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 3)}}, map[string]string{"mode": "burst"})
	resp := doPost(ts, "/api/v1/split", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("split status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content type = %q", ct)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("zip open: %v", err)
	}
	if len(zr.File) != 3 {
		t.Fatalf("zip entries = %d, want 3", len(zr.File))
	}
	names := []string{}
	for _, f := range zr.File {
		names = append(names, f.Name)
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		tmp := filepath.Join(t.TempDir(), f.Name)
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if n := validatePDFPages(t, tmp); n != 1 {
			t.Fatalf("%s pages = %d, want 1", f.Name, n)
		}
	}
	if strings.Join(names, ",") != "doc_1.pdf,doc_2.pdf,doc_3.pdf" {
		t.Fatalf("zip names = %v", names)
	}
}

func TestSplitRangesSingleOutput(t *testing.T) {
	ts := newTestServer(t, nil)
	body, ct := upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 5)}},
		map[string]string{"mode": "ranges", "ranges": "2-4"})
	resp := doPost(ts, "/api/v1/split", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("split status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	if n := countPages(t, resp); n != 3 {
		t.Fatalf("pages = %d, want 3", n)
	}
}

func TestSplitBadParams(t *testing.T) {
	ts := newTestServer(t, nil)
	for _, tc := range []struct {
		fields map[string]string
		code   string
	}{
		{map[string]string{"mode": "bogus"}, "invalid_parameter"},
		{map[string]string{"mode": "span", "span": "0"}, "invalid_parameter"},
		{map[string]string{"mode": "span", "span": "x"}, "invalid_parameter"},
		{map[string]string{"mode": "ranges", "ranges": "9-8"}, "invalid_page_range"},
		{map[string]string{"mode": "ranges", "ranges": ""}, "invalid_page_range"},
	} {
		body, ct := upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 3)}}, tc.fields)
		resp := doPost(ts, "/api/v1/split", body, ct)
		if resp.StatusCode != http.StatusUnprocessableEntity && resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("fields %v: status = %d, (%v)", tc.fields, resp.StatusCode, errBody(t, resp))
		}
		if e := errBody(t, resp); e["code"] != tc.code {
			t.Fatalf("fields %v: code = %v, want %s", tc.fields, e["code"], tc.code)
		}
	}
}

func TestCompressEndpoint(t *testing.T) {
	ts := newTestServer(t, nil)
	body, ct := upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 30)}}, nil)
	resp := doPost(ts, "/api/v1/compress", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("compress status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "doc.pdf") {
		t.Fatalf("content disposition = %q", cd)
	}
	if n := countPages(t, resp); n != 30 {
		t.Fatalf("pages = %d, want 30", n)
	}
}

func TestWatermarkEndpoint(t *testing.T) {
	ts := newTestServer(t, nil)
	body, ct := upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 2)}},
		map[string]string{"text": "DRAFT", "opacity": "0.3", "fontsize": "40", "color": "#FF0000"})
	resp := doPost(ts, "/api/v1/watermark", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("watermark status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	if n := countPages(t, resp); n != 2 {
		t.Fatalf("pages = %d, want 2", n)
	}

	// Image watermark.
	body, ct = upload(t, []filePart{
		{"file", "doc.pdf", pdfFixture(t, 1)},
		{"image", "wm.png", pngFixture(t)},
	}, map[string]string{"type": "image"})
	resp = doPost(ts, "/api/v1/watermark", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("image watermark status = %d: %v", resp.StatusCode, errBody(t, resp))
	}

	// Missing text.
	body, ct = upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 1)}}, nil)
	resp = doPost(ts, "/api/v1/watermark", body, ct)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing text status = %d", resp.StatusCode)
	}

	// Bad opacity.
	body, ct = upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 1)}},
		map[string]string{"text": "x", "opacity": "5"})
	resp = doPost(ts, "/api/v1/watermark", body, ct)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad opacity status = %d", resp.StatusCode)
	}
	if e := errBody(t, resp); e["code"] != "invalid_parameter" {
		t.Fatalf("bad opacity code = %v", e["code"])
	}
}

func TestRotateEndpoint(t *testing.T) {
	ts := newTestServer(t, nil)
	body, ct := upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 2)}},
		map[string]string{"angle": "90"})
	resp := doPost(ts, "/api/v1/rotate", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rotate status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	if n := countPages(t, resp); n != 2 {
		t.Fatalf("pages = %d, want 2", n)
	}

	// Missing angle.
	body, ct = upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 1)}}, nil)
	resp = doPost(ts, "/api/v1/rotate", body, ct)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing angle status = %d", resp.StatusCode)
	}

	// Bad angle.
	body, ct = upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 1)}},
		map[string]string{"angle": "45"})
	resp = doPost(ts, "/api/v1/rotate", body, ct)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad angle status = %d", resp.StatusCode)
	}
}

func TestInfoEndpoint(t *testing.T) {
	ts := newTestServer(t, nil)
	body, ct := upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 4)}}, nil)
	resp := doPost(ts, "/api/v1/info", body, ct)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("info status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data["pageCount"] != float64(4) {
		t.Fatalf("pageCount = %v", data["pageCount"])
	}
}

func TestEncryptDecryptEndpoints(t *testing.T) {
	ts := newTestServer(t, nil)

	body, ct := upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 2)}},
		map[string]string{"password": "s3cret"})
	resp := doPost(ts, "/api/v1/encrypt", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("encrypt status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	encrypted, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Missing password.
	body, ct = upload(t, []filePart{{"file", "doc.pdf", pdfFixture(t, 1)}}, nil)
	resp = doPost(ts, "/api/v1/encrypt", body, ct)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("encrypt without password status = %d", resp.StatusCode)
	}

	// Decrypt with wrong password.
	body, ct = upload(t, []filePart{{"file", "doc.pdf", encrypted}}, map[string]string{"password": "nope"})
	resp = doPost(ts, "/api/v1/decrypt", body, ct)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("decrypt wrong pw status = %d (%v)", resp.StatusCode, errBody(t, resp))
	}

	// Decrypt with right password.
	body, ct = upload(t, []filePart{{"file", "doc.pdf", encrypted}}, map[string]string{"password": "s3cret"})
	resp = doPost(ts, "/api/v1/decrypt", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("decrypt status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	if n := countPages(t, resp); n != 2 {
		t.Fatalf("decrypted pages = %d, want 2", n)
	}
}

func TestNotPDFRejected(t *testing.T) {
	ts := newTestServer(t, nil)
	body, ct := upload(t, []filePart{{"file", "doc.png", pngFixture(t)}}, nil)
	resp := doPost(ts, "/api/v1/compress", body, ct)
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
	if e := errBody(t, resp); e["code"] != "unsupported_file_type" {
		t.Fatalf("code = %v", e["code"])
	}
}

func TestAuth(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.APIToken = "tok-123" })

	resp, err := http.Post(ts.URL+"/api/v1/merge", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token status = %d, want 401", resp.StatusCode)
	}

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/merge", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d, want 401", resp.StatusCode)
	}

	// With the right token the request reaches the handler (fails on body).
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.Close()
	req, _ = http.NewRequest("POST", ts.URL+"/api/v1/merge", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer tok-123")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatal("valid token rejected")
	}

	// healthz stays open.
	resp, _ = http.Get(ts.URL + "/healthz")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz behind auth status = %d", resp.StatusCode)
	}
}

func TestMarkdownEndpoint(t *testing.T) {
	ts := newTestServer(t, nil)

	// Inline text.
	body, ct := upload(t, nil, map[string]string{
		"text": "# 标题\n\n中文段落 **加粗** 与 [链接](https://example.com)\n",
	})
	resp := doPost(ts, "/api/v1/markdown", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("markdown status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	if n := countPages(t, resp); n != 1 {
		t.Fatalf("pages = %d, want 1", n)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "document.pdf") {
		t.Fatalf("content disposition = %q", cd)
	}
}

func TestMarkdownFileWithImage(t *testing.T) {
	ts := newTestServer(t, nil)

	var md bytes.Buffer
	md.WriteString("# 带图文档\n\n![shot](shot.png)\n")
	body, ct := upload(t, []filePart{
		{"file", "doc.md", md.Bytes()},
		{"asset", "shot.png", pngFixture(t)},
	}, map[string]string{"page_size": "letter"})
	resp := doPost(ts, "/api/v1/markdown", body, ct)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("markdown file status = %d: %v", resp.StatusCode, errBody(t, resp))
	}
	if n := countPages(t, resp); n != 1 {
		t.Fatalf("pages = %d, want 1", n)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "doc.pdf") {
		t.Fatalf("content disposition = %q", cd)
	}
}

func TestMarkdownErrors(t *testing.T) {
	ts := newTestServer(t, nil)

	cases := []struct {
		name   string
		files  []filePart
		fields map[string]string
		status int
		code   string
	}{
		{"no input", nil, nil, http.StatusBadRequest, "missing_parameter"},
		{"bad page size", nil, map[string]string{"text": "x", "page_size": "b6"}, http.StatusBadRequest, "invalid_parameter"},
		{"bad margin", nil, map[string]string{"text": "x", "margin": "1"}, http.StatusBadRequest, "invalid_parameter"},
		{"bad orientation", nil, map[string]string{"text": "x", "orientation": "up"}, http.StatusBadRequest, "invalid_parameter"},
		{"pdf not allowed", []filePart{{"file", "doc.pdf", pdfFixture(t, 1)}}, nil, http.StatusUnsupportedMediaType, "unsupported_file_type"},
	}
	for _, tc := range cases {
		body, ct := upload(t, tc.files, tc.fields)
		resp := doPost(ts, "/api/v1/markdown", body, ct)
		if resp.StatusCode != tc.status {
			t.Fatalf("%s: status = %d, want %d (%v)", tc.name, resp.StatusCode, tc.status, errBody(t, resp))
		}
		if e := errBody(t, resp); e["code"] != tc.code {
			t.Fatalf("%s: code = %v, want %s", tc.name, e["code"], tc.code)
		}
	}
}

func TestCORS(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.CORSOrigins = []string{"https://example.com"} })

	req, _ := http.NewRequest("GET", ts.URL+"/api", nil)
	req.Header.Set("Origin", "https://example.com")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if ao := resp.Header.Get("Access-Control-Allow-Origin"); ao != "https://example.com" {
		t.Fatalf("ACAO = %q", ao)
	}

	req, _ = http.NewRequest("GET", ts.URL+"/api", nil)
	req.Header.Set("Origin", "https://evil.example")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if ao := resp.Header.Get("Access-Control-Allow-Origin"); ao != "" {
		t.Fatalf("ACAO for disallowed origin = %q", ao)
	}

	// Preflight.
	req, _ = http.NewRequest("OPTIONS", ts.URL+"/api/v1/merge", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d", resp.StatusCode)
	}
}
