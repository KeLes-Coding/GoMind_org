<template>
  <div class="flex-1 flex flex-col items-center justify-center px-8">
    <div class="flex flex-col items-center gap-6 -mt-16">
      <!-- GoMind Logo -->
      <div class="flex items-center gap-3">
        <svg xmlns="http://www.w3.org/2000/svg" class="h-12 w-12 text-accent-light dark:text-accent-dark" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"/></svg>
        <h1 class="text-4xl font-bold tracking-tight select-none">GoMind</h1>
      </div>
      <p class="text-text-secondary-light dark:text-text-secondary-dark text-sm">Powered by multi-provider AI · Ask anything</p>

      <!-- Social Links -->
      <div class="flex items-center gap-4 mt-6">
        <a href="#" title="GitHub" class="p-2 rounded-lg hover:bg-black/5 dark:hover:bg-white/5 text-text-secondary-light dark:text-text-secondary-dark hover:text-text-primary-light dark:hover:text-text-primary-dark transition-colors">
          <svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
        </a>
        <a href="#" title="Telegram" class="p-2 rounded-lg hover:bg-black/5 dark:hover:bg-white/5 text-text-secondary-light dark:text-text-secondary-dark hover:text-text-primary-light dark:hover:text-text-primary-dark transition-colors">
          <svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24"><path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/></svg>
        </a>
        <a href="#" title="WeChat" class="p-2 rounded-lg hover:bg-black/5 dark:hover:bg-white/5 text-text-secondary-light dark:text-text-secondary-dark hover:text-text-primary-light dark:hover:text-text-primary-dark transition-colors">
          <svg class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24"><path d="M8.691 2.188C3.891 2.188 0 5.476 0 9.53c0 2.212 1.17 4.203 3.002 5.55a.59.59 0 0 1 .213.665l-.39 1.48c-.019.07-.048.141-.048.213 0 .163.13.295.29.295a.326.326 0 0 0 .167-.054l1.903-1.114a.864.864 0 0 1 .717-.098 10.16 10.16 0 0 0 2.837.403c.276 0 .543-.027.812-.05-.168-.628-.254-1.282-.254-1.947 0-3.76 3.508-6.864 7.84-6.864.354 0 .703.032 1.048.07-.626-3.456-4.148-6.09-8.446-6.09zm-1.64 3.39a1.12 1.12 0 1 1 0 2.24 1.12 1.12 0 0 1 0-2.24zm4.32 0a1.12 1.12 0 1 1 0 2.24 1.12 1.12 0 0 1 0-2.24zm3.707 4.666c-3.768 0-6.923 2.72-6.923 6.068 0 3.347 3.155 6.068 6.923 6.068.784 0 1.556-.124 2.295-.348a.724.724 0 0 1 .601.078l1.588.934a.27.27 0 0 0 .14.046.242.242 0 0 0 .242-.244c0-.06-.023-.119-.038-.178l-.327-1.233a.496.496 0 0 1 .177-.556c1.534-1.13 2.524-2.797 2.524-4.667 0-3.348-3.156-6.068-6.923-6.068v.1zm-2.084 3.09a.935.935 0 1 1 0 1.87.935.935 0 0 1 0-1.87zm4.352 0a.935.935 0 1 1 0 1.87.935.935 0 0 1 0-1.87z"/></svg>
        </a>
      </div>
    </div>
  </div>
</template>

<script>
/* eslint-env node */
export default {
  name: 'ChatEmptyState',
  emits: [],
  setup() {
    return {}
  }
}
</script>