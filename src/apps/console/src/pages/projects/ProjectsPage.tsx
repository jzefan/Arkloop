import { useState, useEffect, useRef, useCallback } from 'react'
import { RefreshCw } from 'lucide-react'
import { PageHeader } from '../../components/PageHeader'
import { useLocale } from '../../contexts/LocaleContext'
import { listProjects, type Project } from '../../api/projects'
import { useOutletContext } from 'react-router-dom'
import type { ConsoleOutletContext } from '../../layouts/ConsoleLayout'

function formatProjectMeta(isDefault: boolean, visibility: string, t: ReturnType<typeof useLocale>['t']) {
  const parts = [visibility]
  if (isDefault) {
    parts.unshift(t.pages.projects.defaultBadge)
  }
  return parts.join(' · ')
}

export function ProjectsPage() {
  const { t } = useLocale()
  const { accessToken } = useOutletContext<ConsoleOutletContext>()
  const tc = t.pages.projects
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await listProjects(accessToken)
      if (!mountedRef.current) return
      setProjects(data)
    } finally {
      if (mountedRef.current) setLoading(false)
    }
  }, [accessToken])

  useEffect(() => { void load() }, [load])

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader
        title={tc.title}
        actions={(
          <button
            type="button"
            onClick={() => { void load() }}
            className="inline-flex items-center gap-2 rounded-lg border border-[var(--c-border)] px-3 py-1.5 text-xs text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
          >
            <RefreshCw size={14} />
            {tc.refresh}
          </button>
        )}
      />

      <div className="flex flex-1 flex-col gap-4 overflow-auto p-6">
        <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {loading && projects.length === 0 ? (
            <div className="rounded-2xl border border-[var(--c-border)] bg-[var(--c-bg-card)] p-5 text-sm text-[var(--c-text-muted)]">
              {tc.loading}
            </div>
          ) : projects.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-[var(--c-border)] bg-[var(--c-bg-card)] p-5 text-sm text-[var(--c-text-muted)]">
              {tc.empty}
            </div>
          ) : (
            projects.map((project) => (
              <div
                key={project.id}
                className="rounded-2xl border border-[var(--c-border)] bg-[var(--c-bg-card)] px-5 py-4"
              >
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="text-sm font-medium text-[var(--c-text-primary)]">{project.name}</div>
                    <div className="mt-1 text-xs text-[var(--c-text-muted)]">
                      {formatProjectMeta(project.is_default, project.visibility, t)}
                    </div>
                  </div>
                  {project.is_default && (
                    <span className="rounded-full bg-[var(--c-bg-tag)] px-2 py-0.5 text-[11px] text-[var(--c-text-secondary)]">
                      {tc.defaultBadge}
                    </span>
                  )}
                </div>
                <div className="mt-3 text-xs text-[var(--c-text-muted)]">
                  {project.description?.trim() || tc.noDescription}
                </div>
              </div>
            ))
          )}
        </section>
      </div>
    </div>
  )
}
