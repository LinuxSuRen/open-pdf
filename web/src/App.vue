<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { getJSON, getToken, setToken } from './api'
import MergePanel from './components/MergePanel.vue'
import SplitPanel from './components/SplitPanel.vue'
import CompressPanel from './components/CompressPanel.vue'
import WatermarkPanel from './components/WatermarkPanel.vue'
import RotatePanel from './components/RotatePanel.vue'
import InfoPanel from './components/InfoPanel.vue'
import CryptPanel from './components/CryptPanel.vue'

const tabs = [
  { id: 'merge', label: '合并', comp: MergePanel },
  { id: 'split', label: '分割', comp: SplitPanel },
  { id: 'compress', label: '压缩', comp: CompressPanel },
  { id: 'watermark', label: '水印', comp: WatermarkPanel },
  { id: 'rotate', label: '旋转', comp: RotatePanel },
  { id: 'info', label: '信息', comp: InfoPanel },
  { id: 'crypt', label: '加密 / 解密', comp: CryptPanel },
]
const active = ref(tabs[0].id)
const activeComp = computed(() => tabs.find((t) => t.id === active.value).comp)

const health = ref('unknown')
let timer = null
async function ping() {
  try {
    await getJSON('/healthz')
    health.value = 'ok'
  } catch {
    health.value = 'down'
  }
}

// Keep accidental file drops outside a dropzone from navigating the app away.
function swallowDrag(e) {
  e.preventDefault()
}
onMounted(() => {
  window.addEventListener('dragover', swallowDrag)
  window.addEventListener('drop', swallowDrag)
  ping()
  timer = setInterval(ping, 30_000)
})
onUnmounted(() => {
  clearInterval(timer)
  window.removeEventListener('dragover', swallowDrag)
  window.removeEventListener('drop', swallowDrag)
})

const showSettings = ref(false)
const tokenInput = ref(getToken())
function saveToken() {
  setToken(tokenInput.value.trim())
  showSettings.value = false
  ping()
}
</script>

<template>
  <header class="header">
    <div class="logo">PDF</div>
    <h1>open-pdf</h1>
    <span class="health" :class="health" :title="health === 'ok' ? '服务正常' : '服务不可用'">
      <span class="dot"></span>{{ health === 'ok' ? '在线' : '离线' }}
    </span>
    <div class="spacer"></div>
    <div v-if="showSettings" class="settings">
      <input v-model="tokenInput" type="password" placeholder="API Token（留空关闭鉴权）" @keyup.enter="saveToken" />
      <button class="ghost" style="color:#aab4cc;border-color:#2c3a5c" @click="saveToken">保存</button>
    </div>
    <button v-else class="ghost" style="color:#aab4cc;border-color:#2c3a5c" @click="showSettings = true">接入令牌</button>
  </header>

  <main class="layout">
    <nav class="tabs">
      <button v-for="t in tabs" :key="t.id" :class="{ active: active === t.id }" @click="active = t.id">
        {{ t.label }}
      </button>
    </nav>
    <component :is="activeComp" />
  </main>

  <footer class="site">
    open-pdf · 基于
    <a href="https://github.com/pdfcpu/pdfcpu" target="_blank" rel="noreferrer">pdfcpu</a>
    的 PDF 工具箱
  </footer>
</template>
