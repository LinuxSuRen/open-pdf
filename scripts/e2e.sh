#!/usr/bin/env bash
# open-pdf 端到端测试：真实二进制 + 真实 HTTP 请求。
# 用法: scripts/e2e.sh [二进制路径]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${1:-$ROOT/bin/open-pdf}"
PORT="${E2E_PORT:-18080}"
AUTH_PORT="${E2E_AUTH_PORT:-18081}"
BASE="http://127.0.0.1:$PORT"
WORK="$(mktemp -d /tmp/openpdf-e2e.XXXXXX)"
SERVER_PID=""

cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

fail() { echo "❌ $*" >&2; exit 1; }
ok() { echo "✅ $*"; }

start_server() { # port pidfile extra-env...
  local port="$1"; shift
  env "$@" OPEN_PDF_ADDR=":$port" "$BIN" >"$WORK/server-$port.log" 2>&1 &
  echo $!
}

wait_health() { # url attempts
  local url="$1" i
  for i in $(seq 1 "${2:-30}"); do
    if curl -sf -o /dev/null "$url"; then return 0; fi
    sleep 0.3
  done
  return 1
}

page_count() { # file -> 页数（通过 /api/v1/info）
  curl -sf -F "file=@$1" "$BASE/api/v1/info" | sed -n 's/.*"pageCount":\([0-9]*\).*/\1/p'
}

[ -x "$BIN" ] || fail "二进制不存在: $BIN（先运行 make build）"
echo "== open-pdf e2e，工作目录 $WORK =="

# ---------- 准备夹具 ----------
(cd "$ROOT" && go run ./cmd/mkfixtures "$WORK/fx" >/dev/null)
ls "$WORK/fx/sample.pdf" >/dev/null || fail "夹具生成失败"

# ---------- 基础端点 ----------
SERVER_PID=$(start_server "$PORT")
wait_health "$BASE/healthz" || { cat "$WORK/server-$PORT.log"; fail "服务未启动"; }
ok "服务启动 (:$PORT)"

curl -sf "$BASE/healthz" | grep -q '"ok"' || fail "healthz 内容错误"
ok "healthz"
curl -sf "$BASE/api/v1/version" | grep -q '"open-pdf"' || fail "version 错误"
ok "version"
curl -sf "$BASE/api" | grep -q '/api/v1/merge' || fail "api index 错误"
ok "api index"
curl -sf "$BASE/" | grep -q '<div id="app">' || fail "SPA 未服务"
ok "web UI"
ASSET=$(ls "$ROOT/web/dist/assets" | grep '\.js$' | head -1)
curl -sf -o /dev/null -w '%{http_code}' "$BASE/assets/$ASSET" | grep -q 200 || fail "静态资源 404"
ok "静态资源"

# ---------- 合并（PDF + 图片混合） ----------
curl -sf -o "$WORK/merged.pdf" \
  -F files=@"$WORK/fx/sample.pdf" -F files=@"$WORK/fx/photo.png" \
  -F files=@"$WORK/fx/photo.jpg" -F files=@"$WORK/fx/long.pdf" \
  "$BASE/api/v1/merge" || fail "合并失败"
N=$(page_count "$WORK/merged.pdf")
[ "$N" = "17" ] || fail "合并页数 $N，应为 17 (3+1+1+12)"
ok "混合合并 (页数=$N)"

# ---------- 分割 ----------
curl -sf -o "$WORK/split.zip" -F file=@"$WORK/fx/long.pdf" "$BASE/api/v1/split" \
  || fail "分割失败"
ENTRIES=$(unzip -l "$WORK/split.zip" | grep -c '\.pdf$' || true)
[ "$ENTRIES" = "12" ] || fail "burst 应有 12 个文件，实际 $ENTRIES"
ok "分割 burst → ZIP ($ENTRIES 个文件)"

curl -sf -o "$WORK/part.pdf" -F file=@"$WORK/fx/long.pdf" -F mode=ranges -F ranges=3-5 \
  "$BASE/api/v1/split" || fail "范围分割失败"
N=$(page_count "$WORK/part.pdf")
[ "$N" = "3" ] || fail "范围分割页数 $N，应为 3"
ok "分割 ranges 3-5 (页数=$N)"

curl -sf -o "$WORK/span.zip" -F file=@"$WORK/fx/long.pdf" -F mode=span -F span=5 \
  "$BASE/api/v1/split" || fail "span 分割失败"
ENTRIES=$(unzip -l "$WORK/span.zip" | grep -c '\.pdf$' || true)
[ "$ENTRIES" = "3" ] || fail "span=5 应有 3 个文件，实际 $ENTRIES"
ok "分割 span=5 → ZIP ($ENTRIES 个文件)"

# ---------- 压缩 ----------
ORIG=$(wc -c <"$WORK/fx/long.pdf" | tr -d ' ')
curl -sf -o "$WORK/compressed.pdf" -F file=@"$WORK/fx/long.pdf" "$BASE/api/v1/compress" \
  || fail "压缩失败"
