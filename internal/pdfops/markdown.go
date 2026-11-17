package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	_ "image/jpeg"
	_ "image/png"

	"github.com/signintech/gopdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// ErrRenderFailed marks internal rendering failures; the HTTP layer maps it
// to a stable user-facing error while the cause goes to the log only.
var ErrRenderFailed = errors.New("render failed")

const (
	mdFontFamily    = "openpdf"
	defaultFontSize = 11.0
	defaultMarginMM = 18.0
)

// mdPageSizes lists supported paper sizes in portrait points.
var mdPageSizes = map[string]gopdf.Rect{
	"a4":     {W: 595.28, H: 841.89},
	"a3":     {W: 841.89, H: 1190.55},
	"a5":     {W: 419.53, H: 595.28},
	"letter": {W: 612, H: 792},
	"legal":  {W: 612, H: 1008},
}

// ink colors
var (
	mdInk     = [3]uint8{28, 35, 51}
	mdMuted   = [3]uint8{107, 116, 135}
	mdLink    = [3]uint8{43, 110, 246}
	mdCodeInk = [3]uint8{159, 40, 110}
)

// mdLineFactor is the line height multiplier applied to the font size.
const mdLineFactor = 1.62

// MarkdownOptions configures markdown to PDF conversion.
type MarkdownOptions struct {
	// Source is the markdown document (required, non-blank).
	Source []byte
	// PageSize: a4 (default), a3, a5, letter, legal.
	PageSize string
	// Orientation: portrait (default) or landscape.
	Orientation string
	// MarginMM is the page margin in millimeters (default 18).
	MarginMM float64
	// FontSize is the body font size in points (default 11).
	FontSize float64
	// Assets maps file names referenced by the markdown to local file
	// paths (staged image uploads).
	Assets map[string]string
}

// RenderMarkdown converts markdown to a PDF written to outPath.
func RenderMarkdown(outPath string, o MarkdownOptions) error {
	r, err := newMDRenderer(o)
	if err != nil {
		return err
	}
	if err := r.render(); err != nil {
		return &OpError{Op: "markdown render", Err: fmt.Errorf("%w: %v", ErrRenderFailed, err)}
	}
	if err := r.pdf.WritePdf(outPath); err != nil {
		return &OpError{Op: "markdown write", Err: fmt.Errorf("%w: %v", ErrRenderFailed, err)}
	}
	return nil
}

// IsMarkdownFile reports whether the name carries a markdown extension.
func IsMarkdownFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// ---------------------------------------------------------------- renderer

type mdRenderer struct {
	pdf    *gopdf.GoPdf
	opts   MarkdownOptions
	pageW  float64
	pageH  float64
	margin float64 // points
	x0, x1 float64 // current content bounds
	y      float64
	base   float64 // base font size
}

// fillRect draws an opaque filled rectangle (top-left coordinates).
func (r *mdRenderer) fillRect(x, y, w, h float64, rgb [3]uint8) error {
	r.pdf.SetFillColor(rgb[0], rgb[1], rgb[2])
	return r.pdf.RectFromLowerLeftWithOpts(gopdf.DrawableRectOptions{
		Rect:         gopdf.Rect{W: w, H: h},
		X:            x,
		Y:            y + h,
		PaintStyle:   gopdf.FillPaintStyle,
		Transparency: nil,
	})
}

