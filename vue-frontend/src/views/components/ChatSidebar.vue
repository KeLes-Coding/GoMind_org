<template>
  <!-- Sidebar Overlay (mobile) -->
  <div
    v-if="!isSidebarCollapsed"
    class="fixed inset-0 bg-black/20 z-30 md:hidden"
    @click="$emit('toggle-sidebar')"
  ></div>

  <aside
    ref="sidebarAside"
    :class="[
      'flex flex-col flex-shrink-0 z-40 border-r shadow-sm',
      'bg-white dark:bg-neutral-900/50 dark:backdrop-blur-xl border-border-light dark:border-neutral-800',
    ]"
    :style="{ width: isSidebarCollapsed ? '68px' : '260px', transition: 'width 440ms cubic-bezier(0.22, 1, 0.36, 1)', overflow: 'hidden' }"
  >
    <div class="flex items-center gap-3 px-4 pt-5 pb-3 flex-shrink-0">
      <button
        class="p-2 rounded-full hover:bg-black/5 dark:hover:bg-neutral-800/80 transition-colors cursor-pointer bg-transparent border-none text-text-secondary-light dark:text-neutral-400"
        @click="$emit('toggle-sidebar')"
        :title="isSidebarCollapsed ? 'Expand sidebar' : 'Close sidebar'"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16"/></svg>
      </button>
    </div>

    <div class="px-3 py-2 flex-shrink-0">
      <button
        class="flex items-center rounded-full bg-accent-light hover:bg-accent-light/90 shadow-[0_4px_14px_rgba(255,140,0,0.25)] transition-all cursor-pointer text-sm font-semibold text-white border-none"
        :class="[
          'h-11 w-full justify-start px-3',
          isSidebarCollapsed ? 'gap-0' : 'gap-3'
        ]"
        @click="$emit('create-new-session')"
        title="New chat"
      >
        <span class="flex h-5 w-5 shrink-0 items-center justify-center">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4"/></svg>
        </span>
        <span class="sidebar-label whitespace-nowrap" :class="isSidebarCollapsed ? 'is-collapsed' : 'is-expanded'">New chat</span>
      </button>
    </div>

    <!-- Conversation History -->
    <div class="flex-1 overflow-y-auto pt-2 relative" :style="isSidebarCollapsed ? 'visibility: hidden' : ''">
      <div class="px-2">
        <div class="px-3 pb-2 flex items-center justify-between" style="transition: opacity 150ms ease-in-out">
          <span class="text-xs font-semibold text-text-secondary-light dark:text-neutral-500 tracking-wider whitespace-nowrap uppercase">Folders</span>
          <button
            class="p-1 rounded-md hover:bg-[#EDE8E1] dark:hover:bg-neutral-800/80 transition-colors cursor-pointer text-text-secondary-light dark:text-neutral-400 bg-transparent border-none"
            title="新建文件夹"
            @click="$emit('show-create-folder-dialog')"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4"/></svg>
          </button>
        </div>
        <ul class="list-none m-0 p-0 space-y-0.5">
          <!-- Folders -->
          <template v-if="true">
            <li v-for="folder in foldersList" :key="folder.id" class="mb-1">
              <div
                class="px-3 py-2.5 rounded-lg cursor-pointer text-sm transition-all group flex items-center text-text-secondary-light dark:text-neutral-400 hover:bg-[#EDE8E1] dark:hover:bg-neutral-800/80"
                @click="$emit('toggle-folder', folder.id)"
              >
                <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 mr-2 shrink-0 transition-transform" :class="collapsedFolders[folder.id] ? '-rotate-90' : ''" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" /></svg>
                <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 mr-2 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" /></svg>
                <span class="truncate flex-1 font-medium">{{ folder.name }}</span>
                <el-dropdown trigger="click" @command="(cmd) => $emit('handle-folder-command', { command: cmd, folder })" @click.stop class="ml-1 opacity-0 group-hover:opacity-100 transition-opacity">
                  <span class="p-1 rounded hover:bg-black/10 dark:hover:bg-neutral-700/80 text-text-secondary-light dark:text-neutral-400">
                    <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z" /></svg>
                  </span>
                  <template #dropdown>
                    <el-dropdown-menu>
                      <el-dropdown-item command="rename">重命名</el-dropdown-item>
                      <el-dropdown-item command="delete" class="text-red-500">删除文件夹</el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
              </div>
              <!-- Folder Sessions -->
              <ul v-show="!collapsedFolders[folder.id]" class="list-none m-0 p-0 pl-6 mt-0.5 space-y-0.5">
                <li
                  v-for="session in folder.sessions"
                  :key="session.sessionId"
                  :class="[
                    'px-3 py-2.5 rounded-lg cursor-pointer text-sm transition-all group flex items-center',
                    currentSessionId === session.sessionId
                      ? 'bg-accent-light/10 dark:bg-neutral-800/80 font-semibold text-accent-light dark:text-orange-500 shadow-sm'
                      : 'text-text-secondary-light dark:text-neutral-400 hover:bg-[#EDE8E1] dark:hover:bg-neutral-800/80'
                  ]"
                  @click="$emit('switch-session', session.sessionId)"
                >
                  <span v-if="currentSessionId === session.sessionId" class="w-1.5 h-1.5 rounded-full bg-accent-light dark:bg-accent-dark mr-2 shrink-0"></span>
                  <span class="truncate flex-1">{{ session.name || `会话 ${session.sessionId}` }}</span>
                  <el-dropdown trigger="click" @command="(cmd) => $emit('handle-session-command', { command: cmd, session })" @click.stop class="ml-1 opacity-0 group-hover:opacity-100 transition-opacity">
                    <span class="p-1 rounded hover:bg-black/10 dark:hover:bg-neutral-700/80 text-text-secondary-light dark:text-neutral-400">
                      <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z" /></svg>
                    </span>
                    <template #dropdown>
                      <el-dropdown-menu>
                        <el-dropdown-item command="rename">重命名</el-dropdown-item>
                        <el-dropdown-item command="move">移动到...</el-dropdown-item>
                        <el-dropdown-item command="removeFromFolder">移出文件夹</el-dropdown-item>
                        <el-dropdown-item command="delete" class="text-red-500">删除</el-dropdown-item>
                      </el-dropdown-menu>
                    </template>
                  </el-dropdown>
                </li>
              </ul>
            </li>
          </template>

          <!-- Ungrouped Sessions -->
          <li
            v-for="session in ungroupedSessionsList"
            :key="session.sessionId"
            :class="[
              'px-3 py-2.5 rounded-lg cursor-pointer text-sm transition-all group flex items-center',
              currentSessionId === session.sessionId
                ? 'bg-accent-light/10 dark:bg-neutral-800/80 font-semibold text-accent-light dark:text-orange-500 shadow-sm'
                : 'text-text-secondary-light dark:text-neutral-400 hover:bg-[#EDE8E1] dark:hover:bg-neutral-800/80'
            ]"
            @click="$emit('switch-session', session.sessionId)"
          >
            <span v-if="currentSessionId === session.sessionId" class="w-1.5 h-1.5 rounded-full bg-accent-light dark:bg-accent-dark mr-2 shrink-0"></span>
            <span class="truncate flex-1">{{ session.name || `会话 ${session.sessionId}` }}</span>
            <el-dropdown trigger="click" @command="(cmd) => $emit('handle-session-command', { command: cmd, session })" @click.stop class="ml-1 opacity-0 group-hover:opacity-100 transition-opacity">
              <span class="p-1 rounded hover:bg-black/10 dark:hover:bg-neutral-700/80 text-text-secondary-light dark:text-neutral-400">
                <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z" /></svg>
              </span>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="rename">重命名</el-dropdown-item>
                  <el-dropdown-item command="move">移动到...</el-dropdown-item>
                  <el-dropdown-item command="delete" class="text-red-500">删除</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </li>
        </ul>
      </div>
    </div>

    <div class="px-3 pb-4 pt-3 space-y-1 flex-shrink-0 border-t border-border-light dark:border-neutral-800">
      <div class="relative">
        <button
          class="user-menu-trigger"
          :class="[
            'flex h-[52px] items-center rounded-lg hover:bg-[#EDE8E1] dark:hover:bg-neutral-800/80 transition-colors cursor-pointer bg-transparent border-none',
            isSidebarCollapsed ? 'w-full justify-start px-1' : 'w-full gap-3 px-1 text-left'
          ]"
          @click.stop="isLoggedIn ? $emit('toggle-user-menu') : $emit('go-login')"
        >
          <img
            v-if="isLoggedIn && userProfile.avatar_url"
            :src="userProfile.avatar_url"
            alt="User avatar"
            class="w-9 h-9 min-w-[2.25rem] rounded-full object-cover shrink-0 border border-border-light dark:border-neutral-700"
          />
          <div
            v-else
            class="w-9 h-9 min-w-[2.25rem] rounded-full flex items-center justify-center text-xs font-bold shrink-0 select-none"
            :class="isLoggedIn ? 'bg-accent-light text-white' : 'bg-black/10 dark:bg-neutral-800 text-text-secondary-light dark:text-neutral-400'"
          >
            {{ isLoggedIn ? getUserInitial : '?' }}
          </div>
          <div class="sidebar-label min-w-0 flex-1 whitespace-nowrap" :class="isSidebarCollapsed ? 'is-collapsed' : 'is-expanded'">
            <div class="text-sm truncate" :class="isLoggedIn ? 'text-text-primary-light dark:text-neutral-100' : 'text-text-secondary-light dark:text-neutral-400'">{{ isLoggedIn ? getUserDisplayName : '未登录' }}</div>
            <div v-if="isLoggedIn" class="text-xs truncate text-text-secondary-light dark:text-neutral-500">@{{ userProfile.username || 'user' }}</div>
            <div v-else class="text-xs truncate text-accent-light dark:text-orange-500">点击登录</div>
          </div>
          <svg v-if="isLoggedIn" :class="isSidebarCollapsed ? 'opacity-0 max-w-0' : 'opacity-100 max-w-4'" xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 shrink-0 text-text-secondary-light dark:text-neutral-400 transition-all duration-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7"/></svg>
        </button>
      </div>
    </div>
  </aside>

  <!-- User Menu Dropdown (rendered outside sidebar to avoid overflow clipping) -->
  <div
    v-if="userMenuVisible && isLoggedIn"
    ref="userMenuRef"
    class="fixed z-50 rounded-xl border border-border-light dark:border-neutral-800 bg-surface-light dark:bg-neutral-900 shadow-xl overflow-hidden"
    :style="userMenuStyle"
  >
    <button type="button" class="w-full px-4 py-3 text-left text-sm hover:bg-[#EDE8E1] dark:hover:bg-neutral-800 text-text-primary-light dark:text-neutral-100 bg-transparent border-none cursor-pointer" @click="$emit('handle-settings')">Settings</button>
    <button type="button" class="w-full px-4 py-3 text-left text-sm hover:bg-[#EDE8E1] dark:hover:bg-neutral-800 text-text-primary-light dark:text-neutral-100 bg-transparent border-none cursor-pointer" @click="$emit('open-model-config-dialog')">Model Configs</button>
    <button type="button" class="w-full px-4 py-3 text-left text-sm hover:bg-[#EDE8E1] dark:hover:bg-neutral-800 text-text-primary-light dark:text-neutral-100 bg-transparent border-none cursor-pointer" @click="$emit('open-file-manager-dialog')">File Management</button>
    <button type="button" class="w-full px-4 py-3 text-left text-sm hover:bg-[#EDE8E1] dark:hover:bg-neutral-800 text-text-primary-light dark:text-neutral-100 bg-transparent border-none cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed" :disabled="!currentSessionId || tempSession || loading" @click="$emit('handle-sync-history')">Sync history</button>
    <button type="button" class="w-full px-4 py-3 text-left text-sm hover:bg-red-50 dark:hover:bg-red-900/20 text-red-500 bg-transparent border-none cursor-pointer" @click="$emit('handle-logout')">Log out</button>
  </div>
