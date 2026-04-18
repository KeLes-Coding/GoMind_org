<template>
  <el-dialog v-model="visible" :title="folderDialogType === 'create' ? '新建文件夹' : '重命名文件夹'" width="400px" class="folder-dialog">
    <div class="space-y-4">
      <div class="space-y-2">
        <label class="block text-sm dark:text-text-primary-dark">文件夹名称</label>
        <input :value="folderForm.name" type="text" maxlength="50" placeholder="请输入文件夹名称" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent dark:bg-surface-dark outline-none dark:text-text-primary-dark" @input="onNameInput" @keydown.enter="handleSubmit" />
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="handleCancel">取消</button>
        <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="handleSubmit" :disabled="!folderForm.name.trim() || submittingFolder">确认</button>
      </div>
    </template>
  </el-dialog>
</template>

<script>
/* eslint-env node */
/* eslint-disable vue/no-mutating-props */
import { computed } from 'vue'

export default {
  props: {
    modelValue: { type: Boolean, default: false },
    folderDialogType: { type: String, default: 'create' },
    folderForm: { type: Object, default: () => ({ name: '' }) },
    submittingFolder: { type: Boolean, default: false }
  },
  emits: ['update:modelValue', 'submit'],
  setup(props, { emit }) {
    const visible = computed({
      get: () => props.modelValue,
      set: (val) => emit('update:modelValue', val)
    })

    const onNameInput = (e) => {
      props.folderForm.name = e.target.value
    }

    const handleCancel = () => {
      visible.value = false
    }

    const handleSubmit = () => {
      emit('submit')
    }

    return { visible, onNameInput, handleCancel, handleSubmit }
  }
}
</script>