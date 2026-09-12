import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // Preserve Windows WebView2 and the existing macOS 12 deployment baseline.
    // Syntax transforms do not polyfill Web APIs; those remain platform checks.
    target: ['chrome109', 'safari15']
  }
})
