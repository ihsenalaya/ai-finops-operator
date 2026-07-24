import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// During `npm run dev`, the console-api backend runs on :8090 (started with
// -dev-cors) and this dev server proxies /api to it, so the browser only ever
// talks to one origin. In production, console-api serves this app's built
// dist/ directly, so no proxy/CORS is needed at all.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8090",
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
