import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../utils/api'

const CROP_PREVIEW_SIZE = 288
const CROP_OUTPUT_SIZE = 512

export function useUserProfile({ userMenuVisible }) {
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
  const savingProfile = ref(false)
  const settingsVisible = ref(false)
  const uploadingAvatar = ref(false)
  const cropDialogVisible = ref(false)
  const cropPreviewUrl = ref('')
  const cropScale = ref(1)
  const cropOffsetX = ref(0)
  const cropOffsetY = ref(0)
  const cropImageNaturalWidth = ref(0)
  const cropImageNaturalHeight = ref(0)
  const pendingAvatarFile = ref(null)
  const avatarInput = ref(null)

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

  const handleSettings = async () => {
    userMenuVisible.value = false
    settingsVisible.value = true
    await fetchUserProfile()
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

  return {
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
    avatarInput,
    cropImageStyle,
    applyUserProfile,
    fetchUserProfile,
    getUserDisplayName,
    getUserInitial,
    handleSettings,
    saveProfile,
    triggerAvatarUpload,
    handleAvatarUpload,
    cancelAvatarCrop,
    confirmAvatarCrop
  }
}