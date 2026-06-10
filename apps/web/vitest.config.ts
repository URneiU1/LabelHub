import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'happy-dom',
    setupFiles: ['./src/test/setup.ts'],
    // pool=threads(worker_threads):不要用 'vmForks' —— 它在 node 24 上用 vm 模块创建隔离
    // context + 在 vm 里编译模块极慢(单文件 setup 555s / environment 355s,总 ~929s,体感卡死)。
    // threads 复用 worker、不走 vm 隔离,同一文件 6.4s 跑完(实测快约 146×)。
    pool: 'threads',
  },
})
