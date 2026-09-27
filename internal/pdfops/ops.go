// Package pdfops implements the PDF operations exposed by the open-pdf
// service on top of the pdfcpu library. All functions are file based: the
// HTTP layer stages uploads in a per-request temp directory and hands the
// paths over, which keeps large files out of process memory.
package pdfops

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func init() {
	// Never read or write pdfcpu's user-level config.yml; behavior stays
	// identical across hosts and containers.
	api.DisableConfigDir()
}

// newConf returns a fresh pdfcpu configuration with deterministic defaults.
func newConf() *model.Configuration {
	return model.NewDefaultConfiguration()
}

// Split modes.
const (
	SplitBurst  = "burst"  // one PDF per page
	SplitSpan   = "span"   // chunks of N pages
	SplitRanges = "ranges" // one PDF per comma-separated range group
)

// Supported image extensions for import (mirrors pdfcpu support).
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true,
	".webp": true, ".tif": true, ".tiff": true,
}

// IsImageFile reports whether the file name carries a supported image
// extension.
func IsImageFile(name string) bool {
	return imageExts[strings.ToLower(filepath.Ext(name))]
}

// IsPDFFile reports whether the file name carries a .pdf extension.
func IsPDFFile(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".pdf")
}

// Validate checks that path is a structurally valid PDF.
func Validate(path string) error {
	if err := api.ValidateFile(path, newConf()); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrInvalidPDF, filepath.Base(path), err)
	}
	return nil
}

// PageCount returns the number of pages of the PDF at path.
func PageCount(path string) (int, error) {
	n, err := api.PageCountFile(path)
	if err != nil {
		return 0, opErr("page count", fmt.Errorf("%s: %w", filepath.Base(path), err))
	}
	return n, nil
}

// Merge concatenates the given files, in order, into a single PDF written to
// outFile. Inputs may be an arbitrary mix of PDFs and supported images;
// images are converted to single A4 pages at their position in the order.
func Merge(inFiles []string, outFile string) error {
	if len(inFiles) == 0 {
		return ErrNoInput
	}
	dir := filepath.Dir(outFile)
	pdfs := make([]string, 0, len(inFiles))
	for i, in := range inFiles {
		switch {
		case IsPDFFile(in):
			if err := Validate(in); err != nil {
				return err
			}
			pdfs = append(pdfs, in)
		case IsImageFile(in):
			// Convert the image to a single-page PDF next to the output so
			// that interleaving order is preserved.
			staged := filepath.Join(dir, fmt.Sprintf("import-%04d.pdf", i))
			if err := api.ImportImagesFile([]string{in}, staged, importConfig(), newConf()); err != nil {
				return opErr("import image "+filepath.Base(in), err)
			}
			pdfs = append(pdfs, staged)
		default:
			return fmt.Errorf("%q: %w (supported: pdf, png, jpg, jpeg, webp, tif, tiff)",
				filepath.Base(in), ErrUnsupported)
		}
	}
	if err := api.MergeCreateFile(pdfs, outFile, false, newConf()); err != nil {
		return opErr("merge", err)
	}
	return nil
}

// importConfig returns the image import configuration: images fill the page
// as much as possible while keeping their aspect ratio.
func importConfig() *pdfcpu.Import {
	imp := api.DefaultImportConfig() // A4 page, anchored, relative scale
	return imp
}

// SplitOptions controls how a PDF is split.
type SplitOptions struct {
	// Mode is one of SplitBurst, SplitSpan or SplitRanges.
	Mode string
	// Ranges holds comma separated groups ("1-3,5,8-") for SplitRanges.
	Ranges string
	// Span is the chunk size in pages for SplitSpan.
	Span int
	// Name overrides the base name used for output files (defaults to the
	// input file name without extension).
	Name string
}

