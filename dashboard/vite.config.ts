import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: '/dashboard/', // Served at /dashboard/ by Go
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:18801',
      '/health': 'http://localhost:18801',
      '/dashboard/agents': 'http://localhost:18801',
      '/dashboard/events': 'http://localhost:18801',
    },
  },
  build: {
    outDir: 'dist',
  },
})
