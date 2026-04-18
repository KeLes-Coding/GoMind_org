<template>
  <div class="absolute bottom-6 left-1/2 -translate-x-1/2 w-full max-w-3xl px-4 z-20">
    <div class="bg-surface-light dark:bg-surface-dark rounded-3xl shadow-[0_2px_12px_rgba(0,0,0,0.08)] dark:shadow-[0_2px_12px_rgba(0,0,0,0.3)] ring-1 ring-black/5 dark:ring-white/10 flex flex-col p-2 transition-shadow focus-within:ring-2 focus-within:ring-black/10 dark:focus-within:ring-white/15">

      <!-- Active mode chips -->
      <div v-if="activeModes.length" class="flex flex-wrap gap-1.5 px-2 pt-1">
        <span
          v-for="mode in activeModes"
          :key="mode.key"
          class="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium bg-accent-light/15 dark:bg-accent-dark/15 text-accent-light dark:text-accent-dark border border-accent-light/30 dark:border-accent-dark/30"
        >
          {{ mode.label }}
          <button type="button" class="hover:text-red-500 dark:hover:text-red-400 transition-colors cursor-pointer bg-transparent border-none p-0 leading-none" @click="removeMode(mode.key)">&times;</button>
        </span>
      </div>

      <!-- Slash command menu -->
      <div v-if="showSlashMenu && slashCommands.length" class="mx-2 mt-1 rounded-xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-lg overflow-hidden">
        <div class="px-3 py-2 text-xs font-medium text-text-secondary-light dark:text-text-secondary-dark border-b border-border-light dark:border-border-dark">选择模式</div>
        <button
          v-for="(cmd, idx) in slashCommands"
          :key="cmd.key"
          type="button"
          :class="[
            'w-full flex items-center gap-3 px-3 py-2.5 text-left text-sm transition-colors cursor-pointer bg-transparent border-none text-text-primary-light dark:text-text-primary-dark',
            slashMenuIndex === idx ? 'bg-black/5 dark:bg-white/5' : 'hover:bg-black/5 dark:hover:bg-white/5'
          ]"
          @click="selectSlashCommand(cmd)"
          @mouseenter="slashMenuIndex = idx"
        >
          <span class="text-base">{{ cmd.icon }}</span>
          <div>
            <div class="font-medium">{{ cmd.label }}</div>
            <div class="text-xs text-text-secondary-light dark:text-text-secondary-dark">{{ cmd.description }}</div>
          </div>
        </button>
      </div>

      <!-- Toolbar -->
      <div class="flex items-center gap-0.5 px-2 pt-1">
        <button @click="handleFileUploadClick" :disabled="uploading || loading" class="p-1.5 rounded-full text-text-secondary-light dark:text-text-secondary-dark hover:bg-black/5 dark:hover:bg-white/5 hover:text-text-primary-light dark:hover:text-text-primary-dark transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed bg-transparent border-none" title="上传文档 (.md/.txt)">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M15.172 7l-6.586 6.586a2 2 0 102.828 2.828l6.414-6.586a4 4 0 00-5.656-5.656l-6.415 6.585a6 6 0 108.486 8.486L20.5 13"/></svg>
        </button>
        <button @click="handleImageUploadClick" :disabled="loading" class="p-1.5 rounded-full text-text-secondary-light dark:text-text-secondary-dark hover:bg-black/5 dark:hover:bg-white/5 hover:text-text-primary-light dark:hover:text-text-primary-dark transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed bg-transparent border-none" title="图像识别">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
        </button>
        <input ref="fileInput" type="file" accept=".md,.txt,text/markdown,text/plain" class="hidden" @change="onFileChange" />
        <input ref="imageInput" type="file" accept="image/*" class="hidden" @change="onImageChange" />
      </div>

      <div class="flex items-end pb-1 pr-1">
        <textarea
          :value="inputMessage"
          @input="onInputChange"
          @keydown="onKeyDown"
          :disabled="loading"
          ref="messageInput"
          rows="1"
          placeholder="问点什么… 输入 / 选择 RAG 或 MCP"
          class="flex-1 max-h-40 min-h-[44px] bg-transparent border-none outline-none resize-none px-4 py-2 text-base text-text-primary-light dark:text-text-primary-dark placeholder-text-secondary-light dark:placeholder-text-secondary-dark"
        ></textarea>

        <!-- Model selector + settings -->
        <div class="flex items-center gap-1 mb-1 mx-1">
          <div class="toolbar-select-wrap max-w-[130px] !bg-black/5 dark:!bg-white/5 !border-none !rounded-xl">
            <select
              :value="selectedConfigId"
              @change="onConfigSelect"
              class="toolbar-select !py-1.5 !px-2.5 !pr-7 !text-xs !min-h-[36px] bg-transparent font-medium"
            >
              <option value="" disabled v-if="!availableConfigs.length">No Config</option>
              <option v-for="config in availableConfigs" :key="config.id" :value="config.id" class="bg-surface-light dark:bg-surface-dark">
                {{ config.name }}{{ config.isDefault ? ' ★' : '' }}
              </option>
            </select>
            <span class="toolbar-select-icon right-1.5" aria-hidden="true">
              <svg xmlns="http://www.w3.org/2000/svg" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" /></svg>
            </span>
          </div>

          <button type="button" @click="$emit('open-model-config-dialog')" class="p-2 w-9 h-9 rounded-full hover:bg-black/10 dark:hover:bg-white/10 text-text-secondary-light dark:text-text-secondary-dark hover:text-text-primary-light dark:hover:text-text-primary-dark transition-colors bg-transparent border-none cursor-pointer flex items-center justify-center shrink-0" title="Model Configs">
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" /><path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /></svg>
          </button>
        </div>

        <!-- Stop Button -->
        <button
           v-if="loading"
           type="button"
           @click="$emit('stop-stream')"
           class="p-2 w-10 h-10 mb-1 mr-1 rounded-full flex items-center justify-center transition-all bg-red-500/10 text-red-500 hover:bg-red-500/20 cursor-pointer shadow-sm border-none"
           title="停止生成"
        >
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="2"/></svg>
        </button>

        <!-- Send Button -->
        <button
          v-else
          type="button"
          :disabled="!inputMessage.trim() || loading"
          @click="$emit('send-message')"
          :class="[
            'p-2 w-10 h-10 mb-1 mr-1 rounded-full flex items-center justify-center transition-all disabled:cursor-not-allowed border-none',
            (!inputMessage.trim() || loading)
              ? 'bg-transparent text-text-secondary-light dark:text-text-secondary-dark opacity-50'
              : 'bg-black text-white dark:bg-white dark:text-black shadow-sm'
          ]"
        >
          <svg v-if="!loading" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" class="w-5 h-5">
            <path d="M3.478 2.404a.75.75 0 00-.926.941l2.432 7.905H13.5a.75.75 0 010 1.5H4.984l-2.432 7.905a.75.75 0 00.926.94 60.519 60.519 0 0018.445-8.986.75.75 0 000-1.218A60.517 60.517 0 003.478 2.404z" />
          </svg>
        </button>
      </div>
    </div>
  </div>
