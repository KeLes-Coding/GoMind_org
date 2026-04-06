<template>
  <el-dialog 
    :model-value="modelValue" 
    @update:model-value="$emit('update:modelValue', $event)" 
    title="文件管理 (File Management)" 
    width="700px" 
    :close-on-click-modal="false"
    class="custom-dialog"
  >
    <div class="space-y-4">
      <div class="flex justify-between items-center mb-2">
        <p class="text-xs text-text-secondary-light dark:text-text-secondary-dark">所有上传至 RAG 的知识库文件都在此管理。文件解析需一定时间。</p>
        <button type="button" @click="fetchFiles" class="p-1 px-3 bg-black/5 dark:bg-white/10 hover:bg-black/10 dark:hover:bg-white/20 rounded-lg border-none text-xs text-text-primary-light dark:text-text-primary-dark cursor-pointer transition-colors" title="刷新列表">
          Refresh
        </button>
      </div>

      <div class="border border-border-light dark:border-border-dark rounded-xl overflow-hidden bg-surface-light dark:bg-surface-dark">
        <table class="w-full text-sm text-left">
          <thead class="bg-black/5 dark:bg-white/5 text-text-secondary-light dark:text-text-secondary-dark font-medium border-b border-border-light dark:border-border-dark">
            <tr>
              <th class="px-4 py-3 font-medium">文件名</th>
              <th class="px-4 py-3 font-medium w-24">大小</th>
              <th class="px-4 py-3 font-medium w-32">状态</th>
              <th class="px-4 py-3 font-medium w-36 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading && filesList.length === 0">
              <td colspan="4" class="px-4 py-8 text-center text-text-secondary-light dark:text-text-secondary-dark">加载中... (Loading)</td>
            </tr>
            <tr v-else-if="filesList.length === 0">
              <td colspan="4" class="px-4 py-8 text-center text-text-secondary-light dark:text-text-secondary-dark">暂无文件 (No files yet)</td>
            </tr>
            <tr v-for="file in filesList" :key="file.id" class="border-b last:border-0 border-border-light dark:border-border-dark hover:bg-black/5 dark:hover:bg-white/5 transition-colors">
              <td class="px-4 py-3">
                <div class="truncate max-w-[200px]" :title="file.file_name">{{ file.file_name }}</div>
                <div class="text-[10px] text-text-secondary-light dark:text-text-secondary-dark">{{ formatTime(file.created_at) }}</div>
              </td>
              <td class="px-4 py-3 text-text-secondary-light dark:text-text-secondary-dark">{{ formatSize(file.size) }}</td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-1.5" :title="file.error_msg || file.vector_task_err_msg || ''">
                  <span :class="['w-2 h-2 rounded-full shrink-0', statusColor(file.status)]"></span>
                  <span class="capitalize text-xs whitespace-nowrap">{{ file.status.replace('_', ' ') }}</span>
                  <svg v-if="file.status === 'failed'" xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 text-red-500 cursor-help" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                </div>
              </td>
              <td class="px-4 py-3 text-right space-x-2">
                <el-dropdown trigger="click" @command="(cmd) => handleCommand(cmd, file)">
                  <button type="button" class="p-1 rounded hover:bg-black/10 dark:hover:bg-white/10 text-text-secondary-light dark:text-text-secondary-dark cursor-pointer bg-transparent border-none">
                     <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z" /></svg>
                  </button>
                  <template #dropdown>
                    <el-dropdown-menu>
                      <el-dropdown-item v-if="file.status === 'failed'" command="retry">Retry Vectorize</el-dropdown-item>
                      <el-dropdown-item v-if="file.status === 'ready' || file.status === 'failed'" command="reindex">Reindex File</el-dropdown-item>
                      <!--el-dropdown-item command="download">Download</el-dropdown-item-->
                      <el-dropdown-item command="delete" class="text-red-500">Delete</el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </el-dialog>
</template>

<script>
import { ref, onUnmounted, watch } from 'vue'
import { getFileList, retryVectorizeFile, reindexFile, deleteFile } from '../../utils/fileApi'
import { ElMessage, ElMessageBox } from 'element-plus'

