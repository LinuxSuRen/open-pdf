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
import MarkdownPanel from './components/MarkdownPanel.vue'

const tabs = [
  { id: 'merge', label: '合并', comp: MergePanel },
  { id: 'split', label: '分割', comp: SplitPanel },
  { id: 'compress', label: '压缩', comp: CompressPanel },
  { id: 'watermark', label: '水印', comp: WatermarkPanel },
  { id: 'rotate', label: '旋转', comp: RotatePanel },
  { id: 'markdown', label: 'Markdown', comp: MarkdownPanel },
  { id: 'info', label: '信息', comp: InfoPanel },
  { id: 'crypt', label: '加密 / 解密', comp: CryptPanel },
]
const initialTab = window.location.hash.replace('#', '')
const active = ref(tabs.some((t) => t.id === initialTab) ? initialTab : tabs[0].id)
const activeComp = computed(() => tabs.find((t) => t.id === active.value).comp)

// Keep the chosen tab in the URL so a refresh (or a shared link) reopens
// the same view; replaceState avoids one history entry per click.
function selectTab(id) {
  if (id === active.value) return
  active.value = id
  history.replaceState(null, '', '#' + id)
}
function onHashChange() {
  const id = window.location.hash.replace('#', '')
  if (tabs.some((t) => t.id === id)) selectTab(id)
}

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
  window.addEventListener('hashchange', onHashChange)
  ping()
  timer = setInterval(ping, 30_000)
})
onUnmounted(() => {
  clearInterval(timer)
  window.removeEventListener('dragover', swallowDrag)
  window.removeEventListener('drop', swallowDrag)
  window.removeEventListener('hashchange', onHashChange)
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
    <a class="gh" href="https://github.com/linuxsuren/open-pdf" target="_blank" rel="noreferrer"
      title="GitHub 项目" aria-label="GitHub 项目">
      <svg viewBox="0 0 16 16" width="22" height="22" fill="currentColor" aria-hidden="true">
        <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38
          0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01
          1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95
          0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0
          1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0
          3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01
          8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z" />
      </svg>
    </a>
    <div v-if="showSettings" class="settings">
      <input v-model="tokenInput" type="password" placeholder="API Token（留空关闭鉴权）" @keyup.enter="saveToken" />
      <button class="ghost" style="color:#aab4cc;border-color:#2c3a5c" @click="saveToken">保存</button>
    </div>
    <button v-else class="ghost" style="color:#aab4cc;border-color:#2c3a5c" @click="showSettings = true">接入令牌</button>
  </header>

  <main class="layout">
    <nav class="tabs">
      <button v-for="t in tabs" :key="t.id" :class="{ active: active === t.id }" @click="selectTab(t.id)">
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
