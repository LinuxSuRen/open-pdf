<script setup>
import { ref, computed } from 'vue'
import FileDrop from './FileDrop.vue'
import { postForJSON } from '../api'

const files = ref([])
const loading = ref(false)
const error = ref('')
const info = ref(null)

const pretty = computed(() => (info.value ? JSON.stringify(info.value, null, 2) : ''))
const pageSizeText = computed(() => {
  if (!info.value || !Array.isArray(info.value.pageSizes)) return ''
  const seen = new Set()
  const parts = []
  for (const d of info.value.pageSizes) {
    const key = `${d.width}x${d.height}`
    if (!seen.has(key)) {
      seen.add(key)
      parts.push(`${d.width} × ${d.height} pt`)
    }
  }
  return parts.join('、')
})

async function submit() {
  if (!files.value.length) return
  loading.value = true
  error.value = ''
  info.value = null
  try {
    info.value = await postForJSON('/api/v1/info', {
      files: [{ field: 'file', file: files.value[0] }],
    })
  } catch (e) {
    error.value = `${e.code}: ${e.message}`
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="card">
    <h2>PDF 信息</h2>
    <p class="sub">查看页数、PDF 版本、页面尺寸、元数据与文档结构信息。</p>
    <FileDrop v-model="files" accept=".pdf" label="点击或拖拽 PDF 到此处" />
    <div class="row" style="margin-top: 14px">
      <button class="primary" :disabled="loading || !files.length" @click="submit">
        <span v-if="loading" class="spinner"></span>{{ loading ? '解析中…' : '查看信息' }}
      </button>
    </div>
    <div v-if="error" class="error-box">{{ error }}</div>

    <template v-if="info">
      <div class="stats" style="margin-top: 16px">
        <div class="stat">
          <div class="k">页数</div>
          <div class="v">{{ info.pageCount }}</div>
        </div>
        <div class="stat">
          <div class="k">PDF 版本</div>
          <div class="v">{{ info.version || '-' }}</div>
        </div>
        <div class="stat">
          <div class="k">加密</div>
          <div class="v">{{ info.encrypted ? '是' : '否' }}</div>
        </div>
        <div class="stat">
          <div class="k">水印</div>
          <div class="v">{{ info.watermarked ? '有' : '无' }}</div>
        </div>
        <div class="stat">
          <div class="k">书签</div>
          <div class="v">{{ info.bookmarks ? '有' : '无' }}</div>
        </div>
      </div>
      <p v-if="pageSizeText" class="sub" style="margin-bottom: 10px">
        页面尺寸：{{ pageSizeText }}
      </p>
      <pre class="json">{{ pretty }}</pre>
    </template>
  </section>
</template>
