<template>
  <div class="flex flex-row h-screen w-screen overflow-hidden bg-bg-light dark:bg-bg-dark text-text-primary-light dark:text-text-primary-dark">
    <!-- Sidebar -->
    <aside class="w-64 bg-surface-light dark:bg-surface-dark border-r border-border-light dark:border-border-dark flex flex-col flex-shrink-0">
      <div class="p-4 flex flex-col gap-4">
        <span class="text-sm font-semibold tracking-wide text-text-secondary-light dark:text-text-secondary-dark uppercase pl-2">会话列表</span>
        <button class="flex items-center justify-between w-full p-2.5 rounded-lg border border-border-light dark:border-border-dark bg-transparent hover:border-accent-light/50 dark:hover:border-accent-dark/50 hover:text-accent-light dark:hover:text-accent-dark transition-colors cursor-pointer text-sm font-medium" @click="createNewSession">
          <span>+ 新建会话</span>
          <kbd class="hidden md:inline-block px-1.5 py-0.5 text-xs text-text-secondary-light dark:text-text-secondary-dark bg-bg-light dark:bg-bg-dark rounded border border-border-light dark:border-border-dark font-mono">⌘K</kbd>
        </button>
      </div>
      <ul class="flex-1 overflow-y-auto list-none m-0 p-2 space-y-1">
        <li
          v-for="session in sessions"
          :key="session.id"
          :class="[
            'px-3 py-2.5 rounded-lg cursor-pointer text-sm transition-colors',
            currentSessionId === session.id 
              ? 'bg-black/5 dark:bg-white/5 font-medium text-text-primary-light dark:text-text-primary-dark flex items-center relative before:absolute before:left-0 before:top-1/2 before:-translate-y-1/2 before:h-4 before:w-1 before:bg-accent-light dark:before:bg-accent-dark before:rounded-r-full overflow-hidden' 
              : 'text-text-secondary-light dark:text-text-secondary-dark hover:bg-black/5 dark:hover:bg-white/5'
          ]"
          @click="switchSession(session.id)"
        >
          <span class="truncate block max-w-full">{{ session.name || `会话 ${session.id}` }}</span>
        </li>
      </ul>
    </aside>

    <!-- Main Content -->
    <section class="flex-1 flex flex-col relative min-w-0 bg-bg-light dark:bg-bg-dark">
      <!-- Header -->
      <div class="sticky top-0 z-10 glass border-b border-border-light dark:border-border-dark px-6 py-3 flex items-center gap-3 flex-wrap">
        <button class="px-3 py-1.5 text-sm rounded bg-transparent border border-border-light dark:border-border-dark hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer" @click="$router.push('/menu')">返回</button>
        <button class="px-3 py-1.5 text-sm rounded bg-transparent border border-border-light dark:border-border-dark hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer disabled:opacity-30" @click="syncHistory" :disabled="!currentSessionId || tempSession || loading">同步</button>
        <button class="px-3 py-1.5 text-sm rounded bg-transparent border border-border-light dark:border-border-dark hover:border-red-500 hover:text-red-500 transition-colors cursor-pointer disabled:opacity-30" @click="stopCurrentStream" :disabled="!loading || !isStreaming">停止</button>

        <div class="flex items-center gap-2 ml-auto">
          <label for="modelType" class="text-sm text-text-secondary-light dark:text-text-secondary-dark">模型</label>
          <select id="modelType" v-model="selectedModel" class="px-2 py-1.5 text-sm rounded border border-border-light dark:border-border-dark bg-transparent cursor-pointer outline-none focus:ring-1 focus:ring-accent-light dark:focus:ring-accent-dark disabled:opacity-50" :disabled="loading">
            <option v-for="option in modelOptions" :key="option.value" :value="option.value" class="bg-surface-light dark:bg-surface-dark">
              {{ option.label }}
            </option>
          </select>

          <label class="flex items-center gap-1.5 text-sm ml-2 cursor-pointer select-none">
            <input id="streamingMode" v-model="isStreaming" type="checkbox" class="accent-accent-light dark:accent-accent-dark" :disabled="loading" />
            流式
          </label>

          <button class="px-3 py-1.5 text-sm rounded bg-transparent border border-border-light dark:border-border-dark hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer disabled:opacity-50 ml-2" @click="triggerFileUpload" :disabled="uploading || loading">上传(.md/.txt)</button>
          <input
            ref="fileInput"
            type="file"
            accept=".md,.txt,text/markdown,text/plain"
            class="hidden"
            @change="handleFileUpload"
          />
        </div>
      </div>

      <!-- Messages Stream (Bubble-less) -->
      <div class="flex-1 overflow-y-auto px-8 md:px-24 pt-8 pb-40" ref="messagesRef">
        <div class="max-w-4xl mx-auto flex flex-col gap-12">
          <div
            v-for="(message, index) in currentMessages"
            :key="index"
            class="flex flex-col gap-2 group"
          >
            <!-- Sender Header -->
            <div class="flex items-center gap-3">
              <div :class="['w-8 h-8 rounded-full flex items-center justify-center font-bold text-sm select-none', message.role === 'user' ? 'bg-black text-white dark:bg-white dark:text-black' : 'bg-surface-light border border-border-light dark:bg-surface-dark dark:border-border-dark']">
                {{ message.role === 'user' ? 'U' : 'AI' }}
              </div>
              <span class="font-semibold text-sm">{{ message.role === 'user' ? '你' : 'AI' }}</span>
              <!-- Actions & Meta -->
              <div class="flex items-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                <button
                  v-if="message.role === 'assistant' && message.content"
                  class="px-2 py-0.5 text-xs rounded bg-surface-light dark:bg-surface-dark border border-border-light dark:border-border-dark hover:text-accent-light dark:hover:text-accent-dark cursor-pointer transition-colors"
                  @click="playTTS(message.content)"
                >
                  朗读
                </button>
              </div>
              <span v-if="getMessageMetaStatus(message)" class="text-xs text-text-secondary-light dark:text-text-secondary-dark ml-auto">
                {{ getMessageStatusLabel(getMessageMetaStatus(message)) }}
              </span>
            </div>
            
            <!-- Message Content block -->
            <div class="pl-11 text-base leading-relaxed space-y-4 break-words">
              <div v-html="renderMarkdown(message.content)" class="prose dark:prose-invert prose-p:my-2 prose-pre:bg-surface-light dark:prose-pre:bg-surface-dark prose-pre:border prose-pre:border-border-light dark:prose-pre:border-border-dark prose-pre:shadow-[0_2px_10px_rgba(0,0,0,0.02)] max-w-none"></div>
            </div>
          </div>
        </div>
      </div>

      <!-- Floating Pill Input -->
      <div class="absolute bottom-6 left-1/2 -translate-x-1/2 w-full max-w-3xl px-4 z-20">
        <div class="bg-surface-light dark:bg-surface-dark rounded-2xl shadow-[0_8px_30px_rgba(0,0,0,0.06)] dark:shadow-2xl ring-1 ring-black/5 dark:ring-white/10 flex items-end p-2 transition-shadow focus-within:ring-border-light dark:focus-within:ring-border-dark">
          <textarea
            v-model="inputMessage"
            placeholder="问点什么..."
            @keydown.enter.exact.prevent="sendMessage"
            :disabled="loading"
            ref="messageInput"
            rows="1"
            class="flex-1 max-h-40 min-h-[44px] bg-transparent border-none outline-none resize-none px-4 py-3 text-base text-text-primary-light dark:text-text-primary-dark placeholder-text-secondary-light dark:placeholder-text-secondary-dark"
          ></textarea>
          <button
            type="button"
            :disabled="!inputMessage.trim() || loading"
            @click="sendMessage"
            :class="[
              'p-2 w-10 h-10 mb-1 mr-1 rounded-xl flex items-center justify-center transition-all disabled:cursor-not-allowed',
              (!inputMessage.trim() || loading) 
                ? 'bg-transparent text-text-secondary-light dark:text-text-secondary-dark opacity-50' 
                : 'bg-black text-white dark:bg-white dark:text-black shadow-sm'
            ]"
          >
            <svg v-if="!loading" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" class="w-5 h-5">
              <path d="M3.478 2.404a.75.75 0 00-.926.941l2.432 7.905H13.5a.75.75 0 010 1.5H4.984l-2.432 7.905a.75.75 0 00.926.94 60.519 60.519 0 0018.445-8.986.75.75 0 000-1.218A60.517 60.517 0 003.478 2.404z" />
            </svg>
            <svg v-else class="animate-spin w-5 h-5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
          </button>
        </div>
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
        .replace(/`(.*?)`/g, '<code class="bg-black/5 dark:bg-white/10 px-1 py-0.5 rounded text-sm font-mono">$1</code>')
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
/* Removed old CSS */
</style>