export default {
  name: 'FileManagerDialog',
  props: {
    modelValue: {
      type: Boolean,
      required: true
    }
  },
  emits: ['update:modelValue'],
  setup(props) {
    const filesList = ref([])
    const loading = ref(false)
    let pollInterval = null

    const formatSize = (bytes) => {
      if (!bytes || bytes === 0) return '0 B'
      const k = 1024
      const sizes = ['B', 'KB', 'MB', 'GB']
      const i = Math.floor(Math.log(bytes) / Math.log(k))
      return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
    }

    const formatTime = (timeStr) => {
      if (!timeStr) return ''
      const date = new Date(timeStr)
      return date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    }

    const statusColor = (status) => {
      switch (status) {
        case 'ready': return 'bg-green-500'
        case 'failed': return 'bg-red-500'
        case 'pending_upload':
        case 'uploaded': return 'bg-gray-400'
        case 'parsing': return 'bg-orange-400 animate-pulse'
        case 'vectorizing': return 'bg-blue-400 animate-pulse'
        default: return 'bg-gray-400'
      }
    }

    const hasProcessingFiles = (files) => {
      const processingStatuses = ['pending_upload', 'uploaded', 'parsing', 'vectorizing']
      return files.some(f => processingStatuses.includes(f.status))
    }

    const fetchFiles = async (silent = false) => {
      if (!silent) loading.value = true
      try {
        const res = await getFileList()
        if (res?.status_code === 1000 && res.files) {
          filesList.value = res.files
          
          if (hasProcessingFiles(res.files)) {
            startPolling()
          } else {
            stopPolling()
          }
        } else {
          stopPolling()
        }
      } catch (error) {
        console.error('Fetch file list error:', error)
        stopPolling()
      } finally {
        if (!silent) loading.value = false
      }
    }

    const startPolling = () => {
      if (pollInterval) return
      pollInterval = setInterval(() => {
        fetchFiles(true)
      }, 5000)
    }

    const stopPolling = () => {
      if (pollInterval) {
        clearInterval(pollInterval)
        pollInterval = null
      }
    }

    const handleCommand = async (cmd, file) => {
      if (cmd === 'retry') {
        try {
          const res = await retryVectorizeFile(file.id)
          if (res.status_code === 1000) {
            ElMessage.success('Retrying vectorize...')
            fetchFiles()
          } else {
            ElMessage.error(res.status_msg || 'Retry failed')
          }
        } catch (e) {
          ElMessage.error('Network error')
        }
      } else if (cmd === 'reindex') {
        try {
          const res = await reindexFile(file.id)
          if (res.status_code === 1000) {
            ElMessage.success('Reindexing started...')
            fetchFiles()
          } else {
            ElMessage.error(res.status_msg || 'Reindex failed')
          }
        } catch (e) {
          ElMessage.error('Network error')
        }
      } else if (cmd === 'delete') {
        ElMessageBox.confirm('确定要删除这个文件资产吗？(Are you sure to delete this file?)', 'Warning', {
          confirmButtonText: '删除',
          cancelButtonText: '取消',
          type: 'warning',
        }).then(async () => {
          try {
            const res = await deleteFile(file.id)
            if (res.status_code === 1000) {
              ElMessage.success('Deleted successfully')
              fetchFiles()
            } else {
              ElMessage.error(res.status_msg || 'Delete failed')
            }
          } catch (e) {
            ElMessage.error('Network error')
          }
        }).catch(() => {})
      }
    }

    watch(() => props.modelValue, (newVal) => {
      if (newVal) {
        fetchFiles()
      } else {
        stopPolling()
      }
    })

    onUnmounted(() => {
      stopPolling()
    })

    return {
      filesList,
      loading,
      formatSize,
      formatTime,
      statusColor,
      fetchFiles,
      handleCommand
    }
  }
}
</script>

<style scoped>
/* Inherit standard dialog styles from dark mode context */
:deep(.el-dialog) {
  background-color: #ffffff;
  border: 1px solid #e0e0e0;
  border-radius: 1rem;
}

:deep(.el-dialog__title) {
  color: #1a1a1a;
  font-size: 1.125rem;
  font-weight: 700;
  letter-spacing: -0.025em;
}

:deep(.el-dropdown-menu) {
  background-color: #ffffff;
  border-color: #e0e0e0;
}

:deep(.el-dropdown-menu__item) {
  color: #1a1a1a;
}

:deep(.el-dropdown-menu__item:hover),
:deep(.el-dropdown-menu__item:focus) {
  background-color: rgba(0, 0, 0, 0.05);
  color: #1a1a1a;
}

/* Popper background fixes */
:global(.dark .el-dialog) {
  background-color: #1e1e1e;
  border-color: #333333;
}

:global(.dark .el-dialog__title) {
  color: #f5f5f5;
}

:global(.dark .el-dropdown-menu),
:global(.dark .el-popper.is-light) {
  background-color: #1e1e1e;
  border-color: #333333;
}

:global(.dark .el-dropdown-menu__item) {
  color: #f5f5f5;
}

:global(.dark .el-dropdown-menu__item:hover),
:global(.dark .el-dropdown-menu__item:focus) {
  background-color: rgba(255, 255, 255, 0.05);
  color: #f5f5f5;
}
</style>