</template>

<script>
/* eslint-disable vue/no-mutating-props */
/* eslint-env node */

export default {
  name: 'ChatSidebar',
  props: {
    isSidebarCollapsed: {
      type: Boolean,
      required: true
    },
    userMenuVisible: {
      type: Boolean,
      required: true
    },
    foldersList: {
      type: Array,
      required: true
    },
    collapsedFolders: {
      type: Object,
      required: true
    },
    ungroupedSessionsList: {
      type: Array,
      required: true
    },
    currentSessionId: {
      type: [String, Number],
      required: true
    },
    userProfile: {
      type: Object,
      required: true
    },
    loading: {
      type: Boolean,
      default: false
    },
    tempSession: {
      type: [String, Number, null],
      default: null
    }
  },
  emits: [
    'toggle-sidebar',
    'toggle-user-menu',
    'create-new-session',
    'toggle-folder',
    'handle-folder-command',
    'switch-session',
    'handle-session-command',
    'show-create-folder-dialog',
    'handle-settings',
    'open-model-config-dialog',
    'open-file-manager-dialog',
    'handle-sync-history',
    'handle-logout',
    'go-login'
  ],
  data() {
    return {
      userMenuBottom: 0,
      userMenuLeft: 0,
      userMenuWidth: 0
    }
  },
  computed: {
    isLoggedIn() {
      return Boolean(this.userProfile && (this.userProfile.username || this.userProfile.id))
    },
    getUserInitial() {
      const profile = this.userProfile
      if (!profile) return '?'
      const name = profile.name || profile.username || 'U'
      return name.charAt(0).toUpperCase()
    },
    getUserDisplayName() {
      const profile = this.userProfile
      if (!profile) return 'User'
      return profile.name || profile.username || 'User'
    },
    userMenuStyle() {
      return {
        bottom: `${window.innerHeight - this.userMenuBottom}px`,
        left: `${this.userMenuLeft}px`,
        width: `${this.userMenuWidth}px`
      }
    }
  },
  watch: {
    userMenuVisible(visible) {
      if (visible) {
        this.$nextTick(() => this.positionUserMenu())
      }
    },
    isSidebarCollapsed() {
      if (this.userMenuVisible) {
        this.$nextTick(() => this.positionUserMenu())
      }
    }
  },
  mounted() {
    document.addEventListener('click', this.handleOutsideClick)
    window.addEventListener('resize', this.positionUserMenu)
  },
  beforeUnmount() {
    document.removeEventListener('click', this.handleOutsideClick)
    window.removeEventListener('resize', this.positionUserMenu)
  },
  methods: {
    positionUserMenu() {
      const sidebar = this.$refs.sidebarAside
      const trigger = sidebar?.querySelector('.user-menu-trigger')
      if (!trigger) return
      const rect = trigger.getBoundingClientRect()
      this.userMenuBottom = rect.bottom
      this.userMenuLeft = rect.left
      this.userMenuWidth = this.isSidebarCollapsed ? 180 : rect.width
    },
    handleOutsideClick(e) {
      if (!this.userMenuVisible) return
      const menu = this.$refs.userMenuRef
      if (menu && menu.contains(e.target)) return
      const sidebar = this.$refs.sidebarAside
      const trigger = sidebar?.querySelector('.user-menu-trigger')
      if (trigger && trigger.contains(e.target)) return
      this.$emit('toggle-user-menu')
    }
  }
}
</script>

<style scoped>
aside ::-webkit-scrollbar {
  width: 4px;
}
aside ::-webkit-scrollbar-track {
  background: transparent;
}
aside ::-webkit-scrollbar-thumb {
  background: rgba(0, 0, 0, 0.1);
  border-radius: 999px;
}
.dark aside ::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.1);
}

.sidebar-label {
  max-width: 180px;
  opacity: 1;
  overflow: hidden;
  transition:
    opacity 220ms ease 130ms,
    max-width 360ms cubic-bezier(0.22, 1, 0.36, 1) 60ms;
}

.sidebar-label.is-collapsed {
  max-width: 0;
  opacity: 0;
  transition:
    opacity 160ms ease,
    max-width 260ms cubic-bezier(0.22, 1, 0.36, 1) 120ms;
}

.sidebar-label.is-expanded {
  max-width: 180px;
  opacity: 1;
}
</style>
