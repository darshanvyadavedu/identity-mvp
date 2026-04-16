import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, "../", "VITE_");
  const backendURL = env.VITE_API_BASE_URL || "http://localhost:8080";

  return {
    plugins: [react()],
    envDir: "../",
    server: {
      proxy: {
        "/api": {
          target: backendURL,
          changeOrigin: true,
        },
      },
    },
  };
});
