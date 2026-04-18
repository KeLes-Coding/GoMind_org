<template>
  <el-dialog v-model="visible" title="个人设置" width="520px" class="settings-dialog">
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
          {{ getUserInitial }}
        </div>
        <div class="flex flex-col gap-2">
          <button
            type="button"
            class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer"
            @click="handleAvatarUploadClick"
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
          @change="handleAvatarChange"
        />
      </div>

      <div class="space-y-2">
        <label class="block text-sm dark:text-text-primary-dark">Display name</label>
        <input :value="profileForm.name" type="text" maxlength="50" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent dark:bg-surface-dark outline-none dark:text-text-primary-dark" @input="onNameInput" />
      </div>

      <div class="space-y-2">
        <label class="block text-sm dark:text-text-primary-dark">Username</label>
        <input :value="userProfile.username || ''" type="text" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-black/5 dark:bg-white/5 outline-none dark:text-text-secondary-dark" disabled />
      </div>

      <div class="space-y-2">
        <label class="block text-sm dark:text-text-primary-dark">Email</label>
        <input :value="userProfile.email || ''" type="text" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-black/5 dark:bg-white/5 outline-none dark:text-text-secondary-dark" disabled />
      </div>

      <div class="space-y-2">
        <label class="block text-sm dark:text-text-primary-dark">Bio</label>
        <textarea :value="profileForm.bio" rows="4" maxlength="255" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent dark:bg-surface-dark outline-none resize-none dark:text-text-primary-dark" @input="onBioInput"></textarea>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="handleCancel">Cancel</button>
        <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="handleSave" :disabled="savingProfile">{{ savingProfile ? 'Saving...' : 'Save' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script>
/* eslint-env node */
/* eslint-disable vue/no-mutating-props */
import { computed, ref } from 'vue'

export default {
  props: {
    modelValue: { type: Boolean, default: false },
    userProfile: { type: Object, default: () => ({}) },
    profileForm: { type: Object, default: () => ({}) },
    uploadingAvatar: { type: Boolean, default: false },
    savingProfile: { type: Boolean, default: false }
  },
  emits: ['update:modelValue', 'save', 'trigger-avatar-upload', 'avatar-upload'],
  setup(props, { emit }) {
    const visible = computed({
      get: () => props.modelValue,
      set: (val) => emit('update:modelValue', val)
    })

    const avatarInput = ref(null)

    const getUserInitial = computed(() => {
      const name = props.userProfile.name || props.userProfile.username || 'U'
      return name.slice(0, 1).toUpperCase()
    })

    const onNameInput = (e) => {
      props.profileForm.name = e.target.value
    }

    const onBioInput = (e) => {
      props.profileForm.bio = e.target.value
    }

    const handleAvatarUploadClick = () => {
      emit('trigger-avatar-upload')
      avatarInput.value?.click()
    }

    const handleAvatarChange = (event) => {
      emit('avatar-upload', event)
    }

    const handleCancel = () => {
      visible.value = false
    }

    const handleSave = () => {
      emit('save')
    }

    return { visible, avatarInput, getUserInitial, onNameInput, onBioInput, handleAvatarUploadClick, handleAvatarChange, handleCancel, handleSave }
  }
}
</script>

<style scoped>
:deep(.el-dialog) {
  border-radius: 1rem;
}
:deep(.el-dialog__header) {
  border-bottom: 1px solid #e0e0e0;
  padding-bottom: 12px;
}
:deep(.el-dialog__footer) {
  border-top: 1px solid #e0e0e0;
  padding-top: 12px;
}
</style>

<style>
html.dark .settings-dialog .el-dialog__header {
  border-bottom-color: #333;
}
html.dark .settings-dialog .el-dialog__footer {
  border-top-color: #333;
}
</style>