<script setup>
import { ref } from 'vue'
import FileDrop from './FileDrop.vue'
import { postForBlob, downloadBlob, formatBytes } from '../api'

const files = ref([])
const loading = ref(false)
const error = ref('')
const done = ref(null) // { size }

async function merge() {
  if (!files.value.length) return
  loading.value = true
  error.value = ''
  done.value = null
  try {
    const { blob, filename } = await postForBlob('/api/v1/merge', {
      files: files.value.map((f) => ({ field: 'files', file: f })),
    })
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
    <h2>合并 PDF</h2>
    <p class="sub">
      把多个 PDF 和图片（PNG / JPG / WebP / TIFF）按顺序合并成一个 PDF，图片会自动转换为 A4 页面。
    </p>
    <FileDrop v-model="files" multiple ordered accept=".pdf,.png,.jpg,.jpeg,.webp,.tif,.tiff"
      label="点击或拖拽 PDF / 图片到此处" hint="支持 PDF 与图片混合，可用 ↑ ↓ 调整顺序" />
    <div class="row" style="margin-top: 14px">
      <button class="primary" :disabled="loading || !files.length" @click="merge">
        <span v-if="loading" class="spinner"></span>{{ loading ? '合并中…' : `合并 ${files.length} 个文件` }}
      </button>
    </div>
    <div v-if="error" class="error-box">{{ error }}</div>
    <div v-if="done" class="result-box">
      ✅ 已生成并开始下载（{{ formatBytes(done.size) }}）
    </div>
  </section>
</template>
