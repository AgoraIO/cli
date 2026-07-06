import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import compression from 'vite-plugin-compression'

// agora-rtm is loaded from Agora's CDN as the UMD global `AgoraRTM`.
// agora-rtc-sdk-ng is bundled (its named ESM exports don't map cleanly to a
// UMD global — externalizing it is a spike-gated optimization, not done here).
export default defineConfig({
  plugins: [react(), compression({ ext: '.gz', deleteOriginalAssets: false })],
  build: {
    outDir: '../internal/cli/playground/webassets',
    emptyOutDir: true,
    rollupOptions: {
      external: ['agora-rtm'],
      output: { globals: { 'agora-rtm': 'AgoraRTM' } },
    },
  },
})