func newMDRenderer(o MarkdownOptions) (*mdRenderer, error) {
	if len(bytes.TrimSpace(o.Source)) == 0 {
		return nil, fmt.Errorf("%w: Markdown 内容为空", ErrMissingParam)
	}
	pageSize := strings.ToLower(strings.TrimSpace(o.PageSize))
	if pageSize == "" {
		pageSize = "a4"
	}
	rect, ok := mdPageSizes[pageSize]
	if !ok {
		return nil, fmt.Errorf("%w: 不支持的纸张 %q，可选 a4/a3/a5/letter/legal", ErrInvalidParam, o.PageSize)
	}
	orientation := strings.ToLower(strings.TrimSpace(o.Orientation))
	if orientation == "" {
		orientation = "portrait"
	}
	if orientation != "portrait" && orientation != "landscape" {
		return nil, fmt.Errorf("%w: orientation 仅支持 portrait 或 landscape", ErrInvalidParam)
	}
	margin := o.MarginMM
	if margin == 0 {
		margin = defaultMarginMM
	}
	if margin < 5 || margin > 50 {
		return nil, fmt.Errorf("%w: margin 需在 5-50 毫米之间", ErrInvalidParam)
	}
	fontSize := o.FontSize
	if fontSize == 0 {
		fontSize = defaultFontSize
	}
	if fontSize < 6 || fontSize > 24 {
		return nil, fmt.Errorf("%w: font_size 需在 6-24 pt 之间", ErrInvalidParam)
	}

	w, h := rect.W, rect.H
	if orientation == "landscape" {
		w, h = h, w
	}
	marginPT := margin * 72 / 25.4

	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{Unit: gopdf.UnitPT, PageSize: gopdf.Rect{W: w, H: h}})
	if err := pdf.AddTTFFontDataWithOption(mdFontFamily, mdFontRegular,
		gopdf.TtfOption{Style: gopdf.Regular}); err != nil {
		return nil, fmt.Errorf("register regular font: %w", err)
	}
	if err := pdf.AddTTFFontDataWithOption(mdFontFamily, mdFontBold,
		gopdf.TtfOption{Style: gopdf.Bold}); err != nil {
		return nil, fmt.Errorf("register bold font: %w", err)
	}
	pdf.AddPage()

	r := &mdRenderer{
		pdf:    pdf,
		opts:   o,
		pageW:  w,
		pageH:  h,
		margin: marginPT,
		base:   fontSize,
	}
	r.x0, r.x1, r.y = marginPT, w-marginPT, marginPT
	return r, nil
}

func (r *mdRenderer) render() error {
	src := r.opts.Source
	md := goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Linkify))
	doc := md.Parser().Parse(text.NewReader(src))
	return r.renderBlocks(doc)
}

func (r *mdRenderer) contentW() float64 { return r.x1 - r.x0 }

// ensureSpace starts a new page when the remaining height is insufficient.
func (r *mdRenderer) ensureSpace(h float64) {
	if r.y+h > r.pageH-r.margin {
		r.pdf.AddPage()
		r.y = r.margin
	}
}

func (r *mdRenderer) space(h float64) { r.y += h }

// ---------------------------------------------------------------- blocks

func (r *mdRenderer) renderBlocks(parent ast.Node) error {
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		if err := r.renderBlock(c); err != nil {
			return err
		}
	}
	return nil
}

func (r *mdRenderer) renderBlock(n ast.Node) error {
	src := r.opts.Source
	switch node := n.(type) {
	case *ast.Heading:
		return r.heading(node)
	case *ast.Paragraph, *ast.TextBlock:
		return r.paragraph(n)
	case *ast.List:
		return r.list(node)
	case *ast.Blockquote:
		return r.blockquote(node)
	case *ast.CodeBlock:
		return r.codeBlock(node.Lines(), "")
	case *ast.FencedCodeBlock:
		lang := ""
		if node.Info != nil {
			lang = strings.TrimSpace(string(node.Info.Text(src)))
		}
		return r.codeBlock(node.Lines(), lang)
	case *ast.ThematicBreak:
		return r.thematicBreak()
	case *ast.HTMLBlock:
		return nil // raw HTML is intentionally not rendered
	case *gast.Table:
		return r.table(node)
	default:
		if n.HasChildren() {
			return r.renderBlocks(n)
		}
		return nil
	}
}

