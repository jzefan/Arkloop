import { defineCollection, z } from 'astro:content';

const docs = defineCollection({
  type: 'content',
  schema: z.object({
    title: z.string().optional(),
    description: z.string().optional(),
    sidebarLabel: z.string().optional(),
    order: z.number().optional(),
    draft: z.boolean().default(false),
  }),
});

const research = defineCollection({
  type: 'content',
  schema: z.object({
    title: z.string(),
    description: z.string(),
    date: z.coerce.date(),
    authors: z.array(z.string()),
    tags: z.array(z.string()).optional(),
    draft: z.boolean().default(false),
    cover: z.string().optional(),
  }),
});

export const collections = { docs, research };
