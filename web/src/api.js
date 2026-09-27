// Thin API client shared by all panels. The optional bearer token is kept
// in localStorage and attached to every request.

const TOKEN_KEY = 'openpdf.token'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || ''
}

export function setToken(token) {
  localStorage.setItem(TOKEN_KEY, token || '')
}

export class ApiError extends Error {
  constructor(code, message, status) {
    super(message || code || 'request failed')
    this.code = code
    this.status = status
  }
}

async function toApiError(resp) {
  let code = `http_${resp.status}`
  let message = `${resp.status} ${resp.statusText}`
  try {
    const data = await resp.json()
    if (data && data.error) {
      code = data.error.code || code
      message = data.error.message || message
    }
  } catch {
    /* non-JSON error body */
  }
  return new ApiError(code, message, resp.status)
}

// postFiles sends files and scalar fields as multipart/form-data and either
// returns the response blob (binary endpoints) or parsed JSON (info).
export async function postFiles(path, { files = [], fields = {} } = {}) {
  const form = new FormData()
  for (const f of files) {
    form.append(f.field || 'file', f.file, f.name || f.file.name)
  }
  for (const [k, v] of Object.entries(fields)) {
    if (v !== undefined && v !== null && v !== '') form.append(k, String(v))
  }
  const headers = {}
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  const resp = await fetch(path, { method: 'POST', headers, body: form })
  if (!resp.ok) throw await toApiError(resp)
  return resp
}

export async function postForBlob(path, opts) {
  const resp = await postFiles(path, opts)
  const blob = await resp.blob()
  return { blob, filename: filenameFrom(resp.headers.get('Content-Disposition')) }
}

export async function postForJSON(path, opts) {
  const resp = await postFiles(path, opts)
  return resp.json()
}

export async function getJSON(path) {
  const headers = {}
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  const resp = await fetch(path, { headers })
  if (!resp.ok) throw await toApiError(resp)
  return resp.json()
}

function filenameFrom(disposition) {
  if (!disposition) return 'download'
  const m = disposition.match(/filename\*=UTF-8''([^;]+)/i)
  if (m) return decodeURIComponent(m[1])
  const m2 = disposition.match(/filename="?([^";]+)"?/i)
  return m2 ? m2[1] : 'download'
}

export function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 10_000)
}

export function formatBytes(n) {
  if (!Number.isFinite(n)) return '-'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(2)} MB`
}