func (r *mdRenderer) heading(h *ast.Heading) error {
	level := h.Level
	scale := []float64{2.0, 1.5, 1.25, 1.1, 1.0, 0.92}[min(level, 6)-1]
	size := r.base * scale
	lineH := size * 1.35

	items := r.collectInline(h, spanStyle{})
	plain := plainText(items)

	r.space(r.base * 0.45)
	r.ensureSpace(lineH + r.base*0.3)
	if level <= 2 {
		r.pdf.AddOutline(plain)
	}
	r.y += lineH - size // lead so the baseline distance feels right
	if err := r.writeInline(items, size, mdInk); err != nil {
		return err
	}
	r.space(r.base * 0.28)

	if level == 1 || level == 2 { // subtle rule under big headings
		r.pdf.SetStrokeColor(203, 210, 222)
		r.pdf.SetLineWidth(0.7)
		r.pdf.Line(r.x0, r.y+2, r.x1, r.y+2)
		r.space(5)
	}
	return nil
}

func (r *mdRenderer) paragraph(n ast.Node) error {
	items := r.collectInline(n, spanStyle{})
	if len(items) == 0 {
		return nil
	}
	r.space(r.base * 0.3)
	if err := r.writeInline(items, r.base, mdInk); err != nil {
		return err
	}
	r.space(r.base * 0.3)
	return nil
}

func (r *mdRenderer) list(l *ast.List) error {
	idx := 1
	if l.Start != 0 {
		idx = l.Start
	}
	for item := l.FirstChild(); item != nil; item = item.NextSibling() {
		marker := "•"
		if l.IsOrdered() {
			marker = strconv.Itoa(idx) + "."
		}
		idx++

		r.space(r.base * 0.22)
		r.ensureSpace(r.base * mdLineFactor)

		// Marker on the first line; children flow with a hanging indent.
		mw, err := r.textWidth(marker, false, r.base)
		if err != nil {
			return err
		}
		oldX0 := r.x0
		r.pdf.SetFontWithStyle(mdFontFamily, gopdf.Regular, r.base)
		r.pdf.SetTextColor(mdInk[0], mdInk[1], mdInk[2])
		r.pdf.SetXY(r.x0, r.y)
		_ = r.pdf.Cell(nil, marker)
		r.x0 = oldX0 + mw + 7

		if err := r.renderBlocks(item); err != nil {
			r.x0 = oldX0
			return err
		}
		r.x0 = oldX0
	}
	r.space(r.base * 0.1)
	return nil
}

func (r *mdRenderer) blockquote(bq *ast.Blockquote) error {
	oldX0, yStart := r.x0, r.y
	r.x0 = oldX0 + 16
	r.space(r.base * 0.25)
	if err := r.renderBlocks(bq); err != nil {
		r.x0 = oldX0
		return err
	}
	r.space(r.base * 0.25)
	r.x0 = oldX0

	// Vertical bar along the quoted region.
	h := r.y - yStart
	if h > 0 {
		if err := r.fillRect(oldX0+3, yStart, 3, h, [3]uint8{203, 210, 222}); err != nil {
			return err
		}
	}
	return nil
}

func (r *mdRenderer) codeBlock(lines *text.Segments, lang string) error {
	if lang != "" {
		r.space(r.base * 0.1)
		r.ensureSpace(r.base * 0.8)
		r.pdf.SetFontWithStyle(mdFontFamily, gopdf.Regular, r.base*0.78)
		r.pdf.SetTextColor(mdMuted[0], mdMuted[1], mdMuted[2])
		r.pdf.SetXY(r.x1-r.textWidthOr(lang, r.base*0.78), r.y)
		_ = r.pdf.Cell(nil, lang)
		r.space(r.base * 0.85)
	}
	r.space(r.base * 0.2)

	size := r.base * 0.92
	lineH := size * 1.5
	pad := 7.0
	var content []string
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		for _, ln := range strings.Split(strings.TrimRight(string(seg.Value(r.opts.Source)), "\n"), "\n") {
			content = append(content, ln)
		}
	}
	// Greedy wrap long code lines at spaces/CJK boundaries.
	var wrapped []string
	for _, ln := range content {
		wrapped = append(wrapped, r.wrapPlainText(ln, size)...)
	}
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}

	topPad := pad
	for i, ln := range wrapped {
		if i == 0 {
			r.ensureSpace(topPad + lineH)
			r.y += topPad
		}
		r.ensureSpace(lineH)
		if err := r.fillRect(r.x0, r.y, r.contentW(), lineH, [3]uint8{242, 244, 247}); err != nil {
			return err
		}
		r.pdf.SetFontWithStyle(mdFontFamily, gopdf.Regular, size)
		r.pdf.SetTextColor(mdInk[0], mdInk[1], mdInk[2])
		r.pdf.SetXY(r.x0+pad*0.8, r.y)
		if ln != "" {
			_ = r.pdf.Cell(nil, ln)
		}
		r.y += lineH
	}
	r.y += pad
	r.space(r.base * 0.2)
	return nil
}

