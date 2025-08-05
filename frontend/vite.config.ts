import { defineConfig } from 'vite'
import preact from '@preact/preset-vite'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [preact()],
  resolve: {
    alias: {
    }
  },
  server: {
    port: 5173,
    host: '0.0.0.0', // Open to external access within container
    hmr: {
      // Use the address seen by browser, through port 80 (Traefik).
      // This way HMR connection is established correctly through proxy.
      protocol: 'ws',
      host: 'localhost',
      port: 5173,
      // Disable auto refresh, only do hot reload
      overlay: false, // Disable error overlay
    },
    // Watch options - monitor file changes
    watch: {
      // Use normal watch instead of aggressive polling
      usePolling: false,
      // Ignored patterns - prevent unnecessary refresh
      ignored: ['**/node_modules/**', '**/.git/**'],
    },
    allowedHosts: process.env.VITE_ALLOWED_HOSTS ? process.env.VITE_ALLOWED_HOSTS.split(',') : ['localhost']
  },
  preview: {
    port: 80,
    host: '0.0.0.0',
    allowedHosts: process.env.VITE_ALLOWED_HOSTS ? process.env.VITE_ALLOWED_HOSTS.split(',') : ['localhost']
  },
  // Build configuration
  build: {
    // Don't include HMR codes in production build
    minify: 'terser',
    // Source maps disabled in production (for security and performance)
    sourcemap: false,
    // Optimize bundle size
    target: 'es2015',
    // Limit for chunk size warnings
    chunkSizeWarningLimit: 1000,
    rollupOptions: {
      external: (id) => {
        // Exclude Vite client from production build
        return id.includes('@vite/client') || id.includes('vite/client')
      },
      output: {
        // Code splitting and chunk optimization
        manualChunks: {
          vendor: ['preact'],
          router: ['wouter'],
          api: ['axios']
        },
        // Asset naming
        assetFileNames: 'assets/[name]-[hash][extname]',
        chunkFileNames: 'js/[name]-[hash].js',
        entryFileNames: 'js/[name]-[hash].js'
      }
    },
    // Terser minification options
    terserOptions: {
      compress: {
        // Remove console logs in production (debug utility already handles this)
        drop_console: true,
        drop_debugger: true,
        pure_funcs: ['console.log', 'console.debug']
      }
    }
  }
})
