import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'

const apiProxyTarget = process.env.ARKLOOP_API_PROXY_TARGET ?? 'http://127.0.0.1:8000'

// https://vite.dev/config/
export default defineConfig({
  plugins: [tailwindcss(), react()],
  server: {
    proxy: {
      '/v1': {
        target: apiProxyTarget,
        changeOrigin: true,
      },
    },
  },
})
