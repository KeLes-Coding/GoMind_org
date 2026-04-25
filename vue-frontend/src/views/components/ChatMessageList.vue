<template>
  <div class="flex-1 overflow-y-auto px-4 pt-6 pb-48 scroll-smooth" ref="scrollContainer">
    <div class="max-w-3xl mx-auto flex flex-col">
      <div
        v-for="(message, index) in currentMessages"
        :key="index"
        class="message-row"
      >
        <div v-if="message.role === 'user'" class="flex flex-row-reverse items-start gap-3 mb-7 group">
          <img
            v-if="userProfile.avatar_url"
            :src="userProfile.avatar_url"
            alt="头像"
            class="w-9 h-9 rounded-full object-cover border border-white/80 dark:border-neutral-700 shrink-0 mt-1 shadow-sm"
          />
          <div v-else class="w-9 h-9 rounded-full flex items-center justify-center font-bold text-xs select-none bg-[#2E2A24] text-white dark:bg-neutral-100 dark:text-neutral-950 shrink-0 mt-1 border border-white/80 dark:border-neutral-700 shadow-sm">
            {{ getUserInitial }}
          </div>
          <div class="flex flex-col items-end min-w-0 max-w-[82%] sm:max-w-[74%] pt-1">
            <img v-if="message.imageUrl" :src="message.imageUrl" alt="Uploaded image" class="max-w-xs rounded-lg shadow-md mb-3 border border-border-light dark:border-neutral-700" />
            <div class="relative max-w-full">
              <div
                v-html="renderMarkdown(message.content)"
                @click="handleMarkdownClick"
                class="user-message-bubble prose dark:prose-invert max-w-none rounded-2xl rounded-tr-md border border-[#E6D8C7] bg-[#FFF7EC] px-4 py-3 text-[15px] leading-7 text-[#2E261D] shadow-sm dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-100"
                :class="{ 'user-message-bubble--collapsed': isUserMessageCollapsed(index, message) }"
              ></div>
              <button
                v-if="isExpandableUserMessage(message)"
                type="button"
                class="mt-2 inline-flex items-center rounded-md border border-transparent px-2 py-1 text-xs text-text-secondary-light transition-colors hover:bg-[#EDE8E1] hover:text-text-primary-light dark:text-neutral-400 dark:hover:bg-neutral-800 dark:hover:text-neutral-100"
                @click="toggleUserMessage(index)"
              >
                {{ isUserMessageCollapsed(index, message) ? '展开' : '收起' }}
              </button>
            </div>
          </div>
        </div>

        <div v-else class="flex gap-4 mb-10 group">
          <div class="w-8 h-8 rounded-full bg-accent-light flex items-center justify-center text-white shrink-0 mt-1 shadow-sm">
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2l1.65 5.08L19 5.45l-3.3 4.55L21 12l-5.3 2 3.3 4.55-5.35-1.63L12 22l-1.65-5.08L5 18.55 8.3 14 3 12l5.3-2L5 5.45l5.35 1.63L12 2z"/></svg>
          </div>
          <div class="flex-1 min-w-0 pt-1.5">
            <div v-if="message.content || message.imageUrl || getMessageReasoning(message)" class="flex items-center gap-2 mb-3 flex-wrap">
              <div class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                <button
                  v-if="message.content"
                  class="px-2 py-1 text-xs rounded-md bg-transparent hover:bg-[#EDE8E1] dark:hover:bg-neutral-800/80 text-text-secondary-light dark:text-neutral-400 hover:text-text-primary-light dark:hover:text-neutral-100 cursor-pointer transition-colors border-none"
                  @click="$emit('play-tts', message.content)"
                >
                  朗读
                </button>
                <button
                  v-if="message.content"
                  class="px-2 py-1 text-xs rounded-md bg-transparent hover:bg-[#EDE8E1] dark:hover:bg-neutral-800/80 text-text-secondary-light dark:text-neutral-400 hover:text-text-primary-light dark:hover:text-neutral-100 cursor-pointer transition-colors border-none"
                  @click="toggleViewMode(index)"
                >
                  {{ getViewMode(index) === 'preview' ? '源码' : '预览' }}
                </button>
              </div>
              <span v-if="getMessageMetaStatus(message)" class="text-xs text-text-secondary-light dark:text-neutral-500 ml-auto">
                {{ getMessageStatusLabel(getMessageMetaStatus(message)) }}
              </span>
            </div>
            <img v-if="message.imageUrl" :src="message.imageUrl" alt="Uploaded image" class="max-w-xs rounded-lg shadow-md mb-3 border border-border-light dark:border-neutral-700" />
            <details
              v-if="getMessageReasoning(message)"
              :open="Boolean(reasoningExpanded[index])"
              class="mb-4 rounded-lg border border-accent-light/30 bg-accent-light/10 px-4 py-3 text-sm text-[#5B3510] dark:border-orange-500/30 dark:bg-neutral-900 dark:text-neutral-300"
              @toggle="reasoningExpanded[index] = $event.target.open"
            >
              <summary class="cursor-pointer select-none font-medium">推理链</summary>
              <pre class="mt-3 whitespace-pre-wrap break-words font-mono text-xs leading-6">{{ getMessageReasoning(message) }}</pre>
            </details>
            <pre
              v-if="message.content && getViewMode(index) === 'source'"
              class="whitespace-pre-wrap break-words rounded-lg border border-border-light dark:border-neutral-700 bg-white dark:bg-[#1e1e1e] px-4 py-3 text-sm leading-7 dark:text-neutral-300"
            >{{ getMessageRawMarkdown(message) }}</pre>
            <div
              v-else-if="message.content"
              v-html="renderMarkdown(message.content)"
              @click="handleMarkdownClick"
              class="prose dark:prose-invert prose-p:my-2 prose-p:leading-7 dark:prose-p:text-neutral-300 prose-pre:bg-[#1e1e1e] prose-pre:text-[#e5e5e5] prose-pre:rounded-lg prose-pre:border prose-pre:border-[#3a3a3a] prose-code:text-[#B65A00] dark:prose-code:text-green-400 prose-a:text-accent-light dark:prose-a:text-orange-500 max-w-none"
            ></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
