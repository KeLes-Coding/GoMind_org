<template>
  <el-dialog
    v-model="visible"
    title=""
    width="640px"
    :show-close="false"
    class="model-config-dialog"
    destroy-on-close
    @close="handleClose"
  >
    <!-- Header -->
    <template #header>
      <div class="flex items-center justify-between pb-3 border-b border-border-light dark:border-border-dark">
        <div class="flex items-center gap-3">
          <button
            v-if="mode !== 'list'"
            class="p-1.5 rounded-lg hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer bg-transparent border-none"
            @click="backToList"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 text-text-secondary-light dark:text-text-secondary-dark" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7"/></svg>
          </button>
          <h3 class="text-lg font-semibold m-0">
            {{ mode === 'list' ? 'Model Configurations' : (mode === 'create' ? 'New Configuration' : 'Edit Configuration') }}
          </h3>
        </div>
        <button
          class="p-1.5 rounded-lg hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer bg-transparent border-none"
          @click="handleClose"
        >
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 text-text-secondary-light dark:text-text-secondary-dark" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
        </button>
      </div>
    </template>

    <!-- List Mode -->
    <div v-if="mode === 'list'" class="space-y-3 min-h-[200px]">
      <div v-if="loadingList" class="flex items-center justify-center py-12">
        <svg class="animate-spin w-6 h-6 text-text-secondary-light dark:text-text-secondary-dark" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      </div>
      <template v-else>
        <div v-if="!configs.length" class="text-center py-12 text-text-secondary-light dark:text-text-secondary-dark">
          <p class="text-sm">暂无配置，点击下方按钮新建</p>
        </div>
        <div
          v-for="config in configs"
          :key="config.id"
          class="group flex items-start gap-3 p-3 rounded-xl border border-border-light dark:border-border-dark hover:border-accent-light/30 dark:hover:border-accent-dark/30 hover:bg-black/[0.02] dark:hover:bg-white/[0.02] transition-all"
        >
          <!-- Provider Icon -->
          <div class="w-9 h-9 rounded-lg flex items-center justify-center shrink-0 text-xs font-bold select-none mt-0.5"
            :class="providerBadgeClass(config.provider)"
          >
            {{ providerInitial(config.provider) }}
          </div>
          <!-- Info -->
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <span class="font-medium text-sm truncate">{{ config.name }}</span>
              <span v-if="config.isDefault" class="px-1.5 py-0.5 text-[10px] rounded-full bg-accent-light/10 text-accent-light dark:bg-accent-dark/10 dark:text-accent-dark font-medium">Default</span>
              <span v-if="!config.isEnabled" class="px-1.5 py-0.5 text-[10px] rounded-full bg-red-50 text-red-500 dark:bg-red-500/10 font-medium">Disabled</span>
            </div>
            <div class="text-xs text-text-secondary-light dark:text-text-secondary-dark mt-0.5 flex items-center gap-2">
              <span>{{ config.provider }}</span>
              <span class="opacity-30">·</span>
              <span class="truncate">{{ config.model }}</span>
            </div>
            <div v-if="config.maskedApiKey" class="text-xs text-text-secondary-light dark:text-text-secondary-dark mt-0.5 font-mono opacity-60">
              {{ config.maskedApiKey }}
            </div>
          </div>
          <!-- Actions -->
          <div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
            <button
              v-if="!config.isDefault"
              class="p-1.5 rounded-lg hover:bg-accent-light/10 dark:hover:bg-accent-dark/10 text-text-secondary-light dark:text-text-secondary-dark hover:text-accent-light dark:hover:text-accent-dark transition-colors cursor-pointer bg-transparent border-none"
              title="设为默认"
              @click="setDefault(config.id)"
            >
              <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z"/></svg>
            </button>
            <button
              class="p-1.5 rounded-lg hover:bg-black/5 dark:hover:bg-white/5 text-text-secondary-light dark:text-text-secondary-dark transition-colors cursor-pointer bg-transparent border-none"
              title="编辑"
              @click="startEdit(config)"
            >
              <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"/></svg>
            </button>
            <button
              class="p-1.5 rounded-lg hover:bg-red-50 dark:hover:bg-red-500/10 text-text-secondary-light dark:text-text-secondary-dark hover:text-red-500 transition-colors cursor-pointer bg-transparent border-none"
              title="删除"
              @click="deleteConfig(config)"
            >
              <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
            </button>
          </div>
        </div>
      </template>
    </div>

    <!-- Create / Edit Mode -->
    <div v-else class="space-y-4">
      <div class="space-y-1.5">
        <label class="block text-sm font-medium">配置名称</label>
        <input v-model="form.name" type="text" maxlength="100" placeholder="例如：DeepSeek 正式环境" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none text-sm focus:ring-1 focus:ring-accent-light dark:focus:ring-accent-dark transition-shadow" />
      </div>

      <div class="space-y-1.5">
        <label class="block text-sm font-medium">Provider</label>
        <select v-model="form.provider" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none text-sm cursor-pointer focus:ring-1 focus:ring-accent-light dark:focus:ring-accent-dark transition-shadow">
          <option value="" disabled class="bg-surface-light dark:bg-surface-dark">选择 Provider</option>
          <option v-for="p in providerOptions" :key="p.provider" :value="p.provider" class="bg-surface-light dark:bg-surface-dark">
            {{ p.displayName }}{{ !p.isImplemented ? ' (暂未接入)' : '' }}
          </option>
        </select>
        <p v-if="selectedProviderNotImplemented" class="text-xs text-amber-500 mt-1">
          ⚠ 此 Provider 当前尚未接入后端 SDK，配置后暂时无法用于聊天。
        </p>
      </div>

      <div class="space-y-1.5">
        <label class="block text-sm font-medium">Model</label>
        <input v-model="form.model" type="text" maxlength="100" placeholder="例如：deepseek-chat" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none text-sm focus:ring-1 focus:ring-accent-light dark:focus:ring-accent-dark transition-shadow" />
      </div>

      <div class="space-y-1.5">
        <label class="block text-sm font-medium">API Key</label>
        <input v-model="form.apiKey" type="password" :placeholder="mode === 'edit' ? '留空表示沿用旧 Key' : '请输入 API Key'" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none text-sm font-mono focus:ring-1 focus:ring-accent-light dark:focus:ring-accent-dark transition-shadow" />
      </div>

      <div class="space-y-1.5">
        <label class="block text-sm font-medium">Base URL <span class="text-text-secondary-light dark:text-text-secondary-dark font-normal">(可选)</span></label>
        <input v-model="form.baseUrl" type="text" placeholder="例如：https://api.deepseek.com" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none text-sm focus:ring-1 focus:ring-accent-light dark:focus:ring-accent-dark transition-shadow" />
      </div>

      <div class="flex items-center gap-2">
        <input v-model="form.isDefault" type="checkbox" id="configDefault" class="accent-accent-light dark:accent-accent-dark" />
        <label for="configDefault" class="text-sm cursor-pointer select-none">设为默认配置</label>
      </div>

      <!-- Test Connectivity -->
      <button
        type="button"
        :disabled="!canTest || testingConfig"
        class="flex items-center gap-2 px-4 py-2 rounded-lg border border-border-light dark:border-border-dark hover:border-accent-light/40 dark:hover:border-accent-dark/40 bg-transparent text-sm transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
        @click="testConfig"
      >
        <svg v-if="testingConfig" class="animate-spin w-4 h-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
        <svg v-else xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
        {{ testingConfig ? '测试中...' : '测试连通性' }}
      </button>
      <p v-if="testResult === 'success'" class="text-xs text-green-600 dark:text-green-400">✓ 连通性测试通过</p>
      <p v-if="testResult === 'fail'" class="text-xs text-red-500">✗ 连通性测试失败，请检查配置</p>
    </div>

    <!-- Footer -->
    <template #footer>
      <div class="flex justify-end gap-2 pt-3 border-t border-border-light dark:border-border-dark">
        <template v-if="mode === 'list'">
          <button type="button" class="px-4 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer text-sm hover:bg-black/5 dark:hover:bg-white/5 transition-colors" @click="handleClose">关闭</button>
          <button type="button" class="px-4 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer text-sm font-medium hover:opacity-90 transition-opacity" @click="startCreate">+ 新建配置</button>
        </template>
        <template v-else>
          <button type="button" class="px-4 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer text-sm hover:bg-black/5 dark:hover:bg-white/5 transition-colors" @click="backToList">取消</button>
          <button type="button" class="px-4 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer text-sm font-medium hover:opacity-90 transition-opacity disabled:opacity-40 disabled:cursor-not-allowed" :disabled="!canSubmit || submitting" @click="submitForm">
            {{ submitting ? '保存中...' : '保存' }}
          </button>
        </template>
      </div>
    </template>
  </el-dialog>
