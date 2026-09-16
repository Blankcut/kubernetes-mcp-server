import { defineCollection, z } from 'astro:content';
import { glob } from 'astro/loaders';

const docsCollection = defineCollection({
  loader: glob({ pattern: '**/*.{md,mdx}', base: './src/content/docs' }),
  schema: z.object({
    title: z.string(),
    description: z.string().optional(),
    date: z.date().optional(),
    order: z.number().optional().default(999),
    tags: z.array(z.string()).optional(),
    author: z.string().optional(),
    draft: z.boolean().optional().default(false),
  }),
});

export const collections = {
  'docs': docsCollection,
};
