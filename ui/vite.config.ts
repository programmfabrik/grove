import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Base './' so the built assets work embedded in the Go binary and served
// from any path. `npm run dev` proxies /api to a grove started separately
// (default :80 — pass its -addr here if you moved it).
export default defineConfig({
  plugins: [react()],
  base: './',
  build: { outDir: 'dist', emptyOutDir: true },
  server: {
    proxy: { '/api': 'http://localhost:80' },
  },
})
