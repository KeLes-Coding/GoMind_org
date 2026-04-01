<template>
  <div class="flex flex-row h-screen w-screen overflow-hidden bg-bg-light dark:bg-bg-dark text-text-primary-light dark:text-text-primary-dark">

    <!-- Sidebar Overlay (mobile) -->
    <div
      v-if="!isSidebarCollapsed"
      class="fixed inset-0 bg-black/20 z-30 md:hidden"
      @click="toggleSidebar"
    ></div>

    <!-- Gemini-Style Sidebar -->
    <aside
      :class="[
        'flex flex-col flex-shrink-0 transition-all duration-300 z-40',
        'bg-bg-light dark:bg-[#171717]',
        isSidebarCollapsed ? 'w-0 overflow-hidden' : 'w-[260px] overflow-hidden'
      ]"
    >
      <div class="flex flex-col h-full min-w-[260px]">
        <!-- Top: Sidebar Toggle + New Chat -->
        <div class="flex items-center gap-2 px-3 pt-3 pb-1">
          <button
            class="p-2 rounded-full hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer bg-transparent"
            @click="toggleSidebar"
            title="关闭侧边栏"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 text-text-secondary-light dark:text-text-secondary-dark" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16"/></svg>
          </button>
        </div>

        <!-- New Chat Button (Gemini pill style) -->
        <div class="px-3 py-2">
          <button
            class="flex items-center gap-3 px-5 py-3 rounded-full bg-surface-light dark:bg-[#282828] hover:bg-black/5 dark:hover:bg-white/5 shadow-sm hover:shadow-md transition-all cursor-pointer text-sm font-medium text-text-primary-light dark:text-text-primary-dark border-none w-auto"
            @click="createNewSession"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4"/></svg>
            <span class="whitespace-nowrap">新建对话</span>
          </button>
        </div>

        <!-- Conversation History -->
        <div class="flex-1 overflow-y-auto px-2 pt-2">
          <div class="px-3 pb-2">
            <span class="text-xs font-medium text-text-secondary-light dark:text-text-secondary-dark tracking-wider">近期</span>
          </div>
          <ul class="list-none m-0 p-0 space-y-0.5">
            <li
              v-for="session in sessions"
              :key="session.id"
              :class="[
                'px-3 py-2.5 rounded-xl cursor-pointer text-sm transition-all group flex items-center',
                currentSessionId === session.id
                  ? 'bg-black/5 dark:bg-white/8 font-medium text-text-primary-light dark:text-text-primary-dark'
                  : 'text-text-secondary-light dark:text-text-secondary-dark hover:bg-black/5 dark:hover:bg-white/5'
              ]"
              @click="switchSession(session.id)"
            >
              <span class="truncate flex-1">{{ session.name || `会话 ${session.id}` }}</span>
            </li>
          </ul>
        </div>

        <!-- Bottom Actions (Gemini-style seamless) -->
        <div class="px-2 pb-3 pt-2 space-y-0.5">
          <button
            v-if="false"
            @click="syncHistory"
            :disabled="!currentSessionId || tempSession || loading"
            class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm text-text-secondary-light dark:text-text-secondary-dark hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer bg-transparent border-none disabled:opacity-30 disabled:cursor-not-allowed"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-[18px] w-[18px] shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/></svg>
            <span>同步历史</span>
          </button>
          <button
            v-if="false"
            @click="handleSettings"
            class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm text-text-secondary-light dark:text-text-secondary-dark hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer bg-transparent border-none"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-[18px] w-[18px] shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
            <span>设置</span>
          </button>

          <div v-if="false" class="flex items-center gap-3 px-3 py-2.5 rounded-xl">
            <img
              v-if="userProfile.avatar_url"
              :src="userProfile.avatar_url"
              alt="用户头像"
              class="w-8 h-8 rounded-full object-cover shrink-0 border border-border-light dark:border-border-dark"
            />
            <div
              v-else
              class="w-8 h-8 rounded-full bg-gradient-to-br from-accent-light to-orange-400 flex items-center justify-center text-white text-xs font-bold shrink-0 select-none"
            >
              {{ getUserInitial() }}
            </div>
            <div class="min-w-0 flex-1">
              <div class="text-sm truncate text-text-primary-light dark:text-text-primary-dark">{{ getUserDisplayName() }}</div>
              <div class="text-xs truncate text-text-secondary-light dark:text-text-secondary-dark">@{{ userProfile.username || 'user' }}</div>
            </div>
          </div>

          <div class="relative">
            <button
              type="button"
              class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer bg-transparent border-none text-left"
              @click.stop="toggleUserMenu"
            >
              <img
                v-if="userProfile.avatar_url"
                :src="userProfile.avatar_url"
                alt="用户头像"
                class="w-8 h-8 rounded-full object-cover shrink-0 border border-border-light dark:border-border-dark"
              />
              <div
                v-else
                class="w-8 h-8 rounded-full bg-gradient-to-br from-accent-light to-orange-400 flex items-center justify-center text-white text-xs font-bold shrink-0 select-none"
              >
                {{ getUserInitial() }}
              </div>
              <div class="min-w-0 flex-1">
                <div class="text-sm truncate text-text-primary-light dark:text-text-primary-dark">{{ getUserDisplayName() }}</div>
                <div class="text-xs truncate text-text-secondary-light dark:text-text-secondary-dark">@{{ userProfile.username || 'user' }}</div>
              </div>
              <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 shrink-0 text-text-secondary-light dark:text-text-secondary-dark" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7"/></svg>
            </button>

            <div
              v-if="userMenuVisible"
              class="absolute bottom-full left-2 right-2 mb-2 rounded-2xl border border-border-light dark:border-border-dark bg-surface-light dark:bg-surface-dark shadow-xl overflow-hidden"
            >
              <button type="button" class="w-full px-4 py-3 text-left text-sm hover:bg-black/5 dark:hover:bg-white/5 bg-transparent border-none cursor-pointer" @click="handleSettings">设置</button>
              <button type="button" class="w-full px-4 py-3 text-left text-sm hover:bg-black/5 dark:hover:bg-white/5 bg-transparent border-none cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed" :disabled="!currentSessionId || tempSession || loading" @click="handleSyncHistoryFromMenu">同步历史</button>
              <button type="button" class="w-full px-4 py-3 text-left text-sm hover:bg-black/5 dark:hover:bg-white/5 bg-transparent border-none cursor-pointer" @click="handleLogout">退出登录</button>
            </div>
          </div>

          <!-- User Avatar Row -->
          <div v-if="false" class="flex items-center gap-3 px-3 py-2.5 rounded-xl hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer" @click="handleLogout">
            <div class="w-7 h-7 rounded-full bg-gradient-to-br from-accent-light to-orange-400 flex items-center justify-center text-white text-xs font-bold shrink-0 select-none">
              U
            </div>
            <span class="text-sm text-text-secondary-light dark:text-text-secondary-dark">退出登录</span>
          </div>
        </div>
      </div>
    </aside>

    <!-- Main Content -->
    <section class="flex-1 flex flex-col relative min-w-0 bg-bg-light dark:bg-bg-dark">
      <!-- Header -->
      <div class="sticky top-0 z-10 px-4 py-3 flex items-center gap-3">
        <!-- Sidebar toggle (visible when collapsed) -->
        <button
          v-if="isSidebarCollapsed"
          class="p-2 rounded-full hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer bg-transparent"
          @click="toggleSidebar"
          title="展开侧边栏"
        >
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 text-text-secondary-light dark:text-text-secondary-dark" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16"/></svg>
        </button>

        <!-- New chat button (Gemini collapsed state: icon-only pill) -->
        <button
          v-if="isSidebarCollapsed"
          class="p-2 rounded-full hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer bg-transparent"
          @click="createNewSession"
          title="新建对话"
        >
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 text-text-secondary-light dark:text-text-secondary-dark" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"/></svg>
        </button>

        <div class="flex items-center gap-3 ml-auto">
          <label for="modelType" class="text-sm text-text-secondary-light dark:text-text-secondary-dark hidden sm:block">模型</label>
          <select id="modelType" v-model="selectedModel" class="px-2 py-1.5 text-sm rounded-lg border border-border-light dark:border-border-dark bg-transparent cursor-pointer outline-none focus:ring-1 focus:ring-accent-light dark:focus:ring-accent-dark disabled:opacity-50" :disabled="loading">
            <option v-for="option in modelOptions" :key="option.value" :value="option.value" class="bg-surface-light dark:bg-surface-dark">
              {{ option.label }}
            </option>
          </select>

          <label class="flex items-center gap-1.5 text-sm cursor-pointer select-none">
            <input id="streamingMode" v-model="isStreaming" type="checkbox" class="accent-accent-light dark:accent-accent-dark" :disabled="loading" />
            <span class="hidden sm:inline">流式</span>
          </label>

          <button @click="toggleTheme" class="p-2 rounded-full hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer border-none bg-transparent text-text-primary-light dark:text-text-primary-dark flex items-center justify-center shrink-0">
            <svg v-if="isDark" xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"/></svg>
            <svg v-else xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"/></svg>
          </button>
        </div>
      </div>

      <!-- Messages Stream -->
      <div class="flex-1 overflow-y-auto px-8 md:px-24 pt-8 pb-44" ref="messagesRef">
        <div class="max-w-4xl mx-auto flex flex-col gap-12">
          <div
            v-for="(message, index) in currentMessages"
            :key="index"
            class="flex flex-col gap-2 group"
          >
            <!-- Sender Header -->
            <div class="flex items-center gap-3">
              <img
                v-if="message.role === 'user' && userProfile.avatar_url"
                :src="userProfile.avatar_url"
                alt="用户头像"
                class="w-8 h-8 rounded-full object-cover border border-border-light dark:border-border-dark"
              />
              <div v-else :class="['w-8 h-8 rounded-full flex items-center justify-center font-bold text-sm select-none', message.role === 'user' ? 'bg-black text-white dark:bg-white dark:text-black' : 'bg-surface-light border border-border-light dark:bg-surface-dark dark:border-border-dark']">
                {{ message.role === 'user' ? getUserInitial() : 'AI' }}
              </div>
              <span class="font-semibold text-sm">{{ message.role === 'user' ? '你' : 'AI' }}</span>
              <!-- Actions & Meta -->
              <div class="flex items-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                <button
                  v-if="message.role === 'assistant' && message.content"
                  class="px-2 py-0.5 text-xs rounded bg-surface-light dark:bg-surface-dark border border-border-light dark:border-border-dark hover:text-accent-light dark:hover:text-accent-dark cursor-pointer transition-colors"
                  @click="playTTS(message.content)"
                >
                  朗读
                </button>
              </div>
              <span v-if="getMessageMetaStatus(message)" class="text-xs text-text-secondary-light dark:text-text-secondary-dark ml-auto">
                {{ getMessageStatusLabel(getMessageMetaStatus(message)) }}
              </span>
            </div>

            <!-- Message Content block -->
            <div class="pl-11 text-base leading-relaxed space-y-4 break-words">
              <!-- Image Preview (for image recognition messages) -->
              <img v-if="message.imageUrl" :src="message.imageUrl" alt="上传的图片" class="max-w-xs rounded-xl shadow-md mt-1 mb-2" />
              <div v-html="renderMarkdown(message.content)" class="prose dark:prose-invert prose-p:my-2 prose-pre:bg-surface-light dark:prose-pre:bg-surface-dark prose-pre:border prose-pre:border-border-light dark:prose-pre:border-border-dark prose-pre:shadow-[0_2px_10px_rgba(0,0,0,0.02)] max-w-none"></div>
            </div>
          </div>
        </div>
      </div>

      <!-- Floating Pill Input -->
      <div class="absolute bottom-6 left-1/2 -translate-x-1/2 w-full max-w-3xl px-4 z-20">
        <div class="bg-surface-light dark:bg-surface-dark rounded-2xl shadow-[0_8px_30px_rgba(0,0,0,0.06)] dark:shadow-2xl ring-1 ring-black/5 dark:ring-white/10 flex flex-col p-2 transition-shadow focus-within:ring-2 focus-within:ring-accent-light/30 dark:focus-within:ring-accent-dark/30">
          <!-- Toolbar -->
          <div class="flex items-center gap-0.5 px-2 pt-1">
            <button @click="triggerFileUpload" :disabled="uploading || loading" class="p-1.5 rounded-lg text-text-secondary-light dark:text-text-secondary-dark hover:bg-black/5 dark:hover:bg-white/5 hover:text-text-primary-light dark:hover:text-text-primary-dark transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed bg-transparent border-none" title="上传文档 (.md/.txt)">
              <svg xmlns="http://www.w3.org/2000/svg" class="h-[18px] w-[18px]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M15.172 7l-6.586 6.586a2 2 0 102.828 2.828l6.414-6.586a4 4 0 00-5.656-5.656l-6.415 6.585a6 6 0 108.486 8.486L20.5 13"/></svg>
            </button>
            <button @click="triggerImageUpload" :disabled="loading" class="p-1.5 rounded-lg text-text-secondary-light dark:text-text-secondary-dark hover:bg-black/5 dark:hover:bg-white/5 hover:text-text-primary-light dark:hover:text-text-primary-dark transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed bg-transparent border-none" title="图像识别">
              <svg xmlns="http://www.w3.org/2000/svg" class="h-[18px] w-[18px]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
            </button>
            <input
              ref="fileInput"
              type="file"
              accept=".md,.txt,text/markdown,text/plain"
              class="hidden"
              @change="handleFileUpload"
            />
            <input
              ref="imageInput"
              type="file"
              accept="image/*"
              class="hidden"
              @change="handleImageRecognition"
            />
          </div>

          <div class="flex items-end">
            <textarea
              v-model="inputMessage"
              placeholder="问点什么..."
              @keydown.enter.exact.prevent="sendMessage"
              :disabled="loading"
              ref="messageInput"
              rows="1"
              class="flex-1 max-h-40 min-h-[44px] bg-transparent border-none outline-none resize-none px-4 py-2 text-base text-text-primary-light dark:text-text-primary-dark placeholder-text-secondary-light dark:placeholder-text-secondary-dark"
            ></textarea>

            <!-- Stop Button when streaming and loading -->
            <button
               v-if="isStreaming && loading"
               type="button"
               @click="stopCurrentStream"
               class="p-2 w-10 h-10 mb-1 mr-1 rounded-xl flex items-center justify-center transition-all bg-red-500/10 text-red-500 hover:bg-red-500/20 cursor-pointer shadow-sm border-none"
               title="停止生成"
            >
              <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="currentColor" viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="2"/></svg>
            </button>

            <!-- Default Send / Loading Button -->
            <button
              v-else
              type="button"
              :disabled="!inputMessage.trim() || loading"
              @click="sendMessage"
              :class="[
                'p-2 w-10 h-10 mb-1 mr-1 rounded-xl flex items-center justify-center transition-all disabled:cursor-not-allowed border-none',
                (!inputMessage.trim() || loading)
                  ? 'bg-transparent text-text-secondary-light dark:text-text-secondary-dark opacity-50'
                  : 'bg-black text-white dark:bg-white dark:text-black shadow-sm'
              ]"
            >
              <svg v-if="!loading" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" class="w-5 h-5">
                <path d="M3.478 2.404a.75.75 0 00-.926.941l2.432 7.905H13.5a.75.75 0 010 1.5H4.984l-2.432 7.905a.75.75 0 00.926.94 60.519 60.519 0 0018.445-8.986.75.75 0 000-1.218A60.517 60.517 0 003.478 2.404z" />
              </svg>
              <svg v-else class="animate-spin w-5 h-5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            </button>
          </div>
        </div>
      </div>
      <el-dialog v-model="settingsVisible" title="个人设置" width="520px">
        <div class="space-y-4">
          <div class="flex items-center gap-4">
            <img
              v-if="userProfile.avatar_url"
              :src="userProfile.avatar_url"
              alt="用户头像"
              class="w-16 h-16 rounded-full object-cover border border-border-light dark:border-border-dark"
            />
            <div
              v-else
              class="w-16 h-16 rounded-full bg-gradient-to-br from-accent-light to-orange-400 flex items-center justify-center text-white text-lg font-bold select-none"
            >
              {{ getUserInitial() }}
            </div>
            <div class="flex flex-col gap-2">
              <button
                type="button"
                class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer"
                @click="triggerAvatarUpload"
                :disabled="uploadingAvatar"
              >
                {{ uploadingAvatar ? '上传中...' : '上传头像' }}
              </button>
              <span class="text-xs text-text-secondary-light dark:text-text-secondary-dark">支持 JPG、PNG、WEBP，大小不超过 2MB</span>
            </div>
            <input
              ref="avatarInput"
              type="file"
              accept=".jpg,.jpeg,.png,.webp,image/jpeg,image/png,image/webp"
              class="hidden"
              @change="handleAvatarUpload"
            />
          </div>

          <div class="space-y-2">
            <label class="block text-sm">昵称</label>
            <input v-model="profileForm.name" type="text" maxlength="50" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none" />
          </div>

          <div class="space-y-2">
            <label class="block text-sm">用户名</label>
            <input :value="userProfile.username || ''" type="text" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-black/5 dark:bg-white/5 outline-none" disabled />
          </div>

          <div class="space-y-2">
            <label class="block text-sm">邮箱</label>
            <input :value="userProfile.email || ''" type="text" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-black/5 dark:bg-white/5 outline-none" disabled />
          </div>

          <div class="space-y-2">
            <label class="block text-sm">简介</label>
            <textarea v-model="profileForm.bio" rows="4" maxlength="255" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none resize-none"></textarea>
          </div>
        </div>
        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="settingsVisible = false">取消</button>
            <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="saveProfile" :disabled="savingProfile">{{ savingProfile ? '保存中...' : '保存' }}</button>
          </div>
        </template>
      </el-dialog>
      <el-dialog v-model="cropDialogVisible" title="裁剪头像" width="560px">
        <div class="space-y-4">
          <div class="flex justify-center">
            <div class="relative flex items-center justify-center w-72 h-72 overflow-hidden rounded-2xl border border-border-light dark:border-border-dark bg-black/5 dark:bg-white/5">
              <img
                v-if="cropPreviewUrl"
                :src="cropPreviewUrl"
                alt="裁剪预览"
                class="max-w-none select-none"
                :style="cropImageStyle"
              />
            </div>
          </div>
          <div class="space-y-3">
            <div>
              <label class="block text-sm mb-2">缩放</label>
              <input v-model="cropScale" type="range" min="1" max="3" step="0.01" class="w-full" />
            </div>
            <div>
              <label class="block text-sm mb-2">左右调整</label>
              <input v-model="cropOffsetX" type="range" min="-120" max="120" step="1" class="w-full" />
            </div>
            <div>
              <label class="block text-sm mb-2">上下调整</label>
              <input v-model="cropOffsetY" type="range" min="-120" max="120" step="1" class="w-full" />
            </div>
          </div>
          <p class="text-xs text-text-secondary-light dark:text-text-secondary-dark">当前裁剪比例固定为 1:1，适合作为头像展示。</p>
        </div>
        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="cancelAvatarCrop">取消</button>
            <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="confirmAvatarCrop" :disabled="uploadingAvatar">{{ uploadingAvatar ? '上传中...' : '确认并上传' }}</button>
          </div>
        </template>
      </el-dialog>
    </section>
  </div>
