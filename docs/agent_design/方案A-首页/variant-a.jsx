/* Variant A v3 — z.ai-style tabs + card grid; covers replace first-char icon */
const { AGENTS, PageShell, COVER_MAP } = window;

function VariantA() {
  const [tab, setTab] = React.useState('industry');
  const cat = AGENTS[tab];
  const visible = cat.items.slice(0, 3);

  return (
    <PageShell greetingSize={30}>
      {/* z.ai-style plain text tabs */}
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        marginBottom: 18,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 22 }}>
          {Object.values(AGENTS).map(c => {
            const active = c.key === tab;
            return (
              <button key={c.key} onClick={() => setTab(c.key)} style={{
                padding: '4px 0',
                fontSize: 15,
                fontWeight: active ? 600 : 400,
                color: active ? '#1F1F1E' : '#9C9A95',
                background: 'transparent',
                border: 'none',
                cursor: 'pointer',
                fontFamily: 'inherit',
                position: 'relative',
                transition: 'color .15s',
              }}>
                {c.label}
                {active && (
                  <span style={{
                    position: 'absolute',
                    left: '50%', bottom: -7,
                    width: 4, height: 4, borderRadius: '50%',
                    background: '#1F1F1E',
                    transform: 'translateX(-50%)',
                  }} />
                )}
              </button>
            );
          })}
        </div>
        <a href="#" style={{
          color: '#9C9A95', fontSize: 13,
          textDecoration: 'none', display: 'inline-flex', alignItems: 'center', gap: 4,
        }}>全部 <span style={{ fontSize: 14 }}>→</span></a>
      </div>

      {/* Card grid */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(3, 1fr)',
        gap: 14,
      }}>
        {visible.map(a => {
          const Cover = COVER_MAP[a.id];
          const coverColor = a.enabled ? a.color : '#A8A6A0';
          const coverBg = a.enabled ? a.bg : '#EFEDE9';
          return (
            <button key={a.id} disabled={!a.enabled} style={{
              textAlign: 'left',
              background: '#fff',
              border: '1px solid #E7E5E1',
              borderRadius: 12,
              padding: 0,
              cursor: a.enabled ? 'pointer' : 'not-allowed',
              opacity: a.enabled ? 1 : 0.62,
              position: 'relative',
              overflow: 'hidden',
              transition: 'border-color .15s, box-shadow .15s',
              fontFamily: 'inherit',
            }}
            onMouseEnter={e => {
              if (a.enabled) {
                e.currentTarget.style.borderColor = '#C9C5BD';
                e.currentTarget.style.boxShadow = '0 4px 12px rgba(20,20,20,0.05)';
              }
            }}
            onMouseLeave={e => {
              e.currentTarget.style.borderColor = '#E7E5E1';
              e.currentTarget.style.boxShadow = 'none';
            }}
            >
              {/* Cover */}
              <div style={{ height: 124, borderBottom: '1px solid #EFEDE9', overflow: 'hidden' }}>
                {Cover ? <Cover color={coverColor} bg={coverBg} /> : <div style={{ height: '100%', background: coverBg }} />}
              </div>
              {!a.enabled && (
                <span style={{
                  position: 'absolute', top: 10, right: 10,
                  fontSize: 10.5, padding: '2px 8px', borderRadius: 999,
                  background: 'rgba(255,255,255,0.92)',
                  color: '#6E6C66',
                  backdropFilter: 'blur(4px)',
                  letterSpacing: '0.02em',
                }}>即将上线</span>
              )}
              <div style={{ padding: '14px 16px 16px' }}>
                <div style={{ fontSize: 14, fontWeight: 500, color: '#1F1F1E', marginBottom: 5 }}>
                  {a.name}
                </div>
                <div style={{
                  fontSize: 12.5, color: '#6E6C66', lineHeight: 1.5,
                  display: '-webkit-box', WebkitBoxOrient: 'vertical', WebkitLineClamp: 2, overflow: 'hidden',
                }}>
                  {a.desc}
                </div>
              </div>
            </button>
          );
        })}
        {Array.from({ length: Math.max(0, 3 - visible.length) }).map((_, i) => (
          <div key={`ph-${i}`} style={{
            border: '1px dashed #E7E5E1', borderRadius: 12,
            background: 'transparent', minHeight: 200,
          }} />
        ))}
      </div>
    </PageShell>
  );
}

window.VariantA = VariantA;