</template>

<script>
/* eslint-env node */
/* eslint-disable vue/no-mutating-props */
import { ref, computed, watch } from 'vue'

const SLASH_COMMANDS = [
  { key: 'rag', label: 'RAG', icon: '📄', description: '使用知识库增强检索', mode: 'chat_rag' },
  { key: 'mcp', label: 'MCP', icon: '🔧', description: '使用 MCP 工具调用', mode: 'chat_mcp' }
]

export default {
  name: 'ChatInputArea',
  props: {
    inputMessage: { type: String, default: '' },
    loading: { type: Boolean, default: false },
    uploading: { type: Boolean, default: false },
    selectedConfigId: { type: String, default: '' },
    selectedChatMode: { type: String, default: '' },
    availableConfigs: { type: Array, default: () => [] },
    availableChatModes: { type: Array, default: () => [] },
    chatModeLabel: { type: Function, default: (mode) => mode }
  },
  emits: [
    'update:inputMessage',
    'update:selectedConfigId',
    'update:selectedChatMode',
    'send-message',
    'stop-stream',
    'trigger-file-upload',
    'trigger-image-upload',
    'file-upload',
    'image-recognition',
    'config-change',
    'open-model-config-dialog'
  ],
  setup(props, { emit }) {
    const fileInput = ref(null)
    const imageInput = ref(null)
    const messageInput = ref(null)
    const showSlashMenu = ref(false)
    const activeModes = ref([])
    const slashMenuIndex = ref(0)

    const slashCommands = computed(() => {
      return SLASH_COMMANDS.filter(cmd => !activeModes.value.find(m => m.key === cmd.key))
    })

    watch(slashCommands, () => {
      slashMenuIndex.value = 0
    })

    const computedChatMode = computed(() => {
      const keys = activeModes.value.map(m => m.key)
      const hasRag = keys.includes('rag')
      const hasMcp = keys.includes('mcp')
      if (hasRag && hasMcp) return 'chat_rag_mcp'
      if (hasRag) return 'chat_rag'
      if (hasMcp) return 'chat_mcp'
      return 'chat'
    })

    watch(computedChatMode, (mode) => {
      emit('update:selectedChatMode', mode)
    }, { immediate: true })

    const onInputChange = (e) => {
      const value = e.target.value
      emit('update:inputMessage', value)

      if (value === '/') {
        showSlashMenu.value = true
        slashMenuIndex.value = 0
      } else if (!value.startsWith('/')) {
        showSlashMenu.value = false
      }
    }

    const onKeyDown = (e) => {
      if (showSlashMenu.value) {
        if (e.key === 'ArrowDown') {
          e.preventDefault()
          slashMenuIndex.value = (slashMenuIndex.value + 1) % slashCommands.value.length
          return
        }
        if (e.key === 'ArrowUp') {
          e.preventDefault()
          slashMenuIndex.value = (slashMenuIndex.value - 1 + slashCommands.value.length) % slashCommands.value.length
          return
        }
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault()
          if (slashCommands.value.length) {
            selectSlashCommand(slashCommands.value[slashMenuIndex.value])
          }
          return
        }
        if (e.key === 'Escape') {
          showSlashMenu.value = false
          return
        }
      } else {
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault()
          emit('send-message')
        }
      }
    }

    const selectSlashCommand = (cmd) => {
      if (!activeModes.value.find(m => m.key === cmd.key)) {
        activeModes.value.push({ key: cmd.key, label: cmd.label })
      }
      const current = props.inputMessage
      if (current.startsWith('/')) {
        emit('update:inputMessage', current.slice(1))
      }
      showSlashMenu.value = false
      messageInput.value?.focus()
    }

    const removeMode = (key) => {
      activeModes.value = activeModes.value.filter(m => m.key !== key)
    }

    const onConfigSelect = (e) => {
      emit('update:selectedConfigId', e.target.value)
      emit('config-change')
    }

    // Expose internal refs for parent
    const handleFileUploadClick = () => {
      emit('trigger-file-upload')
      fileInput.value?.click()
    }

    const handleImageUploadClick = () => {
      emit('trigger-image-upload')
      imageInput.value?.click()
    }

    const onFileChange = (e) => {
      emit('file-upload', e)
    }

    const onImageChange = (e) => {
      emit('image-recognition', e)
    }

    return {
      fileInput, imageInput, messageInput,
      showSlashMenu, activeModes, slashCommands, slashMenuIndex,
      onInputChange, onKeyDown, selectSlashCommand, removeMode,
      onConfigSelect, handleFileUploadClick, handleImageUploadClick,
      onFileChange, onImageChange
    }
  }
}
</script>