// Split writes one PDF per resulting group into outDir (created if needed)
// and returns the output paths in page order. File names are derived from
// the input base name: "doc_1.pdf", "doc_2-4.pdf", ...
func Split(inFile, outDir string, o SplitOptions) (outs []string, err error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, opErr("split", fmt.Errorf("create output directory: %w", err))
	}
	pageCount, err := PageCount(inFile)
	if err != nil {
		return nil, err
	}
	base := o.Name
	if base == "" {
		base = strings.TrimSuffix(filepath.Base(inFile), filepath.Ext(inFile))
	}
	if base == "" {
		base = "pdf"
	}

	switch mode := o.Mode; {
	case mode == "" || mode == SplitBurst:
		outs, err = splitSpan(inFile, outDir, base, 1, pageCount)
	case mode == SplitSpan:
		if o.Span <= 0 {
			return nil, fmt.Errorf("%w: span must be a positive number of pages, got %d", ErrInvalidParam, o.Span)
		}
		outs, err = splitSpan(inFile, outDir, base, o.Span, pageCount)
	case mode == SplitRanges:
		outs, err = splitRanges(inFile, outDir, base, o.Ranges, pageCount)
	default:
		return nil, fmt.Errorf("%w: unknown split mode %q (use burst, span or ranges)", ErrInvalidParam, mode)
	}
	return outs, err
}

// splitSpan delegates chunking to pdfcpu's single-pass SplitFile and aligns
// the generated file names with the requested display base name.
func splitSpan(inFile, outDir, base string, span, pageCount int) ([]string, error) {
	if err := api.SplitFile(inFile, outDir, span, newConf()); err != nil {
		return nil, opErr("split", err)
	}
	// pdfcpu derives output names from the input file name; rename to the
	// caller-facing base name when they differ.
	inBase := strings.TrimSuffix(filepath.Base(inFile), filepath.Ext(inFile))
	var outs []string
	for from := 1; from <= pageCount; from += span {
		thru := from + span - 1
		if thru > pageCount {
			thru = pageCount
		}
		generated := filepath.Join(outDir, spanName(inBase, from, thru))
		want := filepath.Join(outDir, spanName(base, from, thru))
		if want != generated {
			if err := os.Rename(generated, want); err != nil {
				return nil, opErr("split", fmt.Errorf("rename output: %w", err))
			}
		}
		outs = append(outs, want)
	}
	return outs, nil
}

// spanName mirrors pdfcpu's split naming: base_from.pdf / base_from-thru.pdf.
func spanName(base string, from, thru int) string {
	name := strconv.Itoa(from)
	if from != thru {
		name = name + "-" + strconv.Itoa(thru)
	}
	return base + "_" + name + ".pdf"
}

// splitRanges produces one file per range group using pdfcpu's TrimFile,
// which keeps exactly the selected pages.
func splitRanges(inFile, outDir, base, ranges string, pageCount int) ([]string, error) {
	groups, err := ParseRanges(ranges, pageCount)
	if err != nil {
		return nil, err
	}
	outs := make([]string, 0, len(groups))
	for _, g := range groups {
		name := strconv.Itoa(g.Start)
		if g.Start != g.End {
			name = name + "-" + strconv.Itoa(g.End)
		}
		out := filepath.Join(outDir, base+"_"+name+".pdf")
		sel := strconv.Itoa(g.Start)
		if g.Start != g.End {
			sel = sel + "-" + strconv.Itoa(g.End)
		}
		if err := api.TrimFile(inFile, out, []string{sel}, newConf()); err != nil {
			return nil, opErr("split range "+sel, err)
		}
		outs = append(outs, out)
	}
	return outs, nil
}

// PageGroup is an inclusive 1-based page interval.
type PageGroup struct {
	Start int
	End   int
}

