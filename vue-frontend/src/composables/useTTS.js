import { ElMessage } from 'element-plus'
import api from '../utils/api'

export function useTTS() {
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

  return { playTTS }
}