func (r *mdRenderer) thematicBreak() error {
	r.space(r.base * 0.5)
	r.ensureSpace(10)
	r.pdf.SetStrokeColor(203, 210, 222)
	r.pdf.SetLineWidth(0.8)
	r.pdf.Line(r.x0, r.y, r.x1, r.y)
	r.space(4)
	r.space(r.base * 0.5)
	return nil
}

func (r *mdRenderer) table(t *gast.Table) error {
	var header [][]inlineItem
	var rows [][][]inlineItem
	for section := t.FirstChild(); section != nil; section = section.NextSibling() {
		for row := section.FirstChild(); row != nil; row = row.NextSibling() {
			var cells [][]inlineItem
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				cells = append(cells, r.collectInline(cell, spanStyle{}))
			}
			if _, ok := section.(*gast.TableHeader); ok {
				header = cells
			} else {
				rows = append(rows, cells)
			}
		}
	}
	if len(header) == 0 && len(rows) == 0 {
		return nil
	}

	// Column widths: natural single-line width, shrunk to fit.
	nCols := max(len(header), maxCellCols(rows))
	natural := make([]float64, nCols)
	for j := 0; j < nCols; j++ {
		if j < len(header) {
			if w, err := r.inlineWidth(header[j], r.base, true); err == nil {
				natural[j] = max(natural[j], w)
			}
		}
		for _, row := range rows {
			if j < len(row) {
				if w, err := r.inlineWidth(row[j], r.base, false); err == nil {
					natural[j] = max(natural[j], w)
				}
			}
		}
		natural[j] = max(natural[j], 36) // minimal column width
	}
	total := 0.0
	for _, w := range natural {
		total += w
	}
	scale := 1.0
	if avail := r.contentW() - 12; total > avail && total > 0 {
		scale = avail / total
	}
	colW := make([]float64, nCols)
	for j, w := range natural {
		colW[j] = w * scale
	}

	size := r.base * 0.95
	lineH := size * mdLineFactor
	padY := 4.0

	drawRow := func(cells [][]inlineItem, headerRow bool, widths []float64) error {
		// Pre-wrap every cell so the row height is known before drawing.
		var cellLines [][][]token
		rowLines := 1
		for j := 0; j < len(widths); j++ {
			var its []inlineItem
			if j < len(cells) {
				its = cells[j]
			}
			lines := r.wrapInline(its, size, widths[j]-10)
			cellLines = append(cellLines, lines)
			rowLines = max(rowLines, len(lines))
		}
		rowH := float64(rowLines)*lineH + padY*2
		r.ensureSpace(rowH)
		x := r.x0
		if headerRow {
			if err := r.fillRect(x, r.y, r.contentW(), rowH, [3]uint8{238, 241, 246}); err != nil {
				return err
			}
		}
		for j, lines := range cellLines {
			cy := r.y + padY
			for _, line := range lines {
				if err := r.drawLine(line, size, x+5, cy, mdInk, headerRow); err != nil {
					return err
				}
				cy += lineH
			}
			x += widths[j]
		}
		// Grid.
		r.pdf.SetStrokeColor(200, 207, 218)
		r.pdf.SetLineWidth(0.5)
		r.pdf.Line(r.x0, r.y, r.x0+r.contentW(), r.y)
		r.pdf.Line(r.x0, r.y+rowH, r.x0+r.contentW(), r.y+rowH)
		gx := r.x0
		for _, w := range widths {
			r.pdf.Line(gx, r.y, gx, r.y+rowH)
			gx += w
		}
		r.pdf.Line(gx, r.y, gx, r.y+rowH)
		r.y += rowH
		return nil
	}

	r.space(r.base * 0.3)
	if len(header) > 0 {
		if err := drawRow(header, true, colW); err != nil {
			return err
		}
	}
	for _, row := range rows {
		if err := drawRow(row, false, colW); err != nil {
			return err
		}
	}
	r.space(r.base * 0.3)
	return nil
}