// ParseRanges parses "1-3,5,8-" against a page count. Open ends ("-3", "8-")
// are bounded by the page count; every segment must resolve to at least one
// page. Segments are evaluated left to right and may overlap.
func ParseRanges(s string, pageCount int) ([]PageGroup, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%w: empty ranges expression", ErrPageRange)
	}
	var groups []PageGroup
	for _, seg := range strings.Split(s, ",") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return nil, fmt.Errorf("%w: empty segment in %q", ErrPageRange, s)
		}
		parts := strings.SplitN(seg, "-", 2)
		parse := func(v string, def int) (int, error) {
			if v == "" {
				return def, nil
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return 0, fmt.Errorf("%w: %q is not a page number", ErrPageRange, v)
			}
			if n < 1 || n > pageCount {
				return 0, fmt.Errorf("%w: page %d out of bounds 1-%d", ErrPageRange, n, pageCount)
			}
			return n, nil
		}
		start, err := parse(parts[0], 1)
		if err != nil {
			return nil, err
		}
		end := start
		if len(parts) == 2 {
			if end, err = parse(parts[1], pageCount); err != nil {
				return nil, err
			}
		}
		if start > end {
			return nil, fmt.Errorf("%w: descending segment %q", ErrPageRange, seg)
		}
		groups = append(groups, PageGroup{Start: start, End: end})
	}
	return groups, nil
}

// Compress rewrites the PDF with object streams, xref streams and duplicate
// content-stream elimination enabled, which is a lossless structural
// optimization.
func Compress(inFile, outFile string) error {
	conf := newConf()
	conf.WriteObjectStream = true
	conf.WriteXRefStream = true
	conf.OptimizeBeforeWriting = true
	conf.OptimizeResourceDicts = true
	conf.OptimizeDuplicateContentStreams = true
	if err := api.OptimizeFile(inFile, outFile, conf); err != nil {
		return opErr("compress", err)
	}
	return nil
}

// WatermarkOptions describes a text or image watermark.
type WatermarkOptions struct {
	// Type is "text" (default) or "image".
	Type string
	// Text is the watermark text for Type == "text".
	Text string
	// ImageFile is the watermark image for Type == "image".
	ImageFile string
	// Pages is an optional pdfcpu page selection ("1-3,5", "even", ...).
	Pages string
	// Rotation in degrees counterclockwise; nil keeps pdfcpu's default.
	Rotation *float64
	// Opacity in (0,1]; nil keeps pdfcpu's default.
	Opacity *float64
	// FontSize in points ("points" in pdfcpu terms).
	FontSize *float64
	// Color as #RRGGBB or a pdfcpu color name (gray, red, ...).
	Color string
	// Position anchor: c, tl, tc, tr, l, r, bl, bc, br.
	Position string
	// FontName defaults to Helvetica.
	FontName string
	// Diagonal: 1 = bottom-left to top-right (default), 2 = top-left to
	// bottom-right, 3 = horizontal.
	Diagonal int
	// OnTop draws over the page content (stamp) instead of under it.
	OnTop bool
}

// AddWatermark stamps the watermark described by o onto the PDF at inFile
// and writes the result to outFile.
func AddWatermark(inFile, outFile string, o WatermarkOptions) error {
	desc, err := o.descriptor()
	if err != nil {
		return err
	}
	pages, err := api.ParsePageSelection(o.Pages)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPageRange, err)
	}
	if o.Type == "image" {
		if o.ImageFile == "" {
			return fmt.Errorf("%w: image watermark requires an image file", ErrMissingParam)
		}
		if err := api.AddImageWatermarksFile(inFile, outFile, pages, o.OnTop, o.ImageFile, desc, newConf()); err != nil {
			return opErr("image watermark", err)
		}
		return nil
	}
	if strings.TrimSpace(o.Text) == "" {
		return fmt.Errorf("%w: text watermark requires non-empty text", ErrMissingParam)
	}
	if err := api.AddTextWatermarksFile(inFile, outFile, pages, o.OnTop, o.Text, desc, newConf()); err != nil {
		return opErr("text watermark", err)
	}
	return nil
}

