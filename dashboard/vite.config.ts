import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: '/', // Served at / by Go
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:9867',
      '/health': 'http://localhost:9867',
    },
  },
  build: {
    outDir: 'dist',
  },
})
