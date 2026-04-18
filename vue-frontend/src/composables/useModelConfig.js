import { computed, ref } from 'vue'
import api from '../utils/api'
import { CHAT_MODE_LABELS } from '../utils/messageHelpers'

export function useModelConfig({ userMenuVisible } = {}) {
  const modelConfigDialogVisible = ref(false)
  const availableConfigs = ref([])
  const selectedConfigId = ref(null)
  const selectedChatMode = ref('chat')
  const configsMeta = ref({ providers: [], chatModes: [] })
  const fileManagerVisible = ref(false)

  const availableChatModes = computed(() => {
    if (!selectedConfigId.value || !availableConfigs.value.length) {
      return configsMeta.value.chatModes || ['chat']
    }
    const cfg = availableConfigs.value.find(c => c.id === selectedConfigId.value)
    if (cfg?.providerCapability?.supportedChatModes?.length) {
      return cfg.providerCapability.supportedChatModes
    }
    return configsMeta.value.chatModes || ['chat']
  })

  const chatModeLabel = (mode) => CHAT_MODE_LABELS[mode] || mode

  const onConfigChange = () => {
    if (!availableChatModes.value.includes(selectedChatMode.value)) {
      selectedChatMode.value = availableChatModes.value[0] || 'chat'
    }
  }

  const loadConfigsMeta = async () => {
    try {
      const res = await api.get('/AI/configs/meta')
      if (res.data?.status_code === 1000) {
        configsMeta.value = {
          providers: res.data.providers || [],
          chatModes: res.data.chatModes || ['chat']
        }
      }
    } catch (e) {
      console.error('Load configs meta error:', e)
    }
  }

  const loadAvailableConfigs = async () => {
    try {
      const res = await api.get('/AI/configs')
      if (res.data?.status_code === 1000 && res.data.configs) {
        availableConfigs.value = res.data.configs
        if (!selectedConfigId.value) {
          const defaultCfg = res.data.configs.find(c => c.isDefault)
          selectedConfigId.value = defaultCfg ? defaultCfg.id : (res.data.configs[0]?.id || null)
        }
      }
    } catch (e) {
      console.error('Load configs error:', e)
    }
  }

  const refreshConfigs = async () => {
    await loadAvailableConfigs()
    onConfigChange()
  }

  const openModelConfigDialog = () => {
    if (userMenuVisible) userMenuVisible.value = false
    modelConfigDialogVisible.value = true
  }

  const openFileManagerDialog = () => {
    if (userMenuVisible) userMenuVisible.value = false
    fileManagerVisible.value = true
  }

  return {
    modelConfigDialogVisible,
    availableConfigs,
    selectedConfigId,
    selectedChatMode,
    configsMeta,
    availableChatModes,
    chatModeLabel,
    onConfigChange,
    loadConfigsMeta,
    loadAvailableConfigs,
    refreshConfigs,
    openModelConfigDialog,
    fileManagerVisible,
    openFileManagerDialog
  }
}