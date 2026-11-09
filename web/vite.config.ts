import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Saat pengembangan, permintaan /api dan /health diteruskan ke API Go di port 8080.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
    },
  },
})
