<script setup>
import { ref } from 'vue'
import FileDrop from './FileDrop.vue'
import { postForBlob, downloadBlob, formatBytes } from '../api'

const mdFiles = ref([])
const assets = ref([])
const text = ref('')
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

async function submit() {
  if (!mdFiles.value.length && !text.value.trim()) return
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
    if (mdFiles.value.length) {
      fields.text = ''
    } else {
      fields.text = text.value
    }
    const files = []
    for (const f of assets.value) {
      files.push({ field: 'asset', file: f })
    }
    if (mdFiles.value.length) {
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
      粘贴或上传 Markdown，排版为带中文字体的 PDF；支持标题、列表、表格、代码块、引用、链接与图片。
    </p>

    <FileDrop v-model="mdFiles" accept=".md,.markdown" label="点击或拖拽 Markdown 文件到此处"
      hint="未选择文件时将使用下方粘贴的内容" />

    <div class="field" style="margin-top: 12px">
      <label>或直接粘贴内容</label>
      <textarea v-model="text" rows="8" :placeholder="sample" class="md-input"></textarea>
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
      <button class="primary" :disabled="loading || (!mdFiles.length && !text.trim())" @click="submit">
        <span v-if="loading" class="spinner"></span>{{ loading ? '转换中…' : '转换为 PDF' }}
      </button>
    </div>

    <div v-if="error" class="error-box">{{ error }}</div>
    <div v-if="done" class="result-box">✅ 已生成（{{ formatBytes(done.size) }}），已开始下载</div>
  </section>
</template>
