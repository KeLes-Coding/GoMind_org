<template>
  <el-dialog v-model="dialogVisible" title="移动到文件夹" width="400px" class="move-session-dialog">
    <div class="space-y-4">
      <div class="space-y-2">
        <label class="block text-sm dark:text-text-primary-dark">选择目标文件夹</label>
        <select :value="moveSessionForm.folderId" class="w-full px-3 py-2 rounded-lg border border-border-light dark:border-border-dark bg-transparent dark:bg-surface-dark outline-none cursor-pointer dark:text-text-primary-dark" @change="onFolderChange">
          <option value="" class="bg-surface-light dark:bg-surface-dark">-- 改为独立会话 (移出文件夹) --</option>
          <option v-for="f in foldersList" :key="f.id" :value="f.id" class="bg-surface-light dark:bg-surface-dark">{{ f.name }}</option>
        </select>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="dialogVisible = false">取消</button>
        <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="handleSubmit" :disabled="submittingMoveSession">确认</button>
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
    modelValue: Boolean,
    moveSessionForm: Object,
    submittingMoveSession: Boolean,
    foldersList: Array
  },
  emits: ['update:modelValue', 'submit'],
  setup(props, { emit }) {
    const dialogVisible = computed({
      get: () => props.modelValue,
      set: (val) => emit('update:modelValue', val)
    })

    const onFolderChange = (e) => {
      props.moveSessionForm.folderId = e.target.value
    }

    const handleSubmit = () => {
      emit('submit')
    }

    return { dialogVisible, onFolderChange, handleSubmit }
  }
}
</script>