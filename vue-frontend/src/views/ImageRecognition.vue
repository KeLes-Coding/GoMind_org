<template>
  <div class="flex flex-row h-screen w-screen overflow-hidden bg-bg-light dark:bg-bg-dark text-text-primary-light dark:text-text-primary-dark">
    <!-- Sidebar -->
    <aside class="w-64 bg-surface-light dark:bg-surface-dark border-r border-border-light dark:border-border-dark flex flex-col flex-shrink-0">
      <div class="p-4 flex flex-col gap-4">
        <span class="text-sm font-semibold tracking-wide text-text-secondary-light dark:text-text-secondary-dark uppercase pl-2">图像识别</span>
      </div>
      <ul class="flex-1 overflow-y-auto list-none m-0 p-2 space-y-1">
        <li class="px-3 py-2.5 rounded-lg cursor-pointer text-sm transition-colors bg-black/5 dark:bg-white/5 font-medium text-text-primary-light dark:text-text-primary-dark flex items-center relative before:absolute before:left-0 before:top-1/2 before:-translate-y-1/2 before:h-4 before:w-1 before:bg-accent-light dark:before:bg-accent-dark before:rounded-r-full overflow-hidden">
          <span class="truncate block max-w-full">图像识别助手</span>
        </li>
      </ul>
    </aside>

    <!-- Main Content -->
    <section class="flex-1 flex flex-col relative min-w-0 bg-bg-light dark:bg-bg-dark">
      <!-- Header -->
      <div class="sticky top-0 z-10 glass border-b border-border-light dark:border-border-dark px-6 py-4 flex items-center gap-4 flex-wrap">
        <button class="px-3 py-1.5 text-sm rounded bg-transparent border border-border-light dark:border-border-dark hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer" @click="$router.push('/menu')">← 返回</button>
        <h2 class="text-lg font-semibold m-0 tracking-tight">AI 图像识别助手</h2>
      </div>

      <!-- Messages Stream (Bubble-less) -->
      <div class="flex-1 overflow-y-auto px-8 md:px-24 pt-8 pb-40" ref="chatContainerRef">
        <div class="max-w-4xl mx-auto flex flex-col gap-12">
          <div
            v-for="(message, index) in messages"
            :key="index"
            class="flex flex-col gap-2 group"
          >
            <!-- Sender Header -->
            <div class="flex items-center gap-3">
              <div :class="['w-8 h-8 rounded-full flex items-center justify-center font-bold text-sm select-none', message.role === 'user' ? 'bg-black text-white dark:bg-white dark:text-black' : 'bg-surface-light border border-border-light dark:bg-surface-dark dark:border-border-dark']">
                {{ message.role === 'user' ? 'U' : 'AI' }}
              </div>
              <span class="font-semibold text-sm">{{ message.role === 'user' ? '你' : 'Antigravity Vision' }}</span>
            </div>
            
            <!-- Message Content block -->
            <div class="pl-11 text-base leading-relaxed space-y-4 break-words">
              <span>{{ message.content }}</span>
              <img v-if="message.imageUrl" :src="message.imageUrl" alt="上传的图片" class="max-w-xs rounded-xl shadow-[0_4px_15px_rgba(0,0,0,0.1)] dark:shadow-2xl mt-4" />
            </div>
          </div>
        </div>
      </div>

      <!-- Floating Pill Input (Upload form) -->
      <div class="absolute bottom-6 left-1/2 -translate-x-1/2 w-full max-w-3xl px-4 z-20">
        <form @submit.prevent="handleSubmit" class="bg-surface-light dark:bg-surface-dark rounded-2xl shadow-[0_8px_30px_rgba(0,0,0,0.06)] dark:shadow-2xl ring-1 ring-black/5 dark:ring-white/10 flex items-center p-2 gap-2 focus-within:ring-border-light dark:focus-within:ring-border-dark transition-all">
          <input
            ref="fileInputRef"
            type="file"
            accept="image/*"
            required
            @change="handleFileSelect"
            class="flex-1 bg-transparent border-none text-sm text-text-secondary-light dark:text-text-secondary-dark px-4 py-2 file:mr-4 file:py-2 file:px-4 file:rounded-xl file:border-0 file:text-sm file:font-semibold file:bg-black/5 dark:file:bg-white/10 file:text-text-primary-light dark:file:text-text-primary-dark hover:file:bg-black/10 dark:hover:file:bg-white/20 transition-all cursor-pointer"
          />
          <button 
            type="submit" 
            :disabled="!selectedFile"
            :class="[
              'px-6 py-2 rounded-xl font-medium transition-all mr-1 disabled:cursor-not-allowed',
              !selectedFile 
                ? 'bg-transparent text-text-secondary-light dark:text-text-secondary-dark opacity-50' 
                : 'bg-black text-white dark:bg-white dark:text-black shadow-sm'
            ]"
          >
            发送图片
          </button>
        </form>
      </div>
    </section>
  </div>
</template>

<script>
import { ref, nextTick } from 'vue'
import api from '../utils/api'

export default {
  name: 'ImageRecognition',
  setup() {
    const messages = ref([])
    const selectedFile = ref(null)
    const fileInputRef = ref()
    const chatContainerRef = ref()

    const handleFileSelect = (event) => {
      selectedFile.value = event.target.files[0]
    }

    const handleSubmit = async () => {
      if (!selectedFile.value) return

      const file = selectedFile.value
      const imageUrl = URL.createObjectURL(file)

      // Add user message to UI
      messages.value.push({
        role: 'user',
        content: `已上传图片: ${file.name}`,
        imageUrl: imageUrl,
      })

      await nextTick()
      scrollToBottom()

      // Create FormData
      const formData = new FormData()
      formData.append('image', file)

      try {
        const response = await api.post('/image/recognize', formData, {
          headers: {
            'Content-Type': 'multipart/form-data',
          },
        })


        if (response.data && response.data.class_name) {
             const aiText = `识别结果: ${response.data.class_name}`
            messages.value.push({
                role: 'assistant',
                content: aiText,
            })
        } else {
             messages.value.push({
                 role: 'assistant',
                 content: `[错误] ${response.data.status_msg || '识别失败'}`,
             })
        }
      } catch (error) {
        console.error('Upload error:', error)
        messages.value.push({
          role: 'assistant',
          content: `[错误] 无法连接到服务器或上传失败: ${error.message}`,
        })
      } finally {

        URL.revokeObjectURL(imageUrl)

            await nextTick()
        scrollToBottom()


        selectedFile.value = null
        if (fileInputRef.value) {
          fileInputRef.value.value = ''
        }
      }
    }

    const scrollToBottom = () => {
      if (chatContainerRef.value) {
        chatContainerRef.value.scrollTop = chatContainerRef.value.scrollHeight
      }
    }

    return {
      messages,
      selectedFile,
      fileInputRef,
      chatContainerRef,
      handleFileSelect,
      handleSubmit
    }
  }
}
</script>

<style scoped>
/* Scoped styles removed */
</style>