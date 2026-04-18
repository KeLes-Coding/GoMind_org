import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../utils/api'

export function useFolderManager({ loadSessions }) {
  const folderDialogVisible = ref(false)
  const folderDialogType = ref('create')
  const folderForm = ref({ id: '', name: '' })
  const submittingFolder = ref(false)

  const showCreateFolderDialog = () => {
    folderDialogType.value = 'create'
    folderForm.value = { id: '', name: '' }
    folderDialogVisible.value = true
  }

  const handleFolderCommand = (cmd, folder) => {
    if (cmd === 'rename') {
      folderDialogType.value = 'rename'
      folderForm.value = { id: folder.id, name: folder.name }
      folderDialogVisible.value = true
    } else if (cmd === 'delete') {
      ElMessageBox.confirm('删除文件夹后，其中的会话将被移出并作为独立会话保留。确定删除吗？', '删除确认', {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }).then(async () => {
        try {
          const res = await api.post('/AI/chat/folder/delete', { folderId: folder.id })
          if (res.data?.status_code === 1000) {
            ElMessage.success('已删除文件夹')
            await loadSessions()
          } else {
            ElMessage.error(res.data?.status_msg || '删除失败')
          }
        } catch (e) {
          ElMessage.error('服务器错误')
        }
      }).catch(() => {})
    }
  }

  const submitFolderDialog = async () => {
    if (!folderForm.value.name.trim() || submittingFolder.value) return
    submittingFolder.value = true
    try {
      const url = folderDialogType.value === 'create' ? '/AI/chat/folder/create' : '/AI/chat/folder/rename'
      const payload = folderDialogType.value === 'create' ? { name: folderForm.value.name } : { folderId: folderForm.value.id, name: folderForm.value.name }
      const res = await api.post(url, payload)
      if (res.data?.status_code === 1000) {
        ElMessage.success(folderDialogType.value === 'create' ? '创建成功' : '重命名成功')
        folderDialogVisible.value = false
        await loadSessions()
      } else {
        ElMessage.error(res.data?.status_msg || '操作失败')
      }
    } catch (e) {
      ElMessage.error('请求异常')
    } finally {
      submittingFolder.value = false
    }
  }

  const createFolder = async () => {
    try {
      const { value } = await ElMessageBox.prompt('Enter a folder name', 'Create Folder', {
        confirmButtonText: 'OK',
        cancelButtonText: 'Cancel',
        inputPattern: /\S+/,
        inputErrorMessage: 'Folder name is required'
      })

      const name = String(value || '').trim()
      if (!name) return

      const response = await api.post('/AI/chat/folder/create', { name })
      if (response.data && response.data.status_code === 1000) {
        await loadSessions()
        ElMessage.success('Folder created')
        return
      }

      ElMessage.error(response.data?.status_msg || 'Create folder failed')
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        console.error('Create folder error:', error)
        ElMessage.error('Create folder failed')
      }
    }
  }

  const renameFolder = async (folder) => {
    if (!folder?.id) return
    try {
      const { value } = await ElMessageBox.prompt('Enter a new folder name', 'Rename Folder', {
        confirmButtonText: 'OK',
        cancelButtonText: 'Cancel',
        inputValue: folder.name || '',
        inputPattern: /\S+/,
        inputErrorMessage: 'Folder name is required'
      })
      const name = String(value || '').trim()
      if (!name) return

      const response = await api.post('/AI/chat/folder/rename', {
        folderId: Number(folder.id),
        name
      })
      if (response.data?.status_code === 1000) {
        await loadSessions()
        ElMessage.success('Folder renamed')
        return
      }
      ElMessage.error(response.data?.status_msg || 'Rename folder failed')
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        console.error('Rename folder error:', error)
        ElMessage.error('Rename folder failed')
      }
    }
  }

  const deleteFolder = async (folder) => {
    if (!folder?.id) return
    try {
      await ElMessageBox.confirm(
        `Delete folder "${folder.name || folder.id}"? Sessions will become ungrouped.`,
        'Delete Folder',
        {
          confirmButtonText: 'Delete',
          cancelButtonText: 'Cancel',
          type: 'warning'
        }
      )

      const response = await api.post('/AI/chat/folder/delete', {
        folderId: Number(folder.id)
      })
      if (response.data?.status_code === 1000) {
        await loadSessions()
        ElMessage.success('Folder deleted')
        return
      }
      ElMessage.error(response.data?.status_msg || 'Delete folder failed')
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        console.error('Delete folder error:', error)
        ElMessage.error('Delete folder failed')
      }
    }
  }

  return {
    folderDialogVisible,
    folderDialogType,
    folderForm,
    submittingFolder,
    showCreateFolderDialog,
    handleFolderCommand,
    submitFolderDialog,
    createFolder,
    renameFolder,
    deleteFolder
  }
}