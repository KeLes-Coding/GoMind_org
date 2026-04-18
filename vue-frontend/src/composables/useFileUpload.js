import { nextTick, ref } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../utils/api'
import { buildMessageMeta } from '../utils/messageHelpers'

export function useFileUpload({ currentMessages, loading, scrollToBottom }) {
  const uploading = ref(false)
  const fileInput = ref(null)
  const imageInput = ref(null)

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
      ElMessage.error('只支持上传 .md 或 .txt 文件')
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
        ElMessage.success('文件上传成功，请至 File Management 查看状态')
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

  const triggerImageUpload = () => {
    if (imageInput.value) {
      imageInput.value.click()
    }
  }

  const handleImageRecognition = async (event) => {
    const file = event.target.files[0]
    if (!file) return

    const imageUrl = URL.createObjectURL(file)

    currentMessages.value.push({
      role: 'user',
      content: `已上传图片 ${file.name}`,
      imageUrl: imageUrl,
      meta: buildMessageMeta('completed')
    })
    await nextTick()
    scrollToBottom()

    const formData = new FormData()
    formData.append('image', file)

    try {
      loading.value = true
      const response = await api.post('/image/recognize', formData, {
        headers: {
          'Content-Type': 'multipart/form-data'
        }
      })

      if (response.data && response.data.class_name) {
        currentMessages.value.push({
          role: 'assistant',
          content: `识别结果: **${response.data.class_name}**`,
          meta: buildMessageMeta('completed')
        })
      } else {
        currentMessages.value.push({
          role: 'assistant',
          content: `[错误] ${response.data?.status_msg || '识别失败'}`,
          meta: buildMessageMeta('failed')
        })
      }
    } catch (error) {
      console.error('Image recognition error:', error)
      currentMessages.value.push({
        role: 'assistant',
        content: `[错误] 无法连接到服务器或识别失败: ${error.message}`,
        meta: buildMessageMeta('failed')
      })
    } finally {
      loading.value = false
      await nextTick()
      scrollToBottom()
      if (imageInput.value) {
        imageInput.value.value = ''
      }
    }
  }

  return {
    uploading,
    fileInput,
    imageInput,
    triggerFileUpload,
    handleFileUpload,
    triggerImageUpload,
    handleImageRecognition
  }
}