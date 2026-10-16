import { defineConfig } from 'astro/config';

export default defineConfig({
  // Galeriku web: static output, API calls go to the Go backend.
  output: 'static',
  devToolbar: { enabled: false },
});
