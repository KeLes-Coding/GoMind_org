<template>
  <div class="ai-chat-container">
    <aside class="session-list">
      <div class="session-list-header">
        <span>会话列表</span>
        <button class="new-chat-btn" @click="createNewSession">+ 新建会话</button>
      </div>
      <ul class="session-list-ul">
        <li
          v-for="session in sessions"
          :key="session.id"
          :class="['session-item', { active: currentSessionId === session.id }]"
          @click="switchSession(session.id)"
        >
          {{ session.name || `会话 ${session.id}` }}
        </li>
      </ul>
    </aside>

    <section class="chat-section">
      <div class="top-bar">
        <button class="back-btn" @click="$router.push('/menu')">返回</button>
        <button class="sync-btn" @click="syncHistory" :disabled="!currentSessionId || tempSession || loading">同步历史</button>
        <button class="stop-btn" @click="stopCurrentStream" :disabled="!loading || !isStreaming">停止生成</button>

        <label for="modelType">选择模型</label>
        <select id="modelType" v-model="selectedModel" class="model-select" :disabled="loading">
          <option v-for="option in modelOptions" :key="option.value" :value="option.value">
            {{ option.label }}
          </option>
        </select>

        <label class="streaming-mode" for="streamingMode">
          <input id="streamingMode" v-model="isStreaming" type="checkbox" :disabled="loading" />
          流式响应
        </label>

        <button class="upload-btn" @click="triggerFileUpload" :disabled="uploading || loading">上传文档 (.md/.txt)</button>
        <input
          ref="fileInput"
          type="file"
          accept=".md,.txt,text/markdown,text/plain"
          style="display: none"
          @change="handleFileUpload"
        />
      </div>

      <div class="chat-messages" ref="messagesRef">
        <div
          v-for="(message, index) in currentMessages"
          :key="index"
          :class="['message', message.role === 'user' ? 'user-message' : 'ai-message']"
        >
          <div class="message-header">
            <b>{{ message.role === 'user' ? '你' : 'AI' }}:</b>
            <button
              v-if="message.role === 'assistant' && message.content"
              class="tts-btn"
              @click="playTTS(message.content)"
            >
              朗读
            </button>
            <span v-if="getMessageMetaStatus(message)" :class="['message-status', `status-${getMessageMetaStatus(message)}`]">
              {{ getMessageStatusLabel(getMessageMetaStatus(message)) }}
            </span>
          </div>
          <div class="message-content" v-html="renderMarkdown(message.content)"></div>
        </div>
      </div>

      <div class="chat-input">
        <textarea
          v-model="inputMessage"
          placeholder="请输入你想问的问题..."
          @keydown.enter.exact.prevent="sendMessage"
          :disabled="loading"
          ref="messageInput"
          rows="1"
        ></textarea>
        <button
          type="button"
          :disabled="!inputMessage.trim() || loading"
          @click="sendMessage"
          class="send-btn"
        >
          {{ loading ? '发送中...' : '发送' }}
        </button>
      </div>
    </section>
  </div>
</template>

