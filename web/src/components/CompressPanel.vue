<script setup>
import { ref } from 'vue'
import FileDrop from './FileDrop.vue'
import { postForBlob, downloadBlob, formatBytes } from '../api'

const files = ref([])
const loading = ref(false)
const error = ref('')
const done = ref(null) // { before, after, ratio }

async function compress() {
  if (!files.value.length) return
  loading.value = true
  error.value = ''
  done.value = null
  try {
    const before = files.value[0].size
    const { blob, filename } = await postForBlob('/api/v1/compress', {
      files: [{ field: 'file', file: files.value[0] }],
    })
    downloadBlob(blob, filename)
    done.value = {
      before,
      after: blob.size,
      ratio: before > 0 ? Math.round((1 - blob.size / before) * 100) : 0,
    }
  } catch (e) {
    error.value = `${e.code}: ${e.message}`
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="card">
    <h2>压缩 PDF</h2>
    <p class="sub">
      无损结构优化：对象流 / 交叉引用流压缩、重复内容流去重，不降低页面内容质量。
    </p>
    <FileDrop v-model="files" accept=".pdf" label="点击或拖拽 PDF 到此处" />
    <div class="row" style="margin-top: 14px">
      <button class="primary" :disabled="loading || !files.length" @click="compress">
        <span v-if="loading" class="spinner"></span>{{ loading ? '压缩中…' : '开始压缩' }}
      </button>
    </div>
    <div v-if="error" class="error-box">{{ error }}</div>
    <div v-if="done" class="result-box">
      ✅ {{ formatBytes(done.before) }} → <strong>{{ formatBytes(done.after) }}</strong>
      <span class="meta">
        {{ done.ratio > 0 ? `减小 ${done.ratio}%` : done.ratio === 0 ? '大小基本不变' : `增大 ${-done.ratio}%（该文件已高度压缩）` }}
      </span>
      <span class="meta">已开始下载</span>
    </div>
  </section>
</template>
