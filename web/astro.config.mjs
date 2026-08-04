// @ts-check
import { defineConfig } from "astro/config";
import svelte from "@astrojs/svelte";

export default defineConfig({
  // Custom domains serve from the root, so there is no base path: a base would
  // break every asset URL and in-page link.
  site: "https://alpage.subnet.ch",
  integrations: [svelte()],
  build: { format: "directory" },
  markdown: {
    shikiConfig: {
      themes: { light: "github-light", dark: "github-dark" },
      wrap: false,
    },
  },
});
