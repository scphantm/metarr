import path from 'node:path'
import { readFileSync } from 'node:fs'

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The Go API sends no CORS headers, so the browser can only reach it through
// this proxy. Point it elsewhere with METARR_API_URL when the API is not on
// localhost.
const apiTarget = process.env.METARR_API_URL ?? 'http://localhost:8080'

// Read the app version from the repo-root VERSION file.
const appVersion = readFileSync(path.resolve(__dirname, '../VERSION'), 'utf-8').trim()

export default defineConfig({
  plugins: [react()],
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
  },
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    port: 5173,
    proxy: {
      // ws:true so the /api/ws streaming connection is proxied too — without
      // it the upgrade request is served as a plain HTTP request and fails.
      '/api': { target: apiTarget, changeOrigin: true, ws: true },
      // gRPC-Web (Connect) traffic: every generated client posts to
      // /<package>.<Service>/<Method>, outside the /api prefix above — see
      // ui/src/api/transport.ts. Migrated domain by domain; REST and Connect
      // coexist under these two rules until the last domain moves over.
      '/metarr.v1.': { target: apiTarget, changeOrigin: true },
    },
  },
})
