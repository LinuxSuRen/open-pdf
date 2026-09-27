// Package testutil generates deterministic PDF and image fixtures for tests
// and manual experiments. It deliberately avoids heavy dependencies: the
// minimal PDF writer below emits spec-conforming uncompressed PDFs.
package testutil

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WritePDF writes a minimal, valid, uncompressed PDF with the given number
// of pages. Every page shows "Page N" in Helvetica 24pt on an A4 portrait
// box. Repeated calls with the same arguments produce identical bytes.
//
// Object layout: 1 = catalog, 2 = pages tree, 3 = font, then page/content
// pairs: page i lives in object 2i+2 with its content stream in 2i+3.
func WritePDF(w io.Writer, pages int) error {
	if pages < 1 {
		return fmt.Errorf("testutil: pages must be >= 1, got %d", pages)
	}

	total := 3 + 2*pages
	objs := make([]string, total+1) // 1-based; index 0 unused

	objs[1] = "<< /Type /Catalog /Pages 2 0 R >>"

	var kids []string
	for i := 1; i <= pages; i++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", 2*i+2))
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>",
		strings.Join(kids, " "), pages)

	objs[3] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"

	for i := 1; i <= pages; i++ {
		content := fmt.Sprintf("BT /F1 24 Tf 72 720 Td (Page %d) Tj ET", i)
		objs[2*i+2] = fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>",
			2*i+3)
		objs[2*i+3] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content)+1, content)
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")

	offsets := make([]int, total+1)
	for i := 1; i <= total; i++ {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i, objs[i])
	}

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", total+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= total; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		total+1, xref)

	_, err := w.Write(buf.Bytes())
	return err
}

// WritePDFFile creates path with a fresh fixture of pages pages.
func WritePDFFile(path string, pages int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return WritePDF(f, pages)
}

// gradientImage returns a deterministic w x h RGBA gradient.
func gradientImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(255 * x / max(1, w-1)),
				G: uint8(255 * y / max(1, h-1)),
				B: uint8((x*x + y*y) % 256),
				A: 255,
			})
		}
	}
	return img
}

// WritePNG writes a deterministic PNG fixture.
func WritePNG(w io.Writer, width, height int) error {
	return png.Encode(w, gradientImage(width, height))
}

// WritePNGFile creates path with a deterministic PNG fixture.
func WritePNGFile(path string, w, h int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return WritePNG(f, w, h)
}

// WriteJPEG writes a deterministic JPEG fixture.
func WriteJPEG(w io.Writer, width, height int) error {
	return jpeg.Encode(w, gradientImage(width, height), &jpeg.Options{Quality: 85})
}

// WriteJPEGFile creates path with a deterministic JPEG fixture.
func WriteJPEGFile(path string, w, h int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return WriteJPEG(f, w, h)
}
