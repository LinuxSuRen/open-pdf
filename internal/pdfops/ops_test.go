package pdfops

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/linuxsuren/open-pdf/internal/testutil"
)

// stagePDF writes a fixture PDF and returns its path.
func stagePDF(t *testing.T, dir, name string, pages int) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := testutil.WritePDFFile(p, pages); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return p
}

func stagePNG(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := testutil.WritePNGFile(p, 64, 48); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return p
}

func stageJPEG(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := testutil.WriteJPEGFile(p, 64, 48); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return p
}

func assertValidPages(t *testing.T, path string, want int) {
	t.Helper()
	if err := Validate(path); err != nil {
		t.Fatalf("output %s is not a valid PDF: %v", path, err)
	}
	n, err := PageCount(path)
	if err != nil {
		t.Fatalf("page count of %s: %v", path, err)
	}
	if n != want {
		t.Fatalf("page count of %s = %d, want %d", path, n, want)
	}
}

func TestValidateAndPageCount(t *testing.T) {
	dir := t.TempDir()
	good := stagePDF(t, dir, "good.pdf", 3)

	if err := Validate(good); err != nil {
		t.Fatalf("valid fixture rejected: %v", err)
	}
	n, err := PageCount(good)
	if err != nil || n != 3 {
		t.Fatalf("PageCount = %d, %v; want 3, nil", n, err)
	}

	bad := filepath.Join(dir, "bad.pdf")
	if err := os.WriteFile(bad, []byte("this is definitely not a pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Validate(bad); err == nil {
		t.Fatal("corrupt file accepted")
	} else if !strings.Contains(err.Error(), "invalid pdf") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMergePDFs(t *testing.T) {
	dir := t.TempDir()
	a := stagePDF(t, dir, "a.pdf", 2)
	b := stagePDF(t, dir, "b.pdf", 3)
	out := filepath.Join(dir, "merged.pdf")

	if err := Merge([]string{a, b}, out); err != nil {
		t.Fatalf("merge: %v", err)
	}
	assertValidPages(t, out, 5)
}

func TestMergeMixed(t *testing.T) {
	dir := t.TempDir()
	a := stagePDF(t, dir, "a.pdf", 2)
	img1 := stagePNG(t, dir, "i1.png")
	img2 := stageJPEG(t, dir, "i2.jpg")
	b := stagePDF(t, dir, "b.pdf", 1)
	out := filepath.Join(dir, "merged.pdf")

	if err := Merge([]string{a, img1, img2, b}, out); err != nil {
		t.Fatalf("merge mixed: %v", err)
	}
	assertValidPages(t, out, 5) // 2 pdf pages + 1 png page + 1 jpg page + 1 pdf page
}

func TestMergeImagesOnly(t *testing.T) {
	dir := t.TempDir()
	img1 := stagePNG(t, dir, "i1.png")
	img2 := stageJPEG(t, dir, "i2.jpg")
	out := filepath.Join(dir, "merged.pdf")

	if err := Merge([]string{img1, img2}, out); err != nil {
		t.Fatalf("merge images: %v", err)
	}
	assertValidPages(t, out, 2)
}

func TestMergeSingleInputs(t *testing.T) {
	dir := t.TempDir()

	out := filepath.Join(dir, "single.pdf")
	if err := Merge([]string{stagePDF(t, dir, "a.pdf", 3)}, out); err != nil {
		t.Fatalf("merge single pdf: %v", err)
	}
	assertValidPages(t, out, 3)

	out2 := filepath.Join(dir, "img.pdf")
	if err := Merge([]string{stagePNG(t, dir, "i.png")}, out2); err != nil {
		t.Fatalf("merge single image: %v", err)
	}
	assertValidPages(t, out2, 1)
}

func TestMergeNoInput(t *testing.T) {
	if err := Merge(nil, filepath.Join(t.TempDir(), "x.pdf")); err == nil {
		t.Fatal("expected error for empty input list")
	}
}

func TestMergeCorruptPDF(t *testing.T) {
	dir := t.TempDir()
	corrupt := filepath.Join(dir, "corrupt.pdf")
	if err := os.WriteFile(corrupt, []byte("%PDF-1.4 broken junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Merge([]string{corrupt}, filepath.Join(dir, "out.pdf"))
	if err == nil {
		t.Fatal("corrupt pdf accepted")
	}
	if got := err.Error(); !strings.Contains(got, "invalid pdf") {
		t.Fatalf("unexpected error: %v", got)
	}
}

func TestMergeUnsupportedType(t *testing.T) {
	dir := t.TempDir()
	gif := filepath.Join(dir, "anim.gif")
	if err := os.WriteFile(gif, []byte("GIF89a...."), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Merge([]string{gif}, filepath.Join(dir, "out.pdf"))
	if err == nil || !strings.Contains(err.Error(), "unsupported file type") {
		t.Fatalf("expected unsupported file type error, got %v", err)
	}
}

func TestSplitBurst(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 5)
	outDir := filepath.Join(dir, "out")

	outs, err := Split(in, outDir, SplitOptions{Mode: SplitBurst})
	if err != nil {
		t.Fatalf("split burst: %v", err)
	}
	if len(outs) != 5 {
		t.Fatalf("got %d outputs, want 5", len(outs))
	}
	for i, p := range outs {
		if want := filepath.Join(outDir, "doc_"+strconv.Itoa(i+1)+".pdf"); p != want {
			t.Fatalf("output[%d] = %s, want %s", i, p, want)
		}
		assertValidPages(t, p, 1)
	}
}

func TestSplitSpan(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 5)
	outDir := filepath.Join(dir, "out")

	outs, err := Split(in, outDir, SplitOptions{Mode: SplitSpan, Span: 2})
	if err != nil {
		t.Fatalf("split span: %v", err)
	}
	if len(outs) != 3 {
		t.Fatalf("got %d outputs, want 3", len(outs))
	}
	wantPages := []int{2, 2, 1}
	for i, p := range outs {
		assertValidPages(t, p, wantPages[i])
	}
}

func TestSplitSpanBeyondEnd(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 3)
	outDir := filepath.Join(dir, "out")

	outs, err := Split(in, outDir, SplitOptions{Mode: SplitSpan, Span: 10})
	if err != nil {
		t.Fatalf("split span: %v", err)
	}
	if len(outs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(outs))
	}
	assertValidPages(t, outs[0], 3)
}

func TestSplitRanges(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 5)
	outDir := filepath.Join(dir, "out")

	outs, err := Split(in, outDir, SplitOptions{Mode: SplitRanges, Ranges: "1-2,4"})
	if err != nil {
		t.Fatalf("split ranges: %v", err)
	}
	if len(outs) != 2 {
		t.Fatalf("got %d outputs, want 2", len(outs))
	}
	if want := filepath.Join(outDir, "doc_1-2.pdf"); outs[0] != want {
		t.Fatalf("outs[0] = %s, want %s", outs[0], want)
	}
	assertValidPages(t, outs[0], 2)
	assertValidPages(t, outs[1], 1)
}