/* eslint-env browser */
import { ref, computed, reactive } from 'vue'
import { getMessageReasoning, getMessageRawMarkdown, getMessageStatusLabel, getMessageMetaStatus, renderMarkdown } from '../../utils/messageHelpers'

export default {
  name: 'ChatMessageList',
  props: {
    currentMessages: { type: Array, default: () => [] },
    userProfile: { type: Object, default: () => ({}) }
  },
  emits: ['play-tts'],
  setup(props) {
    const scrollContainer = ref(null)
    const viewModes = reactive({})
    const reasoningExpanded = reactive({})
    const expandedUserMessages = reactive({})
    const getUserInitial = computed(() => {
      const name = props.userProfile.name || props.userProfile.username || 'U'
      return name.slice(0, 1).toUpperCase()
    })
    const getViewMode = (index) => viewModes[index] || 'preview'
    const toggleViewMode = (index) => {
      viewModes[index] = getViewMode(index) === 'preview' ? 'source' : 'preview'
    }
    const isExpandableUserMessage = (message) => {
      const raw = getMessageRawMarkdown(message)
      return raw.length > 90 || raw.split(/\r\n|\r|\n/).length > 2
    }
    const isUserMessageCollapsed = (index, message) => {
      return isExpandableUserMessage(message) && !expandedUserMessages[index]
    }
    const toggleUserMessage = (index) => {
      expandedUserMessages[index] = !expandedUserMessages[index]
    }
    const copyText = async (text) => {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text)
        return
      }
      const textarea = document.createElement('textarea')
      textarea.value = text
      textarea.setAttribute('readonly', '')
      textarea.style.position = 'fixed'
      textarea.style.top = '-9999px'
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
    }
    const handleMarkdownClick = async (event) => {
      const button = event.target.closest?.('[data-copy-code]')
      if (!button) return
      const encoded = button.getAttribute('data-copy-code') || ''
      await copyText(decodeURIComponent(encoded))
      button.classList.add('is-copied')
      window.setTimeout(() => button.classList.remove('is-copied'), 1200)
    }
    return {
      scrollContainer,
      viewModes,
      reasoningExpanded,
      expandedUserMessages,
      getUserInitial,
      getViewMode,
      toggleViewMode,
      isExpandableUserMessage,
      isUserMessageCollapsed,
      toggleUserMessage,
      handleMarkdownClick,
      renderMarkdown,
      getMessageReasoning,
      getMessageRawMarkdown,
      getMessageStatusLabel,
      getMessageMetaStatus
    }
  }
}
</script>
