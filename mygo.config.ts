import { defineConfig } from "mygo-cli";
import pkg from "./package.json" with { type: "json" };
export default defineConfig({
  name: "Leafmark",
  identifier: "dev.leafmark.app",
  version: pkg.version,
  updates: {
    github: "KevinXC5/leafmark",
    publicKey: "arFeLNqUcZDj4bjo7f1FiZ3ndod0Ghgu6B15b/BKUH8=",
    deltas: 0,
    changelog: ".github/release-notes.md",
  },
  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  frontendDist: "dist",
  bindings: "src/platform/mygo.ts",
  out: "build",
});
