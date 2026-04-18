<template>
  <div class="sticky top-0 z-10 px-4 h-12 flex items-center backdrop-blur-lg bg-bg-light/80 dark:bg-bg-dark/80 border-b border-border-light/50 dark:border-border-dark/50">
    <!-- GoMind Brand (shows in header when messages exist) -->
    <div v-if="hasMessages" class="flex items-center gap-2">
      <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 text-accent-light dark:text-accent-dark" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"/></svg>
      <span class="text-base font-semibold tracking-tight select-none">GoMind</span>
    </div>

    <div class="flex items-center gap-2 ml-auto">
      <button @click="$emit('toggle-theme')" class="p-2 rounded-full hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer border-none bg-transparent text-text-primary-light dark:text-text-primary-dark flex items-center justify-center shrink-0">
        <svg v-if="isDark" xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"/></svg>
        <svg v-else xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"/></svg>
      </button>
    </div>
  </div>
</template>

<script>
/* eslint-env node */
export default {
  name: 'ChatHeader',
  props: {
    hasMessages: { type: Boolean, default: false },
    isDark: { type: Boolean, default: false },
    loading: { type: Boolean, default: false }
  },
  emits: ['toggle-theme']
}
</script>