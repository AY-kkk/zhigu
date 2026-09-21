import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath } from 'node:url'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: [
      {
        find: /^@kousum\/semi-ui-vue$/,
        replacement: fileURLToPath(new URL('./node_modules/@kousum/semi-ui-vue/dist/index.js', import.meta.url))
      }
    ]
  },
  server: {
    host: true,
    port: 5173,
    proxy: {
      '/api': process.env.ZHIGU_API_PROXY || 'http://127.0.0.1:8080'
    }
  }
})
