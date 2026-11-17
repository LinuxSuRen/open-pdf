package pdfops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linuxsuren/open-pdf/internal/testutil"
)

// fullDoc exercises every supported markdown construct.
const fullDoc = `# open-pdf 接入指南

这是一段中文与 English 混排的说明文字，用来验证自动换行、**加粗**、*强调*、~~删除线~~、
行内代码 ` + "`make build`" + ` 以及链接 [pdfcpu](https://github.com/pdfcpu/pdfcpu) 的渲染效果。

## 环境准备

1. 安装 Go 1.27 或更高版本
2. 准备 Node 20+ 用于构建前端
3. 执行 make build 生成二进制

### 常用命令

- ` + "`make test`" + ` 运行全部测试
- ` + "`make web`" + ` 重新构建前端
  - 嵌入产物位于 web/dist
  - 修改界面后需要重新编译
- ` + "`make docker-build`" + ` 构建容器镜像

> 注意：容器镜像为三阶段构建，运行层是 scratch，不包含 shell。
> 引用块支持多行内容，左侧有竖线标识。

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| 合并 | 已支持 | PDF 与图片混合 |
| 分割 | 已支持 | burst / span / ranges |
| Markdown | 已支持 | 本文档所演示的能力 |

~~~go
func main() {
    // 代码块带浅色背景，长行会自动折行
    fmt.Println("你好，open-pdf！Hello from a fenced code block with a long line that should wrap when it exceeds the content width limit")
}
~~~

---

删除线 ~~废弃接口~~ 不再可用。下面是一段很长的重复文字用于触发换行与分页：
路面结冰导致多车连撞，相关部门提醒驾驶员减速慢行保持车距。路面结冰导致多车连撞，相关部门提醒驾驶员减速慢行保持车距。
路面结冰导致多车连撞，相关部门提醒驾驶员减速慢行保持车距。路面结冰导致多车连撞，相关部门提醒驾驶员减速慢行保持车距。
The quick brown fox jumps over the lazy dog again and again and again and again and again and again and again and again.
`

func TestRenderMarkdownFullDoc(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "doc.pdf")
	if err := RenderMarkdown(out, MarkdownOptions{Source: []byte(fullDoc)}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("output is not a valid PDF: %v", err)
	}
	n, _ := PageCount(out)
	if n < 2 {
		t.Fatalf("expected the full document to span at least 2 pages, got %d", n)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() > 600<<10 {
		t.Fatalf("output unexpectedly large: %d bytes (font subsetting broken?)", st.Size())
	}
	t.Logf("full doc: %d pages, %d bytes", n, st.Size())
}

func TestRenderMarkdownImageFromAssets(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "shot.png")
	if err := testutil.WritePNGFile(img, 120, 80); err != nil {
		t.Fatal(err)
	}

	without := filepath.Join(dir, "noimg.pdf")
	with := filepath.Join(dir, "img.pdf")

	src := []byte("# 截图说明\n\n![界面截图](shot.png)\n\n补充文字。\n")
	if err := RenderMarkdown(without, MarkdownOptions{Source: src}); err != nil {
		t.Fatal(err)
	}
	if err := RenderMarkdown(with, MarkdownOptions{
		Source: src,
		Assets: map[string]string{"shot.png": img},
	}); err != nil {
		t.Fatalf("render with image: %v", err)
	}

	assertValidPages(t, with, 1)
	ws, err := os.Stat(without)
	if err != nil {
		t.Fatal(err)
	}
	wi, err := os.Stat(with)
	if err != nil {
		t.Fatal(err)
	}
	if wi.Size() <= ws.Size() {
		t.Fatalf("image not embedded: %d vs %d bytes", wi.Size(), ws.Size())
	}

	// Missing image degrades to a placeholder instead of failing.
	if err := RenderMarkdown(without, MarkdownOptions{
		Source: []byte("![不存在](nope.png)\n"),
	}); err != nil {
		t.Fatalf("missing image should not fail the render: %v", err)
	}
	assertValidPages(t, without, 1)
}

func TestRenderMarkdownOptions(t *testing.T) {
	dir := t.TempDir()

	landscape := filepath.Join(dir, "land.pdf")
	if err := RenderMarkdown(landscape, MarkdownOptions{
		Source:      []byte("# 横向\n\n内容"),
		Orientation: "landscape",
	}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(landscape); err != nil {
		t.Fatal(err)
	}
	dims := pageDimsOf(t, landscape)
	for _, d := range dims {
		if d.Width <= d.Height {
			t.Fatalf("landscape page is portrait: %vx%v", d.Width, d.Height)
		}
	}

	letter := filepath.Join(dir, "letter.pdf")
	if err := RenderMarkdown(letter, MarkdownOptions{
		Source:   []byte("# Letter"),
		PageSize: "letter",
	}); err != nil {
		t.Fatal(err)
	}
	dims = pageDimsOf(t, letter)
	if len(dims) == 0 {
		t.Fatal("no page dimensions")
	}
	if got := dims[0]; got.Width != 612 || got.Height != 792 {
		t.Fatalf("letter dims = %vx%v, want 612x792", got.Width, got.Height)
	}
}

func TestRenderMarkdownInvalidOptions(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		opts MarkdownOptions
	}{
		{"empty source", MarkdownOptions{}},
		{"blank source", MarkdownOptions{Source: []byte("   \n \t")}},
		{"bad page size", MarkdownOptions{Source: []byte("x"), PageSize: "b6"}},
		{"bad orientation", MarkdownOptions{Source: []byte("x"), Orientation: "diagonal"}},
		{"margin too small", MarkdownOptions{Source: []byte("x"), MarginMM: 1}},
		{"font too big", MarkdownOptions{Source: []byte("x"), FontSize: 72}},
	}
	for _, tc := range cases {
		err := RenderMarkdown(filepath.Join(dir, "o.pdf"), tc.opts)
		if err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
		if !strings.Contains(err.Error(), "invalid parameter") && !strings.Contains(err.Error(), "missing parameter") {
			t.Fatalf("%s: unexpected error %v", tc.name, err)
		}
	}
}

func TestRenderMarkdownIsMarkdownFile(t *testing.T) {
	if !IsMarkdownFile("README.md") || !IsMarkdownFile("notes.MARKDOWN") {
		t.Fatal("markdown extensions not recognized")
	}
	if IsMarkdownFile("a.pdf") || IsMarkdownFile("b.png") {
		t.Fatal("non-markdown accepted")
	}
}
