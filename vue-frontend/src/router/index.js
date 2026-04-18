import { createRouter, createWebHistory } from 'vue-router'
import HomePage from '../views/HomePage.vue'
import Login from '../views/Login.vue'
import Register from '../views/Register.vue'
import AIChat from '../views/AIChat.vue'
import { hasSession } from '../utils/token'

const skipAuth = process.env.VUE_APP_SKIP_AUTH === 'true'

const routes = [
  {
    path: '/',
    name: 'Home',
    component: HomePage
  },
  {
    path: '/login',
    name: 'Login',
    component: Login
  },
  {
    path: '/register',
    name: 'Register',
    component: Register
  },
  {
    path: '/ai-chat',
    name: 'AIChat',
    component: AIChat,
    meta: { requiresAuth: true }
  }
]

const router = createRouter({
  history: createWebHistory(process.env.BASE_URL),
  routes
})

router.beforeEach((to, from, next) => {
  if (skipAuth) {
    next()
    return
  }

  if (to.path === '/' && hasSession()) {
    next('/ai-chat')
    return
  }

  if (to.matched.some(record => record.meta.requiresAuth) && !hasSession()) {
    next('/')
    return
  }
  next()
})

export default router