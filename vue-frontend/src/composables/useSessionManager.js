import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../utils/api'

export function useSessionManager({ currentSessionId, currentMessages, tempSession, loadSessions, createNewSession, sessionFolders }) {
  const renameSessionDialogVisible = ref(false)
  const renameSessionForm = ref({ sessionId: '', name: '' })
  const submittingRenameSession = ref(false)

  const moveSessionDialogVisible = ref(false)
  const moveSessionForm = ref({ sessionId: '', folderId: '' })
  const submittingMoveSession = ref(false)

  const handleSessionCommand = (cmd, session) => {
    if (cmd === 'rename') {
      renameSessionForm.value = { sessionId: session.sessionId, name: session.name }
      renameSessionDialogVisible.value = true
    } else if (cmd === 'move') {
      moveSessionForm.value = { sessionId: session.sessionId, folderId: session.folderId || '' }
      moveSessionDialogVisible.value = true
    } else if (cmd === 'removeFromFolder') {
      api.post('/AI/chat/session/remove-from-folder', { sessionId: session.sessionId }).then(res => {
        if (res.data?.status_code === 1000) {
          ElMessage.success('已移出文件夹')
          loadSessions()
        } else {
          ElMessage.error('移出失败')
        }
      })
    } else if (cmd === 'delete') {
      ElMessageBox.confirm('会话删除后无法恢复，确定删除该会话吗？', '删除确认', {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }).then(async () => {
        try {
          const res = await api.post('/AI/chat/session/delete', { sessionId: session.sessionId })
          if (res.data?.status_code === 1000) {
            ElMessage.success('已删除')
            if (currentSessionId.value === String(session.sessionId)) {
              currentSessionId.value = 'temp'
              currentMessages.value = []
              tempSession.value = true
            }
            await loadSessions()
          } else {
            ElMessage.error(res.data?.status_msg || '删除失败')
          }
        } catch (e) {
          ElMessage.error('删除异常')
        }
      }).catch(() => {})
    }
  }

  const submitRenameSession = async () => {
    if (!renameSessionForm.value.name.trim() || submittingRenameSession.value) return
    submittingRenameSession.value = true
    try {
      const res = await api.post('/AI/chat/session/rename', { sessionId: renameSessionForm.value.sessionId, title: renameSessionForm.value.name })
      if (res.data?.status_code === 1000) {
        renameSessionDialogVisible.value = false
        await loadSessions()
      } else {
        ElMessage.error('重命名失败')
      }
    } finally {
      submittingRenameSession.value = false
    }
  }

  const submitMoveSession = async () => {
    if (submittingMoveSession.value) return
    submittingMoveSession.value = true
    try {
      let res
      if (!moveSessionForm.value.folderId) {
        res = await api.post('/AI/chat/session/remove-from-folder', { sessionId: moveSessionForm.value.sessionId })
      } else {
        res = await api.post('/AI/chat/session/move', { sessionId: moveSessionForm.value.sessionId, folderId: moveSessionForm.value.folderId })
      }
      if (res.data?.status_code === 1000) {
        moveSessionDialogVisible.value = false
        await loadSessions()
      } else {
        ElMessage.error('移动失败')
      }
    } finally {
      submittingMoveSession.value = false
    }
  }

  const promptTargetFolderId = async () => {
    const folders = sessionFolders.value || []
    if (!folders.length) {
      ElMessage.warning('Create a folder first')
      return null
    }

    const hint = folders.map(item => `${item.id}:${item.name}`).join(' | ')
    const { value } = await ElMessageBox.prompt(
      `Choose target folder id: ${hint}`,
      'Move Session',
      {
        confirmButtonText: 'Move',
        cancelButtonText: 'Cancel',
        inputPattern: /^\d+$/,
        inputErrorMessage: 'Enter a numeric folder id'
      }
    )

    const folderId = Number(value)
    if (!Number.isInteger(folderId)) {
      ElMessage.error('Invalid folder id')
      return null
    }
    const exists = folders.some(item => Number(item.id) === folderId)
    if (!exists) {
      ElMessage.error('Folder id does not exist')
      return null
    }
    return folderId
  }

  const moveSessionItem = async (session) => {
    if (!session?.id) return
    try {
      const folderId = await promptTargetFolderId()
      if (!folderId) return

      const response = await api.post('/AI/chat/session/move', {
        sessionId: String(session.id),
        folderId
      })
      if (response.data?.status_code === 1000) {
        await loadSessions()
        ElMessage.success('Session moved')
        return
      }
      ElMessage.error(response.data?.status_msg || 'Move session failed')
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        console.error('Move session error:', error)
        ElMessage.error('Move session failed')
      }
    }
  }

  const removeSessionItemFromFolder = async (session) => {
    if (!session?.id) return
    try {
      const response = await api.post('/AI/chat/session/remove-from-folder', {
        sessionId: String(session.id)
      })
      if (response.data?.status_code === 1000) {
        await loadSessions()
        ElMessage.success('Session removed from folder')
        return
      }
      ElMessage.error(response.data?.status_msg || 'Remove from folder failed')
    } catch (error) {
      console.error('Remove from folder error:', error)
      ElMessage.error('Remove from folder failed')
    }
  }

  const renameSessionItem = async (session, sessionsRef) => {
    if (!session?.id) return
    try {
      const { value } = await ElMessageBox.prompt('Enter a new session title', 'Rename Session', {
        confirmButtonText: 'OK',
        cancelButtonText: 'Cancel',
        inputValue: session.name || '',
        inputPattern: /\S+/,
        inputErrorMessage: 'Session title is required'
      })
      const title = String(value || '').trim()
      if (!title) return

      const response = await api.post('/AI/chat/session/rename', {
        sessionId: String(session.id),
        title
      })
      if (response.data?.status_code === 1000) {
        await loadSessions()
        if (sessionsRef.value[String(session.id)]) {
          sessionsRef.value[String(session.id)].name = title
        }
        ElMessage.success('Session renamed')
        return
      }
      ElMessage.error(response.data?.status_msg || 'Rename session failed')
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        console.error('Rename session error:', error)
        ElMessage.error('Rename session failed')
      }
    }
  }

  const deleteSessionItem = async (session) => {
    if (!session?.id) return
    try {
      await ElMessageBox.confirm(
        `Delete session "${session.name || session.id}"?`,
        'Delete Session',
        {
          confirmButtonText: 'Delete',
          cancelButtonText: 'Cancel',
          type: 'warning'
        }
      )

      const response = await api.post('/AI/chat/session/delete', {
        sessionId: String(session.id)
      })
      if (response.data?.status_code === 1000) {
        const deletedId = String(session.id)
        if (currentSessionId.value === deletedId) {
          createNewSession()
        }
        await loadSessions()
        ElMessage.success('Session deleted')
        return
      }
      ElMessage.error(response.data?.status_msg || 'Delete session failed')
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        console.error('Delete session error:', error)
        ElMessage.error('Delete session failed')
      }
    }
  }

  return {
    renameSessionDialogVisible,
    renameSessionForm,
    submittingRenameSession,
    moveSessionDialogVisible,
    moveSessionForm,
    submittingMoveSession,
    handleSessionCommand,
    submitRenameSession,
    submitMoveSession,
    moveSessionItem,
    removeSessionItemFromFolder,
    renameSessionItem,
    deleteSessionItem
  }
}