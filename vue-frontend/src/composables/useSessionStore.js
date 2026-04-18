import { computed, nextTick, ref } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../utils/api'
import { buildMessageMeta } from '../utils/messageHelpers'

export function useSessionStore() {
  const sessions = ref({})
  const foldersList = ref([])
  const ungroupedSessionsList = ref([])
  const sessionFolders = ref([])
  const ungroupedSessionIds = ref([])
  const expandedFolders = ref({})
  const collapsedFolders = ref({})
  const currentSessionId = ref(null)
  const tempSession = ref(false)
  const currentMessages = ref([])
  const messagesRef = ref(null)

  const sidebarFolders = computed(() => sessionFolders.value.map(folder => ({
    ...folder,
    sessions: (folder.sessionIds || [])
      .map(sessionId => sessions.value[sessionId])
      .filter(Boolean)
  })))

  const ungroupedSessions = computed(() => ungroupedSessionIds.value
    .map(sessionId => sessions.value[sessionId])
    .filter(Boolean))

  const isFolderExpanded = (folderId) => expandedFolders.value[String(folderId)] !== false

  const toggleFolder = (folderId) => {
    const key = String(folderId)
    expandedFolders.value = {
      ...expandedFolders.value,
      [key]: !isFolderExpanded(key)
    }
    collapsedFolders.value[key] = !collapsedFolders.value[key]
  }

  const buildSessionRoutingHeaders = (sessionId) => {
    const normalizedId = String(sessionId || '').trim()
    if (!normalizedId || normalizedId === 'temp') {
      return {}
    }
    return {
      'X-Chat-Session-ID': normalizedId
    }
  }

  const buildSessionTitle = (question) => {
    const title = String(question || '').trim()
    return title || 'New session'
  }

  const mapHistoryItemToMessage = (item) => ({
    role: item.is_user ? 'user' : 'assistant',
    content: item.content || '',
    meta: buildMessageMeta(item.status)
  })

  const scrollToBottom = () => {
    if (messagesRef.value) {
      try {
        messagesRef.value.scrollTop = messagesRef.value.scrollHeight
      } catch (error) {
        console.error('Scroll error:', error)
      }
    }
  }

  const ensureSessionEntry = (sessionId) => {
    const normalizedId = String(sessionId || '')
    if (!normalizedId || normalizedId === 'temp') {
      return null
    }
    if (!sessions.value[normalizedId]) {
      sessions.value[normalizedId] = {
        id: normalizedId,
        name: `会话 ${normalizedId}`,
        messages: []
      }
    } else if (!Array.isArray(sessions.value[normalizedId].messages)) {
      sessions.value[normalizedId].messages = []
    }
    return sessions.value[normalizedId]
  }

  const upsertSessionEntry = (sessionData, options = {}) => {
    const normalizedId = String(sessionData?.id || '')
    if (!normalizedId || normalizedId === 'temp') {
      return null
    }

    const existing = sessions.value[normalizedId] || {
      id: normalizedId,
      name: `会话 ${normalizedId}`,
      messages: []
    }
    const nextEntry = {
      ...existing,
      ...sessionData,
      id: normalizedId,
      messages: Array.isArray(sessionData?.messages)
        ? sessionData.messages
        : (Array.isArray(existing.messages) ? existing.messages : [])
    }

    const nextSessions = {}
    const shouldPrepend = options.prepend !== false
    if (shouldPrepend) {
      nextSessions[normalizedId] = nextEntry
    }

    Object.entries(sessions.value).forEach(([key, value]) => {
      if (key !== normalizedId) {
        nextSessions[key] = value
      }
    })

    if (!shouldPrepend) {
      nextSessions[normalizedId] = nextEntry
    }

    sessions.value = nextSessions
    ensureSessionListed(normalizedId)
    return nextEntry
  }

  const ensureSessionListed = (sessionId) => {
    const normalizedId = String(sessionId || '')
    if (!normalizedId || normalizedId === 'temp') return

    const inFolder = sessionFolders.value.some(folder => (folder.sessionIds || []).includes(normalizedId))
    const inUngrouped = ungroupedSessionIds.value.includes(normalizedId)
    if (!inFolder && !inUngrouped) {
      ungroupedSessionIds.value = [normalizedId, ...ungroupedSessionIds.value]
    }

    const entry = sessions.value[normalizedId]
    if (!entry) return
    const listItem = {
      sessionId: normalizedId,
      name: entry.name || `会话 ${normalizedId}`,
      folderId: entry.folderId || null
    }

    foldersList.value = foldersList.value.map(folder => {
      const sessionsInFolder = Array.isArray(folder.sessions) ? folder.sessions : []
      const index = sessionsInFolder.findIndex(item => String(item.sessionId) === normalizedId)
      if (index < 0) {
        return folder
      }
      const nextSessions = [...sessionsInFolder]
      nextSessions[index] = { ...nextSessions[index], ...listItem, folderId: folder.id }
      return { ...folder, sessions: nextSessions }
    })

    if (!inFolder) {
      const withoutExisting = ungroupedSessionsList.value
        .filter(item => String(item.sessionId) !== normalizedId)
      ungroupedSessionsList.value = [listItem, ...withoutExisting]
    }
  }

  const applySessionTree = (tree) => {
    const nextSessionMap = {}
    const nextFolders = []
    const nextExpanded = { ...expandedFolders.value }
    const nextUngrouped = []

    ;(tree?.folders || []).forEach(folder => {
      const folderId = String(folder.id)
      nextExpanded[folderId] = expandedFolders.value[folderId] !== false
      const folderSessionIds = []

      ;(folder.sessions || []).forEach(sessionItem => {
        const sid = String(sessionItem.sessionId)
        const local = sessions.value[sid]
        nextSessionMap[sid] = {
          id: sid,
          name: sessionItem.name || (local && local.name) || `Session ${sid}`,
          messages: local && Array.isArray(local.messages) ? local.messages : []
        }
        folderSessionIds.push(sid)
      })

      nextFolders.push({
        id: folder.id,
        name: folder.name || `Folder ${folder.id}`,
        sessionIds: folderSessionIds
      })
    })

    ;(tree?.ungroupedSessions || tree?.ungrouped_sessions || []).forEach(sessionItem => {
      const sid = String(sessionItem.sessionId)
      const local = sessions.value[sid]
      nextSessionMap[sid] = {
        id: sid,
        name: sessionItem.name || (local && local.name) || `Session ${sid}`,
        messages: local && Array.isArray(local.messages) ? local.messages : []
      }
      nextUngrouped.push(sid)
    })

    sessions.value = nextSessionMap
    sessionFolders.value = nextFolders
    ungroupedSessionIds.value = nextUngrouped
    expandedFolders.value = nextExpanded
  }

  const syncSessionMessagesFromCurrent = async () => {
    if (!tempSession.value && currentSessionId.value && sessions.value[currentSessionId.value]) {
      sessions.value[currentSessionId.value].messages = [...currentMessages.value]
    }
    await nextTick()
    scrollToBottom()
  }

  const loadSessions = async () => {
    try {
      const response = await api.get('/AI/chat/session-tree')
      if (response.data && response.data.status_code === 1000 && response.data.tree) {
        const tree = response.data.tree
        const savedNames = {}
        const savedMessages = {}
        Object.entries(sessions.value).forEach(([id, s]) => {
          if (s.name && s.name !== `会话 ${id}` && s.name !== `Session ${id}`) {
            savedNames[id] = s.name
          }
          if (Array.isArray(s.messages) && s.messages.length > 0) {
            savedMessages[id] = s.messages
          }
        })
        foldersList.value = tree.folders || []
        ungroupedSessionsList.value = tree.ungrouped_sessions || []
        const sessionMap = {}
        foldersList.value.forEach(f => {
          if (!(f.id in collapsedFolders.value)) {
            collapsedFolders.value[f.id] = false
          }
          if (f.sessions) {
            f.sessions.forEach(s => {
              sessionMap[s.sessionId] = {
                id: s.sessionId,
                name: s.name || savedNames[s.sessionId] || `会话 ${s.sessionId}`,
                folderId: s.folderId,
                messages: savedMessages[s.sessionId] || []
              }
            })
          } else {
            f.sessions = []
          }
        })
        ungroupedSessionsList.value.forEach(s => {
          sessionMap[s.sessionId] = {
            id: s.sessionId,
            name: s.name || savedNames[s.sessionId] || `会话 ${s.sessionId}`,
            folderId: null,
            messages: savedMessages[s.sessionId] || []
          }
        })
        sessions.value = sessionMap
        sessionFolders.value = foldersList.value.map(folder => ({
          id: folder.id,
          name: folder.name || `Folder ${folder.id}`,
          sessionIds: (folder.sessions || []).map(s => s.sessionId)
        }))
        ungroupedSessionIds.value = ungroupedSessionsList.value.map(s => s.sessionId)
      }
    } catch (error) {
      console.error('Load session tree error:', error)
    }
  }

  const loadHistoryIntoSession = async (sessionId) => {
    const targetSession = ensureSessionEntry(sessionId)
    const response = await api.post('/AI/chat/history', { sessionId }, {
      headers: buildSessionRoutingHeaders(sessionId)
    })
    if (response.data && response.data.status_code === 1000 && Array.isArray(response.data.history)) {
      targetSession.messages = response.data.history.map(mapHistoryItemToMessage)
      return
    }
    throw new Error(response.data?.status_msg || '无法加载会话历史')
  }

  const createNewSession = () => {
    currentSessionId.value = 'temp'
    tempSession.value = true
    currentMessages.value = []
  }

  const ensureActiveDraftSession = () => {
    if (currentSessionId.value && !tempSession.value) {
      return
    }
    if (tempSession.value && currentSessionId.value === 'temp') {
      return
    }
    currentSessionId.value = 'temp'
    tempSession.value = true
    currentMessages.value = []
  }

  const switchSession = async (sessionId) => {
    if (!sessionId) return
    const targetSession = ensureSessionEntry(sessionId)
    currentSessionId.value = String(sessionId)
    tempSession.value = false

    try {
      if (!targetSession.messages || targetSession.messages.length === 0) {
        await loadHistoryIntoSession(sessionId)
      }
      currentMessages.value = [...(ensureSessionEntry(sessionId)?.messages || [])]
      await nextTick()
      scrollToBottom()
    } catch (error) {
      console.error('Load history error:', error)
      ElMessage.error('加载历史失败')
    }
  }

  const syncHistory = async () => {
    if (!currentSessionId.value || tempSession.value) {
      ElMessage.warning('Please select an existing session first')
      return
    }
    try {
      await loadHistoryIntoSession(currentSessionId.value)
      currentMessages.value = [...(ensureSessionEntry(currentSessionId.value)?.messages || [])]
      await nextTick()
      scrollToBottom()
    } catch (error) {
      console.error('Sync history error:', error)
      ElMessage.error('同步历史失败')
    }
  }

  return {
    sessions,
    foldersList,
    ungroupedSessionsList,
    sessionFolders,
    ungroupedSessionIds,
    expandedFolders,
    collapsedFolders,
    currentSessionId,
    tempSession,
    currentMessages,
    messagesRef,
    sidebarFolders,
    ungroupedSessions,
    isFolderExpanded,
    toggleFolder,
    buildSessionRoutingHeaders,
    buildSessionTitle,
    mapHistoryItemToMessage,
    scrollToBottom,
    ensureSessionEntry,
    upsertSessionEntry,
    ensureSessionListed,
    applySessionTree,
    syncSessionMessagesFromCurrent,
    loadSessions,
    loadHistoryIntoSession,
    createNewSession,
    ensureActiveDraftSession,
    switchSession,
    syncHistory
  }
}
