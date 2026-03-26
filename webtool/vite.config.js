import { defineConfig } from 'vite';

export default defineConfig({
  root: 'static',
  server: {
    port: 5179,
    open: true,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:5180',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: '../dist',
    emptyOutDir: true,
  },
});