</template>

<script>
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../../utils/api'

export default {
  name: 'ModelConfigDialog',
  props: {
    modelValue: { type: Boolean, default: false }
  },
  emits: ['update:modelValue', 'configsChanged'],
  setup(props, { emit }) {
    const visible = computed({
      get: () => props.modelValue,
      set: (val) => emit('update:modelValue', val)
    })

    const mode = ref('list') // 'list' | 'create' | 'edit'
    const configs = ref([])
    const loadingList = ref(false)
    const submitting = ref(false)
    const testingConfig = ref(false)
    const testResult = ref('') // '' | 'success' | 'fail'
    const editingId = ref(null)
    const providerOptions = ref([])

    const form = ref({
      name: '',
      provider: '',
      apiKey: '',
      baseUrl: '',
      model: '',
      isDefault: false
    })

    const selectedProviderNotImplemented = computed(() => {
      if (!form.value.provider) return false
      const p = providerOptions.value.find(item => item.provider === form.value.provider)
      return p && !p.isImplemented
    })

    const canTest = computed(() => {
      return form.value.provider && form.value.model && (form.value.apiKey || mode.value === 'edit')
    })

    const canSubmit = computed(() => {
      return form.value.name.trim() && form.value.provider && form.value.model.trim() && (mode.value === 'edit' || form.value.apiKey.trim())
    })

    const loadMeta = async () => {
      try {
        const res = await api.get('/v1/AI/configs/meta')
        if (res.data?.status_code === 1000 && res.data.providers) {
          providerOptions.value = res.data.providers
        }
      } catch (e) {
        console.error('Load meta error:', e)
      }
    }

    const loadConfigs = async () => {
      loadingList.value = true
      try {
        const res = await api.get('/v1/AI/configs')
        if (res.data?.status_code === 1000 && res.data.configs) {
          configs.value = res.data.configs
        } else {
          configs.value = []
        }
      } catch (e) {
        console.error('Load configs error:', e)
        configs.value = []
      } finally {
        loadingList.value = false
      }
    }

    const startCreate = () => {
      mode.value = 'create'
      editingId.value = null
      testResult.value = ''
      form.value = { name: '', provider: '', apiKey: '', baseUrl: '', model: '', isDefault: false }
    }

    const startEdit = (config) => {
      mode.value = 'edit'
      editingId.value = config.id
      testResult.value = ''
      form.value = {
        name: config.name,
        provider: config.provider,
        apiKey: '',
        baseUrl: config.baseUrl || '',
        model: config.model,
        isDefault: config.isDefault
      }
    }

    const backToList = () => {
      mode.value = 'list'
      editingId.value = null
      testResult.value = ''
    }

    const testConfig = async () => {
      testingConfig.value = true
      testResult.value = ''
      try {
        const res = await api.post('/v1/AI/configs/test', {
          provider: form.value.provider,
          apiKey: form.value.apiKey,
          baseUrl: form.value.baseUrl,
          model: form.value.model
        })
        testResult.value = res.data?.status_code === 1000 ? 'success' : 'fail'
      } catch {
        testResult.value = 'fail'
      } finally {
        testingConfig.value = false
      }
    }

    const submitForm = async () => {
      if (!canSubmit.value || submitting.value) return
      submitting.value = true
      try {
        const payload = {
          name: form.value.name.trim(),
          provider: form.value.provider,
          apiKey: form.value.apiKey,
          baseUrl: form.value.baseUrl.trim(),
          model: form.value.model.trim(),
          isDefault: form.value.isDefault
        }
        let res
        if (mode.value === 'create') {
          res = await api.post('/v1/AI/configs', payload)
        } else {
          res = await api.put(`/v1/AI/configs/${editingId.value}`, payload)
        }
        if (res.data?.status_code === 1000) {
          ElMessage.success(mode.value === 'create' ? '配置已创建' : '配置已更新')
          await loadConfigs()
          emit('configsChanged')
          backToList()
        } else {
          ElMessage.error(res.data?.status_msg || '保存失败')
        }
      } catch (e) {
        ElMessage.error('请求异常')
      } finally {
        submitting.value = false
      }
    }

    const deleteConfig = async (config) => {
      try {
        await ElMessageBox.confirm(
          `确定删除配置「${config.name}」吗？删除后使用此配置的会话将回退到默认配置。`,
          '删除确认',
          { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
        )
        const res = await api.delete(`/v1/AI/configs/${config.id}`)
        if (res.data?.status_code === 1000) {
          ElMessage.success('已删除')
          await loadConfigs()
          emit('configsChanged')
        } else {
          ElMessage.error(res.data?.status_msg || '删除失败')
        }
      } catch {
        // user cancelled
      }
    }

    const setDefault = async (configId) => {
      try {
        const res = await api.post(`/v1/AI/configs/${configId}/default`)
        if (res.data?.status_code === 1000) {
          ElMessage.success('已设为默认')
          await loadConfigs()
          emit('configsChanged')
        } else {
          ElMessage.error(res.data?.status_msg || '设置失败')
        }
      } catch {
        ElMessage.error('请求异常')
      }
    }

    const handleClose = () => {
      visible.value = false
      mode.value = 'list'
      editingId.value = null
      testResult.value = ''
    }

    const providerInitial = (provider) => {
      const map = { openai_compatible: 'O', claude: 'C', gemini: 'G', ollama: 'L' }
      return map[provider] || provider?.charAt(0)?.toUpperCase() || '?'
    }

    const providerBadgeClass = (provider) => {
      const map = {
        openai_compatible: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400',
        claude: 'bg-orange-50 text-orange-600 dark:bg-orange-500/10 dark:text-orange-400',
        gemini: 'bg-blue-50 text-blue-600 dark:bg-blue-500/10 dark:text-blue-400',
        ollama: 'bg-purple-50 text-purple-600 dark:bg-purple-500/10 dark:text-purple-400'
      }
      return map[provider] || 'bg-gray-100 text-gray-600 dark:bg-gray-500/10 dark:text-gray-400'
    }

    // Auto-load when dialog opens
    watch(visible, (val) => {
      if (val) {
        mode.value = 'list'
        loadMeta()
        loadConfigs()
      }
    })

    return {
      visible,
      mode,
      configs,
      loadingList,
      submitting,
      testingConfig,
      testResult,
      form,
      providerOptions,
      selectedProviderNotImplemented,
      canTest,
      canSubmit,
      startCreate,
      startEdit,
      backToList,
      testConfig,
      submitForm,
      deleteConfig,
      setDefault,
      handleClose,
      providerInitial,
      providerBadgeClass
    }
  }
}
</script>
