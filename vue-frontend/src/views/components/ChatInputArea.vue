<template>
  <div class="absolute bottom-0 left-0 w-full px-4 pt-8 pb-6 z-20 bg-gradient-to-t from-bg-light via-bg-light/95 to-transparent dark:from-neutral-950 dark:via-neutral-950/95">
    <div class="mx-auto w-full max-w-3xl">
      <div class="bg-white dark:bg-neutral-900/80 dark:backdrop-blur-xl rounded-3xl shadow-[0_2px_15px_-3px_rgba(0,0,0,0.07),0_10px_20px_-2px_rgba(0,0,0,0.04)] dark:shadow-[0_12px_30px_rgba(0,0,0,0.28)] border border-border-light dark:border-neutral-800 flex flex-col p-3 transition-all focus-within:ring-1 focus-within:ring-orange-500/50 focus-within:border-orange-500/50">

      <!-- Active mode chips -->
      <div v-if="activeModes.length" class="flex flex-wrap gap-1.5 px-2 pt-1">
        <span
          v-for="mode in activeModes"
          :key="mode.key"
          class="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-medium bg-accent-light/10 dark:bg-orange-500/10 text-accent-light dark:text-orange-500 border border-accent-light/30 dark:border-orange-500/30"
        >
          {{ mode.label }}
          <button type="button" class="hover:text-red-500 dark:hover:text-red-400 transition-colors cursor-pointer bg-transparent border-none p-0 leading-none" @click="removeMode(mode.key)">&times;</button>
        </span>
      </div>

      <!-- Slash command menu -->
      <div v-if="showSlashMenu && slashCommands.length" class="mx-2 mt-1 rounded-xl border border-border-light dark:border-neutral-800 bg-white dark:bg-neutral-900 shadow-lg overflow-hidden">
        <div class="px-3 py-2 text-xs font-medium text-text-secondary-light dark:text-neutral-500 border-b border-border-light dark:border-neutral-800">选择模式</div>
        <button
          v-for="(cmd, idx) in slashCommands"
          :key="cmd.key"
          type="button"
          :class="[
            'w-full flex items-center gap-3 px-3 py-2.5 text-left text-sm transition-colors cursor-pointer bg-transparent border-none text-text-primary-light dark:text-neutral-100',
            slashMenuIndex === idx ? 'bg-[#EDE8E1] dark:bg-neutral-800/80' : 'hover:bg-[#EDE8E1] dark:hover:bg-neutral-800/80'
          ]"
          @click="selectSlashCommand(cmd)"
          @mouseenter="slashMenuIndex = idx"
        >
          <span class="text-base">{{ cmd.icon }}</span>
          <div>
            <div class="font-medium">{{ cmd.label }}</div>
            <div class="text-xs text-text-secondary-light dark:text-neutral-500">{{ cmd.description }}</div>
          </div>
        </button>
      </div>

      <!-- Toolbar -->
      <div class="flex items-center gap-0.5 px-2 pt-1">
        <button @click="handleFileUploadClick" :disabled="uploading || loading" class="p-2 rounded-full text-text-secondary-light dark:text-neutral-400 hover:bg-[#EDE8E1] dark:hover:bg-neutral-800/80 hover:text-text-primary-light dark:hover:text-neutral-100 transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed bg-transparent border-none" title="上传文档 (.md/.txt)">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M15.172 7l-6.586 6.586a2 2 0 102.828 2.828l6.414-6.586a4 4 0 00-5.656-5.656l-6.415 6.585a6 6 0 108.486 8.486L20.5 13"/></svg>
        </button>
        <button @click="handleImageUploadClick" :disabled="loading" class="p-2 rounded-full text-text-secondary-light dark:text-neutral-400 hover:bg-[#EDE8E1] dark:hover:bg-neutral-800/80 hover:text-text-primary-light dark:hover:text-neutral-100 transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed bg-transparent border-none" title="图像识别">
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
          placeholder="Message GoMind..."
          class="flex-1 max-h-40 min-h-[44px] bg-transparent border-none outline-none resize-none px-4 py-2 text-[15px] leading-7 text-text-primary-light dark:text-neutral-100 placeholder:text-text-secondary-light dark:placeholder:text-neutral-500 focus:ring-0"
        ></textarea>

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
              ? 'bg-[#EDE8E1] dark:bg-neutral-800 text-text-secondary-light dark:text-neutral-500 opacity-60'
              : 'bg-accent-light text-white shadow-sm hover:bg-accent-light/90'
          ]"
        >
          <svg v-if="!loading" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" class="w-5 h-5">
            <path d="M3.478 2.404a.75.75 0 00-.926.941l2.432 7.905H13.5a.75.75 0 010 1.5H4.984l-2.432 7.905a.75.75 0 00.926.94 60.519 60.519 0 0018.445-8.986.75.75 0 000-1.218A60.517 60.517 0 003.478 2.404z" />
          </svg>
        </button>
      </div>
      </div>
      <p class="text-center text-xs text-text-secondary-light/80 dark:text-neutral-500 mt-3">
        GoMind may occasionally generate inaccurate information. Verify critical details.
      </p>
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
    selectedChatMode: { type: String, default: '' },
    availableChatModes: { type: Array, default: () => [] },
    chatModeLabel: { type: Function, default: (mode) => mode }
  },
  emits: [
    'update:inputMessage',
    'update:selectedChatMode',
    'send-message',
    'stop-stream',
    'trigger-file-upload',
    'trigger-image-upload',
    'file-upload',
    'image-recognition'
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
      handleFileUploadClick, handleImageUploadClick,
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
