import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// During local dev, proxy /api to the Go apiserver so the SPA and API share an
// origin (no CORS hassle). In production both are served behind one Ingress.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.API_TARGET || "http://localhost:8090",
        changeOrigin: true,
      },
    },
  },
});