// descriptor renders the option set into pdfcpu's "key:value,key:value"
// watermark parameter string. Only explicitly set options are included so
// that pdfcpu defaults apply for the rest.
func (o WatermarkOptions) descriptor() (string, error) {
	var parts []string
	add := func(format string, v any) { parts = append(parts, fmt.Sprintf(format, v)) }
	if o.Rotation != nil {
		if *o.Rotation < -360 || *o.Rotation > 360 {
			return "", fmt.Errorf("%w: rotation must be within [-360,360]", ErrInvalidParam)
		}
		add("rotation:%g", *o.Rotation)
	}
	if o.Opacity != nil {
		if *o.Opacity <= 0 || *o.Opacity > 1 {
			return "", fmt.Errorf("%w: opacity must be within (0,1]", ErrInvalidParam)
		}
		add("opacity:%g", *o.Opacity)
	}
	if o.FontSize != nil {
		if *o.FontSize <= 0 {
			return "", fmt.Errorf("%w: font size must be positive", ErrInvalidParam)
		}
		add("points:%g", *o.FontSize)
	}
	if o.Color != "" {
		add("color:%s", o.Color)
	}
	if o.Position != "" {
		add("position:%s", o.Position)
	}
	if o.FontName != "" {
		add("fontname:%s", o.FontName)
	}
	if o.Diagonal != 0 {
		if o.Diagonal < 1 || o.Diagonal > 3 {
			return "", fmt.Errorf("%w: diagonal must be 1, 2 or 3", ErrInvalidParam)
		}
		add("diagonal:%d", o.Diagonal)
	}
	return strings.Join(parts, ","), nil
}

// Rotate rotates the given pages by angle degrees counterclockwise. angle
// must be a non-zero multiple of 90.
func Rotate(inFile, outFile string, angle int, pages string) error {
	if angle%90 != 0 || angle == 0 {
		return fmt.Errorf("%w: rotation angle must be a non-zero multiple of 90, got %d", ErrInvalidParam, angle)
	}
	sel, err := api.ParsePageSelection(pages)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPageRange, err)
	}
	if err := api.RotateFile(inFile, outFile, angle, sel, newConf()); err != nil {
		return opErr("rotate", err)
	}
	return nil
}

// Info returns the document information of the PDF at path. display names
// the file in the result (defaults to the path base name).
func Info(path, display string) (*pdfcpu.PDFInfo, error) {
	if display == "" {
		display = filepath.Base(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("info: open %s: %w", display, err)
	}
	defer f.Close()
	info, err := api.PDFInfo(f, display, nil, false, newConf())
	if err != nil {
		return nil, opErr("info", fmt.Errorf("%s: %w", display, err))
	}
	return info, nil
}

// Encrypt encrypts the PDF with AES. keyLength must be 128 or 256. An empty
// owner password falls back to the user password.
func Encrypt(inFile, outFile, userPW, ownerPW string, keyLength int) error {
	if userPW == "" && ownerPW == "" {
		return fmt.Errorf("%w: encryption requires a password", ErrMissingParam)
	}
	if keyLength == 0 {
		keyLength = 256
	}
	if keyLength != 128 && keyLength != 256 {
		return fmt.Errorf("%w: key length must be 128 or 256, got %d", ErrInvalidParam, keyLength)
	}
	if ownerPW == "" {
		ownerPW = userPW
	}
	conf := model.NewAESConfiguration(userPW, ownerPW, keyLength)
	if err := api.EncryptFile(inFile, outFile, conf); err != nil {
		return opErr("encrypt", err)
	}
	return nil
}

// Decrypt removes protection from the PDF using the supplied password.
func Decrypt(inFile, outFile, password string) error {
	conf := newConf()
	conf.UserPW = password
	if err := api.DecryptFile(inFile, outFile, conf); err != nil {
		return opErr("decrypt", err)
	}
	return nil
}