func TestSplitRangesOpenEnds(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 5)
	outDir := filepath.Join(dir, "out")

	outs, err := Split(in, outDir, SplitOptions{Mode: SplitRanges, Ranges: "-2,4-"})
	if err != nil {
		t.Fatalf("split ranges: %v", err)
	}
	if len(outs) != 2 {
		t.Fatalf("got %d outputs, want 2", len(outs))
	}
	assertValidPages(t, outs[0], 2)
	assertValidPages(t, outs[1], 2)
}

func TestSplitRangeErrors(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 5)
	outDir := filepath.Join(dir, "out")

	for _, tc := range []struct{ ranges, why string }{
		{"", "empty"},
		{"abc", "not a number"},
		{"0", "zero page"},
		{"6", "out of bounds"},
		{"3-1", "descending"},
		{"1,,2", "empty segment"},
	} {
		_, err := Split(in, outDir, SplitOptions{Mode: SplitRanges, Ranges: tc.ranges})
		if err == nil {
			t.Fatalf("ranges %q (%s): expected error", tc.ranges, tc.why)
		}
	}
}

func TestSplitInvalidOptions(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 3)
	outDir := filepath.Join(dir, "out")

	if _, err := Split(in, outDir, SplitOptions{Mode: "bogus"}); err == nil {
		t.Fatal("expected error for unknown mode")
	}
	if _, err := Split(in, outDir, SplitOptions{Mode: SplitSpan, Span: 0}); err == nil {
		t.Fatal("expected error for span 0")
	}
}

