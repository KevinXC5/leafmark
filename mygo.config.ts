import { defineConfig } from "mygo-cli";
export default defineConfig({
  name: "Leafmark",
  identifier: "dev.leafmark.app",
  version: "0.1.0",
  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  frontendDist: "dist",
  bindings: "src/platform/mygo.ts",
  out: "build",
});
