import { defineStore } from 'pinia'
import http, { clearAuthCredential, hasAuthCredential, setBearerToken, setCsrfToken } from '../utils/http.js'

let sessionRole = ''

export function getSessionRole() {
  return sessionRole
}

export const useSession = defineStore('session', {
  state: () => ({
    token: '',
    role: '',
    username: '',
    userId: null,
    authenticated: false,
    checked: false,
  }),
  actions: {
    setAuth(payload = {}) {
      this.role = payload.role || ''
      sessionRole = this.role
      this.username = payload.username || ''
      this.userId = payload.user_id ?? null
      this.authenticated = true
      this.checked = true
      this.token = payload.token || ''
      if (payload.token) setBearerToken(payload.token)
    },
    setChecked(value) {
      this.checked = Boolean(value)
    },
    markAuthenticated() {
      this.authenticated = true
      this.checked = true
    },
    logout() {
      this.token = ''
      this.role = ''
      sessionRole = ''
      this.username = ''
      this.userId = null
      this.authenticated = false
      this.checked = true
      clearAuthCredential()
    },
    async bootstrapSession() {
      if (hasAuthCredential()) {
        this.markAuthenticated()
        return true
      }
      try {
        const response = await http.get('/api/v1/session/csrf')
        const token = response.headers?.['x-csrf-token'] || response.data?.data?.csrf_token || response.data?.csrf_token
        if (token) setCsrfToken(token)
        this.markAuthenticated()
        return true
      } catch {
        this.logout()
        return false
      }
    },
  },
})
