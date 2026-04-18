<template>
  <div class="flex h-screen w-full items-center justify-center bg-bg-light dark:bg-bg-dark text-text-primary-light dark:text-text-primary-dark overflow-hidden relative register-page">
    <div class="w-[420px] bg-surface-light dark:bg-surface-dark rounded-3xl shadow-[0_20px_40px_rgba(0,0,0,0.06)] dark:shadow-[0_20px_40px_rgba(0,0,0,0.2)] border border-border-light dark:border-border-dark relative z-10 p-8">
      <div class="text-center pb-8">
        <h2 class="text-2xl font-semibold m-0 tracking-tight">注册</h2>
      </div>
      <el-form
        ref="registerFormRef"
        :model="registerForm"
        :rules="registerRules"
        label-width="80px"
      >
        <el-form-item label="用户名" prop="username">
          <el-input
            v-model="registerForm.username"
            placeholder="请输入用户名"
            class="!rounded-lg"
          />
          <div class="mt-1.5 text-xs text-text-secondary-light dark:text-text-secondary-dark leading-relaxed">4-20位，以字母开头，只能包含字母、数字和下划线</div>
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input
            v-model="registerForm.email"
            placeholder="请输入邮箱"
            type="email"
            class="!rounded-lg"
          />
        </el-form-item>
        <el-form-item label="验证码" prop="captcha">
          <el-row :gutter="10" class="w-full m-0">
            <el-col :span="16" class="!pl-0">
              <el-input
                v-model="registerForm.captcha"
                placeholder="请输入验证码"
                class="!rounded-lg"
              />
            </el-col>
            <el-col :span="8" class="!pr-0">
              <button
                type="button"
                :disabled="codeLoading || countdown > 0"
                @click="sendCode"
                class="w-full h-[32px] rounded-lg text-sm font-medium border border-border-light dark:border-border-dark bg-transparent text-text-primary-light dark:text-text-primary-dark hover:border-accent-light dark:hover:border-accent-dark hover:text-accent-light dark:hover:text-accent-dark transition-colors disabled:opacity-50 cursor-pointer"
              >
                {{ countdown > 0 ? `${countdown}s` : '发送验证码' }}
              </button>
            </el-col>
          </el-row>
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input
            v-model="registerForm.password"
            placeholder="请输入密码"
            type="password"
            show-password
            class="!rounded-lg"
          />
        </el-form-item>
        <el-form-item label="确认" prop="confirmPassword">
          <el-input
            v-model="registerForm.confirmPassword"
            placeholder="请再次输入密码"
            type="password"
            show-password
            class="!rounded-lg"
          />
        </el-form-item>
        <div class="mt-8 space-y-4">
          <button
            type="button"
            :disabled="loading"
            @click="handleRegister"
            class="w-full h-12 rounded-xl font-semibold bg-accent-light dark:bg-accent-dark border-none hover:opacity-90 transition-opacity text-white flex items-center justify-center disabled:opacity-50 cursor-pointer"
          >
            {{ loading ? '注册中...' : '注册' }}
          </button>
          <button
            type="button"
            @click="$router.push('/login')"
            class="w-full h-10 text-sm text-text-secondary-light dark:text-text-secondary-dark hover:text-accent-light dark:hover:text-accent-dark transition-colors bg-transparent border-none cursor-pointer"
          >
            已有账号？去登录
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
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../utils/api'
import { saveTokens } from '../utils/token'

export default {
  name: 'RegisterView',
  setup() {
    const router = useRouter()
    const registerFormRef = ref()
    const loading = ref(false)
    const codeLoading = ref(false)
    const countdown = ref(0)

    const registerForm = reactive({
      username: '',
      email: '',
      captcha: '',
      password: '',
      confirmPassword: ''
    })

    const validateConfirmPassword = (rule, value, callback) => {
      if (value !== registerForm.password) {
        callback(new Error('两次输入密码不一致'))
      } else {
        callback()
      }
    }

    const validateUsername = (rule, value, callback) => {
      if (!value) {
        callback(new Error('请输入用户名'))
        return
      }
      if (!/^[A-Za-z][A-Za-z0-9_]{3,19}$/.test(value)) {
        callback(new Error('用户名需为4-20位，字母开头，仅支持字母、数字和下划线'))
        return
      }
      callback()
    }

    const registerRules = {
      username: [
        { validator: validateUsername, trigger: 'blur' }
      ],
      email: [
        { required: true, message: '请输入邮箱', trigger: 'blur' },
        { type: 'email', message: '请输入正确的邮箱格式', trigger: 'blur' }
      ],
      captcha: [
        { required: true, message: '请输入验证码', trigger: 'blur' }
      ],
      password: [
        { required: true, message: '请输入密码', trigger: 'blur' },
        { min: 6, message: '密码长度不能少于6位', trigger: 'blur' }
      ],
      confirmPassword: [
        { required: true, message: '请确认密码', trigger: 'blur' },
        { validator: validateConfirmPassword, trigger: 'blur' }
      ]
    }

    const sendCode = async () => {
      if (!registerForm.email) {
        ElMessage.warning('请先输入邮箱')
        return
      }
      try {
        codeLoading.value = true
        const response = await api.post('/user/captcha', { email: registerForm.email })
        if (response.data.status_code === 1000) {
          ElMessage.success('验证码发送成功')
          countdown.value = 60
          const timer = setInterval(() => {
            countdown.value--
            if (countdown.value <= 0) {
              clearInterval(timer)
            }
          }, 1000)
        } else {
          ElMessage.error(response.data.status_msg || '验证码发送失败')
        }
      } catch (error) {
        console.error('Send code error:', error)
        ElMessage.error('验证码发送失败，请重试')
      } finally {
        codeLoading.value = false
      }
    }

    const handleRegister = async () => {
      try {
        await registerFormRef.value.validate()
        loading.value = true
        const response = await api.post('/user/register', {
          username: registerForm.username,
          email: registerForm.email,
          captcha: registerForm.captcha,
          password: registerForm.password
        })
        if (response.data.status_code === 1000) {
          saveTokens(response.data)
          ElMessage.success(`注册成功，当前用户名：${response.data.username || registerForm.username}`)
          router.push('/ai-chat')
        } else {
          ElMessage.error(response.data.status_msg || '注册失败')
        }
      } catch (error) {
        console.error('Register error:', error)
        ElMessage.error('注册失败，请重试')
      } finally {
        loading.value = false
      }
    }

    return {
      registerFormRef,
      loading,
      codeLoading,
      countdown,
      registerForm,
      registerRules,
      sendCode,
      handleRegister
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
html.dark .register-page .el-form-item__label {
  color: #F5F5F5;
}
html.dark .register-page .el-input__wrapper {
  background-color: #1E1E1E;
  border-color: #333333;
}
html.dark .register-page .el-input__inner {
  color: #F5F5F5;
}
html.dark .register-page .el-input__inner::placeholder {
  color: #A0A0A0;
}
</style>
