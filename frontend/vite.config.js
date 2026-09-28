import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import { DEV_SERVER_ORIGIN, devApiProxy } from './devProxy.js'

const devServer = new URL(DEV_SERVER_ORIGIN)

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(import.meta.dirname, './src')
    }
  },
  server: {
    // 只在回环上监听；页面源必须是 DEV_SERVER_ORIGIN，代理才会改写它的 Origin
    host: devServer.hostname,
    port: Number(devServer.port),
    strictPort: true,
    proxy: {
      '/api': devApiProxy()
    }
  }
})
