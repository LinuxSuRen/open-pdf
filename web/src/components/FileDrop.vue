<script setup>
import { ref } from 'vue'

const props = defineProps({
  multiple: { type: Boolean, default: false },
  accept: { type: String, default: '' },
  label: { type: String, default: '点击或拖拽文件到此处' },
  hint: { type: String, default: '' },
  // When multiple, show ordering controls.
  ordered: { type: Boolean, default: false },
})
const emit = defineEmits(['change'])

const files = defineModel({ type: Array, default: () => [] })
const inputEl = ref(null)
const over = ref(false)

function addFiles(list) {
  const incoming = Array.from(list)
  if (props.multiple) {
    files.value = [...files.value, ...incoming]
  } else {
    files.value = incoming.slice(0, 1)
  }
  emit('change', files.value)
}

function onDrop(e) {
  over.value = false
  addFiles(e.dataTransfer.files)
}

function onPick(e) {
  addFiles(e.target.files)
  e.target.value = ''
}

function remove(i) {
  files.value = files.value.filter((_, idx) => idx !== i)
  emit('change', files.value)
}

function move(i, delta) {
  const next = [...files.value]
  const j = i + delta
  if (j < 0 || j >= next.length) return
  ;[next[i], next[j]] = [next[j], next[i]]
  files.value = next
  emit('change', files.value)
}

function extOf(name) {
  const m = /\.([a-z0-9]+)$/i.exec(name || '')
  return m ? m[1].toLowerCase() : '?'
}

function iconFor(name) {
  const ext = extOf(name)
  if (ext === 'pdf') return '📄'
  return '🖼️'
}

function sizeOf(f) {
  if (!Number.isFinite(f.size)) return ''
  if (f.size < 1024) return `${f.size} B`
  if (f.size < 1024 * 1024) return `${(f.size / 1024).toFixed(1)} KB`
  return `${(f.size / 1024 / 1024).toFixed(2)} MB`
}

defineExpose({ inputEl })
</script>

<template>
  <div>
    <div
      class="dropzone"
      :class="{ over }"
      @click="inputEl.click()"
      @dragover.prevent="over = true"
      @dragleave.prevent="over = false"
      @drop.prevent="onDrop"
    >
      <strong>{{ label }}</strong>
      <div class="formats">{{ hint || (multiple ? '可同时选择多个文件' : '单个文件') }}</div>
    </div>
    <input
      ref="inputEl"
      type="file"
      :multiple="multiple"
      :accept="accept"
      style="display: none"
      @change="onPick"
    />

    <ul v-if="files.length" class="filelist">
      <li v-for="(f, i) in files" :key="i">
        <span class="icon">{{ iconFor(f.name) }}</span>
        <span class="name" :title="f.name">{{ f.name }}</span>
        <span class="size">{{ sizeOf(f) }}</span>
        <template v-if="ordered && files.length > 1">
          <button class="mini" title="上移" :disabled="i === 0" @click="move(i, -1)">↑</button>
          <button class="mini" title="下移" :disabled="i === files.length - 1" @click="move(i, 1)">↓</button>
        </template>
        <button class="mini" title="移除" @click="remove(i)">✕</button>
      </li>
    </ul>
  </div>
</template>
