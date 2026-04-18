import { nextTick, ref } from 'vue'
import { ElMessage } from 'element-plus'
import api, { refreshClient } from '../utils/api'
import { ensureAccessToken } from '../utils/token'
import {
  OWNER_MISMATCH_CODE,
  MAX_OWNER_MISMATCH_RETRIES,
  normalizeMessageStatus,
  buildMessageMeta,
  delay,
  resolveRetryAfterMs,
  buildServerError,
  isOwnerMismatchError
} from '../utils/messageHelpers'

export function useChatStream({
  sessions,
  currentSessionId,
  tempSession,
  currentMessages,
  syncSessionMessagesFromCurrent,
  ensureSessionEntry,
  upsertSessionEntry,
  ensureActiveDraftSession,
  scrollToBottom,
  buildSessionRoutingHeaders,
  buildSessionTitle,
  loadSessions,
  selectedConfigId,
  selectedChatMode
}) {
  const inputMessage = ref('')
  const loading = ref(false)
  const messageInput = ref(null)
  const isStreaming = ref(true)

  const activeAbortController = ref(null)
  const activeStreamingSessionId = ref(null)
  const activeStreamId = ref(null)
  const activeMessageId = ref(null)
  const activeStreamLastSeq = ref(0)
  const activeAssistantIndex = ref(-1)
  const manualStopRequested = ref(false)

  const postWithOwnerRetry = async (url, body, options = {}) => {
    const maxRetries = typeof options.maxRetries === 'number' ? options.maxRetries : MAX_OWNER_MISMATCH_RETRIES
    for (let attempt = 0; attempt <= maxRetries; attempt += 1) {
      const response = await api.post(url, body, options.requestConfig || {})
      if (response.data?.status_code === 1000) {
        return response
      }

      if (response.data?.status_code === OWNER_MISMATCH_CODE && attempt < maxRetries) {
        await delay(resolveRetryAfterMs(response.data?.retry_after_ms, 800))
        continue
      }

      throw buildServerError(
        response.data?.status_code,
        response.data?.status_msg || 'Send failed',
        response.data?.retry_after_ms || 0
      )
    }
  }

  const setAssistantStatus = async (status) => {
    if (activeAssistantIndex.value < 0 || !currentMessages.value[activeAssistantIndex.value]) {
      return
    }
    const prevMeta = currentMessages.value[activeAssistantIndex.value].meta || {}
    currentMessages.value[activeAssistantIndex.value].meta = buildMessageMeta(status, {
      streamId: prevMeta.streamId || activeStreamId.value,
      messageId: prevMeta.messageId || activeMessageId.value,
      lastSeq: typeof prevMeta.lastSeq === 'number' ? prevMeta.lastSeq : activeStreamLastSeq.value
    })
    currentMessages.value = [...currentMessages.value]
    await syncSessionMessagesFromCurrent()
  }

  const patchActiveAssistantMeta = async (patch = {}) => {
    if (activeAssistantIndex.value < 0 || !currentMessages.value[activeAssistantIndex.value]) {
      return
    }
    const prevMeta = currentMessages.value[activeAssistantIndex.value].meta || {}
    const nextStatus = patch.status ? normalizeMessageStatus(patch.status) : normalizeMessageStatus(prevMeta.status || 'streaming')
    currentMessages.value[activeAssistantIndex.value].meta = {
      ...prevMeta,
      ...patch,
      status: nextStatus
    }
    currentMessages.value = [...currentMessages.value]
    await syncSessionMessagesFromCurrent()
  }

  const appendAssistantChunk = async (chunk, seq = null) => {
    if (activeAssistantIndex.value < 0 || !currentMessages.value[activeAssistantIndex.value]) {
      return
    }
    if (seq !== null && Number(seq) <= activeStreamLastSeq.value) {
      return
    }
    currentMessages.value[activeAssistantIndex.value].content += chunk
    if (seq !== null) {
      activeStreamLastSeq.value = Number(seq)
    }
    const prevMeta = currentMessages.value[activeAssistantIndex.value].meta || {}
    currentMessages.value[activeAssistantIndex.value].meta = {
      ...prevMeta,
      streamId: prevMeta.streamId || activeStreamId.value,
      messageId: prevMeta.messageId || activeMessageId.value,
      lastSeq: activeStreamLastSeq.value,
      status: normalizeMessageStatus(prevMeta.status || 'streaming')
    }
    currentMessages.value = [...currentMessages.value]
    await syncSessionMessagesFromCurrent()
  }

  const applyAssistantSnapshot = async (content, lastSeq) => {
    if (activeAssistantIndex.value < 0 || !currentMessages.value[activeAssistantIndex.value]) {
      return
    }
    currentMessages.value[activeAssistantIndex.value].content = content || ''
    activeStreamLastSeq.value = Number(lastSeq || 0)
    const prevMeta = currentMessages.value[activeAssistantIndex.value].meta || {}
    currentMessages.value[activeAssistantIndex.value].meta = {
      ...prevMeta,
      streamId: prevMeta.streamId || activeStreamId.value,
      messageId: prevMeta.messageId || activeMessageId.value,
      lastSeq: activeStreamLastSeq.value,
      status: normalizeMessageStatus(prevMeta.status || 'streaming')
    }
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
      return
    }

    if (parsed.type === 'ready') {
      if (parsed.streamId) {
        activeStreamId.value = String(parsed.streamId)
        await patchActiveAssistantMeta({ streamId: activeStreamId.value })
      }
      return
    }

    if (parsed.type === 'chunk') {
      await appendAssistantChunk(parsed.delta || '', parsed.seq)
      return
    }

    if (parsed.type === 'snapshot') {
      if (parsed.streamId) {
        activeStreamId.value = String(parsed.streamId)
      }
      if (parsed.messageId) {
        activeMessageId.value = String(parsed.messageId)
      }
      await applyAssistantSnapshot(parsed.content || '', parsed.lastSeq || 0)
      return
    }

    if (parsed.type === 'done') {
      if (typeof parsed.lastSeq === 'number') {
        activeStreamLastSeq.value = parsed.lastSeq
      }
      loading.value = false
      await setAssistantStatus(parsed.status || 'completed')
      return 'done'
    }

    if (parsed.type === 'session') {
      if (parsed.streamId) {
        activeStreamId.value = String(parsed.streamId)
      }
      if (parsed.messageId) {
        activeMessageId.value = String(parsed.messageId)
      }
      activeStreamLastSeq.value = 0
      await patchActiveAssistantMeta({
        streamId: activeStreamId.value,
        messageId: activeMessageId.value,
        lastSeq: 0
      })
      return
    }

    if (parsed.sessionId) {
      const newSid = String(parsed.sessionId)
      activeStreamingSessionId.value = newSid
      if (tempSession.value) {
        upsertSessionEntry({
          id: newSid,
          name: buildSessionTitle(currentMessages.value.find(message => message.role === 'user')?.content),
          messages: [...currentMessages.value]
        })
        currentSessionId.value = newSid
        tempSession.value = false
        loadSessions()
      }
      await patchActiveAssistantMeta({
        streamId: activeStreamId.value,
        messageId: activeMessageId.value,
        lastSeq: activeStreamLastSeq.value
      })
      return
    }

    if (parsed.type === 'error') {
      throw buildServerError(parsed.status_code, parsed.message || '流式响应失败', parsed.retry_after_ms || 0)
    }
  }

  const clearActiveStreamState = () => {
    activeAbortController.value = null
    activeStreamingSessionId.value = null
    activeStreamId.value = null
    activeMessageId.value = null
    activeStreamLastSeq.value = 0
    activeAssistantIndex.value = -1
    manualStopRequested.value = false
  }

  const stopCurrentStream = async () => {
    if (!loading.value) return

    manualStopRequested.value = true
    const targetSessionId = activeStreamingSessionId.value && activeStreamingSessionId.value !== 'temp'
      ? activeStreamingSessionId.value
      : (!tempSession.value ? currentSessionId.value : null)

    try {
      if (targetSessionId) {
        const response = await api.post('/AI/chat/stop', { sessionId: targetSessionId }, {
          headers: buildSessionRoutingHeaders(targetSessionId)
        })
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
      ElMessage.success('Stopped current generation')
    }
  }

  const sendMessage = async () => {
    if (!inputMessage.value || !inputMessage.value.trim()) {
      ElMessage.warning('Please enter a message')
      return
    }

    if (!currentSessionId.value) {
      ensureActiveDraftSession()
    }

    const currentInput = inputMessage.value.trim()
    const userMessage = {
      role: 'user',
      content: currentInput,
      meta: buildMessageMeta('completed')
    }
    inputMessage.value = ''

    currentMessages.value.push(userMessage)
    if (!tempSession.value && currentSessionId.value && sessions.value[currentSessionId.value]) {
      upsertSessionEntry({
        ...sessions.value[currentSessionId.value],
        messages: [...currentMessages.value]
      })
    }
    await syncSessionMessagesFromCurrent()

    try {
      loading.value = true
      await handleStreaming(currentInput)
    } catch (error) {
      console.error('Send message error:', error)
      ElMessage.error(error.message || 'Send failed, please try again')

      if (!tempSession.value && currentSessionId.value && sessions.value[currentSessionId.value]?.messages?.length) {
        sessions.value[currentSessionId.value].messages.pop()
      }
      currentMessages.value.pop()
    } finally {
      loading.value = false
      await nextTick()
      scrollToBottom()
    }
  }

  async function handleStreaming(question) {
    const aiMessage = {
      role: 'assistant',
      content: '',
      meta: buildMessageMeta('streaming', {
        streamId: null,
        messageId: null,
        lastSeq: 0
      })
    }

    activeAssistantIndex.value = currentMessages.value.length
    currentMessages.value.push(aiMessage)
    await syncSessionMessagesFromCurrent()

    const url = tempSession.value ? '/api/AI/chat/send-stream-new-session' : '/api/AI/chat/send-stream'
    const accessToken = await ensureAccessToken(refreshClient)
    const baseHeaders = {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${accessToken}`
    }
    if (!tempSession.value && currentSessionId.value) {
      Object.assign(baseHeaders, buildSessionRoutingHeaders(currentSessionId.value))
    }
    const body = tempSession.value
      ? { question, llmConfigId: selectedConfigId.value, chatMode: selectedChatMode.value, modelType: '1' }
      : { question, llmConfigId: selectedConfigId.value, chatMode: selectedChatMode.value, modelType: '1', sessionId: currentSessionId.value }

    const controller = new AbortController()
    activeAbortController.value = controller
    activeStreamingSessionId.value = tempSession.value ? 'temp' : currentSessionId.value
    activeStreamId.value = null
    activeMessageId.value = null
    activeStreamLastSeq.value = 0
    manualStopRequested.value = false

    const consumeSSEStream = async (response) => {
      if (!response.ok || !response.body) {
        throw new Error('流式请求失败')
      }

      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      let doneReceived = false
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

      return doneReceived
    }

    const openInitialStream = async () => fetch(url, {
      method: 'POST',
      headers: baseHeaders,
      body: JSON.stringify(body),
      signal: controller.signal
    })

    const openResumeStream = async () => {
      if (!activeStreamId.value || !activeStreamingSessionId.value || activeStreamingSessionId.value === 'temp') {
        throw new Error('当前流缺少恢复信息')
      }
      const resumeHeaders = {
        ...baseHeaders,
        ...buildSessionRoutingHeaders(activeStreamingSessionId.value)
      }
      return fetch('/api/AI/chat/resume-stream', {
        method: 'POST',
        headers: resumeHeaders,
        body: JSON.stringify({
          sessionId: activeStreamingSessionId.value,
          streamId: activeStreamId.value,
          lastSeq: activeStreamLastSeq.value
        }),
        signal: controller.signal
      })
    }

    let doneReceived = false
    let streamError = null
    let mode = 'initial'
    let resumeAttempts = 0
    let ownerMismatchRetries = 0

    try {
      while (!doneReceived) {
        try {
          const response = mode === 'initial'
            ? await openInitialStream()
            : await openResumeStream()
          doneReceived = await consumeSSEStream(response)
          if (!doneReceived) {
            if (manualStopRequested.value || !activeStreamId.value || resumeAttempts >= 2) {
              break
            }
            resumeAttempts += 1
            mode = 'resume'
          }
        } catch (error) {
          if (manualStopRequested.value || error.name === 'AbortError' || error.serverCode === 5004) {
            throw error
          }
          if (isOwnerMismatchError(error) && ownerMismatchRetries < MAX_OWNER_MISMATCH_RETRIES) {
            ownerMismatchRetries += 1
            await delay(resolveRetryAfterMs(error.retryAfterMs, 800))
            continue
          }
          if (activeStreamId.value && activeStreamingSessionId.value && activeStreamingSessionId.value !== 'temp' && resumeAttempts < 2) {
            resumeAttempts += 1
            mode = 'resume'
            continue
          }
          streamError = error
          break
        }
      }

      if (streamError) {
        throw streamError
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
      } else if (isOwnerMismatchError(error)) {
        await setAssistantStatus('failed')
        ElMessage.error(error.message || '会话正在切换执行节点，请稍后重试')
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
      const response = await postWithOwnerRetry('/AI/chat/send-new-session', {
        question,
        llmConfigId: selectedConfigId.value,
        chatMode: selectedChatMode.value,
        modelType: '1'
      })
      if (response.data && response.data.status_code === 1000) {
        const sessionId = String(response.data.sessionId)
        const aiMessage = {
          role: 'assistant',
          content: response.data.Information || '',
          meta: buildMessageMeta('completed')
        }

        upsertSessionEntry({
          id: sessionId,
          name: buildSessionTitle(question),
          messages: [{ role: 'user', content: question, meta: buildMessageMeta('completed') }, aiMessage]
        })
        currentSessionId.value = sessionId
        tempSession.value = false
        currentMessages.value = [...sessions.value[sessionId].messages]
        loadSessions()
      }
    } else {
      const targetSession = ensureSessionEntry(currentSessionId.value)
      const sessionMsgs = targetSession ? (targetSession.messages || []) : []
      sessionMsgs.push({ role: 'user', content: question, meta: buildMessageMeta('completed') })
      if (targetSession) {
        targetSession.messages = sessionMsgs
      }

      const response = await postWithOwnerRetry('/AI/chat/send', {
        question,
        llmConfigId: selectedConfigId.value,
        chatMode: selectedChatMode.value,
        modelType: '1',
        sessionId: currentSessionId.value
      }, {
        requestConfig: {
          headers: buildSessionRoutingHeaders(currentSessionId.value)
        }
      })
      if (response.data && response.data.status_code === 1000) {
        sessionMsgs.push({
          role: 'assistant',
          content: response.data.Information || '',
          meta: buildMessageMeta('completed')
        })
        currentMessages.value = [...sessionMsgs]
      }
    }
  }

  return {
    inputMessage,
    loading,
    messageInput,
    isStreaming,
    postWithOwnerRetry,
    setAssistantStatus,
    patchActiveAssistantMeta,
    appendAssistantChunk,
    applyAssistantSnapshot,
    handleSSEPayload,
    clearActiveStreamState,
    stopCurrentStream,
    sendMessage,
    handleStreaming,
    handleNormal
  }
}