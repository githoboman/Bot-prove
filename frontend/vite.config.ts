import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        manualChunks(id: string) {
          if (id.includes('node_modules')) {
            if (
              id.includes('react-router-dom') ||
              id.includes('/react/') ||
              id.includes('/react-dom/')
            ) {
              return 'vendor'
            }
            if (
              id.includes('@make-software/csprclick-ui') ||
              id.includes('styled-components')
            ) {
              return 'csprclick'
            }
          }
        }
      }
    }
  }
})
