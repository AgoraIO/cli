import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import compression from 'vite-plugin-compression'

// agora-rtc-sdk-ng and agora-rtm are both bundled. (CDN-externalizing the
// RTC SDK is a possible future optimization; agora-rtm has no reliable UMD
// CDN build, so it is bundled like the official quickstart.)
export default defineConfig({
  plugins: [react(), compression({ ext: '.gz', deleteOriginFile: true })],
  build: {
    outDir: '../internal/cli/playground/webassets',
    emptyOutDir: true,
  },
})