<script>
import { computed, nextTick, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import api, { refreshClient } from '../utils/api'
import { ensureAccessToken } from '../utils/token'

const TERMINAL_STATUSES = new Set(['completed', 'cancelled', 'timeout', 'failed', 'partial'])
const MODEL_OPTIONS = [
  { value: '1', label: 'DeepSeek' },
  { value: '2', label: 'DeepSeek RAG' },
  { value: '3', label: 'DeepSeek MCP' }
]

export default {
  name: 'AIChat',
  setup() {
    const sessions = ref({})
    const currentSessionId = ref(null)
    const tempSession = ref(false)
    const currentMessages = ref([])
    const inputMessage = ref('')
    const loading = ref(false)
    const messagesRef = ref(null)
    const messageInput = ref(null)
    const selectedModel = ref('1')
    const isStreaming = ref(false)
    const uploading = ref(false)
    const fileInput = ref(null)
    const modelOptions = MODEL_OPTIONS

    // 用于中断当前请求，保证停止按钮和异常处理共用同一个 controller。
    const activeAbortController = ref(null)
    // 记录当前流式响应对应的会话 ID，新会话开始时会先使用 temp。
    const activeStreamingSessionId = ref(null)
    // 指向当前 assistant 消息，便于更新停止、超时和失败状态。
    const activeAssistantIndex = ref(-1)
    // 区分用户手动停止与请求异常中断。
    const manualStopRequested = ref(false)

    const renderMarkdown = (text) => {
      if (!text && text !== '') return ''
      return String(text)
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.*?)\*/g, '<em>$1</em>')
        .replace(/`(.*?)`/g, '<code>$1</code>')
        .replace(/\n/g, '<br>')
    }

    // 统一后端返回和前端临时消息的状态值。
    const normalizeMessageStatus = (status) => {
      if (!status) return 'completed'
      const normalized = String(status).toLowerCase()
      return TERMINAL_STATUSES.has(normalized) || normalized === 'streaming' ? normalized : 'completed'
    }

    const buildMessageMeta = (status) => ({ status: normalizeMessageStatus(status) })

    const mapHistoryItemToMessage = (item) => ({
      role: item.is_user ? 'user' : 'assistant',
      content: item.content || '',
      meta: buildMessageMeta(item.status)
    })

    const getMessageStatusLabel = (status) => {
      switch (normalizeMessageStatus(status)) {
      case 'streaming':
        return '生成中'
      case 'cancelled':
        return '已停止'
      case 'timeout':
        return '已超时'
      case 'failed':
        return '失败'
      case 'partial':
        return '部分完成'
      default:
        return ''
      }
    }

    const getMessageMetaStatus = (message) => {
      if (!message || !message.meta) return ''
      return message.meta.status || ''
    }

    const scrollToBottom = () => {
      if (messagesRef.value) {
        try {
          messagesRef.value.scrollTop = messagesRef.value.scrollHeight
        } catch (error) {
          console.error('Scroll error:', error)
        }
      }
    }

    const syncSessionMessagesFromCurrent = async () => {
      if (!tempSession.value && currentSessionId.value && sessions.value[currentSessionId.value]) {
        sessions.value[currentSessionId.value].messages = [...currentMessages.value]
      }
      await nextTick()
      scrollToBottom()
    }

    const setAssistantStatus = async (status) => {
      if (activeAssistantIndex.value < 0 || !currentMessages.value[activeAssistantIndex.value]) {
        return
      }
      currentMessages.value[activeAssistantIndex.value].meta = buildMessageMeta(status)
      currentMessages.value = [...currentMessages.value]
      await syncSessionMessagesFromCurrent()
    }

    const appendAssistantChunk = async (chunk) => {
      if (activeAssistantIndex.value < 0 || !currentMessages.value[activeAssistantIndex.value]) {
        return
      }
      currentMessages.value[activeAssistantIndex.value].content += chunk
      currentMessages.value = [...currentMessages.value]
      await syncSessionMessagesFromCurrent()
    }

    const handleSSEPayload = async (data) => {
      if (!data) {
        return
      }

      if (data === '[DONE]') {
        loading.value = false
        await setAssistantStatus('completed')
        return 'done'
      }

      if (!data.startsWith('{')) {
        await appendAssistantChunk(data)
        return
      }

      let parsed
      try {
        parsed = JSON.parse(data)
      } catch {
        // 非法 JSON 直接忽略，避免把协议数据漏到聊天内容里。
        return
      }

      if (parsed.type === 'ready') {
        return
      }

      if (parsed.type === 'chunk') {
        await appendAssistantChunk(parsed.delta || '')
        return
      }

      if (parsed.sessionId) {
        const newSid = String(parsed.sessionId)
        activeStreamingSessionId.value = newSid
        if (tempSession.value) {
          sessions.value[newSid] = {
            id: newSid,
            name: '新会话',
            messages: [...currentMessages.value]
          }
          currentSessionId.value = newSid
          tempSession.value = false
        }
        return
      }

      if (parsed.type === 'error') {
        const error = new Error(parsed.message || '流式响应失败')
        error.serverCode = parsed.status_code
        throw error
      }
    }

    const clearActiveStreamState = () => {
      activeAbortController.value = null
      activeStreamingSessionId.value = null
      activeAssistantIndex.value = -1
      manualStopRequested.value = false
    }

    const playTTS = async (text) => {
      try {
        const createResponse = await api.post('/AI/chat/tts', { text })
        if (createResponse.data && createResponse.data.status_code === 1000 && createResponse.data.task_id) {
          const taskId = createResponse.data.task_id
          await new Promise(resolve => setTimeout(resolve, 5000))

          const maxAttempts = 30
          const pollInterval = 2000
          let attempts = 0

          const pollResult = async () => {
            const queryResponse = await api.get('/AI/chat/tts/query', { params: { task_id: taskId } })
            if (queryResponse.data && queryResponse.data.status_code === 1000) {
              const taskStatus = queryResponse.data.task_status
              if (taskStatus === 'Success' && queryResponse.data.task_result) {
                const audio = new Audio(queryResponse.data.task_result)
                audio.play()
                return true
              }
              if (taskStatus === 'Running' || taskStatus === 'Created') {
                attempts++
                if (attempts < maxAttempts) {
                  await new Promise(resolve => setTimeout(resolve, pollInterval))
                  return pollResult()
                }
                ElMessage.error('语音合成超时')
                return true
              }
              ElMessage.error('语音合成失败')
              return true
            }

            attempts++
            if (attempts < maxAttempts) {
              await new Promise(resolve => setTimeout(resolve, pollInterval))
              return pollResult()
            }
            ElMessage.error('语音合成超时')
            return true
          }

          await pollResult()
        } else {
          ElMessage.error('无法创建语音合成任务')
        }
      } catch (error) {
        console.error('TTS error:', error)
        ElMessage.error('语音接口请求失败')
      }
    }

    const loadSessions = async () => {
      try {
        const response = await api.get('/AI/chat/sessions')
        if (response.data && response.data.status_code === 1000 && Array.isArray(response.data.sessions)) {
          const sessionMap = {}
          response.data.sessions.forEach((sessionItem) => {
            const sid = String(sessionItem.sessionId)
            sessionMap[sid] = {
              id: sid,
              name: sessionItem.name || `会话 ${sid}`,
              messages: []
            }
          })
          sessions.value = sessionMap
        }
      } catch (error) {
        console.error('Load sessions error:', error)
      }
    }

    const loadHistoryIntoSession = async (sessionId) => {
      const response = await api.post('/AI/chat/history', { sessionId })
      if (response.data && response.data.status_code === 1000 && Array.isArray(response.data.history)) {
        sessions.value[sessionId].messages = response.data.history.map(mapHistoryItemToMessage)
        return
      }
      throw new Error(response.data?.status_msg || '无法加载会话历史')
    }

    const createNewSession = () => {
      currentSessionId.value = 'temp'
      tempSession.value = true
      currentMessages.value = []
      nextTick(() => {
        if (messageInput.value) messageInput.value.focus()
      })
    }

    const switchSession = async (sessionId) => {
      if (!sessionId || loading.value) return
      currentSessionId.value = String(sessionId)
      tempSession.value = false

      try {
        if (!sessions.value[sessionId].messages || sessions.value[sessionId].messages.length === 0) {
          await loadHistoryIntoSession(sessionId)
        }
        currentMessages.value = [...(sessions.value[sessionId].messages || [])]
        await nextTick()
        scrollToBottom()
      } catch (error) {
        console.error('Load history error:', error)
        ElMessage.error('加载历史失败')
      }
    }

    const syncHistory = async () => {
      if (!currentSessionId.value || tempSession.value) {
        ElMessage.warning('请先选择已有会话再同步历史')
        return
      }
      try {
        await loadHistoryIntoSession(currentSessionId.value)
        currentMessages.value = [...sessions.value[currentSessionId.value].messages]
        await nextTick()
        scrollToBottom()
      } catch (error) {
        console.error('Sync history error:', error)
        ElMessage.error('同步历史失败')
      }
    }

    const stopCurrentStream = async () => {
      if (!loading.value || !isStreaming.value) return

      manualStopRequested.value = true
      const targetSessionId = activeStreamingSessionId.value && activeStreamingSessionId.value !== 'temp'
        ? activeStreamingSessionId.value
        : (!tempSession.value ? currentSessionId.value : null)

      try {
        if (targetSessionId) {
          const response = await api.post('/AI/chat/stop', { sessionId: targetSessionId })
          if (response.data?.status_code !== 1000 && response.data?.status_code !== 2012) {
            throw new Error(response.data?.status_msg || '停止生成失败')
          }
        }
      } catch (error) {
        console.error('Stop stream error:', error)
      } finally {
        if (activeAbortController.value) {
          activeAbortController.value.abort()
        }
        loading.value = false
        await setAssistantStatus('cancelled')
        ElMessage.success('已停止当前生成')
      }
    }

    const sendMessage = async () => {
      if (!inputMessage.value || !inputMessage.value.trim()) {
        ElMessage.warning('请输入消息内容')
        return
      }

      const currentInput = inputMessage.value.trim()
      const userMessage = {
        role: 'user',
        content: currentInput,
        meta: buildMessageMeta('completed')
      }
      inputMessage.value = ''

      currentMessages.value.push(userMessage)
      await syncSessionMessagesFromCurrent()

      try {
        loading.value = true
        if (isStreaming.value) {
          await handleStreaming(currentInput)
        } else {
          await handleNormal(currentInput)
        }
      } catch (error) {
        console.error('Send message error:', error)
        ElMessage.error(error.message || '发送失败，请重试')

        if (!tempSession.value && currentSessionId.value && sessions.value[currentSessionId.value]?.messages?.length) {
          sessions.value[currentSessionId.value].messages.pop()
        }
        currentMessages.value.pop()
      } finally {
        if (!isStreaming.value) {
          loading.value = false
        }
        await nextTick()
        scrollToBottom()
      }
    }

    async function handleStreaming(question) {
      const aiMessage = {
        role: 'assistant',
        content: '',
        meta: buildMessageMeta('streaming')
      }

      activeAssistantIndex.value = currentMessages.value.length
      currentMessages.value.push(aiMessage)
      await syncSessionMessagesFromCurrent()

      const url = tempSession.value ? '/api/AI/chat/send-stream-new-session' : '/api/AI/chat/send-stream'
      const accessToken = await ensureAccessToken(refreshClient)
      const headers = {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${accessToken}`
      }
      const body = tempSession.value
        ? { question, modelType: selectedModel.value }
        : { question, modelType: selectedModel.value, sessionId: currentSessionId.value }

      const controller = new AbortController()
      activeAbortController.value = controller
      activeStreamingSessionId.value = tempSession.value ? 'temp' : currentSessionId.value
      manualStopRequested.value = false

      let doneReceived = false

      try {
        const response = await fetch(url, {
          method: 'POST',
          headers,
          body: JSON.stringify(body),
          signal: controller.signal
        })

        if (!response.ok || !response.body) {
          throw new Error('流式请求失败')
        }

        const reader = response.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''

        let streamClosed = false
        while (!streamClosed) {
          const { done, value } = await reader.read()
          if (done) {
            streamClosed = true
            break
          }

          buffer += decoder.decode(value, { stream: true })
          const events = buffer.split('\n\n')
          buffer = events.pop() || ''

          for (const eventText of events) {
            const dataLines = eventText
              .split('\n')
              .map(line => line.trim())
              .filter(line => line.startsWith('data:'))
              .map(line => line.slice(5).trim())

            if (!dataLines.length) {
              continue
            }

            const eventData = dataLines.join('\n')
            const result = await handleSSEPayload(eventData)
            if (result === 'done') {
              doneReceived = true
            }
          }
        }

        loading.value = false
        if (!doneReceived) {
          await setAssistantStatus(manualStopRequested.value ? 'cancelled' : 'partial')
        }
      } catch (error) {
        console.error('Stream error:', error)
        loading.value = false

        if (manualStopRequested.value || error.name === 'AbortError' || error.serverCode === 5004) {
          await setAssistantStatus('cancelled')
        } else if (error.serverCode === 4002) {
          await setAssistantStatus('timeout')
          ElMessage.error(error.message || '请求超时')
        } else {
          await setAssistantStatus('failed')
          ElMessage.error(error.message || '流式响应异常')
        }
      } finally {
        clearActiveStreamState()
      }
    }

    async function handleNormal(question) {
      if (tempSession.value) {
        const response = await api.post('/AI/chat/send-new-session', {
          question,
          modelType: selectedModel.value
        })
        if (response.data && response.data.status_code === 1000) {
          const sessionId = String(response.data.sessionId)
          const aiMessage = {
            role: 'assistant',
            content: response.data.Information || '',
            meta: buildMessageMeta('completed')
          }

          sessions.value[sessionId] = {
            id: sessionId,
            name: '新会话',
            messages: [{ role: 'user', content: question, meta: buildMessageMeta('completed') }, aiMessage]
          }
          currentSessionId.value = sessionId
          tempSession.value = false
          currentMessages.value = [...sessions.value[sessionId].messages]
        } else {
          throw new Error(response.data?.status_msg || '发送失败')
        }
      } else {
        const sessionMsgs = sessions.value[currentSessionId.value].messages || []
        sessionMsgs.push({ role: 'user', content: question, meta: buildMessageMeta('completed') })
        sessions.value[currentSessionId.value].messages = sessionMsgs

        const response = await api.post('/AI/chat/send', {
          question,
          modelType: selectedModel.value,
          sessionId: currentSessionId.value
        })
        if (response.data && response.data.status_code === 1000) {
          sessionMsgs.push({
            role: 'assistant',
            content: response.data.Information || '',
            meta: buildMessageMeta('completed')
          })
          currentMessages.value = [...sessionMsgs]
        } else {
          sessionMsgs.pop()
          throw new Error(response.data?.status_msg || '发送失败')
        }
      }
    }

    const triggerFileUpload = () => {
      if (fileInput.value) {
        fileInput.value.click()
      }
    }

    const handleFileUpload = async (event) => {
      const file = event.target.files[0]
      if (!file) return

      const fileName = file.name.toLowerCase()
      if (!fileName.endsWith('.md') && !fileName.endsWith('.txt')) {
        ElMessage.error('只支持上传 .md 和 .txt 文件')
        if (fileInput.value) {
          fileInput.value.value = ''
        }
        return
      }

      try {
        uploading.value = true
        const formData = new FormData()
        formData.append('file', file)

        const response = await api.post('/file/upload', formData, {
          headers: {
            'Content-Type': 'multipart/form-data'
          }
        })

        if (response.data && response.data.status_code === 1000) {
          ElMessage.success('文件上传成功')
        } else {
          ElMessage.error(response.data?.status_msg || '文件上传失败')
        }
      } catch (error) {
        console.error('File upload error:', error)
        ElMessage.error('文件上传失败')
      } finally {
        uploading.value = false
        if (fileInput.value) {
          fileInput.value.value = ''
        }
      }
    }

    onMounted(() => {
      loadSessions()
    })

    return {
      sessions: computed(() => Object.values(sessions.value)),
      currentSessionId,
      tempSession,
      currentMessages,
      inputMessage,
      loading,
      messagesRef,
      messageInput,
      selectedModel,
      isStreaming,
      uploading,
      fileInput,
      renderMarkdown,
      modelOptions,
      getMessageStatusLabel,
      getMessageMetaStatus,
      playTTS,
      createNewSession,
      switchSession,
      syncHistory,
      stopCurrentStream,
      sendMessage,
      triggerFileUpload,
      handleFileUpload
    }
  }
}
</script>

<style scoped>
.ai-chat-container {
  height: 100vh;
  display: flex;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: #222;
}

.session-list {
  width: 280px;
  display: flex;
  flex-direction: column;
  background: rgba(255, 255, 255, 0.95);
  border-right: 1px solid rgba(0, 0, 0, 0.08);
}

.session-list-header {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  font-weight: 600;
}

.new-chat-btn,
.sync-btn,
.stop-btn,
.upload-btn,
.back-btn,
.send-btn,
.tts-btn {
  border: none;
  border-radius: 10px;
  cursor: pointer;
  color: #fff;
}

.new-chat-btn,
.send-btn {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.sync-btn {
  background: linear-gradient(135deg, #67c23a 0%, #409eff 100%);
}

.stop-btn {
  background: linear-gradient(135deg, #f56c6c 0%, #e67e22 100%);
}

.upload-btn {
  background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%);
}

.back-btn {
  background: #5b6c8f;
}

.tts-btn {
  padding: 4px 8px;
  background: #409eff;
}

.new-chat-btn,
.sync-btn,
.stop-btn,
.upload-btn,
.back-btn {
  padding: 8px 14px;
}

.session-list-ul {
  list-style: none;
  padding: 0;
  margin: 0;
  flex: 1;
  overflow-y: auto;
}

.session-item {
  padding: 14px 20px;
  cursor: pointer;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.session-item.active {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: #fff;
  font-weight: 600;
}

.chat-section {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.top-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 20px;
  background: rgba(255, 255, 255, 0.95);
  border-bottom: 1px solid rgba(0, 0, 0, 0.08);
  flex-wrap: wrap;
}

.streaming-mode {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.model-select {
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid rgba(0, 0, 0, 0.12);
}

.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.message {
  max-width: 72%;
  padding: 14px 18px;
  border-radius: 16px;
  line-height: 1.6;
  word-break: break-word;
}

.user-message {
  align-self: flex-end;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: #fff;
}

.ai-message {
  align-self: flex-start;
  background: rgba(255, 255, 255, 0.96);
  color: #2c3e50;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
}

.message-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.message-status {
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
}

.status-streaming {
  background: rgba(64, 158, 255, 0.12);
  color: #409eff;
}

.status-completed {
  background: rgba(103, 194, 58, 0.12);
  color: #67c23a;
}

.status-cancelled {
  background: rgba(230, 126, 34, 0.14);
  color: #e67e22;
}

.status-timeout,
.status-failed,
.status-partial {
  background: rgba(245, 108, 108, 0.12);
  color: #f56c6c;
}

.message-content {
  white-space: pre-wrap;
}

.chat-input {
  position: relative;
  padding: 20px;
  background: rgba(255, 255, 255, 0.96);
  border-top: 1px solid rgba(0, 0, 0, 0.06);
}

.chat-input textarea {
  width: 100%;
  min-height: 52px;
  max-height: 160px;
  resize: none;
  border-radius: 12px;
  border: 1px solid rgba(0, 0, 0, 0.12);
  padding: 14px 16px;
  box-sizing: border-box;
}

.send-btn {
  position: absolute;
  right: 32px;
  bottom: 30px;
  padding: 10px 18px;
}

.new-chat-btn:disabled,
.sync-btn:disabled,
.stop-btn:disabled,
.upload-btn:disabled,
.send-btn:disabled {
  background: #c0c4cc;
  cursor: not-allowed;
}
</style>
