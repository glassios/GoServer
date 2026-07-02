import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import path from 'path';
import { defineConfig } from 'vite';

// New WebGL (React Three Fiber) client for GoServer, served by the gateway at /client3d/.
// Seeded from the SpaceShip2d renderer. The existing Canvas2D client at / is untouched.
export default defineConfig({
  base: '/client3d/',
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '.'),
    },
  },
});