func maxCellCols(rows [][][]inlineItem) int {
	m := 0
	for _, row := range rows {
		m = max(m, len(row))
	}
	return m
}

// ---------------------------------------------------------------- inline

type spanStyle struct {
	bold   bool
	link   string
	code   bool
	strike bool
}

type inlineItem struct {
	text  string
	style spanStyle
	img   string // non-empty: image destination
	alt   string
	brk   bool // hard line break
}

func (r *mdRenderer) collectInline(n ast.Node, st spanStyle) []inlineItem {
	var out []inlineItem
	var walk func(n ast.Node, st spanStyle)
	walk = func(n ast.Node, st spanStyle) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch t := c.(type) {
			case *ast.Text:
				s := string(t.Segment.Value(r.opts.Source))
				if t.SoftLineBreak() {
					s += " "
				}
				out = append(out, inlineItem{text: s, style: st})
				if t.HardLineBreak() {
					out = append(out, inlineItem{brk: true})
				}
			case *ast.String:
				out = append(out, inlineItem{text: string(t.Value), style: st})
			case *ast.CodeSpan:
				var sb strings.Builder
				for cc := t.FirstChild(); cc != nil; cc = cc.NextSibling() {
					if tt, ok := cc.(*ast.Text); ok {
						sb.Write(tt.Segment.Value(r.opts.Source))
					}
				}
				st.code = true
				out = append(out, inlineItem{text: sb.String(), style: st})
				st.code = false
			case *ast.Emphasis:
				st2 := st
				if t.Level == 2 {
					st2.bold = true
				}
				walk(t, st2)
			case *gast.Strikethrough:
				st2 := st
				st2.strike = true
				walk(t, st2)
			case *ast.Link:
				st2 := st
				st2.link = string(t.Destination)
				walk(t, st2)
			case *ast.AutoLink:
				url := string(t.URL(r.opts.Source))
				out = append(out, inlineItem{text: url, style: spanStyle{link: url}})
			case *ast.Image:
				alt := plainText(r.collectInline(t, spanStyle{}))
				out = append(out, inlineItem{
					img:   string(t.Destination),
					alt:   alt,
					style: st,
				})
			default:
				walk(t, st)
			}
		}
	}
	walk(n, st)
	return out
}

func plainText(items []inlineItem) string {
	var sb strings.Builder
	for _, it := range items {
		if it.img != "" {
			continue
		}
		sb.WriteString(it.text)
	}
	return strings.TrimSpace(sb.String())
}

// token is an atomic, wrappable piece of a line.
type token struct {
	text  string
	style spanStyle
	w     float64
}

