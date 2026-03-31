import axios from 'axios'
import { clearTokens, getAccessToken, getRefreshToken, redirectToLogin, refreshAccessToken } from './token'

const api = axios.create({
  baseURL: '/api',
  timeout: 0
})

const refreshClient = axios.create({
  baseURL: '/api',
  timeout: 0
})

api.interceptors.request.use(
  config => {
    const token = getAccessToken()
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  error => Promise.reject(error)
)

api.interceptors.response.use(
  async response => {
    const originalRequest = response.config || {}
    const invalidToken = response.data?.status_code === 2006
    const isRefreshRequest = String(originalRequest.url || '').includes('/user/refresh')

    if (invalidToken && !originalRequest._retry && !isRefreshRequest && getRefreshToken()) {
      originalRequest._retry = true
      try {
        const nextAccessToken = await refreshAccessToken(refreshClient)
        originalRequest.headers = originalRequest.headers || {}
        originalRequest.headers.Authorization = `Bearer ${nextAccessToken}`
        return api(originalRequest)
      } catch (error) {
        clearTokens()
        redirectToLogin()
        return Promise.reject(error)
      }
    }

    if (invalidToken && isRefreshRequest) {
      clearTokens()
      redirectToLogin()
    }

    return response
  },
  error => {
    if (error.response && error.response.status === 401) {
      clearTokens()
      redirectToLogin()
    }
    return Promise.reject(error)
  }
)

export { refreshClient }
export default api
