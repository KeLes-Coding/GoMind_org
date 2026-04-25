<template>
  <div class="sticky top-0 z-10 px-4 md:px-6 h-14 flex items-center backdrop-blur-xl bg-bg-light/85 dark:bg-neutral-950/80 border-b border-border-light/60 dark:border-neutral-800">
    <div v-if="hasMessages" class="flex items-center gap-2 min-w-0">
      <button class="flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-left transition-colors hover:bg-black/5 dark:hover:bg-white/10">
        <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 text-accent-light dark:text-orange-500 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"/></svg>
        <span class="truncate text-lg font-semibold tracking-tight select-none dark:text-neutral-50">GoMind <span class="font-normal text-text-secondary-light dark:text-neutral-500">1.0</span></span>
        <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 text-text-secondary-light dark:text-neutral-500 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7"/></svg>
      </button>
    </div>

    <div class="flex items-center gap-2 ml-auto">
      <button @click="$emit('toggle-theme')" class="p-2 rounded-full hover:bg-black/5 dark:hover:bg-neutral-800/80 transition-colors cursor-pointer border-none bg-transparent text-text-primary-light dark:text-neutral-100 flex items-center justify-center shrink-0" title="切换主题">
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
