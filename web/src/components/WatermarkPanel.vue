<script setup>
import { ref } from 'vue'
import FileDrop from './FileDrop.vue'
import { postForBlob, downloadBlob, formatBytes } from '../api'

const files = ref([])
const wmImages = ref([])
const type = ref('text')
const text = ref('机密文件 CONFIDENTIAL')
const pages = ref('')
const rotation = ref(45)
const opacity = ref(0.3)
const fontsize = ref(48)
const color = ref('#CCCCCC')
const position = ref('c')
const diagonal = ref('')
const onTop = ref(false)
const loading = ref(false)
const error = ref('')
const done = ref(null)

async function submit() {
  if (!files.value.length) return
  loading.value = true
  error.value = ''
  done.value = null
  try {
    const fields = {
      type: type.value,
      pages: pages.value,
      rotation: rotation.value === '' ? '' : String(rotation.value),
      opacity: opacity.value === '' ? '' : String(opacity.value),
      fontsize: fontsize.value === '' ? '' : String(fontsize.value),
      color: color.value,
      position: position.value,
      diagonal: diagonal.value,
      on_top: onTop.value ? '1' : '',
    }
    if (type.value === 'text') {
      fields.text = text.value
    }
    const list = [{ field: 'file', file: files.value[0] }]
    if (type.value === 'image' && wmImages.value.length) {
      list.push({ field: 'image', file: wmImages.value[0] })
    }
    const { blob, filename } = await postForBlob('/api/v1/watermark', { files: list, fields })
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
    <h2>添加水印</h2>
    <p class="sub">支持文字水印与图片水印，可设置透明度、旋转、字号、颜色与作用页码。</p>

    <FileDrop v-model="files" accept=".pdf" label="点击或拖拽 PDF 到此处" />

    <div class="row" style="margin-top: 14px">
      <div class="field">
        <label>水印类型</label>
        <select v-model="type">
          <option value="text">文字</option>
          <option value="image">图片</option>
        </select>
      </div>
      <div v-if="type === 'text'" class="field" style="flex: 1; min-width: 220px">
        <label>水印文字</label>
        <input v-model="text" type="text" />
      </div>
      <div class="field">
        <label>作用页码（留空为全部）</label>
        <input v-model="pages" type="text" placeholder="如 1-3,5" />
      </div>
    </div>

    <div v-if="type === 'image'" style="margin-top: 12px">
      <FileDrop v-model="wmImages" accept=".png,.jpg,.jpeg,.webp,.tif,.tiff"
        label="点击或拖拽水印图片到此处" hint="将作为图片水印铺在页面上" />
    </div>

    <div v-if="type === 'text'" class="row" style="margin-top: 12px">
      <div class="field">
        <label>透明度（0-1）</label>
        <input v-model.number="opacity" type="number" min="0.05" max="1" step="0.05" />
      </div>
      <div class="field">
        <label>旋转角度</label>
        <input v-model.number="rotation" type="number" step="15" />
      </div>
      <div class="field">
        <label>字号（pt）</label>
        <input v-model.number="fontsize" type="number" min="6" />
      </div>
      <div class="field">
        <label>颜色</label>
        <input v-model="color" type="text" placeholder="#CCCCCC" />
      </div>
      <div class="field">
        <label>方向</label>
        <select v-model="diagonal">
          <option value="">默认（斜向 ↗）</option>
          <option value="1">斜向 ↗</option>
          <option value="2">斜向 ↘</option>
          <option value="3">水平 →</option>
        </select>
      </div>
      <div class="field">
        <label>位置</label>
        <select v-model="position">
          <option value="c">居中</option>
          <option value="tl">左上</option>
          <option value="tr">右上</option>
          <option value="bl">左下</option>
          <option value="br">右下</option>
        </select>
      </div>
      <div class="field">
        <label>覆盖在内容上方</label>
        <select :value="onTop ? '1' : ''" @change="onTop = $event.target.value === '1'">
          <option value="">否（垫在内容下）</option>
          <option value="1">是</option>
        </select>
      </div>
    </div>

    <div class="row" style="margin-top: 14px">
      <button class="primary" :disabled="loading || !files.length" @click="submit">
        <span v-if="loading" class="spinner"></span>{{ loading ? '添加中…' : '添加水印' }}
      </button>
    </div>
    <div v-if="error" class="error-box">{{ error }}</div>
    <div v-if="done" class="result-box">✅ 水印已添加（{{ formatBytes(done.size) }}），已开始下载</div>
  </section>
</template>
