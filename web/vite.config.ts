import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

// Saat pengembangan, permintaan /api dan /health diteruskan ke API Go di port 8080.
// VITE_API_TARGET mengganti tujuan itu, misalnya untuk API tiruan saat menguji.
export default defineConfig(({ mode }) => {
  const target = loadEnv(mode, '.', 'VITE_').VITE_API_TARGET || 'http://localhost:8080'
  return {
    plugins: [react()],
    server: {
      // changeOrigin stays false so the API sees the same Host as the browser's Origin;
      // its same-origin check would refuse sign-in otherwise.
      proxy: {
        '/api': { target, changeOrigin: false },
        '/health': { target, changeOrigin: false },
      },
    },
  }
})
