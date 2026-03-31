<template>
  <div class="flex flex-col min-h-screen bg-bg-light dark:bg-bg-dark text-text-primary-light dark:text-text-primary-dark">
    <!-- Header (Glassmorphism) -->
    <header class="sticky top-0 z-10 glass border-b border-border-light dark:border-border-dark px-8 py-4 flex justify-between items-center">
      <h1 class="text-xl font-semibold m-0 tracking-tight">AI应用平台</h1>
      <div class="flex items-center gap-4">
        <button @click="toggleTheme" class="p-2 rounded-full hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer border-none bg-transparent text-text-primary-light dark:text-text-primary-dark text-lg flex items-center justify-center w-10 h-10">
          {{ isDark ? '🌞' : '🌙' }}
        </button>
        <button type="button" @click="handleLogout" class="px-4 py-2 text-sm font-medium border border-border-light dark:border-border-dark rounded-lg hover:border-accent-light dark:hover:border-accent-dark hover:text-accent-light dark:hover:text-accent-dark transition-colors bg-transparent text-text-primary-light dark:text-text-primary-dark cursor-pointer">
          退出登录
        </button>
      </div>
    </header>

    <!-- Main Content -->
    <main class="flex-1 flex justify-center items-center p-8">
      <div class="grid grid-cols-1 md:grid-cols-2 gap-8 max-w-4xl w-full">
        <!-- Card 1 -->
        <div @click="$router.push('/ai-chat')" class="group cursor-pointer bg-surface-light dark:bg-surface-dark rounded-2xl p-10 border border-border-light dark:border-border-dark shadow-[0_2px_10px_rgba(0,0,0,0.02)] hover:shadow-[0_8px_30px_rgba(0,0,0,0.06)] dark:hover:shadow-[0_8px_30px_rgba(0,0,0,0.2)] hover:border-accent-light/30 dark:hover:border-accent-dark/30 transition-all duration-300 text-center">
          <el-icon size="48" class="mb-6 text-text-secondary-light dark:text-text-secondary-dark group-hover:text-accent-light dark:group-hover:text-accent-dark transition-colors duration-300 transform group-hover:-translate-y-1"><ChatDotRound /></el-icon>
          <h3 class="text-xl font-medium mb-3 text-text-primary-light dark:text-text-primary-dark group-hover:text-accent-light dark:group-hover:text-accent-dark transition-colors duration-300">AI聊天</h3>
          <p class="text-text-secondary-light dark:text-text-secondary-dark m-0">与AI进行智能对话</p>
        </div>
        <!-- Card 2 -->
        <div @click="$router.push('/image-recognition')" class="group cursor-pointer bg-surface-light dark:bg-surface-dark rounded-2xl p-10 border border-border-light dark:border-border-dark shadow-[0_2px_10px_rgba(0,0,0,0.02)] hover:shadow-[0_8px_30px_rgba(0,0,0,0.06)] dark:hover:shadow-[0_8px_30px_rgba(0,0,0,0.2)] hover:border-accent-light/30 dark:hover:border-accent-dark/30 transition-all duration-300 text-center">
          <el-icon size="48" class="mb-6 text-text-secondary-light dark:text-text-secondary-dark group-hover:text-accent-light dark:group-hover:text-accent-dark transition-colors duration-300 transform group-hover:-translate-y-1"><Camera /></el-icon>
          <h3 class="text-xl font-medium mb-3 text-text-primary-light dark:text-text-primary-dark group-hover:text-accent-light dark:group-hover:text-accent-dark transition-colors duration-300">图像识别</h3>
          <p class="text-text-secondary-light dark:text-text-secondary-dark m-0">上传图片进行AI识别</p>
        </div>
      </div>
    </main>
  </div>
</template>

<script>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ChatDotRound, Camera } from '@element-plus/icons-vue'
import api from '../utils/api'
import { clearTokens } from '../utils/token'

export default {
  name: 'MenuView',
  components: {
    ChatDotRound,
    Camera
  },
  setup() {
    const router = useRouter()
    const isDark = ref(false)

    onMounted(() => {
      isDark.value = document.documentElement.classList.contains('dark')
    })

    const toggleTheme = () => {
      isDark.value = !isDark.value
      if (isDark.value) {
        document.documentElement.classList.add('dark')
      } else {
        document.documentElement.classList.remove('dark')
      }
    }

    const handleLogout = async () => {
      try {
        await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning'
        })
        await api.post('/user/logout')
        clearTokens()
        ElMessage.success('退出登录成功')
        router.push('/login')
      } catch {
        // 用户取消操作
      }
    }

    return {
      handleLogout,
      isDark,
      toggleTheme
    }
  }
}
</script>

<style scoped>
/* Scoped styles removed. Using Tailwind CSS classes for layout and UI styling. */
</style>
