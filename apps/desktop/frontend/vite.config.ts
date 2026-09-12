import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // Match Windows WebView2 and the user-approved macOS 13 deployment baseline.
    // Syntax transforms do not polyfill Web APIs; those remain platform checks.
    target: ['chrome109', 'safari16']
  }
})
