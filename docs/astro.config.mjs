import { defineConfig } from 'astro/config';
import mdx from '@astrojs/mdx';
import tailwind from '@astrojs/tailwind';
import preact from '@astrojs/preact';
import sitemap from '@astrojs/sitemap';

// https://astro.build/config
export default defineConfig({
  site: 'https://kubernetes-mcp-server.blankcut.com',
  integrations: [
    mdx({
      syntaxHighlight: 'shiki',
      shikiConfig: { theme: 'github-dark' },
      remarkPlugins: ['remark-toc'],
      rehypePlugins: ['rehype-autolink-headings']
    }),
    tailwind({
      config: { applyBaseStyles: false }
    }),
    preact(),
    sitemap()
  ],
  markdown: {
    syntaxHighlight: 'shiki',
    shikiConfig: {
      theme: 'github-dark',
      wrap: true
    }
  }
});
