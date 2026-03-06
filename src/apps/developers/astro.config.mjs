import { defineConfig } from 'astro/config';
import mdx from '@astrojs/mdx';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  integrations: [mdx({ gfm: true })],
  markdown: {
    gfm: true,
    syntaxHighlight: 'shiki',
  },
  vite: {
    plugins: [tailwindcss()],
  },
});
