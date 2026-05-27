import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const apiProxyTarget = process.env.VITE_API_PROXY_TARGET || 'http://localhost:8080'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      'lottie-web': 'lottie-web/build/player/lottie_light.js',
    },
  },
  server: {
    proxy: {
      '/api': {
        target: apiProxyTarget,
        changeOrigin: true,
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/react') || id.includes('node_modules/react-dom') || id.includes('node_modules/react-router-dom')) {
            return 'vendor-react'
          }
          if (id.includes('node_modules/lottie-web')) {
            return 'vendor-lottie'
          }
          if (id.includes('node_modules/@douyinfe')) {
            return 'vendor-semi'
          }
          // @visactor(VChart)只被懒加载的 StatsBoard 引用:返回 undefined 让它跟随
          // 异步 chunk,而不是被塞进 eager 的 catch-all vendor(否则首屏白白拉 600KB+)。
          if (id.includes('node_modules/@visactor')) {
            return undefined
          }
          if (id.includes('node_modules')) {
            return 'vendor'
          }
        },
      },
    },
  },
})
