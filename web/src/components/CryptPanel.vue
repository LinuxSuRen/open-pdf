<script setup>
import { ref } from 'vue'
import FileDrop from './FileDrop.vue'
import { postForBlob, downloadBlob, formatBytes } from '../api'

const encFiles = ref([])
const password = ref('')
const owner = ref('')
const keyLength = ref(256)
const encLoading = ref(false)
const encError = ref('')
const encDone = ref(null)

const decFiles = ref([])
const decPassword = ref('')
const decLoading = ref(false)
const decError = ref('')
const decDone = ref(null)

async function encrypt() {
  if (!encFiles.value.length) return
  encLoading.value = true
  encError.value = ''
  encDone.value = null
  try {
    const { blob, filename } = await postForBlob('/api/v1/encrypt', {
      files: [{ field: 'file', file: encFiles.value[0] }],
      fields: {
        password: password.value,
        owner: owner.value,
        key_length: String(keyLength.value),
      },
    })
    downloadBlob(blob, filename)
    encDone.value = { size: blob.size }
  } catch (e) {
    encError.value = `${e.code}: ${e.message}`
  } finally {
    encLoading.value = false
  }
}

async function decrypt() {
  if (!decFiles.value.length) return
  decLoading.value = true
  decError.value = ''
  decDone.value = null
  try {
    const { blob, filename } = await postForBlob('/api/v1/decrypt', {
      files: [{ field: 'file', file: decFiles.value[0] }],
      fields: { password: decPassword.value },
    })
    downloadBlob(blob, filename)
    decDone.value = { size: blob.size }
  } catch (e) {
    decError.value = `${e.code}: ${e.message}`
  } finally {
    decLoading.value = false
  }
}
</script>

<template>
  <div>
    <section class="card">
      <h2>加密 PDF</h2>
      <p class="sub">使用 AES-128 / AES-256 设置打开密码，可选独立的所有者密码。</p>
      <FileDrop v-model="encFiles" accept=".pdf" label="点击或拖拽 PDF 到此处" />
      <div class="row" style="margin-top: 14px">
        <div class="field">
          <label>打开密码（必填）</label>
          <input v-model="password" type="password" autocomplete="new-password" />
        </div>
        <div class="field">
          <label>所有者密码（可选）</label>
          <input v-model="owner" type="password" autocomplete="new-password" />
        </div>
        <div class="field">
          <label>加密强度</label>
          <select v-model.number="keyLength">
            <option :value="256">AES-256</option>
            <option :value="128">AES-128</option>
          </select>
        </div>
        <button class="primary" :disabled="encLoading || !encFiles.length || !password" @click="encrypt">
          <span v-if="encLoading" class="spinner"></span>{{ encLoading ? '加密中…' : '加密' }}
        </button>
      </div>
      <div v-if="encError" class="error-box">{{ encError }}</div>
      <div v-if="encDone" class="result-box">✅ 已加密（{{ formatBytes(encDone.size) }}），已开始下载</div>
    </section>

    <section class="card">
      <h2>解密 PDF</h2>
      <p class="sub">输入正确密码，移除文档的口令保护。</p>
      <FileDrop v-model="decFiles" accept=".pdf" label="点击或拖拽 PDF 到此处" />
      <div class="row" style="margin-top: 14px">
        <div class="field">
          <label>密码</label>
          <input v-model="decPassword" type="password" autocomplete="current-password" @keyup.enter="decrypt" />
        </div>
        <button class="primary" :disabled="decLoading || !decFiles.length" @click="decrypt">
          <span v-if="decLoading" class="spinner"></span>{{ decLoading ? '解密中…' : '解密' }}
        </button>
      </div>
      <div v-if="decError" class="error-box">{{ decError }}</div>
      <div v-if="decDone" class="result-box">✅ 已解密（{{ formatBytes(decDone.size) }}），已开始下载</div>
    </section>
  </div>
</template>
