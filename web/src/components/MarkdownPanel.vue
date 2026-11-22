<script setup>
import { ref, computed, watch, onUnmounted } from 'vue'
import FileDrop from './FileDrop.vue'
import { postForBlob, downloadBlob, formatBytes } from '../api'

// Three switchable ways to provide the markdown source; the chosen mode
// decides which value is sent to the API (file / text / url).
const modes = [
  { id: 'upload', label: '本地上传' },
  { id: 'paste', label: '粘贴文本' },
  { id: 'link', label: '文件链接' },
]
const mode = ref('upload')

const mdFiles = ref([])
const text = ref('')
const link = ref('')
const assets = ref([])
const pageSize = ref('a4')
const orientation = ref('portrait')
const margin = ref(18)
const fontSize = ref(11)

const error = ref('')
const previewing = ref(false)
const previewUrl = ref('')
const lastResult = ref(null) // { blob, filename } of the last conversion
const lastKey = ref('') // inputKey() snapshot of the last conversion

const sample = `# 演示文档

支持 **加粗**、~~删除线~~、[链接](https://github.com/LinuxSuRen/open-pdf) 与行内代码。

- 中文自动换行
- 表格、代码块、引用块均可渲染
`

const ready = computed(() => {
  if (mode.value === 'upload') return mdFiles.value.length > 0
  if (mode.value === 'paste') return text.value.trim() !== ''
  return link.value.trim() !== ''
})

// inputKey fingerprints every conversion input so the download button can
// reuse the previewed PDF instead of re-converting.
function inputKey() {
  const f = mdFiles.value[0]
  return JSON.stringify({
    mode: mode.value,
    text: mode.value === 'paste' ? text.value : '',
    link: mode.value === 'link' ? link.value.trim() : '',
    file: f ? `${f.name}:${f.size}:${f.lastModified}` : '',
    assets: assets.value.map((a) => `${a.name}:${a.size}`).sort(),
    page_size: pageSize.value,
    orientation: orientation.value,
    margin: margin.value,
    font_size: fontSize.value,
  })
}

function buildRequest() {
  const fields = {
    page_size: pageSize.value,
    orientation: orientation.value,
    margin: String(margin.value),
    font_size: String(fontSize.value),
  }
  if (mode.value === 'paste') fields.text = text.value
  if (mode.value === 'link') fields.url = link.value.trim()
  const files = assets.value.map((f) => ({ field: 'asset', file: f }))
  if (mode.value === 'upload') {
    files.push({ field: 'file', file: mdFiles.value[0] })
  }
  return { files, fields }
}

let seq = 0
let controller = null
let timer = null

function applyResult(out) {
  if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
  previewUrl.value = URL.createObjectURL(out.blob)
  lastResult.value = out
  lastKey.value = inputKey()
}

// convert runs one conversion; a newer call (or input change) aborts the
// in-flight request and its result is discarded. Returns the result or
// null when superseded/aborted.
async function convert() {
  const mySeq = ++seq
  if (controller) controller.abort()
  controller = new AbortController()
  previewing.value = true
  error.value = ''
  try {
    const { files, fields } = buildRequest()
    const out = await postForBlob('/api/v1/markdown', { files, fields, signal: controller.signal })
    if (mySeq !== seq) return null
    applyResult(out)
    return out
  } catch (e) {
    if (mySeq !== seq || e?.name === 'AbortError') return null
    error.value = `${e.code}: ${e.message}`
    return null
  } finally {
    if (mySeq === seq) previewing.value = false
  }
}

function scheduleConvert(ms = 700) {
  if (!ready.value) return
  clearTimeout(timer)
  timer = setTimeout(() => convert(), ms)
}

// Live preview: content and mode changes re-convert, debounced per mode
// (typing in the paste editor gets the short path). The link mode converts
// explicitly on enter/blur so typing a URL does not spam upstream fetches.
watch([mode, text, mdFiles], () => {
  if (!ready.value) return
  scheduleConvert(mode.value === 'paste' ? 700 : 200)
})
watch([assets, pageSize, orientation, margin, fontSize], () => scheduleConvert(300))

function onLinkSettled() {
  if (ready.value) scheduleConvert(200)
}

