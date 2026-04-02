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
            title="Close sidebar"
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
            <span class="whitespace-nowrap">New chat</span>
          </button>
        </div>
        <div class="px-3 pb-2">
          <button
            class="flex items-center gap-3 px-4 py-2 rounded-xl bg-transparent hover:bg-black/5 dark:hover:bg-white/5 transition-all cursor-pointer text-sm text-text-secondary-light dark:text-text-secondary-dark border-none w-full"
            @click="showCreateFolderDialog"
          >
            <span>+ Folder</span>
          </button>
        </div>

        <!-- Conversation History -->
        <div class="flex-1 overflow-y-auto px-2 pt-2">
          <div class="px-3 pb-2 flex items-center justify-between">
            <span class="text-xs font-medium text-text-secondary-light dark:text-text-secondary-dark tracking-wider">近期</span>
            <button
              class="p-1 rounded-md hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer text-text-secondary-light dark:text-text-secondary-dark bg-transparent border-none"
              title="新建文件夹"
              @click="showCreateFolderDialog"
            >
              <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4"/></svg>
            </button>
          </div>
          <ul class="list-none m-0 p-0 space-y-0.5">
            <!-- Folders -->
            <li v-for="folder in foldersList" :key="folder.id" class="mb-1">
              <div
                class="px-3 py-2 rounded-xl cursor-pointer text-sm transition-all group flex items-center text-text-secondary-light dark:text-text-secondary-dark hover:bg-black/5 dark:hover:bg-white/5"
                @click="toggleFolder(folder.id)"
              >
                <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 mr-2 shrink-0 transition-transform" :class="collapsedFolders[folder.id] ? '-rotate-90' : ''" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" /></svg>
                <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 mr-2 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" /></svg>
                <span class="truncate flex-1 font-medium">{{ folder.name }}</span>
                <el-dropdown trigger="click" @command="(cmd) => handleFolderCommand(cmd, folder)" @click.stop class="ml-1 opacity-0 group-hover:opacity-100 transition-opacity">
                  <span class="p-1 rounded hover:bg-black/10 dark:hover:bg-white/10 text-text-secondary-light dark:text-text-secondary-dark">
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
                    'px-3 py-2 rounded-xl cursor-pointer text-sm transition-all group flex items-center',
                    currentSessionId === session.sessionId
                      ? 'bg-black/5 dark:bg-white/8 font-medium text-text-primary-light dark:text-text-primary-dark'
                      : 'text-text-secondary-light dark:text-text-secondary-dark hover:bg-black/5 dark:hover:bg-white/5'
                  ]"
                  @click="switchSession(session.sessionId)"
                >
                  <span class="truncate flex-1">{{ session.name || `会话 ${session.sessionId}` }}</span>
                  <el-dropdown trigger="click" @command="(cmd) => handleSessionCommand(cmd, session)" @click.stop class="ml-1 opacity-0 group-hover:opacity-100 transition-opacity">
                    <span class="p-1 rounded hover:bg-black/10 dark:hover:bg-white/10 text-text-secondary-light dark:text-text-secondary-dark">
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

            <!-- Ungrouped Sessions -->
            <li
              v-for="session in ungroupedSessionsList"
              :key="session.sessionId"
              :class="[
                'px-3 py-2 rounded-xl cursor-pointer text-sm transition-all group flex items-center',
                currentSessionId === session.sessionId
                  ? 'bg-black/5 dark:bg-white/8 font-medium text-text-primary-light dark:text-text-primary-dark'
                  : 'text-text-secondary-light dark:text-text-secondary-dark hover:bg-black/5 dark:hover:bg-white/5'
              ]"
              @click="switchSession(session.sessionId)"
            >
              <span class="truncate flex-1">{{ session.name || `会话 ${session.sessionId}` }}</span>
              <el-dropdown trigger="click" @command="(cmd) => handleSessionCommand(cmd, session)" @click.stop class="ml-1 opacity-0 group-hover:opacity-100 transition-opacity">
                <span class="p-1 rounded hover:bg-black/10 dark:hover:bg-white/10 text-text-secondary-light dark:text-text-secondary-dark">
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

        <!-- Bottom Actions -->
        <div class="px-2 pb-3 pt-2 space-y-0.5">
          <div class="relative">
            <button
              class="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer bg-transparent border-none text-left"
              @click.stop="toggleUserMenu"
            >
              <img
                v-if="userProfile.avatar_url"
                :src="userProfile.avatar_url"
                alt="User avatar"
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
              <button type="button" class="w-full px-4 py-3 text-left text-sm hover:bg-black/5 dark:hover:bg-white/5 bg-transparent border-none cursor-pointer" @click="handleSettings">Settings</button>
              <button type="button" class="w-full px-4 py-3 text-left text-sm hover:bg-black/5 dark:hover:bg-white/5 bg-transparent border-none cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed" :disabled="!currentSessionId || tempSession || loading" @click="handleSyncHistoryFromMenu">Sync history</button>
              <button type="button" class="w-full px-4 py-3 text-left text-sm hover:bg-black/5 dark:hover:bg-white/5 bg-transparent border-none cursor-pointer" @click="handleLogout">Log out</button>
            </div>
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
          title="Open sidebar"
        >
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 text-text-secondary-light dark:text-text-secondary-dark" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16"/></svg>
        </button>

        <!-- New chat button (Gemini collapsed state: icon-only pill) -->
        <button
          v-if="isSidebarCollapsed"
          class="p-2 rounded-full hover:bg-black/5 dark:hover:bg-white/5 transition-colors cursor-pointer bg-transparent"
          @click="createNewSession"
          title="New chat"
        >
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 text-text-secondary-light dark:text-text-secondary-dark" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"/></svg>
        </button>

        <div class="flex items-center gap-3 ml-auto">
          <label for="modelType" class="text-sm text-text-secondary-light dark:text-text-secondary-dark hidden sm:block">Model</label>
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
              <span class="font-semibold text-sm">{{ message.role === 'user' ? 'You' : 'AI' }}</span>
              <!-- Actions & Meta -->
              <div class="flex items-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                <button
                  v-if="message.role === 'assistant' && message.content"
                  class="px-2 py-0.5 text-xs rounded bg-surface-light dark:bg-surface-dark border border-border-light dark:border-border-dark hover:text-accent-light dark:hover:text-accent-dark cursor-pointer transition-colors"
                  @click="playTTS(message.content)"
                >
                  Read aloud
                </button>
              </div>
              <span v-if="getMessageMetaStatus(message)" class="text-xs text-text-secondary-light dark:text-text-secondary-dark ml-auto">
                {{ getMessageStatusLabel(getMessageMetaStatus(message)) }}
              </span>
            </div>

            <!-- Message Content block -->
            <div class="pl-11 text-base leading-relaxed space-y-4 break-words">
              <!-- Image Preview (for image recognition messages) -->
              <img v-if="message.imageUrl" :src="message.imageUrl" alt="Uploaded image" class="max-w-xs rounded-xl shadow-md mt-1 mb-2" />
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
            <label class="block text-sm">Display name</label>
            <input v-model="profileForm.name" type="text" maxlength="50" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none" />
          </div>

          <div class="space-y-2">
            <label class="block text-sm">Username</label>
            <input :value="userProfile.username || ''" type="text" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-black/5 dark:bg-white/5 outline-none" disabled />
          </div>

          <div class="space-y-2">
            <label class="block text-sm">Email</label>
            <input :value="userProfile.email || ''" type="text" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-black/5 dark:bg-white/5 outline-none" disabled />
          </div>

          <div class="space-y-2">
            <label class="block text-sm">Bio</label>
            <textarea v-model="profileForm.bio" rows="4" maxlength="255" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none resize-none"></textarea>
          </div>
        </div>
        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="settingsVisible = false">Cancel</button>
            <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="saveProfile" :disabled="savingProfile">{{ savingProfile ? 'Saving...' : 'Save' }}</button>
          </div>
        </template>
      </el-dialog>
      <el-dialog v-model="cropDialogVisible" title="Crop avatar" width="560px">
        <div class="space-y-4">
          <div class="flex justify-center">
            <div class="relative flex items-center justify-center w-72 h-72 overflow-hidden rounded-2xl border border-border-light dark:border-border-dark bg-black/5 dark:bg-white/5">
              <img
                v-if="cropPreviewUrl"
                :src="cropPreviewUrl"
                alt="Crop preview"
                class="max-w-none select-none"
                :style="cropImageStyle"
              />
            </div>
          </div>
          <div class="space-y-3">
            <div>
              <label class="block text-sm mb-2">Zoom</label>
              <input v-model="cropScale" type="range" min="1" max="3" step="0.01" class="w-full" />
            </div>
            <div>
              <label class="block text-sm mb-2">Horizontal offset</label>
              <input v-model="cropOffsetX" type="range" min="-120" max="120" step="1" class="w-full" />
            </div>
            <div>
              <label class="block text-sm mb-2">Vertical offset</label>
              <input v-model="cropOffsetY" type="range" min="-120" max="120" step="1" class="w-full" />
            </div>
          </div>
          <p class="text-xs text-text-secondary-light dark:text-text-secondary-dark">The crop ratio is fixed at 1:1 for avatar display.</p>
        </div>
        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="cancelAvatarCrop">Cancel</button>
            <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="confirmAvatarCrop" :disabled="uploadingAvatar">{{ uploadingAvatar ? 'Uploading...' : 'Confirm upload' }}</button>
          </div>
        </template>
      </el-dialog>

      <!-- 新建/重命名 文件夹弹窗 -->
      <el-dialog v-model="folderDialogVisible" :title="folderDialogType === 'create' ? '新建文件夹' : '重命名文件夹'" width="400px">
        <div class="space-y-4">
          <div class="space-y-2">
            <label class="block text-sm">文件夹名称</label>
            <input v-model="folderForm.name" type="text" maxlength="50" placeholder="请输入文件夹名称" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none" @keydown.enter="submitFolderDialog" />
          </div>
        </div>
        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="folderDialogVisible = false">取消</button>
            <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="submitFolderDialog" :disabled="!folderForm.name.trim() || submittingFolder">确认</button>
          </div>
        </template>
      </el-dialog>

      <!-- 重命名会话弹窗 -->
      <el-dialog v-model="renameSessionDialogVisible" title="重命名会话" width="400px">
        <div class="space-y-4">
          <div class="space-y-2">
            <label class="block text-sm">会话名称</label>
            <input v-model="renameSessionForm.name" type="text" maxlength="50" placeholder="请输入会话名称" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none" @keydown.enter="submitRenameSession" />
          </div>
        </div>
        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="renameSessionDialogVisible = false">取消</button>
            <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="submitRenameSession" :disabled="!renameSessionForm.name.trim() || submittingRenameSession">确认</button>
          </div>
        </template>
      </el-dialog>

      <!-- 移动到文件夹弹窗 -->
      <el-dialog v-model="moveSessionDialogVisible" title="移动到文件夹" width="400px">
        <div class="space-y-4">
          <div class="space-y-2">
            <label class="block text-sm">选择目标文件夹</label>
            <select v-model="moveSessionForm.folderId" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent outline-none cursor-pointer">
              <option value="" class="bg-surface-light dark:bg-surface-dark">-- 改为独立会话 (移出文件夹) --</option>
              <option v-for="f in foldersList" :key="f.id" :value="f.id" class="bg-surface-light dark:bg-surface-dark">{{ f.name }}</option>
            </select>
          </div>
        </div>
        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="moveSessionDialogVisible = false">取消</button>
            <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="submitMoveSession" :disabled="submittingMoveSession">确认</button>
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
    const sessions = ref({}) // 仍保留平铺的数据结构管理消息状态
    
    // ----------- 文件夹 & 会话树状态 -----------
    const foldersList = ref([])
    const ungroupedSessionsList = ref([])
    const collapsedFolders = ref({}) // track folder collapse state
    
    // Dialog state
    const folderDialogVisible = ref(false)
    const folderDialogType = ref('create') // 'create' or 'rename'
    const folderForm = ref({ id: '', name: '' })
    const submittingFolder = ref(false)

    const renameSessionDialogVisible = ref(false)
    const renameSessionForm = ref({ sessionId: '', name: '' })
    const submittingRenameSession = ref(false)

    const moveSessionDialogVisible = ref(false)
    const moveSessionForm = ref({ sessionId: '', folderId: '' })
    const submittingMoveSession = ref(false)
    // -------------------------------------------
    const sessionFolders = ref([])
    const ungroupedSessionIds = ref([])
    const expandedFolders = ref({})
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

    // 用于中断当前请求，保证停止按钮和异常处理共用同一 controller。
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
      return title || 'New session'
    }

    const mapHistoryItemToMessage = (item) => ({
      role: item.is_user ? 'user' : 'assistant',
      content: item.content || '',
      meta: buildMessageMeta(item.status)
    })

    const getMessageStatusLabel = (status) => {
      switch (normalizeMessageStatus(status)) {
      case 'streaming':
        return 'Streaming'
      case 'cancelled':
        return 'Stopped'
      case 'timeout':
        return 'Timed out'
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
      ensureSessionListed(normalizedId)
      return nextEntry
    }

    const sidebarFolders = computed(() => sessionFolders.value.map(folder => ({
      ...folder,
      sessions: (folder.sessionIds || [])
        .map(sessionId => sessions.value[sessionId])
        .filter(Boolean)
    })))

    const ungroupedSessions = computed(() => ungroupedSessionIds.value
      .map(sessionId => sessions.value[sessionId])
      .filter(Boolean))

    const isFolderExpanded = (folderId) => expandedFolders.value[String(folderId)] !== false

    const toggleFolder = (folderId) => {
      const key = String(folderId)
      expandedFolders.value = {
        ...expandedFolders.value,
        [key]: !isFolderExpanded(key)
      }
      collapsedFolders.value[key] = !collapsedFolders.value[key]
    }

    const ensureSessionListed = (sessionId) => {
      const normalizedId = String(sessionId || '')
      if (!normalizedId || normalizedId === 'temp') return

      const inFolder = sessionFolders.value.some(folder => (folder.sessionIds || []).includes(normalizedId))
      const inUngrouped = ungroupedSessionIds.value.includes(normalizedId)
      if (!inFolder && !inUngrouped) {
        ungroupedSessionIds.value = [normalizedId, ...ungroupedSessionIds.value]
      }
    }

    const applySessionTree = (tree) => {
      const nextSessionMap = {}
      const nextFolders = []
      const nextExpanded = { ...expandedFolders.value }
      const nextUngrouped = []

      ;(tree?.folders || []).forEach(folder => {
        const folderId = String(folder.id)
        nextExpanded[folderId] = expandedFolders.value[folderId] !== false
        const sessionIds = []

        ;(folder.sessions || []).forEach(sessionItem => {
          const sid = String(sessionItem.sessionId)
          const existing = sessions.value[sid] || {}
          nextSessionMap[sid] = {
            ...existing,
            id: sid,
            name: sessionItem.name || existing.name || `Session ${sid}`,
            messages: Array.isArray(existing.messages) ? existing.messages : []
          }
          sessionIds.push(sid)
        })

        nextFolders.push({
          id: folder.id,
          name: folder.name || `Folder ${folder.id}`,
          sessionIds
        })
      })

      ;(tree?.ungroupedSessions || []).forEach(sessionItem => {
        const sid = String(sessionItem.sessionId)
        const existing = sessions.value[sid] || {}
        nextSessionMap[sid] = {
          ...existing,
          id: sid,
          name: sessionItem.name || existing.name || `Session ${sid}`,
          messages: Array.isArray(existing.messages) ? existing.messages : []
        }
        nextUngrouped.push(sid)
      })

      sessions.value = nextSessionMap
      sessionFolders.value = nextFolders
      ungroupedSessionIds.value = nextUngrouped
      expandedFolders.value = nextExpanded
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
        const response = await api.get('/AI/chat/session-tree')
        if (response.data && response.data.status_code === 1000 && response.data.tree) {
          const tree = response.data.tree
          foldersList.value = tree.folders || []
          ungroupedSessionsList.value = tree.ungrouped_sessions || []
          applySessionTree({
            folders: tree.folders || [],
            ungroupedSessions: tree.ungroupedSessions || tree.ungrouped_sessions || []
          })
          const sessionMap = {}
          // Initialize flat sessions map and collapsed states
          foldersList.value.forEach(f => {
            if (!(f.id in collapsedFolders.value)) {
              collapsedFolders.value[f.id] = false
            }
            if (f.sessions) {
              f.sessions.forEach(s => {
                sessionMap[s.sessionId] = { id: s.sessionId, name: s.name, folderId: s.folderId, messages: sessions.value[s.sessionId]?.messages || [] }
              })
            } else {
              f.sessions = []
            }
          })
          ungroupedSessionsList.value.forEach(s => {
            sessionMap[s.sessionId] = { id: s.sessionId, name: s.name, folderId: null, messages: sessions.value[s.sessionId]?.messages || [] }
          })
          sessions.value = sessionMap
        }
      } catch (error) {
        console.error('Load session tree error:', error)
      }
    }

    // --- 文件夹管理 ---
    const showCreateFolderDialog = () => {
      folderDialogType.value = 'create'
      folderForm.value = { id: '', name: '' }
      folderDialogVisible.value = true
    }

    const handleFolderCommand = (cmd, folder) => {
      if (cmd === 'rename') {
        folderDialogType.value = 'rename'
        folderForm.value = { id: folder.id, name: folder.name }
        folderDialogVisible.value = true
      } else if (cmd === 'delete') {
        ElMessageBox.confirm('删除文件夹后，其中的会话将被移出并作为独立会话保留。确定删除吗？', '删除确认', {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning'
        }).then(async () => {
          try {
            const res = await api.post('/AI/chat/folder/delete', { folderId: folder.id })
            if (res.data?.status_code === 1000) {
              ElMessage.success('已删除文件夹')
              await loadSessions()
            } else {
              ElMessage.error(res.data?.status_msg || '删除失败')
            }
          } catch (e) {
            ElMessage.error('服务器错误')
          }
        }).catch(() => {})
      }
    }

    const submitFolderDialog = async () => {
      if (!folderForm.value.name.trim() || submittingFolder.value) return
      submittingFolder.value = true
      try {
        const url = folderDialogType.value === 'create' ? '/AI/chat/folder/create' : '/AI/chat/folder/rename'
        const payload = folderDialogType.value === 'create' ? { name: folderForm.value.name } : { folderId: folderForm.value.id, name: folderForm.value.name }
        const res = await api.post(url, payload)
        if (res.data?.status_code === 1000) {
          ElMessage.success(folderDialogType.value === 'create' ? '创建成功' : '重命名成功')
          folderDialogVisible.value = false
          await loadSessions()
        } else {
          ElMessage.error(res.data?.status_msg || '操作失败')
        }
      } catch (e) {
        ElMessage.error('请求异常')
      } finally {
        submittingFolder.value = false
      }
    }

    // --- 会话管理 ---
    const handleSessionCommand = (cmd, session) => {
      if (cmd === 'rename') {
        renameSessionForm.value = { sessionId: session.sessionId, name: session.name }
        renameSessionDialogVisible.value = true
      } else if (cmd === 'move') {
        moveSessionForm.value = { sessionId: session.sessionId, folderId: session.folderId || '' }
        moveSessionDialogVisible.value = true
      } else if (cmd === 'removeFromFolder') {
        api.post('/AI/chat/session/remove-from-folder', { sessionId: session.sessionId }).then(res => {
          if (res.data?.status_code === 1000) {
            ElMessage.success('已移出文件夹')
            loadSessions()
          } else {
            ElMessage.error('移出失败')
          }
        })
      } else if (cmd === 'delete') {
        ElMessageBox.confirm('会话删除后无法恢复，确定删除该会话吗？', '删除确认', {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning'
        }).then(async () => {
          try {
            const res = await api.post('/AI/chat/session/delete', { sessionId: session.sessionId })
            if (res.data?.status_code === 1000) {
              ElMessage.success('已删除')
              if (currentSessionId.value === String(session.sessionId)) {
                currentSessionId.value = 'temp'
                currentMessages.value = []
                tempSession.value = true
              }
              await loadSessions()
            } else {
              ElMessage.error(res.data?.status_msg || '删除失败')
            }
          } catch (e) {
            ElMessage.error('删除异常')
          }
        }).catch(() => {})
      }
    }

    const submitRenameSession = async () => {
      if (!renameSessionForm.value.name.trim() || submittingRenameSession.value) return
      submittingRenameSession.value = true
      try {
        const res = await api.post('/AI/chat/session/rename', { sessionId: renameSessionForm.value.sessionId, title: renameSessionForm.value.name })
        if (res.data?.status_code === 1000) {
          renameSessionDialogVisible.value = false
          await loadSessions()
        } else {
          ElMessage.error('重命名失败')
        }
      } finally {
        submittingRenameSession.value = false
      }
    }

    const submitMoveSession = async () => {
      if (submittingMoveSession.value) return
      submittingMoveSession.value = true
      try {
        let res
        if (!moveSessionForm.value.folderId) {
          res = await api.post('/AI/chat/session/remove-from-folder', { sessionId: moveSessionForm.value.sessionId })
        } else {
          res = await api.post('/AI/chat/session/move', { sessionId: moveSessionForm.value.sessionId, folderId: moveSessionForm.value.folderId })
        }
        if (res.data?.status_code === 1000) {
          moveSessionDialogVisible.value = false
          await loadSessions()
        } else {
          ElMessage.error('移动失败')
        }
      } finally {
        submittingMoveSession.value = false
      }
    }

    const createFolder = async () => {
      try {
        const { value } = await ElMessageBox.prompt('Enter a folder name', 'Create Folder', {
          confirmButtonText: 'OK',
          cancelButtonText: 'Cancel',
          inputPattern: /\S+/,
          inputErrorMessage: 'Folder name is required'
        })

        const name = String(value || '').trim()
        if (!name) return

        const response = await api.post('/AI/chat/folder/create', { name })
        if (response.data && response.data.status_code === 1000) {
          await loadSessions()
          ElMessage.success('Folder created')
          return
        }

        ElMessage.error(response.data?.status_msg || 'Create folder failed')
      } catch (error) {
        if (error !== 'cancel' && error !== 'close') {
          console.error('Create folder error:', error)
          ElMessage.error('Create folder failed')
        }
      }
    }

    const renameFolder = async (folder) => {
      if (!folder?.id) return
      try {
        const { value } = await ElMessageBox.prompt('Enter a new folder name', 'Rename Folder', {
          confirmButtonText: 'OK',
          cancelButtonText: 'Cancel',
          inputValue: folder.name || '',
          inputPattern: /\S+/,
          inputErrorMessage: 'Folder name is required'
        })
        const name = String(value || '').trim()
        if (!name) return

        const response = await api.post('/AI/chat/folder/rename', {
          folderId: Number(folder.id),
          name
        })
        if (response.data?.status_code === 1000) {
          await loadSessions()
          ElMessage.success('Folder renamed')
          return
        }
        ElMessage.error(response.data?.status_msg || 'Rename folder failed')
      } catch (error) {
        if (error !== 'cancel' && error !== 'close') {
          console.error('Rename folder error:', error)
          ElMessage.error('Rename folder failed')
        }
      }
    }

    const deleteFolder = async (folder) => {
      if (!folder?.id) return
      try {
        await ElMessageBox.confirm(
          `Delete folder "${folder.name || folder.id}"? Sessions will become ungrouped.`,
          'Delete Folder',
          {
            confirmButtonText: 'Delete',
            cancelButtonText: 'Cancel',
            type: 'warning'
          }
        )

        const response = await api.post('/AI/chat/folder/delete', {
          folderId: Number(folder.id)
        })
        if (response.data?.status_code === 1000) {
          await loadSessions()
          ElMessage.success('Folder deleted')
          return
        }
        ElMessage.error(response.data?.status_msg || 'Delete folder failed')
      } catch (error) {
        if (error !== 'cancel' && error !== 'close') {
          console.error('Delete folder error:', error)
          ElMessage.error('Delete folder failed')
        }
      }
    }

    const promptTargetFolderId = async () => {
      const folders = sessionFolders.value || []
      if (!folders.length) {
        ElMessage.warning('Create a folder first')
        return null
      }

      const hint = folders.map(item => `${item.id}:${item.name}`).join(' | ')
      const { value } = await ElMessageBox.prompt(
        `Choose target folder id: ${hint}`,
        'Move Session',
        {
          confirmButtonText: 'Move',
          cancelButtonText: 'Cancel',
          inputPattern: /^\d+$/,
          inputErrorMessage: 'Enter a numeric folder id'
        }
      )

      const folderId = Number(value)
      if (!Number.isInteger(folderId)) {
        ElMessage.error('Invalid folder id')
        return null
      }
      const exists = folders.some(item => Number(item.id) === folderId)
      if (!exists) {
        ElMessage.error('Folder id does not exist')
        return null
      }
      return folderId
    }

    const moveSessionItem = async (session) => {
      if (!session?.id) return
      try {
        const folderId = await promptTargetFolderId()
        if (!folderId) return

        const response = await api.post('/AI/chat/session/move', {
          sessionId: String(session.id),
          folderId
        })
        if (response.data?.status_code === 1000) {
          await loadSessions()
          ElMessage.success('Session moved')
          return
        }
        ElMessage.error(response.data?.status_msg || 'Move session failed')
      } catch (error) {
        if (error !== 'cancel' && error !== 'close') {
          console.error('Move session error:', error)
          ElMessage.error('Move session failed')
        }
      }
    }

    const removeSessionItemFromFolder = async (session) => {
      if (!session?.id) return
      try {
        const response = await api.post('/AI/chat/session/remove-from-folder', {
          sessionId: String(session.id)
        })
        if (response.data?.status_code === 1000) {
          await loadSessions()
          ElMessage.success('Session removed from folder')
          return
        }
        ElMessage.error(response.data?.status_msg || 'Remove from folder failed')
      } catch (error) {
        console.error('Remove from folder error:', error)
        ElMessage.error('Remove from folder failed')
      }
    }

    const renameSessionItem = async (session) => {
      if (!session?.id) return
      try {
        const { value } = await ElMessageBox.prompt('Enter a new session title', 'Rename Session', {
          confirmButtonText: 'OK',
          cancelButtonText: 'Cancel',
          inputValue: session.name || '',
          inputPattern: /\S+/,
          inputErrorMessage: 'Session title is required'
        })
        const title = String(value || '').trim()
        if (!title) return

        const response = await api.post('/AI/chat/session/rename', {
          sessionId: String(session.id),
          title
        })
        if (response.data?.status_code === 1000) {
          await loadSessions()
          if (sessions.value[String(session.id)]) {
            sessions.value[String(session.id)].name = title
          }
          ElMessage.success('Session renamed')
          return
        }
        ElMessage.error(response.data?.status_msg || 'Rename session failed')
      } catch (error) {
        if (error !== 'cancel' && error !== 'close') {
          console.error('Rename session error:', error)
          ElMessage.error('Rename session failed')
        }
      }
    }

    const deleteSessionItem = async (session) => {
      if (!session?.id) return
      try {
        await ElMessageBox.confirm(
          `Delete session "${session.name || session.id}"?`,
          'Delete Session',
          {
            confirmButtonText: 'Delete',
            cancelButtonText: 'Cancel',
            type: 'warning'
          }
        )

        const response = await api.post('/AI/chat/session/delete', {
          sessionId: String(session.id)
        })
        if (response.data?.status_code === 1000) {
          const deletedId = String(session.id)
          if (currentSessionId.value === deletedId) {
            createNewSession()
          }
          await loadSessions()
          ElMessage.success('Session deleted')
          return
        }
        ElMessage.error(response.data?.status_msg || 'Delete session failed')
      } catch (error) {
        if (error !== 'cancel' && error !== 'close') {
          console.error('Delete session error:', error)
          ElMessage.error('Delete session failed')
        }
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
        ElMessage.warning('Please select an existing session first')
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
        ElMessage.success('Stopped current generation')
      }
    }

    const sendMessage = async () => {
      if (!inputMessage.value || !inputMessage.value.trim()) {
        ElMessage.warning('Please enter a message')
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
        ElMessage.error(error.message || 'Send failed, please try again')

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
          throw new Error(response.data?.status_msg || 'Send failed')
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
          throw new Error(response.data?.status_msg || 'Send failed')
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
        ElMessage.error('只支持上传 .md 或 .txt 文件')
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
        content: `已上传图片 ${file.name}`,
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
          ElMessage.success('Profile updated')
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
        await ElMessageBox.confirm('Are you sure you want to log out?', 'Confirm', {
          confirmButtonText: 'OK',
          cancelButtonText: 'Cancel',
          type: 'warning'
        })
        await api.post('/user/logout')
        clearTokens()
        ElMessage.success('Logged out')
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
      
      // -- Folders & Session --
      foldersList,
      ungroupedSessionsList,
      collapsedFolders,
      folderDialogVisible,
      folderDialogType,
      folderForm,
      submittingFolder,
      renameSessionDialogVisible,
      renameSessionForm,
      submittingRenameSession,
      moveSessionDialogVisible,
      moveSessionForm,
      submittingMoveSession,

      toggleFolder,
      showCreateFolderDialog,
      handleFolderCommand,
      submitFolderDialog,
      handleSessionCommand,
      submitRenameSession,
      submitMoveSession,
      // ----------------------
      sidebarFolders,
      ungroupedSessions,
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
      isFolderExpanded,
      toggleUserMenu,
      playTTS,
      createFolder,
      renameFolder,
      deleteFolder,
      moveSessionItem,
      removeSessionItemFromFolder,
      renameSessionItem,
      deleteSessionItem,
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
