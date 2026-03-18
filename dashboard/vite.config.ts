/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig(() => {
  const backendPort = process.env.PINCHTAB_DEV_PORT || '9867'
  const backendUrl = `http://localhost:${backendPort}`

  // In dev mode, proxy all API routes to the Go backend.
  // Add new top-level API paths here as they're created.
  const apiPaths = [
    '/api', '/health', '/metrics', '/instances', '/profiles', '/tabs',
    '/navigate', '/action', '/screenshot', '/evaluate', '/find', '/text',
    '/snapshot', '/download', '/upload', '/cookies', '/fingerprint', '/scheduler',
  ]
  const proxy: Record<string, string> = {}
  for (const path of apiPaths) {
    proxy[path] = backendUrl
  }

  return {
  plugins: [react(), tailwindcss()],
  base: '/dashboard/', // Served at /dashboard by Go
  server: {
    port: 5173,
    proxy,
  },
  build: {
    outDir: 'dist',
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-ui': ['recharts'],
          'vendor-react': ['react', 'react-dom', 'react-router-dom', 'zustand'],
        },
      },
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
  }
})
