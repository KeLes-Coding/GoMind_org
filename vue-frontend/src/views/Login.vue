<template>
  <div class="flex h-screen w-full items-center justify-center bg-bg-light dark:bg-bg-dark text-text-primary-light dark:text-text-primary-dark overflow-hidden relative login-page">
    <div class="w-[420px] bg-surface-light dark:bg-surface-dark rounded-3xl shadow-[0_20px_40px_rgba(0,0,0,0.06)] dark:shadow-[0_20px_40px_rgba(0,0,0,0.2)] border border-border-light dark:border-border-dark relative z-10 p-8">
      <div class="text-center pb-8">
        <h2 class="text-2xl font-semibold m-0 tracking-tight">登录</h2>
      </div>
      <el-form
        ref="loginFormRef"
        :model="loginForm"
        :rules="loginRules"
        label-width="80px"
      >
        <el-form-item label="账号" prop="username">
          <el-input
            v-model="loginForm.username"
            placeholder="请输入用户名或邮箱"
            class="!rounded-lg"
          />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input
            v-model="loginForm.password"
            placeholder="请输入密码"
            type="password"
            show-password
            class="!rounded-lg"
          />
        </el-form-item>
        <div class="mt-8 space-y-4">
          <button
            type="button"
            :disabled="loading"
            @click="handleLogin"
            class="w-full h-12 rounded-xl font-semibold bg-accent-light dark:bg-accent-dark border-none hover:opacity-90 transition-opacity text-white flex items-center justify-center disabled:opacity-50"
          >
            {{ loading ? '登录中...' : '登录' }}
          </button>
          <button
            type="button"
            @click="$router.push('/register')"
            class="w-full h-10 text-sm text-text-secondary-light dark:text-text-secondary-dark hover:text-accent-light dark:hover:text-accent-dark transition-colors bg-transparent border-none cursor-pointer"
          >
            还没有账号？去注册
          </button>
          <router-link to="/" class="block text-center text-sm text-text-secondary-light dark:text-text-secondary-dark hover:text-accent-light dark:hover:text-accent-dark transition-colors mt-2">
            返回首页
          </router-link>
        </div>
      </el-form>
    </div>
  </div>
</template>

<script>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../utils/api'
import { saveTokens } from '../utils/token'

export default {
  name: 'LoginView',
  setup() {
    const router = useRouter()
    const loginFormRef = ref()
    const loading = ref(false)
    const loginForm = ref({
      username: '',
      password: ''
    })

    const loginRules = {
      username: [
        { required: true, message: '请输入用户名或邮箱', trigger: 'blur' }
      ],
      password: [
        { required: true, message: '请输入密码', trigger: 'blur' },
        { min: 6, message: '密码长度不能少于6位', trigger: 'blur' }
      ]
    }

    const handleLogin = async () => {
      try {
        await loginFormRef.value.validate()
        loading.value = true
        const response = await api.post('/user/login', {
          username: loginForm.value.username,
          password: loginForm.value.password
        })
        if (response.data.status_code === 1000) {
          saveTokens(response.data)
          ElMessage.success('登录成功')
          router.push('/ai-chat')
        } else {
          ElMessage.error(response.data.status_msg || '登录失败')
        }
      } catch (error) {
        console.error('Login error:', error)
        ElMessage.error('登录失败，请重试')
      } finally {
        loading.value = false
      }
    }

    return {
      loginFormRef,
      loading,
      loginForm,
      loginRules,
      handleLogin
    }
  }
}
</script>

<style scoped>
:deep(.el-form-item__label) {
  color: #1A1A1A;
}
:deep(.el-input__wrapper) {
  background-color: #FFFFFF;
  border: 1px solid #E0E0E0;
  border-radius: 0.5rem;
  box-shadow: none;
}
:deep(.el-input__inner) {
  color: #1A1A1A;
}
:deep(.el-input__inner::placeholder) {
  color: #666666;
}
</style>

<style>
html.dark .login-page .el-form-item__label {
  color: #F5F5F5;
}
html.dark .login-page .el-input__wrapper {
  background-color: #1E1E1E;
  border-color: #333333;
}
html.dark .login-page .el-input__inner {
  color: #F5F5F5;
}
html.dark .login-page .el-input__inner::placeholder {
  color: #A0A0A0;
}
</style>
