import { useRef, useEffect, useCallback, useImperativeHandle, forwardRef, useState } from 'react'
import { apiBaseUrl } from '@arkloop/shared/api'
import type { ArtifactRef } from '../storage'

export type ArtifactAction =
  | { type: 'prompt'; text: string }
  | { type: 'resize'; height: number }
  | { type: 'error'; message: string }

export type ArtifactIframeHandle = {
  setStreamingContent: (html: string) => void
  finalizeContent: (html: string) => void
}

type Props = {
  mode: 'streaming' | 'static'
  artifact?: ArtifactRef
  accessToken?: string
  onAction?: (action: ArtifactAction) => void
  frameTitle?: string
  className?: string
  style?: React.CSSProperties
}

function collectCSSVariables(): string {
  const root = document.documentElement
  const computed = getComputedStyle(root)
  const vars: string[] = []
  for (const sheet of document.styleSheets) {
    try {
      for (const rule of sheet.cssRules) {
        if (rule instanceof CSSStyleRule && rule.selectorText === ':root') {
          for (let i = 0; i < rule.style.length; i++) {
            const prop = rule.style[i]
            if (prop.startsWith('--c-')) {
              vars.push(`${prop}: ${computed.getPropertyValue(prop).trim()};`)
            }
          }
        }
      }
    } catch {
      // cross-origin stylesheets
    }
  }
  return vars.join('\n    ')
}