<style scoped>
.toolbar-select-wrap {
  position: relative;
  display: flex;
  min-width: 80px;
  flex: 0 1 auto;
}

.toolbar-select {
  width: 100%;
  min-width: 0;
  appearance: none;
  border: 1px solid rgba(0, 0, 0, 0.08);
  border-radius: 0.75rem;
  background: rgba(255, 255, 255, 0.88);
  color: inherit;
  cursor: pointer;
  outline: none;
  padding: 0.45rem 2rem 0.45rem 0.75rem;
  font-size: 0.875rem;
  line-height: 1.25rem;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background-color 0.2s ease, opacity 0.2s ease;
}

.toolbar-select:hover:not(:disabled) {
  border-color: rgba(0, 0, 0, 0.16);
}

.toolbar-select:focus {
  border-color: rgba(245, 158, 11, 0.5);
  box-shadow: 0 0 0 3px rgba(245, 158, 11, 0.14);
}

.toolbar-select:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.toolbar-select-icon {
  position: absolute;
  top: 50%;
  right: 0.75rem;
  display: flex;
  align-items: center;
  pointer-events: none;
  transform: translateY(-50%);
  color: inherit;
  opacity: 0.68;
}

.dark .toolbar-select {
  border-color: rgba(255, 255, 255, 0.1);
  background: rgba(23, 23, 23, 0.92);
}

.dark .toolbar-select:hover:not(:disabled) {
  border-color: rgba(255, 255, 255, 0.18);
}

.dark .toolbar-select:focus {
  border-color: rgba(251, 191, 36, 0.55);
  box-shadow: 0 0 0 3px rgba(251, 191, 36, 0.18);
}
</style>