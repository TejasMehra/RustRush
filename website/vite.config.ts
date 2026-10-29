import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  base: "./",
  plugins: [react()],
  publicDir: "../assets",
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
