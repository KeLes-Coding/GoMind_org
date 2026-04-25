<template>
  <div class="flex-1 overflow-y-auto px-4 md:px-16 lg:px-32 pt-6 pb-44" ref="scrollContainer">
    <div class="max-w-4xl mx-auto flex flex-col">
      <div
        v-for="(message, index) in currentMessages"
        :key="index"
      >
        <!-- User message: right-aligned colored bubble -->
        <div v-if="message.role === 'user'" class="flex justify-end mb-6">
          <div class="max-w-[85%]">
            <div class="flex items-end gap-2 justify-end">
              <div class="bg-black text-white dark:bg-white dark:text-black rounded-2xl rounded-br-sm px-4 py-3">
                <img v-if="message.imageUrl" :src="message.imageUrl" alt="Uploaded image" class="max-w-xs rounded-xl shadow-md mb-2" />
                <div class="whitespace-pre-wrap break-words">{{ getMessageRawMarkdown(message) }}</div>
              </div>
              <img
                v-if="userProfile.avatar_url"
                :src="userProfile.avatar_url"
                alt="头像"
                class="w-7 h-7 rounded-full object-cover border border-border-light dark:border-border-dark shrink-0"
              />
              <div v-else class="w-7 h-7 rounded-full flex items-center justify-center font-bold text-xs select-none bg-black text-white dark:bg-white dark:text-black shrink-0">
                {{ getUserInitial }}
              </div>
            </div>
          </div>
        </div>

        <!-- AI message: left-aligned with avatar -->
        <div v-else class="flex items-start gap-3 mb-6">
          <div class="w-7 h-7 rounded-full bg-surface-light dark:bg-surface-dark flex items-center justify-center text-xs font-bold shrink-0 border border-border-light dark:border-border-dark select-none">
            AI
          </div>
          <div class="flex-1 min-w-0 max-w-[85%]">
            <!-- Actions & Meta above content -->
            <div v-if="message.content || message.imageUrl || getMessageReasoning(message)" class="flex items-center gap-2 mb-2 flex-wrap">
              <div class="flex items-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                <button
                  v-if="message.content"
                  class="px-2 py-0.5 text-xs rounded-full bg-surface-light dark:bg-surface-dark border border-border-light dark:border-border-dark hover:text-accent-light dark:hover:text-accent-dark cursor-pointer transition-colors"
                  @click="$emit('play-tts', message.content)"
                >
                  朗读
                </button>
                <button
                  v-if="message.content"
                  class="px-2 py-0.5 text-xs rounded-full bg-surface-light dark:bg-surface-dark border border-border-light dark:border-border-dark hover:text-accent-light dark:hover:text-accent-dark cursor-pointer transition-colors"
                  @click="toggleViewMode(index)"
                >
                  {{ getViewMode(index) === 'preview' ? '源码' : '预览' }}
                </button>
              </div>
              <span v-if="getMessageMetaStatus(message)" class="text-xs text-text-secondary-light dark:text-text-secondary-dark ml-auto">
                {{ getMessageStatusLabel(getMessageMetaStatus(message)) }}
              </span>
            </div>
            <!-- Image Preview -->
            <img v-if="message.imageUrl" :src="message.imageUrl" alt="Uploaded image" class="max-w-xs rounded-xl shadow-md mb-2" />
            <details
              v-if="getMessageReasoning(message)"
              :open="Boolean(reasoningExpanded[index])"
              class="mb-3 rounded-2xl border border-amber-200/70 bg-amber-50/80 px-4 py-3 text-sm text-amber-950 dark:border-amber-900/80 dark:bg-amber-950/40 dark:text-amber-100"
              @toggle="reasoningExpanded[index] = $event.target.open"
            >
              <summary class="cursor-pointer select-none font-medium">推理链</summary>
              <pre class="mt-3 whitespace-pre-wrap break-words font-mono text-xs leading-6">{{ getMessageReasoning(message) }}</pre>
            </details>
            <pre
              v-if="message.content && getViewMode(index) === 'source'"
              class="whitespace-pre-wrap break-words rounded-2xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark px-4 py-3 text-sm"
            >{{ getMessageRawMarkdown(message) }}</pre>
            <div
              v-else-if="message.content"
              v-html="renderMarkdown(message.content)"
              class="prose dark:prose-invert prose-p:my-2 prose-pre:bg-surface-light dark:prose-pre:bg-surface-dark prose-pre:border prose-pre:border-border-light dark:prose-pre:border-border-dark prose-pre:shadow-[0_2px_10px_rgba(0,0,0,0.02)] max-w-none"
            ></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
/* eslint-env node */
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
    const getUserInitial = computed(() => {
      const name = props.userProfile.name || props.userProfile.username || 'U'
      return name.slice(0, 1).toUpperCase()
    })
    const getViewMode = (index) => viewModes[index] || 'preview'
    const toggleViewMode = (index) => {
      viewModes[index] = getViewMode(index) === 'preview' ? 'source' : 'preview'
    }
    return {
      scrollContainer,
      viewModes,
      reasoningExpanded,
      getUserInitial,
      getViewMode,
      toggleViewMode,
      renderMarkdown,
      getMessageReasoning,
      getMessageRawMarkdown,
      getMessageStatusLabel,
      getMessageMetaStatus
    }
  }
}
</script>
