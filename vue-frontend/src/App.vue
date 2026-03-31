<template>
  <div id="app" class="flex flex-row h-screen w-screen overflow-hidden bg-bg-light dark:bg-bg-dark text-text-primary-light dark:text-text-primary-dark font-sans transition-colors duration-300">
    <router-view v-slot="{ Component }">
      <transition name="page" mode="out-in">
        <component :is="Component" />
      </transition>
    </router-view>
  </div>
</template>

<script>
export default {
  name: 'App'
}
</script>

<style>
/* 页面切换动画 */
.page-enter-active,
.page-leave-active {
  transition: all 0.4s cubic-bezier(0.55, 0, 0.1, 1);
}

.page-enter-from {
  opacity: 0;
  transform: translateX(30px);
}

.page-leave-to {
  opacity: 0;
  transform: translateX(-30px);
}

/* 响应式动画调整 */
@media (max-width: 768px) {
  .page-enter-from,
  .page-leave-to {
    transform: translateX(0);
    opacity: 0;
  }

  .page-enter-active,
  .page-leave-active {
    transition: opacity 0.3s ease;
  }
}

/* 全局滚动条样式 */
::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: rgb(0 0 0 / 10%);
  border-radius: 9999px;
  transition: background-color 0.2s ease;
}

::-webkit-scrollbar-thumb:hover {
  background: rgb(0 0 0 / 20%);
}

.dark ::-webkit-scrollbar-thumb {
  background: rgb(255 255 255 / 10%);
}

.dark ::-webkit-scrollbar-thumb:hover {
  background: rgb(255 255 255 / 20%);
}
</style>