func TestCompress(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 120)
	out := filepath.Join(dir, "compressed.pdf")

	if err := Compress(in, out); err != nil {
		t.Fatalf("compress: %v", err)
	}
	assertValidPages(t, out, 120)

	orig, err := os.Stat(in)
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if compressed.Size() >= orig.Size() {
		t.Fatalf("compression did not shrink file: %d -> %d bytes", orig.Size(), compressed.Size())
	}
	t.Logf("compressed %d -> %d bytes (%.1f%%)", orig.Size(), compressed.Size(),
		100*float64(compressed.Size())/float64(orig.Size()))
}

func f64(v float64) *float64 { return &v }

func TestWatermarkText(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 3)
	out := filepath.Join(dir, "wm.pdf")

	opts := WatermarkOptions{
		Type:     "text",
		Text:     "CONFIDENTIAL",
		Opacity:  f64(0.4),
		FontSize: f64(48),
		Color:    "#CCCCCC",
	}
	if err := AddWatermark(in, out, opts); err != nil {
		t.Fatalf("watermark: %v", err)
	}
	assertValidPages(t, out, 3)

	if ok, err := hasWatermarks(out); err != nil || !ok {
		t.Fatalf("watermark not detected: ok=%v err=%v", ok, err)
	}
}

func TestWatermarkImage(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 2)
	img := stagePNG(t, dir, "wm.png")
	out := filepath.Join(dir, "wm.pdf")

	opts := WatermarkOptions{
		Type:      "image",
		ImageFile: img,
		Opacity:   f64(0.5),
		Position:  "c",
	}
	if err := AddWatermark(in, out, opts); err != nil {
		t.Fatalf("image watermark: %v", err)
	}
	assertValidPages(t, out, 2)

	if ok, err := hasWatermarks(out); err != nil || !ok {
		t.Fatalf("watermark not detected: ok=%v err=%v", ok, err)
	}
}

func TestWatermarkSelectedPages(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 4)
	out := filepath.Join(dir, "wm.pdf")

	opts := WatermarkOptions{Text: "DRAFT", Pages: "1,3"}
	if err := AddWatermark(in, out, opts); err != nil {
		t.Fatalf("watermark: %v", err)
	}
	assertValidPages(t, out, 4)
	if ok, _ := hasWatermarks(out); !ok {
		t.Fatal("watermark not detected")
	}
}

func TestWatermarkInvalidOptions(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 2)
	out := filepath.Join(dir, "wm.pdf")

	cases := []struct {
		name string
		opts WatermarkOptions
	}{
		{"empty text", WatermarkOptions{Type: "text"}},
		{"image without file", WatermarkOptions{Type: "image"}},
		{"bad opacity", WatermarkOptions{Text: "x", Opacity: f64(1.5)}},
		{"bad rotation", WatermarkOptions{Text: "x", Rotation: f64(720)}},
		{"bad fontsize", WatermarkOptions{Text: "x", FontSize: f64(-1)}},
		{"bad diagonal", WatermarkOptions{Text: "x", Diagonal: 9}},
		{"bad pages", WatermarkOptions{Text: "x", Pages: "?!"}},
	}
	for _, tc := range cases {
		if err := AddWatermark(in, out, tc.opts); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
}

