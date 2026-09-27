<script setup>
import { ref } from 'vue'
import FileDrop from './FileDrop.vue'
import { postForBlob, downloadBlob, formatBytes } from '../api'

const files = ref([])
const angle = ref(90)
const pages = ref('')
const loading = ref(false)
const error = ref('')
const done = ref(null)

async function submit() {
  if (!files.value.length) return
  loading.value = true
  error.value = ''
  done.value = null
  try {
    const { blob, filename } = await postForBlob('/api/v1/rotate', {
      files: [{ field: 'file', file: files.value[0] }],
      fields: { angle: String(angle.value), pages: pages.value },
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
    <h2>旋转页面</h2>
    <p class="sub">按 90° 的整数倍旋转全部或指定页面。</p>
    <FileDrop v-model="files" accept=".pdf" label="点击或拖拽 PDF 到此处" />
    <div class="row" style="margin-top: 14px">
      <div class="field">
        <label>角度（逆时针）</label>
        <select v-model.number="angle">
          <option :value="90">90°</option>
          <option :value="180">180°</option>
          <option :value="270">270°</option>
          <option :value="-90">-90°（顺时针 90°）</option>
        </select>
      </div>
      <div class="field">
        <label>作用页码（留空为全部）</label>
        <input v-model="pages" type="text" placeholder="如 1-3,5" />
      </div>
      <button class="primary" :disabled="loading || !files.length" @click="submit">
        <span v-if="loading" class="spinner"></span>{{ loading ? '旋转中…' : '开始旋转' }}
      </button>
    </div>
    <div v-if="error" class="error-box">{{ error }}</div>
    <div v-if="done" class="result-box">✅ 已旋转（{{ formatBytes(done.size) }}），已开始下载</div>
  </section>
</template>
