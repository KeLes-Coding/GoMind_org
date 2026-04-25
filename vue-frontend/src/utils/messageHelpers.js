export const TERMINAL_STATUSES = new Set(['completed', 'cancelled', 'timeout', 'failed', 'partial'])

export const CHAT_MODE_LABELS = {
  chat: 'Chat',
  chat_rag: 'RAG',
  chat_mcp: 'MCP',
  chat_rag_mcp: 'RAG + MCP'
}

export const OWNER_MISMATCH_CODE = 2014
export const MAX_OWNER_MISMATCH_RETRIES = 2

export const normalizeMessageStatus = (status) => {
  if (!status) return 'completed'
  const normalized = String(status).toLowerCase()
  return TERMINAL_STATUSES.has(normalized) || normalized === 'streaming' ? normalized : 'completed'
}

export const buildMessageMeta = (status, extra = {}) => ({
  status: normalizeMessageStatus(status),
  ...extra
})

export const buildChatMessage = (overrides = {}) => {
  const base = {
    role: 'assistant',
    content: '',
    reasoningContent: '',
    imageUrl: ''
  }

  return {
    ...base,
    ...overrides,
    meta: {
      ...buildMessageMeta(overrides?.meta?.status || overrides?.status || 'completed'),
      ...(overrides?.meta || {})
    }
  }
}

export const getMessageStatusLabel = (status) => {
  switch (normalizeMessageStatus(status)) {
    case 'streaming':
      return 'Streaming'
    case 'cancelled':
      return 'Stopped'
    case 'timeout':
      return 'Timed out'
    case 'failed':
      return '失败'
    case 'partial':
      return '部分完成'
    default:
      return ''
  }
}

export const getMessageMetaStatus = (message) => {
  if (!message || !message.meta) return ''
  return message.meta.status || ''
}

export const getMessageReasoning = (message) => String(message?.reasoningContent || '')

export const getMessageRawMarkdown = (message) => String(message?.content || '')

import { marked } from 'marked'
import DOMPurify from 'dompurify'

const escapeHtml = (value) => String(value)
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;')
  .replace(/'/g, '&#39;')

const highlightCode = (code) => {
  const placeholders = []
  let html = escapeHtml(code).replace(/(&quot;.*?&quot;|&#39;.*?&#39;|`.*?`)/g, (match) => {
    const key = `@@CODE_STRING_${placeholders.length}@@`
    placeholders.push(`<span class="code-token--string">${match}</span>`)
    return key
  })

  html = html
    .replace(/\b(import|from|export|const|let|var|return|function|class|if|else|for|while|try|catch|async|await|new|def|raise|except|with|as)\b/g, '<span class="code-token--keyword">$1</span>')
    .replace(/(&lt;\/?)([A-Za-z][\w-]*)/g, '$1<span class="code-token--tag">$2</span>')
    .replace(/\b([A-Za-z_$][\w$]*)(?=\s*\()/g, '<span class="code-token--function">$1</span>')
    .replace(/\b([A-Za-z_:][-A-Za-z0-9_:]*)(?==)/g, '<span class="code-token--attr">$1</span>')

  placeholders.forEach((value, index) => {
    html = html.replace(`@@CODE_STRING_${index}@@`, value)
  })

  return html
}

const renderer = new marked.Renderer()
renderer.code = (token) => {
  const lang = token.lang ? escapeHtml(token.lang) : 'code'
  return `<div class="markdown-code"><div class="markdown-code__header"><span>${lang}</span></div><pre><code class="language-${lang}">${highlightCode(token.text || '')}</code></pre></div>`
}

marked.use({ renderer })

marked.setOptions({
  breaks: true,
  gfm: true
})

export const renderMarkdown = (text) => {
  if (!text && text !== '') return ''
  const rawHtml = marked.parse(String(text))
  return DOMPurify.sanitize(rawHtml)
}

export const delay = (ms) => new Promise(resolve => setTimeout(resolve, ms))

export const resolveRetryAfterMs = (value, fallback = 800) => {
  const numeric = Number(value)
  if (!Number.isFinite(numeric) || numeric <= 0) {
    return fallback
  }
  return Math.min(Math.max(Math.floor(numeric), 100), 3000)
}

export const buildServerError = (statusCode, message, retryAfterMs = 0) => {
  const error = new Error(message || '请求失败')
  error.serverCode = Number(statusCode || 0)
  error.retryAfterMs = resolveRetryAfterMs(retryAfterMs, 800)
  return error
}

export const isOwnerMismatchError = (error) => Number(error?.serverCode) === OWNER_MISMATCH_CODE