func TestRotate(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 3)
	out := filepath.Join(dir, "rotated.pdf")

	if err := Rotate(in, out, 90, ""); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	assertValidPages(t, out, 3)

	if err := Rotate(in, out, -90, "1-2"); err != nil {
		t.Fatalf("rotate pages: %v", err)
	}

	for _, bad := range []int{0, 45, 100} {
		if err := Rotate(in, out, bad, ""); err == nil {
			t.Fatalf("angle %d accepted", bad)
		}
	}
	if err := Rotate(in, out, 90, "bogus!"); err == nil {
		t.Fatal("bogus page selection accepted")
	}
}

func TestInfo(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 3)

	info, err := Info(in, "")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.PageCount != 3 {
		t.Fatalf("page count = %d, want 3", info.PageCount)
	}
	if info.Version != "1.4" {
		t.Fatalf("version = %q, want 1.4", info.Version)
	}
	if info.Encrypted {
		t.Fatal("fixture reported as encrypted")
	}

	named, err := Info(in, "report-final.pdf")
	if err != nil {
		t.Fatalf("info with display name: %v", err)
	}
	if named.FileName != "report-final.pdf" {
		t.Fatalf("source = %q, want report-final.pdf", named.FileName)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 3)
	enc := filepath.Join(dir, "enc.pdf")
	dec := filepath.Join(dir, "dec.pdf")

	if err := Encrypt(in, enc, "secret", "", 256); err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Without the password the file must be unreadable.
	if err := Validate(enc); err == nil {
		t.Fatal("encrypted file validates without password")
	}

	if err := Decrypt(enc, dec, "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}

	if err := Decrypt(enc, dec, "secret"); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	assertValidPages(t, dec, 3)

	// The encrypted file itself stays valid when opened with the password.
	if err := validateWithPassword(enc, "secret"); err != nil {
		t.Fatalf("encrypted file invalid with password: %v", err)
	}
}

func TestEncryptInvalid(t *testing.T) {
	dir := t.TempDir()
	in := stagePDF(t, dir, "doc.pdf", 1)
	out := filepath.Join(dir, "enc.pdf")

	if err := Encrypt(in, out, "", "", 256); err == nil {
		t.Fatal("empty passwords accepted")
	}
	if err := Encrypt(in, out, "pw", "", 512); err == nil {
		t.Fatal("bad key length accepted")
	}
}

// TestPipeline chains every operation end to end on the ops layer.
func TestPipeline(t *testing.T) {
	dir := t.TempDir()
	base := stagePDF(t, dir, "base.pdf", 6)
	png := stagePNG(t, dir, "side.png")

	merged := filepath.Join(dir, "merged.pdf")
	if err := Merge([]string{base, png, base}, merged); err != nil {
		t.Fatalf("merge: %v", err)
	}
	assertValidPages(t, merged, 13)

	splitDir := filepath.Join(dir, "split")
	outs, err := Split(merged, splitDir, SplitOptions{Mode: SplitRanges, Ranges: "1-3,4-13"})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(outs) != 2 {
		t.Fatalf("split outputs = %d, want 2", len(outs))
	}

	remerged := filepath.Join(dir, "remerged.pdf")
	if err := Merge(outs, remerged); err != nil {
		t.Fatalf("remerge: %v", err)
	}
	assertValidPages(t, remerged, 13)

	wm := filepath.Join(dir, "wm.pdf")
	if err := AddWatermark(remerged, wm, WatermarkOptions{Text: "open-pdf", Opacity: f64(0.3)}); err != nil {
		t.Fatalf("watermark: %v", err)
	}

	rotated := filepath.Join(dir, "rotated.pdf")
	if err := Rotate(wm, rotated, 180, ""); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	final := filepath.Join(dir, "final.pdf")
	if err := Compress(rotated, final); err != nil {
		t.Fatalf("compress: %v", err)
	}
	assertValidPages(t, final, 13)

	info, err := Info(final, "")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if !info.Watermarked {
		t.Fatal("final file lost its watermark")
	}
}
