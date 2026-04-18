import { ref } from 'vue'

export function useSidebarUI() {
  const isSidebarCollapsed = ref(false)
  const userMenuVisible = ref(false)

  const toggleSidebar = () => {
    isSidebarCollapsed.value = !isSidebarCollapsed.value
  }

  const toggleUserMenu = () => {
    userMenuVisible.value = !userMenuVisible.value
  }

  return { isSidebarCollapsed, userMenuVisible, toggleSidebar, toggleUserMenu }
}