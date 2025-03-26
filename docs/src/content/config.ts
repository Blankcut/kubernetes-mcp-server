import { defineCollection, z } from 'astro:content';

// Debug: Log that we're loading the content config
console.log("Loading content collection config...");

const docsCollection = defineCollection({
  type: 'content', // Make sure type is specified
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

// Debug: Log that we've defined the collections
console.log("Content collections defined:", Object.keys(collections));
