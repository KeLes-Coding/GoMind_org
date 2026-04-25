<template>
  <div class="flex flex-row h-screen w-screen overflow-hidden bg-bg-light dark:bg-bg-dark text-text-primary-light dark:text-text-primary-dark selection:bg-accent-light selection:text-white">
    <ChatSidebar
      :is-sidebar-collapsed="isSidebarCollapsed"
      :user-menu-visible="userMenuVisible"
      :folders-list="foldersList"
      :collapsed-folders="collapsedFolders"
      :ungrouped-sessions-list="ungroupedSessionsList"
      :current-session-id="currentSessionId"
      :user-profile="userProfile"
      :loading="loading"
      :temp-session="tempSession"
      @toggle-sidebar="toggleSidebar"
      @toggle-user-menu="toggleUserMenu"
      @create-new-session="createNewSessionAndFocus"
      @toggle-folder="toggleFolder"
      @handle-folder-command="({ command, folder }) => handleFolderCommand(command, folder)"
      @switch-session="switchSessionWithGuard"
      @handle-session-command="({ command, session }) => handleSessionCommand(command, session)"
      @show-create-folder-dialog="showCreateFolderDialog"
      @handle-settings="handleSettings"
      @open-model-config-dialog="openModelConfigDialog"
      @open-file-manager-dialog="openFileManagerDialog"
      @handle-sync-history="handleSyncHistoryFromMenu"
      @handle-logout="handleLogout"
      @go-login="goLogin"
    />

    <section class="flex-1 flex flex-col relative min-w-0 bg-bg-light dark:bg-bg-dark overflow-hidden">
      <ChatHeader
        :has-messages="currentMessages.length > 0"
        :is-dark="isDark"
        :loading="loading"
        :selected-config-id="selectedConfigId"
        @update:selected-config-id="val => selectedConfigId = val"
        :available-configs="availableConfigs"
        @config-change="onConfigChange"
        @open-model-config-dialog="openModelConfigDialog"
        @toggle-theme="toggleTheme"
      />

      <ChatEmptyState v-if="currentMessages.length === 0" />

      <ChatMessageList
        v-else
        ref="chatMessageListRef"
        :current-messages="currentMessages"
        :user-profile="userProfile"
        @play-tts="playTTS"
      />

      <ChatInputArea
        ref="chatInputAreaRef"
        :input-message="inputMessage"
        @update:input-message="val => inputMessage = val"
        :loading="loading"
        :uploading="uploading"
        :selected-chat-mode="selectedChatMode"
        @update:selected-chat-mode="val => selectedChatMode = val"
        :available-chat-modes="availableChatModes"
        :chat-mode-label="chatModeLabel"
        @send-message="sendMessage"
        @stop-stream="stopCurrentStream"
        @file-upload="handleFileUpload"
        @image-recognition="handleImageRecognition"
      />

      <SettingsDialog
        v-model="settingsVisible"
        :user-profile="userProfile"
        :profile-form="profileForm"
        :uploading-avatar="uploadingAvatar"
        :saving-profile="savingProfile"
        @save="saveProfile"
        @trigger-avatar-upload="triggerAvatarUpload"
        @avatar-upload="handleAvatarUpload"
      />
      <CropAvatarDialog
        v-model="cropDialogVisible"
        :crop-scale="cropScale"
        :crop-offset-x="cropOffsetX"
        :crop-offset-y="cropOffsetY"
        @update:cropScale="val => cropScale = val"
        @update:cropOffsetX="val => cropOffsetX = val"
        @update:cropOffsetY="val => cropOffsetY = val"
        :crop-preview-url="cropPreviewUrl"
        :crop-image-style="cropImageStyle"
        :uploading-avatar="uploadingAvatar"
        @cancel="cancelAvatarCrop"
        @confirm="confirmAvatarCrop"
      />
      <FolderDialog
        v-model="folderDialogVisible"
        :folder-dialog-type="folderDialogType"
        :folder-form="folderForm"
        :submitting-folder="submittingFolder"
        @submit="submitFolderDialog"
      />
      <RenameSessionDialog
        v-model="renameSessionDialogVisible"
        :rename-session-form="renameSessionForm"
        :submitting-rename-session="submittingRenameSession"
        @submit="submitRenameSession"
      />
      <MoveSessionDialog
        v-model="moveSessionDialogVisible"
        :move-session-form="moveSessionForm"
        :submitting-move-session="submittingMoveSession"
        :folders-list="foldersList"
        @submit="submitMoveSession"
      />

      <!-- Model Config Dialog -->
      <ModelConfigDialog v-model="modelConfigDialogVisible" @configsChanged="refreshConfigs" />

      <!-- File Manager Dialog -->
      <FileManagerDialog v-model="fileManagerVisible" />

    </section>
  </div>
