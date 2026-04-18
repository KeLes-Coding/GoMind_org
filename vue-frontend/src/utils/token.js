const ACCESS_TOKEN_KEY = 'access_token'
const REFRESH_TOKEN_KEY = 'refresh_token'
const LEGACY_TOKEN_KEY = 'token'

let refreshPromise = null

export function getAccessToken() {
  return localStorage.getItem(ACCESS_TOKEN_KEY) || localStorage.getItem(LEGACY_TOKEN_KEY) || ''
}

export function getRefreshToken() {
  return localStorage.getItem(REFRESH_TOKEN_KEY) || ''
}

export function hasSession() {
  return Boolean(getAccessToken())
}

export function saveTokens(payload = {}) {
  const accessToken = payload.access_token || payload.token || ''
  const refreshToken = payload.refresh_token || ''

  if (accessToken) {
    localStorage.setItem(ACCESS_TOKEN_KEY, accessToken)
    localStorage.setItem(LEGACY_TOKEN_KEY, accessToken)
  }
  if (refreshToken) {
    localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken)
  }
}

export function clearTokens() {
  localStorage.removeItem(ACCESS_TOKEN_KEY)
  localStorage.removeItem(REFRESH_TOKEN_KEY)
  localStorage.removeItem(LEGACY_TOKEN_KEY)
}

export function redirectToLogin() {
  if (typeof window !== 'undefined' && window.location.pathname !== '/') {
    window.location.href = '/'
  }
}

export async function refreshAccessToken(refreshClient) {
  if (!getRefreshToken()) {
    throw new Error('missing refresh token')
  }
  if (!refreshPromise) {
    refreshPromise = refreshClient.post('/user/refresh', {
      refresh_token: getRefreshToken()
    }).then((response) => {
      if (response.data?.status_code !== 1000) {
        throw new Error(response.data?.status_msg || 'refresh failed')
      }
      saveTokens(response.data)
      return getAccessToken()
    }).finally(() => {
      refreshPromise = null
    })
  }
  return refreshPromise
}

export async function ensureAccessToken(refreshClient) {
  const accessToken = getAccessToken()
  if (accessToken) {
    return accessToken
  }
  return refreshAccessToken(refreshClient)
}
