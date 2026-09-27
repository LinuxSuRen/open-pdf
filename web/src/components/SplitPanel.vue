<script setup>
import { ref } from 'vue'
import FileDrop from './FileDrop.vue'
import { postForBlob, downloadBlob, formatBytes } from '../api'

const files = ref([])
const mode = ref('burst')
const span = ref(2)
const ranges = ref('1-3,5')
const loading = ref(false)
const error = ref('')
const done = ref(null)

async function split() {
  if (!files.value.length) return
  loading.value = true
  error.value = ''
  done.value = null
  try {
    const fields = { mode: mode.value }
    if (mode.value === 'span') fields.span = String(span.value)
    if (mode.value === 'ranges') fields.ranges = ranges.value
    const { blob, filename } = await postForBlob('/api/v1/split', {
      files: [{ field: 'file', file: files.value[0] }],
      fields,
    })
    downloadBlob(blob, filename)
    done.value = { size: blob.size, filename, zip: filename.endsWith('.zip') }
  } catch (e) {
    error.value = `${e.code}: ${e.message}`
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="card">
    <h2>分割 PDF</h2>
    <p class="sub">按页拆分为多个 PDF；结果超过一个文件时自动打包为 ZIP 下载。</p>
    <FileDrop v-model="files" accept=".pdf" label="点击或拖拽 PDF 到此处" />
    <div class="row" style="margin-top: 14px">
      <div class="field">
        <label>分割方式</label>
        <select v-model="mode">
          <option value="burst">每页一个文件</option>
          <option value="span">每 N 页一个文件</option>
          <option value="ranges">自定义页码范围</option>
        </select>
      </div>
      <div v-if="mode === 'span'" class="field">
        <label>N（每份页数）</label>
        <input v-model.number="span" type="number" min="1" />
      </div>
      <div v-if="mode === 'ranges'" class="field">
        <label>范围（如 1-3,5,8-）</label>
        <input v-model="ranges" type="text" placeholder="1-3,5,8-" />
      </div>
      <button class="primary" :disabled="loading || !files.length" @click="split">
        <span v-if="loading" class="spinner"></span>{{ loading ? '分割中…' : '开始分割' }}
      </button>
    </div>
    <div v-if="error" class="error-box">{{ error }}</div>
    <div v-if="done" class="result-box">
      ✅ 已生成 {{ done.filename }}（{{ formatBytes(done.size) }}）并开始下载
    </div>
  </section>
</template>