</template>

<script>
import { nextTick, onMounted, ref, watchEffect } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import api from '../utils/api'
import { clearTokens } from '../utils/token'
import { useTheme } from '../composables/useTheme'
import { useSidebarUI } from '../composables/useSidebarUI'
import { useTTS } from '../composables/useTTS'
import { useModelConfig } from '../composables/useModelConfig'
import { useSessionStore } from '../composables/useSessionStore'
import { useFolderManager } from '../composables/useFolderManager'
import { useSessionManager } from '../composables/useSessionManager'
import { useUserProfile } from '../composables/useUserProfile'
import { useChatStream } from '../composables/useChatStream'
import { useFileUpload } from '../composables/useFileUpload'
import ChatSidebar from './components/ChatSidebar.vue'
import ChatHeader from './components/ChatHeader.vue'
import ChatEmptyState from './components/ChatEmptyState.vue'
import ChatMessageList from './components/ChatMessageList.vue'
import ChatInputArea from './components/ChatInputArea.vue'
import ModelConfigDialog from './components/ModelConfigDialog.vue'
import FileManagerDialog from './components/FileManagerDialog.vue'
import SettingsDialog from './components/SettingsDialog.vue'
import CropAvatarDialog from './components/CropAvatarDialog.vue'
import FolderDialog from './components/FolderDialog.vue'
import RenameSessionDialog from './components/RenameSessionDialog.vue'
import MoveSessionDialog from './components/MoveSessionDialog.vue'