COMP=$(wc -c <"$WORK/compressed.pdf" | tr -d ' ')
N=$(page_count "$WORK/compressed.pdf")
[ "$N" = "12" ] || fail "压缩后页数 $N"
[ "$COMP" -lt "$ORIG" ] || fail "压缩未减小体积 ($ORIG → $COMP)"
ok "压缩 $ORIG → $COMP 字节"

# ---------- 水印（文字 + 图片） ----------
curl -sf -o "$WORK/wm.pdf" -F file=@"$WORK/fx/sample.pdf" -F text='E2E 水印' \
  -F opacity=0.3 -F fontsize=42 -F color=#FF8800 \
  "$BASE/api/v1/watermark" || fail "文字水印失败"
page_count "$WORK/wm.pdf" >/dev/null || fail "水印文件损坏"
ok "文字水印"
curl -sf -o "$WORK/wmi.pdf" -F file=@"$WORK/fx/sample.pdf" -F type=image \
  -F image=@"$WORK/fx/logo.png" "$BASE/api/v1/watermark" || fail "图片水印失败"
ok "图片水印"

# ---------- 旋转 ----------
curl -sf -o "$WORK/rot.pdf" -F file=@"$WORK/fx/sample.pdf" -F angle=90 \
  "$BASE/api/v1/rotate" || fail "旋转失败"
N=$(page_count "$WORK/rot.pdf")
[ "$N" = "3" ] || fail "旋转后页数 $N"
ok "旋转 90°"

# ---------- 加密 / 解密 ----------
curl -sf -o "$WORK/enc.pdf" -F file=@"$WORK/fx/sample.pdf" -F password=e2epass \
  "$BASE/api/v1/encrypt" || fail "加密失败"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -F file=@"$WORK/enc.pdf" "$BASE/api/v1/info")
[ "$STATUS" = "422" ] || fail "加密文件应无法直接读取 (status=$STATUS)"
curl -sf -o "$WORK/dec.pdf" -F file=@"$WORK/enc.pdf" -F password=e2epass \
  "$BASE/api/v1/decrypt" || fail "解密失败"
N=$(page_count "$WORK/dec.pdf")
[ "$N" = "3" ] || fail "解密后页数 $N"
ok "加密 → 解密闭环"

# ---------- 错误路径 ----------
STATUS=$(printf 'GIF89a' >"$WORK/x.gif"; curl -s -o /dev/null -w '%{http_code}' \
  -F files=@"$WORK/x.gif" "$BASE/api/v1/merge")
[ "$STATUS" = "415" ] || fail "GIF 应返回 415，实际 $STATUS"
ok "不支持类型 → 415"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -F dummy=1 "$BASE/api/v1/merge")
[ "$STATUS" = "400" ] || fail "无文件应返回 400，实际 $STATUS"
ok "无文件 → 400"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/merge")
[ "$STATUS" = "405" ] || fail "GET 应返回 405，实际 $STATUS"
ok "错误方法 → 405"

# ---------- 并发 ----------
echo "  并发 24 个混合请求…"
SEQ=$(seq 1 8)
( for i in $SEQ; do
    curl -sf -o "$WORK/c-merge-$i.pdf" -F files=@"$WORK/fx/sample.pdf" -F files=@"$WORK/fx/photo.png" "$BASE/api/v1/merge" &
    curl -sf -o "$WORK/c-wm-$i.pdf" -F file=@"$WORK/fx/sample.pdf" -F text=并发 "$BASE/api/v1/watermark" &
    curl -sf -o "$WORK/c-info-$i.json" -F file=@"$WORK/fx/sample.pdf" "$BASE/api/v1/info" &
  done; wait ) || fail "并发请求失败"
ls "$WORK"/c-*.pdf | wc -l | grep -q 16 || fail "并发结果数量不对"
ok "并发 24 请求全部成功"

# ---------- 优雅退出 ----------
kill -TERM "$SERVER_PID"
for i in $(seq 1 30); do
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then break; fi
  sleep 0.2
done
if kill -0 "$SERVER_PID" 2>/dev/null; then fail "服务未在 SIGTERM 后退出"; fi
grep -q "shutdown complete" "$WORK/server-$PORT.log" || fail "缺少优雅退出日志"
ok "SIGTERM 优雅退出"

# ---------- 鉴权 ----------
SERVER_PID=$(start_server "$AUTH_PORT" OPEN_PDF_API_TOKEN=sec-token)
wait_health "http://127.0.0.1:$AUTH_PORT/healthz" || fail "鉴权服务未启动"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -F file=@"$WORK/fx/sample.pdf" \
  "http://127.0.0.1:$AUTH_PORT/api/v1/info")
[ "$STATUS" = "401" ] || fail "无 token 应 401，实际 $STATUS"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer sec-token' \
  -F file=@"$WORK/fx/sample.pdf" "http://127.0.0.1:$AUTH_PORT/api/v1/info")
[ "$STATUS" = "200" ] || fail "正确 token 应 200，实际 $STATUS"
ok "Bearer 鉴权 (401 / 200)"

echo
echo "🎉 e2e 全部通过"
