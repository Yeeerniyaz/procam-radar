import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist/windows/web',
    emptyOutDir: true,
    sourcemap: false,
    chunkSizeWarningLimit: 1200,
    target: ['es2020', 'chrome80'],
  },
  server: {
    port: 5174,
  },
});