// writeInline lays out items with wrapping, images, links and page breaks.
func (r *mdRenderer) writeInline(items []inlineItem, size float64, color [3]uint8) error {
	lineH := size * mdLineFactor

	var line []token
	flush := func() error {
		if len(line) == 0 {
			return nil
		}
		if err := r.drawLine(line, size, r.x0, r.y, color, false); err != nil {
			return err
		}
		line = line[:0]
		return nil
	}

	for _, it := range items {
		if it.brk {
			if err := flush(); err != nil {
				return err
			}
			r.y += lineH
			continue
		}
		if it.img != "" {
			if err := flush(); err != nil {
				return err
			}
			r.y += lineH // images sit on their own line
			if err := r.drawImage(it.img, it.alt); err != nil {
				return err
			}
			continue
		}
		for _, tok := range r.tokenize(it.text, it.style, size) {
			if len(line) > 0 && r.measureLineWidth(line)+tok.w > r.contentW() {
				if err := flush(); err != nil {
					return err
				}
				r.y += lineH
			}
			if len(line) == 0 && tok.text == " " {
				continue // no leading spaces after a wrap
			}
			line = append(line, tok)
		}
	}
	if err := flush(); err != nil {
		return err
	}
	if len(items) > 0 {
		r.y += lineH
	}
	return nil
}

// tokenize splits text into words (kept intact) and CJK characters (broken
// freely), pre-measuring each token with its style.
func (r *mdRenderer) tokenize(s string, st spanStyle, size float64) []token {
	var toks []token
	var word []rune
	flushWord := func() {
		if len(word) > 0 {
			t := string(word)
			toks = append(toks, token{text: t, style: st, w: r.widthOr(t, st.bold, size)})
			word = word[:0]
		}
	}
	for _, rn := range s {
		switch {
		case unicode.IsSpace(rn):
			flushWord()
			toks = append(toks, token{text: " ", style: st, w: r.widthOr(" ", st.bold, size)})
		case breakableRune(rn):
			flushWord()
			t := string(rn)
			toks = append(toks, token{text: t, style: st, w: r.widthOr(t, st.bold, size)})
		default:
			word = append(word, rn)
		}
	}
	flushWord()
	return toks
}

func breakableRune(rn rune) bool {
	return unicode.Is(unicode.Han, rn) ||
		unicode.Is(unicode.Hiragana, rn) ||
		unicode.Is(unicode.Katakana, rn) ||
		unicode.Is(unicode.Hangul, rn) ||
		(rn >= 0x3000 && rn <= 0x303F) || // CJK punctuation
		(rn >= 0xFF00 && rn <= 0xFFEF) // fullwidth forms
}

// drawLine draws one prepared line of tokens at x,y. When the line does not
// fit on the current page it starts a new page and draws at the top margin.
func (r *mdRenderer) drawLine(toks []token, size float64, x, y float64, color [3]uint8, bold bool) error {
	if len(toks) == 0 {
		return nil
	}
	lineH := size * mdLineFactor
	if y+lineH > r.pageH-r.margin {
		r.pdf.AddPage()
		r.y = r.margin
		y = r.margin
	}
	for _, tok := range toks {
		style := gopdf.Regular
		col := color
		if bold || tok.style.bold {
			style = gopdf.Bold
		}
		switch {
		case tok.style.link != "":
			col = mdLink
		case tok.style.code:
			col = mdCodeInk
		}
		r.pdf.SetFontWithStyle(mdFontFamily, style, size)
		r.pdf.SetTextColor(col[0], col[1], col[2])
		r.pdf.SetXY(x, y)
		if err := r.pdf.Cell(nil, tok.text); err != nil {
			return err
		}
		if tok.style.link != "" {
			r.pdf.AddExternalLink(tok.style.link, x, y, tok.w, size*1.25)
		}
		if tok.style.strike {
			r.pdf.SetStrokeColor(col[0], col[1], col[2])
			r.pdf.SetLineWidth(size * 0.06)
			r.pdf.Line(x, y+size*0.55, x+tok.w, y+size*0.55)
		}
		x += tok.w
	}
	return nil
}

// wrapInline wraps items to a pixel budget (table cells) and returns lines
// of tokens.
func (r *mdRenderer) wrapInline(items []inlineItem, size float64, maxW float64) [][]token {
	var lines [][]token
	var line []token
	for _, it := range items {
		if it.img != "" || it.brk {
			continue // images and hard breaks do not occur in cells
		}
		for _, tok := range r.tokenize(it.text, it.style, size) {
			if len(line) > 0 && r.measureLineWidth(line)+tok.w > maxW {
				lines = append(lines, line)
				line = line[:0]
			}
			if len(line) == 0 && tok.text == " " {
				continue
			}
			line = append(line, tok)
		}
	}
	if len(line) > 0 {
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, nil)
	}
	return lines
}