</template>

<script>
import { computed, nextTick, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import api, { refreshClient } from '../utils/api'
import { ensureAccessToken, clearTokens } from '../utils/token'

const TERMINAL_STATUSES = new Set(['completed', 'cancelled', 'timeout', 'failed', 'partial'])
const MODEL_OPTIONS = [
  { value: '1', label: 'DeepSeek' },
  { value: '2', label: 'DeepSeek RAG' },
  { value: '3', label: 'DeepSeek MCP' }
]

export default {
  name: 'AIChat',
  setup() {
    const router = useRouter()
    const isSidebarCollapsed = ref(false)
    const isDark = ref(false)
    const sessions = ref({})
    const currentSessionId = ref(null)
    const tempSession = ref(false)
    const currentMessages = ref([])
    const inputMessage = ref('')
    const loading = ref(false)
    const messagesRef = ref(null)
    const messageInput = ref(null)
    const selectedModel = ref('1')
    const isStreaming = ref(true)
    const uploading = ref(false)
    const fileInput = ref(null)
    const imageInput = ref(null)
    const avatarInput = ref(null)
    const settingsVisible = ref(false)
    const userMenuVisible = ref(false)
    const savingProfile = ref(false)
    const uploadingAvatar = ref(false)
    const cropDialogVisible = ref(false)
    const cropPreviewUrl = ref('')
    const cropScale = ref(1)
    const cropOffsetX = ref(0)
    const cropOffsetY = ref(0)
    const cropImageNaturalWidth = ref(0)
    const cropImageNaturalHeight = ref(0)
    const pendingAvatarFile = ref(null)
    const userProfile = ref({
      id: null,
      name: '',
      username: '',
      email: '',
      avatar_url: '',
      bio: ''
    })
    const profileForm = ref({
      name: '',
      bio: ''
    })
    const modelOptions = MODEL_OPTIONS

    // 用于中断当前请求，保证停止按钮和异常处理共用同一个 controller。
    const activeAbortController = ref(null)
    // 记录当前流式响应对应的会话 ID，新会话开始时会先使用 temp。
    const activeStreamingSessionId = ref(null)
    // 指向当前 assistant 消息，便于更新停止、超时和失败状态。
    const activeAssistantIndex = ref(-1)
    // 区分用户手动停止与请求异常中断。
    const manualStopRequested = ref(false)

    const renderMarkdown = (text) => {
      if (!text && text !== '') return ''
      return String(text)
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.*?)\*/g, '<em>$1</em>')
        .replace(/`(.*?)`/g, '<code class="bg-black/5 dark:bg-white/10 px-1 py-0.5 rounded text-sm font-mono">$1</code>')
        .replace(/\n/g, '<br>')
    }

    // 统一后端返回和前端临时消息的状态值。
    const normalizeMessageStatus = (status) => {
      if (!status) return 'completed'
      const normalized = String(status).toLowerCase()
      return TERMINAL_STATUSES.has(normalized) || normalized === 'streaming' ? normalized : 'completed'
    }

    const buildMessageMeta = (status) => ({ status: normalizeMessageStatus(status) })

    const buildSessionTitle = (question) => {
      const title = String(question || '').trim()
      return title || '新会话'
    }

    const mapHistoryItemToMessage = (item) => ({
      role: item.is_user ? 'user' : 'assistant',
      content: item.content || '',
      meta: buildMessageMeta(item.status)
    })

    const getMessageStatusLabel = (status) => {
      switch (normalizeMessageStatus(status)) {
      case 'streaming':
        return '生成中'
      case 'cancelled':
        return '已停止'
      case 'timeout':
        return '已超时'
      case 'failed':
        return '失败'
      case 'partial':
        return '部分完成'
      default:
        return ''
      }
    }

    const getMessageMetaStatus = (message) => {
      if (!message || !message.meta) return ''
      return message.meta.status || ''
    }

    const scrollToBottom = () => {
      if (messagesRef.value) {
        try {
          messagesRef.value.scrollTop = messagesRef.value.scrollHeight
        } catch (error) {
          console.error('Scroll error:', error)
        }
      }
    }

    // 确保会话对象一定存在，避免新会话切换或异步返回时访问空对象。
    const ensureSessionEntry = (sessionId) => {
      const normalizedId = String(sessionId || '')
      if (!normalizedId || normalizedId === 'temp') {
        return null
      }
      if (!sessions.value[normalizedId]) {
        sessions.value[normalizedId] = {
          id: normalizedId,
          name: `会话 ${normalizedId}`,
          messages: []
        }
      } else if (!Array.isArray(sessions.value[normalizedId].messages)) {
        sessions.value[normalizedId].messages = []
      }
      return sessions.value[normalizedId]
    }

    const upsertSessionEntry = (sessionData, options = {}) => {
      const normalizedId = String(sessionData?.id || '')
      if (!normalizedId || normalizedId === 'temp') {
        return null
      }

      const existing = sessions.value[normalizedId] || {
        id: normalizedId,
        name: `会话 ${normalizedId}`,
        messages: []
      }
      const nextEntry = {
        ...existing,
        ...sessionData,
        id: normalizedId,
        messages: Array.isArray(sessionData?.messages)
          ? sessionData.messages
          : (Array.isArray(existing.messages) ? existing.messages : [])
      }

      const nextSessions = {}
      const shouldPrepend = options.prepend !== false
      if (shouldPrepend) {
        nextSessions[normalizedId] = nextEntry
      }

      Object.entries(sessions.value).forEach(([key, value]) => {
        if (key !== normalizedId) {
          nextSessions[key] = value
        }
      })

      if (!shouldPrepend) {
        nextSessions[normalizedId] = nextEntry
      }

      sessions.value = nextSessions
      return nextEntry
    }

    const CROP_PREVIEW_SIZE = 288
    const CROP_OUTPUT_SIZE = 512

    const cropImageStyle = computed(() => {
      if (!cropImageNaturalWidth.value || !cropImageNaturalHeight.value) {
        return {
          transform: `translate(${cropOffsetX.value}px, ${cropOffsetY.value}px) scale(${cropScale.value})`,
          transformOrigin: 'center center'
        }
      }

      const baseScale = Math.max(
        CROP_PREVIEW_SIZE / cropImageNaturalWidth.value,
        CROP_PREVIEW_SIZE / cropImageNaturalHeight.value
      )

      return {
        width: `${cropImageNaturalWidth.value * baseScale}px`,
        height: `${cropImageNaturalHeight.value * baseScale}px`,
        transform: `translate(${cropOffsetX.value}px, ${cropOffsetY.value}px) scale(${cropScale.value})`,
        transformOrigin: 'center center'
      }
    })

    const syncSessionMessagesFromCurrent = async () => {
      if (!tempSession.value && currentSessionId.value && sessions.value[currentSessionId.value]) {
        sessions.value[currentSessionId.value].messages = [...currentMessages.value]
      }
      await nextTick()
      scrollToBottom()
    }

    const setAssistantStatus = async (status) => {
      if (activeAssistantIndex.value < 0 || !currentMessages.value[activeAssistantIndex.value]) {
        return
      }
      currentMessages.value[activeAssistantIndex.value].meta = buildMessageMeta(status)
      currentMessages.value = [...currentMessages.value]
      await syncSessionMessagesFromCurrent()
    }

    const appendAssistantChunk = async (chunk) => {
      if (activeAssistantIndex.value < 0 || !currentMessages.value[activeAssistantIndex.value]) {
        return
      }
      currentMessages.value[activeAssistantIndex.value].content += chunk
      currentMessages.value = [...currentMessages.value]
      await syncSessionMessagesFromCurrent()
    }

    const handleSSEPayload = async (data) => {
      if (!data) {
        return
      }

      if (data === '[DONE]') {
        loading.value = false
        await setAssistantStatus('completed')
        return 'done'
      }

      if (!data.startsWith('{')) {
        await appendAssistantChunk(data)
        return
      }

      let parsed
      try {
        parsed = JSON.parse(data)
      } catch {
        // 非法 JSON 直接忽略，避免把协议数据漏到聊天内容里。
        return
      }

      if (parsed.type === 'ready') {
        return
      }

      if (parsed.type === 'chunk') {
        await appendAssistantChunk(parsed.delta || '')
        return
      }

      if (parsed.sessionId) {
        const newSid = String(parsed.sessionId)
        activeStreamingSessionId.value = newSid
        if (tempSession.value) {
          upsertSessionEntry({
            id: newSid,
            name: buildSessionTitle(currentMessages.value.find(message => message.role === 'user')?.content),
            messages: [...currentMessages.value]
          })
          currentSessionId.value = newSid
          tempSession.value = false
        }
        return
      }

      if (parsed.type === 'error') {
        const error = new Error(parsed.message || '流式响应失败')
        error.serverCode = parsed.status_code
        throw error
      }
    }

    const clearActiveStreamState = () => {
      activeAbortController.value = null
      activeStreamingSessionId.value = null
      activeAssistantIndex.value = -1
      manualStopRequested.value = false
    }

    const playTTS = async (text) => {
      try {
        const createResponse = await api.post('/AI/chat/tts', { text })
        if (createResponse.data && createResponse.data.status_code === 1000 && createResponse.data.task_id) {
          const taskId = createResponse.data.task_id
          await new Promise(resolve => setTimeout(resolve, 5000))

          const maxAttempts = 30
          const pollInterval = 2000
          let attempts = 0

          const pollResult = async () => {
            const queryResponse = await api.get('/AI/chat/tts/query', { params: { task_id: taskId } })
            if (queryResponse.data && queryResponse.data.status_code === 1000) {
              const taskStatus = queryResponse.data.task_status
              if (taskStatus === 'Success' && queryResponse.data.task_result) {
                const audio = new Audio(queryResponse.data.task_result)
                audio.play()
                return true
              }
              if (taskStatus === 'Running' || taskStatus === 'Created') {
                attempts++
                if (attempts < maxAttempts) {
                  await new Promise(resolve => setTimeout(resolve, pollInterval))
                  return pollResult()
                }
                ElMessage.error('语音合成超时')
                return true
              }
              ElMessage.error('语音合成失败')
              return true
            }

            attempts++
            if (attempts < maxAttempts) {
              await new Promise(resolve => setTimeout(resolve, pollInterval))
              return pollResult()
            }
            ElMessage.error('语音合成超时')
            return true
          }

          await pollResult()
        } else {
          ElMessage.error('无法创建语音合成任务')
        }
      } catch (error) {
        console.error('TTS error:', error)
        ElMessage.error('语音接口请求失败')
      }
    }

    const loadSessions = async () => {
      try {
        const response = await api.get('/AI/chat/sessions')
        if (response.data && response.data.status_code === 1000 && Array.isArray(response.data.sessions)) {
          const sessionMap = {}
          response.data.sessions.forEach((sessionItem) => {
            const sid = String(sessionItem.sessionId)
            sessionMap[sid] = {
              id: sid,
              name: sessionItem.name || `会话 ${sid}`,
              messages: []
            }
          })
          sessions.value = sessionMap
        }
      } catch (error) {
        console.error('Load sessions error:', error)
      }
    }

    const loadHistoryIntoSession = async (sessionId) => {
      const targetSession = ensureSessionEntry(sessionId)
      const response = await api.post('/AI/chat/history', { sessionId })
      if (response.data && response.data.status_code === 1000 && Array.isArray(response.data.history)) {
        targetSession.messages = response.data.history.map(mapHistoryItemToMessage)
        return
      }
      throw new Error(response.data?.status_msg || '无法加载会话历史')
    }

    const createNewSession = () => {
      currentSessionId.value = 'temp'
      tempSession.value = true
      currentMessages.value = []
      nextTick(() => {
        if (messageInput.value) messageInput.value.focus()
      })
    }

    const ensureActiveDraftSession = () => {
      if (currentSessionId.value && !tempSession.value) {
        return
      }
      if (tempSession.value && currentSessionId.value === 'temp') {
        return
      }
      currentSessionId.value = 'temp'
      tempSession.value = true
      currentMessages.value = []
    }

    const switchSession = async (sessionId) => {
      if (!sessionId || loading.value) return
      const targetSession = ensureSessionEntry(sessionId)
      currentSessionId.value = String(sessionId)
      tempSession.value = false

      try {
        if (!targetSession.messages || targetSession.messages.length === 0) {
          await loadHistoryIntoSession(sessionId)
        }
        currentMessages.value = [...(ensureSessionEntry(sessionId)?.messages || [])]
        await nextTick()
        scrollToBottom()
      } catch (error) {
        console.error('Load history error:', error)
        ElMessage.error('加载历史失败')
      }
    }

    const syncHistory = async () => {
      if (!currentSessionId.value || tempSession.value) {
        ElMessage.warning('请先选择已有会话再同步历史')
        return
      }
      try {
        await loadHistoryIntoSession(currentSessionId.value)
        currentMessages.value = [...(ensureSessionEntry(currentSessionId.value)?.messages || [])]
        await nextTick()
        scrollToBottom()
      } catch (error) {
        console.error('Sync history error:', error)
        ElMessage.error('同步历史失败')
      }
    }

    const stopCurrentStream = async () => {
      if (!loading.value || !isStreaming.value) return

      manualStopRequested.value = true
      const targetSessionId = activeStreamingSessionId.value && activeStreamingSessionId.value !== 'temp'
        ? activeStreamingSessionId.value
        : (!tempSession.value ? currentSessionId.value : null)

      try {
        if (targetSessionId) {
          const response = await api.post('/AI/chat/stop', { sessionId: targetSessionId })
          if (response.data?.status_code !== 1000 && response.data?.status_code !== 2012) {
            throw new Error(response.data?.status_msg || '停止生成失败')
          }
        }
      } catch (error) {
        console.error('Stop stream error:', error)
      } finally {
        if (activeAbortController.value) {
          activeAbortController.value.abort()
        }
        loading.value = false
        await setAssistantStatus('cancelled')
        ElMessage.success('已停止当前生成')
      }
    }

    const sendMessage = async () => {
      if (!inputMessage.value || !inputMessage.value.trim()) {
        ElMessage.warning('请输入消息内容')
        return
      }

      if (!currentSessionId.value) {
        ensureActiveDraftSession()
      }

      const currentInput = inputMessage.value.trim()
      const userMessage = {
        role: 'user',
        content: currentInput,
        meta: buildMessageMeta('completed')
      }
      inputMessage.value = ''

      currentMessages.value.push(userMessage)
      if (!tempSession.value && currentSessionId.value && sessions.value[currentSessionId.value]) {
        upsertSessionEntry({
          ...sessions.value[currentSessionId.value],
          messages: [...currentMessages.value]
        })
      }
      await syncSessionMessagesFromCurrent()

      try {
        loading.value = true
        if (isStreaming.value) {
          await handleStreaming(currentInput)
        } else {
          await handleNormal(currentInput)
        }
      } catch (error) {
        console.error('Send message error:', error)
        ElMessage.error(error.message || '发送失败，请重试')

        if (!tempSession.value && currentSessionId.value && sessions.value[currentSessionId.value]?.messages?.length) {
          sessions.value[currentSessionId.value].messages.pop()
        }
        currentMessages.value.pop()
      } finally {
        if (!isStreaming.value) {
          loading.value = false
        }
        await nextTick()
        scrollToBottom()
      }
    }

    async function handleStreaming(question) {
      const aiMessage = {
        role: 'assistant',
        content: '',
        meta: buildMessageMeta('streaming')
      }

      activeAssistantIndex.value = currentMessages.value.length
      currentMessages.value.push(aiMessage)
      await syncSessionMessagesFromCurrent()

      const url = tempSession.value ? '/api/AI/chat/send-stream-new-session' : '/api/AI/chat/send-stream'
      const accessToken = await ensureAccessToken(refreshClient)
      const headers = {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${accessToken}`
      }
      const body = tempSession.value
        ? { question, modelType: selectedModel.value }
        : { question, modelType: selectedModel.value, sessionId: currentSessionId.value }

      const controller = new AbortController()
      activeAbortController.value = controller
      activeStreamingSessionId.value = tempSession.value ? 'temp' : currentSessionId.value
      manualStopRequested.value = false

      let doneReceived = false

      try {
        const response = await fetch(url, {
          method: 'POST',
          headers,
          body: JSON.stringify(body),
          signal: controller.signal
        })

        if (!response.ok || !response.body) {
          throw new Error('流式请求失败')
        }

        const reader = response.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''

        let streamClosed = false
        while (!streamClosed) {
          const { done, value } = await reader.read()
          if (done) {
            streamClosed = true
            break
          }

          buffer += decoder.decode(value, { stream: true })
          const events = buffer.split('\n\n')
          buffer = events.pop() || ''

          for (const eventText of events) {
            const dataLines = eventText
              .split('\n')
              .map(line => line.trim())
              .filter(line => line.startsWith('data:'))
              .map(line => line.slice(5).trim())

            if (!dataLines.length) {
              continue
            }

            const eventData = dataLines.join('\n')
            const result = await handleSSEPayload(eventData)
            if (result === 'done') {
              doneReceived = true
            }
          }
        }

        loading.value = false
        if (!doneReceived) {
          await setAssistantStatus(manualStopRequested.value ? 'cancelled' : 'partial')
        }
      } catch (error) {
        console.error('Stream error:', error)
        loading.value = false

        if (manualStopRequested.value || error.name === 'AbortError' || error.serverCode === 5004) {
          await setAssistantStatus('cancelled')
        } else if (error.serverCode === 4002) {
          await setAssistantStatus('timeout')
          ElMessage.error(error.message || '请求超时')
        } else {
          await setAssistantStatus('failed')
          ElMessage.error(error.message || '流式响应异常')
        }
      } finally {
        clearActiveStreamState()
      }
    }

    async function handleNormal(question) {
      if (tempSession.value) {
        const response = await api.post('/AI/chat/send-new-session', {
          question,
          modelType: selectedModel.value
        })
        if (response.data && response.data.status_code === 1000) {
          const sessionId = String(response.data.sessionId)
          const aiMessage = {
            role: 'assistant',
            content: response.data.Information || '',
            meta: buildMessageMeta('completed')
          }

          upsertSessionEntry({
            id: sessionId,
            name: buildSessionTitle(question),
            messages: [{ role: 'user', content: question, meta: buildMessageMeta('completed') }, aiMessage]
          })
          currentSessionId.value = sessionId
          tempSession.value = false
          currentMessages.value = [...sessions.value[sessionId].messages]
        } else {
          throw new Error(response.data?.status_msg || '发送失败')
        }
      } else {
        const targetSession = ensureSessionEntry(currentSessionId.value)
        const sessionMsgs = targetSession ? (targetSession.messages || []) : []
        sessionMsgs.push({ role: 'user', content: question, meta: buildMessageMeta('completed') })
        if (targetSession) {
          targetSession.messages = sessionMsgs
        }

        const response = await api.post('/AI/chat/send', {
          question,
          modelType: selectedModel.value,
          sessionId: currentSessionId.value
        })
        if (response.data && response.data.status_code === 1000) {
          sessionMsgs.push({
            role: 'assistant',
            content: response.data.Information || '',
            meta: buildMessageMeta('completed')
          })
          currentMessages.value = [...sessionMsgs]
        } else {
          sessionMsgs.pop()
          throw new Error(response.data?.status_msg || '发送失败')
        }
      }
    }

    const triggerFileUpload = () => {
      if (fileInput.value) {
        fileInput.value.click()
      }
    }

    const handleFileUpload = async (event) => {
      const file = event.target.files[0]
      if (!file) return

      const fileName = file.name.toLowerCase()
      if (!fileName.endsWith('.md') && !fileName.endsWith('.txt')) {
        ElMessage.error('只支持上传 .md 和 .txt 文件')
        if (fileInput.value) {
          fileInput.value.value = ''
        }
        return
      }

      try {
        uploading.value = true
        const formData = new FormData()
        formData.append('file', file)

        const response = await api.post('/file/upload', formData, {
          headers: {
            'Content-Type': 'multipart/form-data'
          }
        })

        if (response.data && response.data.status_code === 1000) {
          ElMessage.success('文件上传成功')
        } else {
          ElMessage.error(response.data?.status_msg || '文件上传失败')
        }
      } catch (error) {
        console.error('File upload error:', error)
        ElMessage.error('文件上传失败')
      } finally {
        uploading.value = false
        if (fileInput.value) {
          fileInput.value.value = ''
        }
      }
    }

    // ===== Image Recognition (inline in chat) =====
    const triggerImageUpload = () => {
      if (imageInput.value) {
        imageInput.value.click()
      }
    }

    const handleImageRecognition = async (event) => {
      const file = event.target.files[0]
      if (!file) return

      const imageUrl = URL.createObjectURL(file)

      // Add user message with image preview
      currentMessages.value.push({
        role: 'user',
        content: `已上传图片: ${file.name}`,
        imageUrl: imageUrl,
        meta: buildMessageMeta('completed')
      })
      await nextTick()
      scrollToBottom()

      const formData = new FormData()
      formData.append('image', file)

      try {
        loading.value = true
        const response = await api.post('/image/recognize', formData, {
          headers: {
            'Content-Type': 'multipart/form-data'
          }
        })

        if (response.data && response.data.class_name) {
          currentMessages.value.push({
            role: 'assistant',
            content: `识别结果: **${response.data.class_name}**`,
            meta: buildMessageMeta('completed')
          })
        } else {
          currentMessages.value.push({
            role: 'assistant',
            content: `[错误] ${response.data?.status_msg || '识别失败'}`,
            meta: buildMessageMeta('failed')
          })
        }
      } catch (error) {
        console.error('Image recognition error:', error)
        currentMessages.value.push({
          role: 'assistant',
          content: `[错误] 无法连接到服务器或识别失败: ${error.message}`,
          meta: buildMessageMeta('failed')
        })
      } finally {
        loading.value = false
        await nextTick()
        scrollToBottom()
        if (imageInput.value) {
          imageInput.value.value = ''
        }
      }
    }

    const applyUserProfile = (profile) => {
      const nextProfile = profile || {}
      userProfile.value = {
        id: nextProfile.id || null,
        name: nextProfile.name || '',
        username: nextProfile.username || '',
        email: nextProfile.email || '',
        avatar_url: nextProfile.avatar_url || '',
        bio: nextProfile.bio || ''
      }
      profileForm.value = {
        name: userProfile.value.name || '',
        bio: userProfile.value.bio || ''
      }
    }

    const fetchUserProfile = async () => {
      try {
        const response = await api.get('/user/profile')
        if (response.data?.status_code === 1000 && response.data.profile) {
          applyUserProfile(response.data.profile)
        }
      } catch (error) {
        console.error('Load profile error:', error)
      }
    }

    const getUserDisplayName = () => userProfile.value.name || userProfile.value.username || '用户'

    const getUserInitial = () => {
      const source = getUserDisplayName().trim()
      return source ? source.slice(0, 1).toUpperCase() : 'U'
    }

    const toggleUserMenu = () => {
      userMenuVisible.value = !userMenuVisible.value
    }

    const handleSettings = async () => {
      userMenuVisible.value = false
      settingsVisible.value = true
      await fetchUserProfile()
    }

    const handleSyncHistoryFromMenu = async () => {
      userMenuVisible.value = false
      await syncHistory()
    }

    const saveProfile = async () => {
      try {
        savingProfile.value = true
        const response = await api.post('/user/profile/update', {
          name: profileForm.value.name || '',
          bio: profileForm.value.bio || ''
        })
        if (response.data?.status_code === 1000 && response.data.profile) {
          applyUserProfile(response.data.profile)
          settingsVisible.value = false
          ElMessage.success('个人资料已更新')
          return
        }
        ElMessage.error(response.data?.status_msg || '个人资料更新失败')
      } catch (error) {
        console.error('Save profile error:', error)
        ElMessage.error('个人资料更新失败')
      } finally {
        savingProfile.value = false
      }
    }

    const triggerAvatarUpload = () => {
      if (avatarInput.value) {
        avatarInput.value.click()
      }
    }

    const handleAvatarUpload = async (event) => {
      const file = event.target.files[0]
      if (!file) return

      try {
        pendingAvatarFile.value = file
        const { width, height } = await loadImageDimensions(file)
        cropImageNaturalWidth.value = width
        cropImageNaturalHeight.value = height
        cropScale.value = 1
        cropOffsetX.value = 0
        cropOffsetY.value = 0
        if (cropPreviewUrl.value) {
          URL.revokeObjectURL(cropPreviewUrl.value)
        }
        cropPreviewUrl.value = URL.createObjectURL(file)
        cropDialogVisible.value = true
      } catch (error) {
        pendingAvatarFile.value = null
        cropImageNaturalWidth.value = 0
        cropImageNaturalHeight.value = 0
        console.error('Load avatar preview error:', error)
        ElMessage.error('头像预览加载失败')
      } finally {
        if (avatarInput.value) {
          avatarInput.value.value = ''
        }
      }
    }

    const cancelAvatarCrop = () => {
      cropDialogVisible.value = false
      pendingAvatarFile.value = null
      cropImageNaturalWidth.value = 0
      cropImageNaturalHeight.value = 0
      if (cropPreviewUrl.value) {
        URL.revokeObjectURL(cropPreviewUrl.value)
        cropPreviewUrl.value = ''
      }
    }

    const confirmAvatarCrop = async () => {
      if (!pendingAvatarFile.value) return

      try {
        uploadingAvatar.value = true
        const croppedFile = await buildCroppedAvatarFile(pendingAvatarFile.value)
        const formData = new FormData()
        formData.append('avatar', croppedFile)

        const response = await api.post('/user/avatar/upload', formData, {
          headers: {
            'Content-Type': 'multipart/form-data'
          }
        })

        if (response.data?.status_code === 1000 && response.data.profile) {
          applyUserProfile(response.data.profile)
          cropDialogVisible.value = false
          ElMessage.success('头像上传成功')
        } else {
          ElMessage.error(response.data?.status_msg || '头像上传失败')
        }
      } catch (error) {
        console.error('Upload avatar error:', error)
        ElMessage.error('头像上传失败')
      } finally {
        uploadingAvatar.value = false
        cancelAvatarCrop()
      }
    }

    const buildCroppedAvatarFile = (file) => new Promise((resolve, reject) => {
      const image = new Image()
      const objectUrl = URL.createObjectURL(file)
      image.onload = () => {
        const canvas = document.createElement('canvas')
        const size = CROP_OUTPUT_SIZE
        canvas.width = size
        canvas.height = size
        const ctx = canvas.getContext('2d')
        if (!ctx) {
          URL.revokeObjectURL(objectUrl)
          reject(new Error('无法创建裁剪画布'))
          return
        }

        const baseScale = Math.max(size / image.width, size / image.height)
        const finalScale = baseScale * cropScale.value
        const drawWidth = image.width * finalScale
        const drawHeight = image.height * finalScale
        const previewToOutputRatio = size / CROP_PREVIEW_SIZE
        const offsetX = (size - drawWidth) / 2 + cropOffsetX.value * previewToOutputRatio
        const offsetY = (size - drawHeight) / 2 + cropOffsetY.value * previewToOutputRatio
        ctx.clearRect(0, 0, size, size)
        ctx.drawImage(image, offsetX, offsetY, drawWidth, drawHeight)

        canvas.toBlob((blob) => {
          URL.revokeObjectURL(objectUrl)
          if (!blob) {
            reject(new Error('无法生成裁剪结果'))
            return
          }
          resolve(new File([blob], 'avatar.png', { type: 'image/png' }))
        }, 'image/png')
      }
      image.onerror = () => {
        URL.revokeObjectURL(objectUrl)
        reject(new Error('头像预览加载失败'))
      }
      image.src = objectUrl
    })

    const loadImageDimensions = (file) => new Promise((resolve, reject) => {
      const image = new Image()
      const objectUrl = URL.createObjectURL(file)
      image.onload = () => {
        URL.revokeObjectURL(objectUrl)
        resolve({
          width: image.width,
          height: image.height
        })
      }
      image.onerror = () => {
        URL.revokeObjectURL(objectUrl)
        reject(new Error('头像预览加载失败'))
      }
      image.src = objectUrl
    })

    const toggleTheme = () => {
      isDark.value = !isDark.value
      if (isDark.value) {
        document.documentElement.classList.add('dark')
      } else {
        document.documentElement.classList.remove('dark')
      }
    }

    const handleLogout = async () => {
      userMenuVisible.value = false
      try {
        await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning'
        })
        await api.post('/user/logout')
        clearTokens()
        ElMessage.success('退出登录成功')
        router.push('/login')
      } catch {
        // 用户取消操作
      }
    }

    // eslint-disable-next-line no-unused-vars
    const handleSettingsLegacy = () => {
      ElMessage.info('设置功能开发中...')
    }

    const toggleSidebar = () => {
      isSidebarCollapsed.value = !isSidebarCollapsed.value
    }

    onMounted(() => {
      isDark.value = document.documentElement.classList.contains('dark')
      loadSessions()
      fetchUserProfile()
    })

    return {
      isSidebarCollapsed,
      isDark,
      sessions: computed(() => Object.values(sessions.value)),
      currentSessionId,
      tempSession,
      currentMessages,
      inputMessage,
      loading,
      messagesRef,
      messageInput,
      selectedModel,
      isStreaming,
      uploading,
      fileInput,
      imageInput,
      avatarInput,
      settingsVisible,
      userMenuVisible,
      savingProfile,
      uploadingAvatar,
      cropDialogVisible,
      cropPreviewUrl,
      cropScale,
      cropOffsetX,
      cropOffsetY,
      userProfile,
      profileForm,
      renderMarkdown,
      cropImageStyle,
      modelOptions,
      getMessageStatusLabel,
      getMessageMetaStatus,
      getUserDisplayName,
      getUserInitial,
      toggleUserMenu,
      playTTS,
      createNewSession,
      switchSession,
      syncHistory,
      stopCurrentStream,
      sendMessage,
      triggerFileUpload,
      handleFileUpload,
      triggerImageUpload,
      handleImageRecognition,
      triggerAvatarUpload,
      handleAvatarUpload,
      handleSyncHistoryFromMenu,
      cancelAvatarCrop,
      confirmAvatarCrop,
      saveProfile,
      toggleTheme,
      handleLogout,
      handleSettings,
      toggleSidebar
    }
  }
}
</script>

<style scoped>
/* Custom scrollbar for sidebar */
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
</style>
