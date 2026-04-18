<template>
  <el-dialog v-model="dialogVisible" title="重命名会话" width="400px" class="rename-session-dialog">
    <div class="space-y-4">
      <div class="space-y-2">
        <label class="block text-sm dark:text-text-primary-dark">会话名称</label>
        <input :value="renameSessionForm.name" type="text" maxlength="50" placeholder="请输入会话名称" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent dark:bg-surface-dark outline-none dark:text-text-primary-dark" @input="onNameInput" @keydown.enter="handleSubmit" />
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="handleCancel">取消</button>
        <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="handleSubmit" :disabled="!renameSessionForm.name.trim() || submittingRenameSession">确认</button>
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
    modelValue: {
      type: Boolean,
      required: true
    },
    renameSessionForm: {
      type: Object,
      required: true
    },
    submittingRenameSession: {
      type: Boolean,
      default: false
    }
  },
  emits: ['update:modelValue', 'submit'],
  setup(props, { emit }) {
    const dialogVisible = computed({
      get() {
        return props.modelValue
      },
      set(value) {
        emit('update:modelValue', value)
      }
    })

    const onNameInput = (e) => {
      props.renameSessionForm.name = e.target.value
    }

    function handleCancel() {
      emit('update:modelValue', false)
    }

    function handleSubmit() {
      emit('submit')
    }

    return { dialogVisible, onNameInput, handleCancel, handleSubmit }
  }
}
</script>