function buildShellHTML(): string {
  const cssVars = collectCSSVariables()
  return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline' https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com https://esm.sh; style-src 'unsafe-inline'; img-src data: blob: https:; font-src https:; connect-src https:;">
<style>
  :root {
    ${cssVars}
    --color-text-primary: var(--c-text-primary);
    --color-text-secondary: var(--c-text-secondary);
    --color-background-primary: var(--c-bg-sub);
    --color-background-secondary: var(--c-bg-page);
    --color-border-tertiary: var(--c-border-subtle);
    --color-border-secondary: var(--c-border-mid);
    --color-text-success: var(--c-status-success-text, var(--c-text-primary));
    --color-background-success: var(--c-status-ok-bg, var(--c-bg-sub));
    --color-text-warning: var(--c-status-warning-text, var(--c-text-primary));
    --color-background-warning: var(--c-status-warn-bg, var(--c-bg-sub));
    --color-text-danger: var(--c-status-error-text, var(--c-text-primary));
    --color-background-danger: var(--c-status-danger-bg, var(--c-bg-sub));
    --color-text-info: var(--c-text-secondary);
    --color-background-info: var(--c-bg-sub);
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  html {
    background: transparent;
    overflow-x: hidden;
  }
  body {
    font-family: var(--c-font-body, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif);
    font-size: 14px;
    line-height: 1.7;
    color: var(--c-text-primary, #faf9f5);
    background: transparent;
    padding: 10px 0;
    overflow-x: hidden;
  }
  #root {
    display: block;
    width: 100%;
    background: transparent;
  }
  #root > :first-child { margin-top: 0 !important; }
  #root > :last-child { margin-bottom: 0 !important; }
  :where(button, select, input, textarea) {
    font: inherit;
    color: var(--color-text-primary);
  }
  :where(button) {
    appearance: none;
    border: 0.5px solid var(--color-border-tertiary);
    border-radius: 8px;
    background: var(--color-background-primary);
    padding: 5px 12px;
  }
  :where(select, input[type="text"], input[type="number"], textarea) {
    appearance: none;
    border: 0.5px solid var(--color-border-tertiary);
    border-radius: 8px;
    background: var(--color-background-primary);
    padding: 4px 8px;
  }
  :where(input[type="range"]) {
    appearance: none;
    width: 100%;
    min-width: 88px;
    height: 20px;
    background: transparent;
  }
  :where(input[type="range"]::-webkit-slider-runnable-track) {
    height: 2px;
    border-radius: 999px;
    background: var(--color-border-secondary);
  }
  :where(input[type="range"]::-webkit-slider-thumb) {
    appearance: none;
    width: 12px;
    height: 12px;
    margin-top: -5px;
    border-radius: 999px;
    border: none;
    background: var(--color-text-primary);
  }
  :where(input[type="range"]::-moz-range-track) {
    height: 2px;
    border: none;
    border-radius: 999px;
    background: var(--color-border-secondary);
  }
  :where(input[type="range"]::-moz-range-thumb) {
    width: 12px;
    height: 12px;
    border: none;
    border-radius: 999px;
    background: var(--color-text-primary);
  }
  @keyframes _fadeIn {
    from { opacity: 0; transform: translateY(4px); }
    to { opacity: 1; transform: translateY(0); }
  }
  @media (prefers-reduced-motion: reduce) {
    *, *::before, *::after {
      animation-duration: 0.001ms !important;
      animation-iteration-count: 1 !important;
      transition-duration: 0.001ms !important;
      scroll-behavior: auto !important;
    }
  }
</style>
</head>
<body>
<div id="root"></div>
<script src="https://cdn.jsdelivr.net/npm/morphdom@2/dist/morphdom-umd.min.js"></script>
<script>
(function() {
  var morphReady = false;
  var pending = null;

  window.arkloop = {
    sendPrompt: function(text) {
      window.parent.postMessage({ type: 'arkloop:artifact:action', action: 'prompt', text: String(text).slice(0, 4000) }, '*');
    }
  };

  window.addEventListener('arkloop:send-prompt', function(e) {
    if (!e) return;
    var text = typeof e.detail === 'string' ? e.detail : '';
    if (text) window.arkloop.sendPrompt(text);
  });

  function reportError(message) {
    window.parent.postMessage({ type: 'arkloop:artifact:action', action: 'error', message: String(message || 'render error').slice(0, 4000) }, '*');
  }

  window.addEventListener('error', function(e) {
    reportError(e && e.message ? e.message : 'render error');
  });

  window.addEventListener('unhandledrejection', function(e) {
    var reason = e && e.reason;
    reportError(reason && reason.message ? reason.message : String(reason || 'render error'));
  });

  window._setContent = function(html, finalize) {
    if (!morphReady) {
      pending = { html: html, finalize: finalize === true };
      return;
    }
    var root = document.getElementById('root');
    if (!root) return;
    var target = document.createElement('div');
    target.id = 'root';
    target.innerHTML = html;
    morphdom(root, target, {
      onBeforeElUpdated: function(from, to) {
        if (from.isEqualNode(to)) return false;
        return true;
      },
      onNodeAdded: function(node) {
        if (node.nodeType === 1 && node.tagName !== 'SCRIPT') {
          node.style.animation = '_fadeIn 0.3s ease both';
        }
        return node;
      }
    });
    window._notifyHeight();
    if (finalize === true) {
      window._runScripts();
    }
  };

  window._runScripts = async function() {
    var scripts = Array.prototype.slice.call(document.querySelectorAll('#root script'));
    for (var index = 0; index < scripts.length; index++) {
      await new Promise(function(resolve) {
        var old = scripts[index];
        if (!old || !old.parentNode) { resolve(); return; }
        var script = document.createElement('script');
        var isExternal = !!old.src;
        if (isExternal) {
          script.src = old.src;
          script.onload = function() { resolve(); };
          script.onerror = function() {
            reportError('failed to load script: ' + old.src);
            resolve();
          };
        } else {
          script.textContent = old.textContent;
        }
        for (var i = 0; i < old.attributes.length; i++) {
          var attr = old.attributes[i];
          if (attr.name !== 'src') script.setAttribute(attr.name, attr.value);
        }
        old.parentNode.replaceChild(script, old);
        if (!isExternal) resolve();
      });
    }
    window._notifyHeight();
  };

  window._notifyHeight = function() {
    var root = document.getElementById('root');
    if (!root) return;
    var rect = root.getBoundingClientRect();
    var height = Math.max(root.scrollHeight, Math.ceil(rect.height)) + 20;
    window.parent.postMessage({ type: 'arkloop:artifact:action', action: 'resize', height: height }, '*');
  };

  var morphScript = document.querySelector('script[src*="morphdom"]');
  if (typeof window.morphdom === 'function') {
    morphReady = true;
  } else if (morphScript) {
    morphScript.onload = function() {
      morphReady = true;
      if (pending) {
        window._setContent(pending.html, pending.finalize);
        pending = null;
      }
    };
    morphScript.onerror = function() {
      morphReady = true;
      if (pending) {
        document.getElementById('root').innerHTML = pending.html;
        if (pending.finalize === true) {
          window._runScripts();
        }
        window._notifyHeight();
        pending = null;
      }
    };
  }

  window.addEventListener('message', function(e) {
    var data = e.data;
    if (!data || data.type !== 'arkloop:artifact:set-content') return;
    var html = typeof data.html === 'string' ? data.html : '';
    window._setContent(html, data.finalize === true);
  });

  new MutationObserver(function() { window._notifyHeight(); })
    .observe(document.getElementById('root'), { childList: true, subtree: true, attributes: true });

  if (typeof ResizeObserver === 'function') {
    var resizeObserver = new ResizeObserver(function() { window._notifyHeight(); });
    resizeObserver.observe(document.body);
    resizeObserver.observe(document.getElementById('root'));
  }

  window.addEventListener('load', function() {
    window._notifyHeight();
  });
})();
</script>
</body>
</html>`
}

export const ArtifactIframe = forwardRef<ArtifactIframeHandle, Props>(
  function ArtifactIframe({ mode, artifact, accessToken, onAction, frameTitle, className, style }, ref) {
    const iframeRef = useRef<HTMLIFrameElement>(null)
    const [blobUrl, setBlobUrl] = useState<string | null>(null)
    const [loading, setLoading] = useState(mode === 'static' || mode === 'streaming')
    const [error, setError] = useState(false)
    const shellBlobRef = useRef<string | null>(null)
    const isReadyRef = useRef(false)
    const pendingContentRef = useRef<{ html: string; finalize: boolean } | null>(null)

    useEffect(() => {
      if (mode !== 'streaming') return
      isReadyRef.current = false
      setLoading(true)
      const html = buildShellHTML()
      const blob = new Blob([html], { type: 'text/html' })
      const url = URL.createObjectURL(blob)
      shellBlobRef.current = url
      setBlobUrl(url)
      setLoading(false)
      return () => {
        if (shellBlobRef.current === url) {
          shellBlobRef.current = null
        }
        URL.revokeObjectURL(url)
      }
    }, [mode])

    useEffect(() => {
      if (mode !== 'static' || !artifact || !accessToken) return
      let cancelled = false
      const url = `${apiBaseUrl()}/v1/artifacts/${artifact.key}`
      fetch(url, { headers: { Authorization: `Bearer ${accessToken}` } })
        .then((res) => {
          if (!res.ok) throw new Error(`${res.status}`)
          return res.blob()
        })
        .then((blob) => {
          if (cancelled) return
          setBlobUrl(URL.createObjectURL(blob))
          setLoading(false)
        })
        .catch(() => {
          if (!cancelled) {
            setError(true)
            setLoading(false)
          }
        })
      return () => { cancelled = true }
    }, [mode, artifact?.key, accessToken])

    useEffect(() => {
      return () => {
        if (blobUrl && blobUrl !== shellBlobRef.current) URL.revokeObjectURL(blobUrl)
      }
    }, [blobUrl])

    const postStreamingContent = useCallback((html: string, finalize: boolean) => {
      pendingContentRef.current = { html, finalize }
      if (mode !== 'streaming' || !isReadyRef.current) return
      const iframe = iframeRef.current
      if (!iframe?.contentWindow) return
      try {
        iframe.contentWindow.postMessage({
          type: 'arkloop:artifact:set-content',
          html,
          finalize,
        }, '*')
      } catch {
        // iframe not ready
      }
    }, [mode])

    useImperativeHandle(ref, () => ({
      setStreamingContent(html: string) {
        postStreamingContent(html, false)
      },
      finalizeContent(html: string) {
        postStreamingContent(html, true)
      },
    }), [postStreamingContent])

    useEffect(() => {
      const handler = (e: MessageEvent) => {
        const iframe = iframeRef.current
        if (!iframe || e.source !== iframe.contentWindow) return
        if (e.data?.type !== 'arkloop:artifact:action') return
        const action = e.data.action
        if (action === 'resize' && typeof e.data.height === 'number') {
          iframe.style.height = `${Math.min(e.data.height, 2000)}px`
          onAction?.({ type: 'resize', height: e.data.height })
        } else if (action === 'prompt' && typeof e.data.text === 'string') {
          onAction?.({ type: 'prompt', text: e.data.text.slice(0, 4000) })
        } else if (action === 'error' && typeof e.data.message === 'string') {
          onAction?.({ type: 'error', message: e.data.message.slice(0, 4000) })
        }
      }
      window.addEventListener('message', handler)
      return () => window.removeEventListener('message', handler)
    }, [onAction])

    if (error) return null

    if (loading) {
      return (
        <div
          className={className}
          style={{
            width: '100%',
            height: '200px',
            borderRadius: '10px',
            background: 'var(--c-bg-sub)',
            ...style,
          }}
        />
      )
    }

    return (
      <iframe
        ref={iframeRef}
        src={blobUrl!}
        title={frameTitle ?? 'artifact'}
        sandbox="allow-scripts"
        onLoad={() => {
          isReadyRef.current = true
          const pending = pendingContentRef.current
          if (pending) {
            postStreamingContent(pending.html, pending.finalize)
          }
        }}
        style={{
          width: '100%',
          minHeight: '200px',
          border: '0.5px solid var(--c-border-subtle)',
          borderRadius: '10px',
          background: 'transparent',
          display: 'block',
          ...style,
        }}
        className={className}
      />
    )
  },
)