// wrapPlainText wraps a raw string with regular weight.
func (r *mdRenderer) wrapPlainText(s string, size float64) []string {
	var out []string
	var cur strings.Builder
	curW := 0.0
	for _, tok := range r.tokenize(s, spanStyle{}, size) {
		if curW > 0 && curW+tok.w > r.contentW()-12 {
			out = append(out, strings.TrimRight(cur.String(), " "))
			cur.Reset()
			curW = 0
			if tok.text == " " {
				continue
			}
		}
		cur.WriteString(tok.text)
		curW += tok.w
	}
	if s == "" {
		return []string{""}
	}
	out = append(out, strings.TrimRight(cur.String(), " "))
	return out
}

func (r *mdRenderer) measureLineWidth(toks []token) float64 {
	w := 0.0
	for _, t := range toks {
		w += t.w
	}
	return w
}

func (r *mdRenderer) inlineWidth(items []inlineItem, size float64, bold bool) (float64, error) {
	return r.textWidth(plainText(items), bold, size)
}

func (r *mdRenderer) textWidth(s string, bold bool, size float64) (float64, error) {
	style := gopdf.Regular
	if bold {
		style = gopdf.Bold
	}
	if err := r.pdf.SetFontWithStyle(mdFontFamily, style, size); err != nil {
		return 0, err
	}
	return r.pdf.MeasureTextWidth(s)
}

func (r *mdRenderer) widthOr(s string, bold bool, size float64) float64 {
	w, err := r.textWidth(s, bold, size)
	if err != nil {
		return 0
	}
	return w
}

func (r *mdRenderer) textWidthOr(s string, size float64) float64 {
	return r.widthOr(s, false, size)
}

// ---------------------------------------------------------------- images

func (r *mdRenderer) drawImage(dest, alt string) error {
	data, err := r.loadAsset(dest)
	if err != nil {
		return r.writeInline([]inlineItem{{
			text:  "〔图片缺失 " + dest + "〕",
			style: spanStyle{},
		}}, r.base*0.9, mdMuted)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return r.writeInline([]inlineItem{{
			text:  "〔无法识别的图片 " + dest + "〕",
			style: spanStyle{},
		}}, r.base*0.9, mdMuted)
	}
	wPt := float64(cfg.Width) * 72 / 96
	hPt := float64(cfg.Height) * 72 / 96
	scale := 1.0
	if wPt > r.contentW() {
		scale = r.contentW() / wPt
	}
	dw, dh := wPt*scale, hPt*scale
	r.ensureSpace(dh + 6)
	holder, err := gopdf.ImageHolderByBytes(data)
	if err != nil {
		return err
	}
	if err := r.pdf.ImageByHolder(holder, r.x0, r.y, &gopdf.Rect{W: dw, H: dh}); err != nil {
		return err
	}
	r.y += dh + 6
	if alt != "" {
		return r.writeInline([]inlineItem{{text: alt, style: spanStyle{}}}, r.base*0.82, mdMuted)
	}
	return nil
}

// loadAsset resolves a markdown destination against the staged uploads.
func (r *mdRenderer) loadAsset(dest string) ([]byte, error) {
	if dest == "" {
		return nil, os.ErrNotExist
	}
	candidates := []string{
		dest,
		filepath.Base(dest),
		strings.TrimPrefix(dest, "./"),
	}
	known := make(map[string]string, len(r.opts.Assets))
	for k, v := range r.opts.Assets {
		known[k] = v
		known[filepath.Base(k)] = v
	}
	for _, c := range candidates {
		if p, ok := known[c]; ok {
			return os.ReadFile(p)
		}
	}
	return nil, os.ErrNotExist
}
