import type { Metadata } from 'next';
import { DocsLayout } from 'fumadocs-ui/layouts/docs';
import { apiZhSource, apiEnSource } from '@/lib/source';

export const metadata: Metadata = {
  title: 'API Reference',
};

export default async function ApiLayoutRoute({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ lang: string }>;
}) {
  const { lang } = await params;
  const source = lang === 'zh' ? apiZhSource : apiEnSource;

  return (
    <DocsLayout
      tree={source.pageTree}
      nav={{ enabled: false }}
      sidebar={{ collapsible: false }}
    >
      {children}
    </DocsLayout>
  );
}
