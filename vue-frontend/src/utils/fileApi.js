import api from './api'

export const getFileList = async () => {
  const response = await api.get('/file/list')
  return response.data
}

export const uploadRagFile = async (file, onUploadProgress) => {
  const formData = new FormData()
  formData.append('file', file)
  const response = await api.post('/file/upload', formData, {
    headers: {
      'Content-Type': 'multipart/form-data'
    },
    onUploadProgress
  })
  return response.data
}

export const retryVectorizeFile = async (fileId) => {
  const response = await api.post(`/file/retry/${fileId}`)
  return response.data
}

export const reindexFile = async (fileId) => {
  const response = await api.post(`/file/reindex/${fileId}`)
  return response.data
}

export const deleteFile = async (fileId) => {
  const response = await api.delete(`/file/${fileId}`)
  return response.data
}

// 可选：如下载等
export const downloadFile = async (fileId) => {
  // 如果是预签名重定向，浏览器会直接处理重定向，若后端响应的是blob的话，需要用 blob 的方式拿，不过后端代码写的是如果未获得预签名就返回stream
  const response = await api.get(`/file/download/${fileId}`, { responseType: 'blob' })
  return response
}
