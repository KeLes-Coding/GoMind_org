<template>
  <div class="sticky top-0 z-10 px-4 md:px-6 h-14 flex items-center backdrop-blur-xl bg-bg-light/85 dark:bg-neutral-950/80 border-b border-border-light/60 dark:border-neutral-800">
    <div class="flex items-center gap-2 min-w-0">
      <div ref="modelSelectRoot" class="model-select-wrap">
        <button
          type="button"
          class="model-select-trigger"
          :disabled="loading"
          title="Model Selector"
          @click.stop="modelMenuOpen = !modelMenuOpen"
        >
          <span class="model-select-dot"></span>
          <span class="model-select-text">{{ selectedConfigLabel }}</span>
          <svg xmlns="http://www.w3.org/2000/svg" class="h-3.5 w-3.5 shrink-0 opacity-70" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7"/></svg>
        </button>
        <div v-if="modelMenuOpen" class="model-select-menu">
          <button
            v-for="config in availableConfigs"
            :key="config.id"
            type="button"
            class="model-select-option"
            :class="{ 'is-active': selectedConfigId === String(config.id) }"
            @click="selectConfig(config.id)"
          >
            <span class="truncate">{{ config.name }}</span>
            <span v-if="config.isDefault" class="model-select-badge">Default</span>
          </button>
          <div v-if="!availableConfigs.length" class="model-select-empty">No Config</div>
          <div class="model-select-divider"></div>
          <button type="button" class="model-select-settings" @click="openModelConfigs">
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" /><path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /></svg>
            <span>Model Configs</span>
          </button>
        </div>
      </div>
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
    loading: { type: Boolean, default: false },
    selectedConfigId: { type: String, default: '' },
    availableConfigs: { type: Array, default: () => [] }
  },
  emits: ['toggle-theme', 'update:selectedConfigId', 'config-change', 'open-model-config-dialog'],
  data() {
    return {
      modelMenuOpen: false
    }
  },
  computed: {
    selectedConfigLabel() {
      const found = this.availableConfigs.find(config => String(config.id) === String(this.selectedConfigId))
      return found ? `${found.name}${found.isDefault ? ' ★' : ''}` : 'Model Selector'
    }
  },
  mounted() {
    document.addEventListener('click', this.handleDocumentClick)
  },
  beforeUnmount() {
    document.removeEventListener('click', this.handleDocumentClick)
  },
  methods: {
    selectConfig(id) {
      this.modelMenuOpen = false
      this.$emit('update:selectedConfigId', String(id))
      this.$emit('config-change')
    },
    openModelConfigs() {
      this.modelMenuOpen = false
      this.$emit('open-model-config-dialog')
    },
    handleDocumentClick(e) {
      if (!this.modelMenuOpen) return
      const root = this.$refs.modelSelectRoot
      if (root && root.contains(e.target)) return
      this.modelMenuOpen = false
    }
  }
}
</script>

<style scoped>
.model-select-wrap {
  position: relative;
  display: flex;
  width: clamp(150px, 24vw, 220px);
  min-width: 0;
}

.model-select-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.45rem;
  width: 100%;
  min-height: 34px;
  border: 0;
  border-radius: 0.5rem;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font-size: 0.9rem;
  font-weight: 650;
  letter-spacing: 0;
  line-height: 1.25rem;
  outline: none;
  padding: 0.4rem 0.55rem 0.4rem 0.65rem;
  text-align: left;
  transition: background-color 180ms ease, box-shadow 180ms ease;
}

.model-select-dot {
  width: 0.45rem;
  height: 0.45rem;
  flex: 0 0 0.45rem;
  border-radius: 999px;
  background: #ff8c00;
  box-shadow: 0 0 0 3px rgba(255, 140, 0, 0.14);
}

.model-select-text {
  min-width: 0;
  flex: 1 1 auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-select-trigger:hover {
  background: rgba(0, 0, 0, 0.05);
}

.model-select-trigger:focus {
  box-shadow: 0 0 0 2px rgba(255, 140, 0, 0.2);
}

.model-select-trigger:disabled {
  cursor: not-allowed;
  opacity: 0.65;
}

.model-select-menu {
  position: absolute;
  left: 0;
  top: calc(100% + 0.5rem);
  z-index: 60;
  width: 280px;
  max-width: calc(100vw - 2rem);
  overflow: hidden;
  border: 1px solid #e7e0d8;
  border-radius: 0.75rem;
  background: #ffffff;
  box-shadow: 0 18px 50px rgba(25, 28, 29, 0.14);
}

.model-select-option {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  border: 0;
  background: transparent;
  color: #191c1d;
  cursor: pointer;
  padding: 0.75rem 0.875rem;
  text-align: left;
  font-size: 0.875rem;
}

.model-select-option span:first-child {
  min-width: 0;
}

.model-select-option:hover,
.model-select-option.is-active {
  background: #f3efea;
  color: #ff8c00;
}

.model-select-badge {
  flex-shrink: 0;
  border-radius: 999px;
  background: rgba(255, 140, 0, 0.12);
  color: #ff8c00;
  padding: 0.15rem 0.45rem;
  font-size: 0.68rem;
  font-weight: 700;
}

.model-select-empty {
  padding: 0.8rem 0.875rem;
  color: #74716d;
  font-size: 0.875rem;
}

.model-select-divider {
  height: 1px;
  background: #e7e0d8;
}

.model-select-settings {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 0.6rem;
  border: 0;
  background: transparent;
  color: #74716d;
  cursor: pointer;
  padding: 0.75rem 0.875rem;
  text-align: left;
  font-size: 0.875rem;
}

.model-select-settings:hover {
  background: #f3efea;
  color: #191c1d;
}

.dark .model-select-trigger {
  color: #fafafa;
}

.dark .model-select-trigger:hover {
  background: rgba(38, 38, 38, 0.8);
}

.dark .model-select-menu {
  border-color: #262626;
  background: rgba(23, 23, 23, 0.98);
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.35);
}

.dark .model-select-divider {
  background: #262626;
}

.dark .model-select-option {
  color: #e5e5e5;
}

.dark .model-select-option:hover,
.dark .model-select-option.is-active {
  background: rgba(38, 38, 38, 0.86);
  color: #ff8c00;
}

.dark .model-select-empty {
  color: #737373;
}

.dark .model-select-settings {
  color: #a3a3a3;
}

.dark .model-select-settings:hover {
  background: rgba(38, 38, 38, 0.86);
  color: #fafafa;
}
</style>
