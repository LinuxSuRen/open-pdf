# open-pdf

基于 [pdfcpu](https://github.com/pdfcpu/pdfcpu) 的 PDF 工具箱服务：**Go 后端 + Vue 3 Web 界面**，一个容器即可部署，提供合并、分割、压缩、水印、旋转、加密等常用 PDF 操作的 REST API。

- 纯 Go 实现，无外部运行时依赖（无 LibreOffice / ImageMagick / Python）
- 自带 Web 界面（Vue 3 构建，嵌入二进制，离线可用）
- 单文件 + 单容器部署，支持 amd64 / arm64
- 完整 REST API，可独立于界面集成到你的系统

## 功能

| 功能 | 说明 |
| --- | --- |
| 合并 | 多个 PDF 按顺序合并；PDF 与图片（PNG/JPG/WebP/TIFF）混合合并为一个 PDF，图片自动转为 A4 页面 |
| 分割 | 每页一个文件 / 每 N 页一个文件 / 自定义页码范围（`1-3,5,8-`）；多结果自动打包 ZIP |
| 压缩 | 无损结构优化：对象流 / xref 流压缩、重复内容流去重 |
| 水印 | 文字水印（透明度、旋转、字号、颜色、方向、位置、指定页码）与图片水印 |
| 旋转 | 按 90° 整数倍旋转全部或指定页面 |
| 信息 | 页数、PDF 版本、页面尺寸、元数据、加密 / 水印 / 书签状态 |
| 加密 / 解密 | AES-128 / AES-256 口令保护与解除 |
| Markdown 转 PDF | 标题、列表、表格、代码块、引用、链接、图片，内嵌 Noto Sans SC 中文字体，子集化后文档仅几十 KB |

## 快速开始

### Docker（推荐）

```bash
docker build -t open-pdf:dev .
docker run --rm -p 8080:8080 --tmpfs /tmp open-pdf:dev
```

浏览器打开 <http://localhost:8080/> 即可使用 Web 界面。

或使用 compose：

```bash
docker compose up -d --build
```

### 源码运行

```bash
make build   # 需要先安装 Go 1.27+
./bin/open-pdf
```

重新构建前端（改动 `web/` 后）：`make web`（需要 Node 20+）。

也可以直接从 [GitHub Releases](https://github.com/linuxsuren/open-pdf/releases) 下载对应平台的
二进制（linux/darwin × amd64/arm64、windows/amd64，发布时由 CI 自动构建并附带 sha256 校验）。

## Web 界面

界面提供全部功能的操作面板：拖拽上传、文件排序、压缩前后体积对比、
PDF 信息卡片、API 令牌配置（右上角「接入令牌」，保存在浏览器本地）。

开发模式（前端热更新，代理到本地 8080 后端）：

```bash
./bin/open-pdf &          # 后端
cd web && npm install && npm run dev   # 前端 http://localhost:5173
```

## REST API

所有操作端点均为 `POST multipart/form-data`，返回结果文件（`application/pdf` 或 `application/zip`）或 JSON。错误统一为：

```json
{ "error": { "code": "invalid_pdf", "message": "..." } }
```

| 状态码 | 含义 |
| --- | --- |
| 400 | 缺少参数 / 请求格式错误 |
| 401 | 未携带或错误的 Bearer Token（启用鉴权时） |
| 405 | HTTP 方法错误 |
| 413 | 超出上传体积 / 文件数量限制 |
| 415 | 不支持的文件类型 |
| 422 | PDF 无效 / 页码范围错误等 |

### POST /api/v1/merge — 合并

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `files` | file（可多个） | PDF 或图片，按上传顺序合并 |
| `output` | string（可选） | 输出文件名，默认 `merged.pdf` |

```bash
curl -o merged.pdf -F files=@a.pdf -F files=@b.pdf -F files=@photo.png \
  http://localhost:8080/api/v1/merge
```

### POST /api/v1/split — 分割

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `file` | file | 待分割的 PDF |
| `mode` | string | `burst`（默认，每页一文件）/ `span` / `ranges` |
| `span` | int | `mode=span` 时每份页数 |
| `ranges` | string | `mode=ranges` 时的范围，如 `1-3,5,8-`（支持开区间） |

```bash
# 每页一个文件，多文件返回 ZIP
curl -o split.zip -F file=@doc.pdf http://localhost:8080/api/v1/split

# 自定义范围（返回单个 PDF）
curl -o part.pdf -F file=@doc.pdf -F mode=ranges -F ranges=2-5 \
  http://localhost:8080/api/v1/split
```

### POST /api/v1/compress — 压缩

```bash
curl -o small.pdf -F file=@big.pdf http://localhost:8080/api/v1/compress
```

### POST /api/v1/watermark — 水印

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `file` | file | 目标 PDF |
| `type` | string | `text`（默认）/ `image` |
| `text` | string | 水印文字（`type=text` 必填） |
| `image` | file | 水印图片（`type=image` 必填，PNG/JPG 等） |
| `pages` | string | 作用页码，如 `1-3,5`，默认全部 |
| `opacity` | float | 透明度 (0,1]，建议 0.2 ~ 0.5 |
| `rotation` | float | 旋转角度 |
| `fontsize` | float | 字号（pt） |
| `color` | string | `#RRGGBB` 或颜色名（gray/red/...） |
| `position` | string | `c`/`tl`/`tr`/`bl`/`br` 等锚点 |
| `diagonal` | int | 1 斜向↗（默认）/ 2 斜向↘ / 3 水平 |
| `on_top` | bool | 是否覆盖在内容之上（默认垫在内容下） |

```bash
curl -o wm.pdf -F file=@doc.pdf -F text='机密 CONFIDENTIAL' \
  -F opacity=0.3 -F fontsize=48 -F color=#FF0000 \
  http://localhost:8080/api/v1/watermark
```

### POST /api/v1/rotate — 旋转

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `file` | file | 目标 PDF |
| `angle` | int | 必填，±90 / ±180 / ±270（逆时针为正） |
| `pages` | string | 作用页码，默认全部 |

```bash
curl -o rot.pdf -F file=@doc.pdf -F angle=90 http://localhost:8080/api/v1/rotate
```

### POST /api/v1/info — 信息（JSON）

```bash
curl -F file=@doc.pdf http://localhost:8080/api/v1/info
```

### POST /api/v1/encrypt — 加密

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `file` | file | 目标 PDF |
| `password` | string | 打开密码（必填） |
| `owner` | string | 所有者密码（可选，默认同打开密码） |
| `key_length` | int | `256`（默认）/ `128` |

```bash
curl -o enc.pdf -F file=@doc.pdf -F password=s3cret \
  http://localhost:8080/api/v1/encrypt
```

### POST /api/v1/decrypt — 解密

```bash
curl -o dec.pdf -F file=@enc.pdf -F password=s3cret \
  http://localhost:8080/api/v1/decrypt
```

### POST /api/v1/markdown — Markdown 转 PDF

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `file` | file | `.md` 文档，与 `text` 二选一，文件优先 |
| `text` | string | Markdown 文本内容 |
| file 任意字段名 | file | 引用的图片资源，按文件名匹配 `![](shot.png)` |
| `page_size` | string | `a4` 默认 / `a3` / `a5` / `letter` / `legal` |
| `orientation` | string | `portrait` 默认 / `landscape` |
| `margin` | float | 页边距毫米，默认 18 |
| `font_size` | float | 正文字号磅，默认 11 |
| `output` | string | 下载文件名，默认取自文档名 |

支持 CommonMark 与 GFM 表格 / 删除线；内嵌 Noto Sans SC（OFL 1.1），字体子集化后输出通常只有几十 KB；链接可点击，图片缺失时以占位文字降级。

```bash
# 文本直转
curl -o doc.pdf -F text='# 你好
正文支持 **加粗** 与 [链接](https://github.com/LinuxSuRen/open-pdf)' \
  http://localhost:8080/api/v1/markdown

# 上传 .md 并内嵌图片
curl -o doc.pdf -F file=@README.md -F asset=@screenshot.png \
  http://localhost:8080/api/v1/markdown
```

### 元端点

- `GET /healthz` — 健康检查（始终无需鉴权，供容器探针使用）
- `GET /api/v1/version` — 版本信息
- `GET /api` — 端点列表

## 配置

全部通过环境变量配置，均有默认值：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `OPEN_PDF_ADDR` | `:8080` | 监听地址 |
| `OPEN_PDF_API_TOKEN` | 空 | 设置后 API 需要 `Authorization: Bearer <token>`（`/healthz` 除外） |
| `OPEN_PDF_MAX_UPLOAD_MB` | `512` | 单请求体积上限（MB） |
| `OPEN_PDF_MAX_FILES` | `20` | 单请求文件数上限 |
| `OPEN_PDF_CORS_ORIGINS` | `*` | 允许的跨域来源，逗号分隔 |
| `OPEN_PDF_TMP_DIR` | 系统临时目录 | 请求级暂存目录 |
| `OPEN_PDF_READ_TIMEOUT_SEC` | `300` | 读请求（含上传）超时 |
| `OPEN_PDF_WRITE_TIMEOUT_SEC` | `300` | 写响应超时 |
| `OPEN_PDF_IDLE_TIMEOUT_SEC` | `120` | Keep-alive 超时 |
| `OPEN_PDF_SHUTDOWN_TIMEOUT_SEC` | `20` | 优雅退出超时 |
| `OPEN_PDF_LOG_JSON` | `false` | 结构化 JSON 日志 |

## 容器部署

镜像为三阶段构建（Node 构建前端 → Go 静态编译 → `scratch` 运行层），
最终镜像只包含一个静态二进制 + 可写的 `/tmp`，以非 root（UID 65534）运行，
内置 Docker 健康检查（`open-pdf -self-check`，无需 wget/curl）。

国内网络环境可覆盖基础镜像与 Go 模块代理：

```bash
docker build \
  --build-arg GO_IMAGE=docker.1ms.run/library/golang:1.27-alpine \
  --build-arg NODE_IMAGE=docker.1ms.run/library/node:22-alpine \
  --build-arg GOPROXY=https://goproxy.io,direct \
  -t open-pdf:dev .
```

多架构构建：

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t yourrepo/open-pdf:latest --push .
```

生产建议：

- 通过反向代理限制公网访问并配置 TLS
- 设置 `OPEN_PDF_API_TOKEN`
- 按需调低 `OPEN_PDF_MAX_UPLOAD_MB` 与 `OPEN_PDF_MAX_FILES`
- 保持 `read_only` + `tmpfs /tmp` 的加固配置（见 docker-compose.yml）

## 开发

```bash
make test       # 全部测试
make test-race  # race 检测
make lint       # go vet + gofmt
make fixtures   # 生成示例 PDF/图片到 build/fixtures
```

项目结构：

```
cmd/open-pdf/       服务入口（启动、优雅退出、-self-check）
cmd/mkfixtures/     示例文件生成器
internal/config/    环境变量配置
internal/pdfops/    PDF 操作层（pdfcpu 封装）
internal/httpapi/   REST 路由、中间件、multipart 处理
internal/testutil/  测试夹具（最小 PDF 生成器、图片生成）
web/                Vue 3 前端（构建产物嵌入二进制）
```

## 许可证

[Apache-2.0](LICENSE)。底层 PDF 引擎为 [pdfcpu](https://github.com/pdfcpu/pdfcpu)（Apache-2.0）。
