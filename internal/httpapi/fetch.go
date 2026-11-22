package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"
)

// Limits for server-side markdown downloads: markdown documents are small,
// so a tight cap keeps a bad link from pinning memory or a connection.
const (
	mdFetchTimeout  = 20 * time.Second
	mdMaxFetchBytes = 8 << 20
)

var mdFetchClient = &http.Client{Timeout: mdFetchTimeout}

// mdFetchError maps a markdown link failure to a user-facing response;
// cause carries the technical detail and goes to the log only.
type mdFetchError struct {
	status int
	code   string
	msg    string
	cause  error
}

func (e *mdFetchError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.msg, e.cause)
	}
	return e.msg
}

func fetchErr(status int, code, msg string, cause error) *mdFetchError {
	return &mdFetchError{status: status, code: code, msg: msg, cause: cause}
}

// fetchMarkdown downloads the markdown document referenced by rawURL and
// derives a display name from the URL path. Failures come back as
// *mdFetchError; blank bodies are left to RenderMarkdown to reject.
func fetchMarkdown(ctx context.Context, rawURL string) ([]byte, string, error) {
	u, perr := url.Parse(rawURL)
	if perr != nil || u.Host == "" {
		return nil, "", fetchErr(http.StatusBadRequest, "bad_url",
			"文件链接格式不正确，需要 http:// 或 https:// 开头的完整链接", perr)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", fetchErr(http.StatusBadRequest, "bad_url",
			"文件链接仅支持 http 与 https 协议", nil)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", fetchErr(http.StatusBadRequest, "bad_url",
			"文件链接格式不正确", err)
	}
	req.Header.Set("User-Agent", "open-pdf")
	resp, err := mdFetchClient.Do(req)
	if err != nil {
		return nil, "", fetchErr(http.StatusBadGateway, "fetch_failed",
			"无法下载链接指向的文件，请确认链接无需登录即可访问后重试", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fetchErr(http.StatusBadGateway, "fetch_failed",
			"无法下载链接指向的文件，请确认链接无需登录即可访问后重试",
			fmt.Errorf("HTTP %s", resp.Status))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, mdMaxFetchBytes+1))
	if err != nil {
		return nil, "", fetchErr(http.StatusBadGateway, "fetch_failed",
			"下载链接文件时中断，请重试", err)
	}
	if len(data) > mdMaxFetchBytes {
		return nil, "", fetchErr(http.StatusRequestEntityTooLarge, "payload_too_large",
			fmt.Sprintf("链接指向的文件超过 %d MB 上限", mdMaxFetchBytes>>20), nil)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, "", fetchErr(http.StatusBadRequest, "empty_document",
			"链接指向的文件没有内容", nil)
	}
	return data, mdLinkName(u), nil
}

// mdLinkName derives a display name from the URL path, e.g.
// https://example.com/docs/intro.md → intro.md; empty when the path has no
// usable base name.
func mdLinkName(u *url.URL) string {
	base := path.Base(u.Path)
	if dec, err := url.PathUnescape(base); err == nil {
		base = dec
	}
	if base == "" || base == "/" || base == "." {
		return ""
	}
	return safeBaseName(base)
}