async function download() {
  if (!ready.value || previewing.value) return
  let out = lastResult.value
  if (!out || lastKey.value !== inputKey()) out = await convert()
  if (out) downloadBlob(out.blob, out.filename)
}

onUnmounted(() => {
  clearTimeout(timer)
  if (controller) controller.abort()
  if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
})
</script>

<template>
  <section class="card">
    <h2>Markdown 转 PDF</h2>
    <p class="sub">
      通过上传、粘贴或链接提供 Markdown，左侧编辑、右侧实时预览，满意后再下载。
    </p>

    <div class="md-workspace">
      <div class="md-input-col">
        <div class="mode-switch" role="tablist" aria-label="Markdown 来源">
          <button v-for="m in modes" :key="m.id" role="tab" type="button"
            :aria-selected="mode === m.id" :class="{ active: mode === m.id }" @click="mode = m.id">
            {{ m.label }}
          </button>
        </div>

        <div v-show="mode === 'upload'" role="tabpanel">
          <FileDrop v-model="mdFiles" accept=".md,.markdown" label="点击或拖拽 Markdown 文件到此处"
            hint="单个 .md / .markdown 文件" />
        </div>

        <div v-show="mode === 'paste'" role="tabpanel">
          <textarea v-model="text" rows="12" :placeholder="sample" class="md-input"></textarea>
        </div>

        <div v-show="mode === 'link'" role="tabpanel">
          <div class="field" style="max-width: 100%">
            <label>Markdown 文件链接</label>
            <input v-model="link" type="url" placeholder="https://example.com/docs/README.md"
              class="md-link" @change="onLinkSettled" @keyup.enter="onLinkSettled" />
            <span class="field-hint">由服务端下载该链接的内容并转换；链接需无需登录即可访问，回车或失焦后更新预览</span>
          </div>
        </div>

        <div style="margin-top: 12px">
          <FileDrop v-model="assets" multiple accept=".png,.jpg,.jpeg,.webp,.tif,.tiff"
            label="点击或拖拽引用的图片到此处" hint="按文件名引用，写作 ![](shot.png) 后上传 shot.png 即可嵌入" />
        </div>

        <div class="row" style="margin-top: 14px">
          <div class="field">
            <label>纸张</label>
            <select v-model="pageSize">
              <option value="a4">A4</option>
              <option value="a3">A3</option>
              <option value="a5">A5</option>
              <option value="letter">Letter</option>
              <option value="legal">Legal</option>
            </select>
          </div>
          <div class="field">
            <label>方向</label>
            <select v-model="orientation">
              <option value="portrait">纵向</option>
              <option value="landscape">横向</option>
            </select>
          </div>
          <div class="field">
            <label>页边距，毫米</label>
            <input v-model.number="margin" type="number" min="5" max="50" />
          </div>
          <div class="field">
            <label>字号，磅</label>
            <input v-model.number="fontSize" type="number" min="6" max="24" />
          </div>
        </div>

        <div style="margin-top: 12px">
          <button class="primary" :disabled="!ready || previewing" @click="download">
            <span v-if="previewing" class="spinner"></span>{{ previewing ? '转换中…' : '下载 PDF' }}
          </button>
        </div>
      </div>

      <div class="md-preview">
        <div class="md-preview-head">
          <span v-if="previewing" class="spinner dark"></span>
          <strong>预览</strong>
          <span v-if="lastResult" class="meta">
            {{ lastResult.filename }} · {{ formatBytes(lastResult.blob.size) }}
          </span>
          <span class="spacer"></span>
          <a v-if="previewUrl" :href="previewUrl" target="_blank" rel="noreferrer">新窗口打开</a>
        </div>
        <iframe v-if="previewUrl" :src="previewUrl" class="md-preview-frame" title="PDF 预览"></iframe>
        <div v-else class="md-preview-empty">
          <span>内容就绪后将自动转换并在此处预览</span>
          <span class="sub-line">支持标题、列表、表格、代码块、引用、链接与图片</span>
        </div>
      </div>
    </div>

    <div v-if="error" class="error-box">{{ error }}</div>
  </section>
</template>
