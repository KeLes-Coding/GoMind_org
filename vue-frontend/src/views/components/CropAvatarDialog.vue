<template>
  <el-dialog v-model="dialogVisible" title="Crop avatar" width="560px" class="crop-avatar-dialog">
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
          <label class="block text-sm mb-2 dark:text-text-primary-dark">Zoom</label>
          <input :value="cropScale" type="range" min="1" max="3" step="0.01" class="w-full" @input="onScaleInput" />
        </div>
        <div>
          <label class="block text-sm mb-2 dark:text-text-primary-dark">Horizontal offset</label>
          <input :value="cropOffsetX" type="range" min="-120" max="120" step="1" class="w-full" @input="onOffsetXInput" />
        </div>
        <div>
          <label class="block text-sm mb-2 dark:text-text-primary-dark">Vertical offset</label>
          <input :value="cropOffsetY" type="range" min="-120" max="120" step="1" class="w-full" @input="onOffsetYInput" />
        </div>
      </div>
      <p class="text-xs text-text-secondary-light dark:text-text-secondary-dark">The crop ratio is fixed at 1:1 for avatar display.</p>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="px-3 py-2 rounded-lg bg-transparent border border-border-light dark:border-border-dark cursor-pointer" @click="handleCancel">Cancel</button>
        <button type="button" class="px-3 py-2 rounded-lg bg-black text-white dark:bg-white dark:text-black border-none cursor-pointer" @click="handleConfirm" :disabled="uploadingAvatar">{{ uploadingAvatar ? 'Uploading...' : 'Confirm upload' }}</button>
      </div>
    </template>
  </el-dialog>
</template>

<script>
/* eslint-env node */
import { computed } from 'vue'

export default {
  props: {
    modelValue: Boolean,
    cropPreviewUrl: String,
    cropImageStyle: Object,
    cropScale: Number,
    cropOffsetX: Number,
    cropOffsetY: Number,
    uploadingAvatar: Boolean
  },
  emits: [
    'update:modelValue',
    'update:cropScale',
    'update:cropOffsetX',
    'update:cropOffsetY',
    'cancel',
    'confirm'
  ],
  setup(props, { emit }) {
    const dialogVisible = computed({
      get: () => props.modelValue,
      set: (val) => emit('update:modelValue', val)
    })

    const onScaleInput = (e) => {
      emit('update:cropScale', Number(e.target.value))
    }

    const onOffsetXInput = (e) => {
      emit('update:cropOffsetX', Number(e.target.value))
    }

    const onOffsetYInput = (e) => {
      emit('update:cropOffsetY', Number(e.target.value))
    }

    function handleCancel() {
      emit('cancel')
    }

    function handleConfirm() {
      emit('confirm')
    }

    return { dialogVisible, onScaleInput, onOffsetXInput, onOffsetYInput, handleCancel, handleConfirm }
  }
}
</script>