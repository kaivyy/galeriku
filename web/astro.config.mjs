import { defineConfig } from 'astro/config';

export default defineConfig({
  // Galeriku web: static output, API calls go to the Go backend.
  output: 'static',
  devToolbar: { enabled: false },
  server: {
    host: true,
    port: 6464,
  },
  vite: {
    server: {
      // Same-origin API proxy: phones only need one reachable port. The
      // islands call /api/* and Vite forwards to the Go server, so no
      // second port and no CORS are involved in the browser.
      proxy: {
        '/api': {
          target: 'http://127.0.0.1:6666',
          changeOrigin: true,
        },
      },
    },
  },
});
