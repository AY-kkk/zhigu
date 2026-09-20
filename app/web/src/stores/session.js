import { defineStore } from 'pinia'

export const useSession = defineStore('session', {
  state: () => ({
    token: localStorage.getItem('zhigu_token') || '',
    role: localStorage.getItem('zhigu_role') || '',
    username: localStorage.getItem('zhigu_user') || ''
  }),
  actions: {
    setAuth({ token, role, username }) {
      this.token = token
      this.role = role
      this.username = username
      localStorage.setItem('zhigu_token', token)
      localStorage.setItem('zhigu_role', role)
      localStorage.setItem('zhigu_user', username)
    },
    logout() {
      this.token = ''
      this.role = ''
      this.username = ''
      localStorage.removeItem('zhigu_token')
      localStorage.removeItem('zhigu_role')
      localStorage.removeItem('zhigu_user')
    }
  }
})
