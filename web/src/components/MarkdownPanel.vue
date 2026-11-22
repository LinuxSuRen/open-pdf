<script setup>
import { ref, computed } from 'vue'
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
const loading = ref(false)
const error = ref('')
const done = ref(null)

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

async function submit() {
  if (!ready.value || loading.value) return
  loading.value = true
  error.value = ''
  done.value = null
  try {
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
    const { blob, filename } = await postForBlob('/api/v1/markdown', { files, fields })
    downloadBlob(blob, filename)
    done.value = { size: blob.size }
  } catch (e) {
    error.value = `${e.code}: ${e.message}`
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="card">
    <h2>Markdown 转 PDF</h2>
    <p class="sub">
      通过上传、粘贴或链接提供 Markdown，排版为带中文字体的 PDF；支持标题、列表、表格、代码块、引用、链接与图片。
    </p>

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
      <textarea v-model="text" rows="10" :placeholder="sample" class="md-input"></textarea>
    </div>

    <div v-show="mode === 'link'" role="tabpanel">
      <div class="field" style="max-width: 100%">
        <label>Markdown 文件链接</label>
        <input v-model="link" type="url" placeholder="https://example.com/docs/README.md"
          class="md-link" @keyup.enter="submit" />
        <span class="field-hint">由服务端下载该链接的内容并转换；链接需无需登录即可访问</span>
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
      <button class="primary" :disabled="loading || !ready" @click="submit">
        <span v-if="loading" class="spinner"></span>{{ loading ? '转换中…' : '转换为 PDF' }}
      </button>
    </div>

    <div v-if="error" class="error-box">{{ error }}</div>
    <div v-if="done" class="result-box">✅ 已生成（{{ formatBytes(done.size) }}），已开始下载</div>
  </section>
</template>