export default {
  name: 'AIChat',
  components: {
    ChatSidebar, ChatHeader, ChatEmptyState, ChatMessageList, ChatInputArea,
    ModelConfigDialog, FileManagerDialog, SettingsDialog, CropAvatarDialog,
    FolderDialog, RenameSessionDialog, MoveSessionDialog
  },
  setup() {
    const router = useRouter()
    const chatMessageListRef = ref(null)
    const chatInputAreaRef = ref(null)

    const { isDark, toggleTheme } = useTheme()
    const { isSidebarCollapsed, userMenuVisible, toggleSidebar, toggleUserMenu } = useSidebarUI()
    const { playTTS } = useTTS()
    const {
      modelConfigDialogVisible,
      availableConfigs,
      selectedConfigId,
      selectedChatMode,
      availableChatModes,
      chatModeLabel,
      onConfigChange,
      loadConfigsMeta,
      loadAvailableConfigs,
      refreshConfigs,
      openModelConfigDialog,
      fileManagerVisible,
      openFileManagerDialog
    } = useModelConfig({ userMenuVisible })
    const {
      sessions,
      foldersList,
      ungroupedSessionsList,
      sessionFolders,
      collapsedFolders,
      currentSessionId,
      tempSession,
      currentMessages,
      messagesRef,
      toggleFolder,
      buildSessionRoutingHeaders,
      buildSessionTitle,
      scrollToBottom,
      ensureSessionEntry,
      upsertSessionEntry,
      syncSessionMessagesFromCurrent,
      loadSessions,
      createNewSession,
      ensureActiveDraftSession,
      switchSession,
      syncHistory
    } = useSessionStore()

    const {
      folderDialogVisible,
      folderDialogType,
      folderForm,
      submittingFolder,
      showCreateFolderDialog,
      handleFolderCommand,
      submitFolderDialog
    } = useFolderManager({ loadSessions })

    const {
      renameSessionDialogVisible,
      renameSessionForm,
      submittingRenameSession,
      moveSessionDialogVisible,
      moveSessionForm,
      submittingMoveSession,
      handleSessionCommand,
      submitRenameSession,
      submitMoveSession
    } = useSessionManager({ currentSessionId, currentMessages, tempSession, loadSessions, createNewSession, sessionFolders })

    const {
      userProfile,
      profileForm,
      savingProfile,
      settingsVisible,
      uploadingAvatar,
      cropDialogVisible,
      cropPreviewUrl,
      cropScale,
      cropOffsetX,
      cropOffsetY,
      cropImageStyle,
      fetchUserProfile,
      handleSettings,
      saveProfile,
      triggerAvatarUpload,
      handleAvatarUpload,
      cancelAvatarCrop,
      confirmAvatarCrop
    } = useUserProfile({ userMenuVisible })

    const {
      inputMessage,
      loading,
      messageInput,
      isStreaming,
      stopCurrentStream,
      sendMessage
    } = useChatStream({
      sessions,
      currentSessionId,
      tempSession,
      currentMessages,
      syncSessionMessagesFromCurrent,
      ensureSessionEntry,
      upsertSessionEntry,
      ensureActiveDraftSession,
      scrollToBottom,
      buildSessionRoutingHeaders,
      buildSessionTitle,
      loadSessions,
      selectedConfigId,
      selectedChatMode
    })

    const {
      uploading,
      fileInput,
      imageInput,
      handleFileUpload,
      handleImageRecognition
    } = useFileUpload({ currentMessages, loading, scrollToBottom })

    // Proxy child component internal refs to composable refs
    watchEffect(() => {
      if (chatMessageListRef.value) {
        messagesRef.value = chatMessageListRef.value.scrollContainer
      }
      if (chatInputAreaRef.value) {
        messageInput.value = chatInputAreaRef.value.messageInput
        fileInput.value = chatInputAreaRef.value.fileInput
        imageInput.value = chatInputAreaRef.value.imageInput
      }
    })

    const switchSessionWithGuard = (sessionId) => {
      if (loading.value) return
      switchSession(sessionId)
    }

    const createNewSessionAndFocus = () => {
      createNewSession()
      nextTick(() => {
        if (messageInput.value) {
          messageInput.value.focus()
        }
      })
    }

    const handleSyncHistoryFromMenu = async () => {
      userMenuVisible.value = false
      await syncHistory()
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
        router.push('/')
      } catch {
        // user cancelled
      }
    }

    const goLogin = () => {
      router.push('/login')
    }

    onMounted(() => {
      loadSessions()
      fetchUserProfile()
      loadConfigsMeta()
      loadAvailableConfigs()
    })

    return {
      chatMessageListRef,
      chatInputAreaRef,
      isSidebarCollapsed,
      isDark,
      foldersList,
      ungroupedSessionsList,
      collapsedFolders,
      currentSessionId,
      tempSession,
      currentMessages,
      inputMessage,
      loading,
      isStreaming,
      uploading,
      userProfile,
      userMenuVisible,
      toggleSidebar,
      toggleUserMenu,
      toggleTheme,
      toggleFolder,
      switchSessionWithGuard,
      createNewSessionAndFocus,
      showCreateFolderDialog,
      handleFolderCommand,
      handleSessionCommand,
      playTTS,
      sendMessage,
      stopCurrentStream,
      handleFileUpload,
      handleImageRecognition,
      selectedConfigId,
      selectedChatMode,
      availableConfigs,
      availableChatModes,
      chatModeLabel,
      onConfigChange,
      openModelConfigDialog,
      refreshConfigs,
      modelConfigDialogVisible,
      fileManagerVisible,
      openFileManagerDialog,
      handleSyncHistoryFromMenu,
            handleLogout,
      goLogin,
      handleSettings,
      // Dialog states
      folderDialogVisible,
      folderDialogType,
      folderForm,
      submittingFolder,
      submitFolderDialog,
      renameSessionDialogVisible,
      renameSessionForm,
      submittingRenameSession,
      submitRenameSession,
      moveSessionDialogVisible,
      moveSessionForm,
      submittingMoveSession,
      submitMoveSession,
      settingsVisible,
      profileForm,
      savingProfile,
      uploadingAvatar,
      cropDialogVisible,
      cropPreviewUrl,
      cropScale,
      cropOffsetX,
      cropOffsetY,
      cropImageStyle,
      triggerAvatarUpload,
      handleAvatarUpload,
      cancelAvatarCrop,
      confirmAvatarCrop,
      saveProfile
    }
  }
}
</script>
