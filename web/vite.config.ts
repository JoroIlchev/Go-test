import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// `npm run dev` serves the UI on :5173 and forwards API calls to a running marquee.exe.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: { "/api": "http://127.0.0.1:8081" },
  },
  build: { outDir: "dist", emptyOutDir: true },
});
