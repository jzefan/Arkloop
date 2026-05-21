/* Shared data + helpers for both variants */

const AGENTS = {
  industry: {
    key: 'industry',
    label: '产教融合',
    items: [
      { id: 'pj', name: '双高产教融合评估', desc: '评估双高院校产教融合建设水平', color: '#3B5BDB', bg: '#EDF0FF', enabled: true },
      { id: 'gw', name: '岗位能力模型',     desc: '构建岗位胜任力素质画像',     color: '#0CA678', bg: '#E6F8F1', enabled: false },
      { id: 'cy', name: '产业学院',         desc: '产业学院规划与运营辅助',     color: '#E8590C', bg: '#FFF1E6', enabled: false },
      { id: 'xc', name: '现场工程师',       desc: '现场工程师培养方案设计',     color: '#7048E8', bg: '#F0EBFF', enabled: false },
    ],
  },
  edu: {
    key: 'edu',
    label: '教育',
    items: [
      { id: 'xx', name: '学习辅导', desc: '个性化学习路径规划与答疑', color: '#D6336C', bg: '#FFE9F0', enabled: true },
    ],
  },
};

/* Notion-style first-character icon block */
function AgentIcon({ name, color, bg, size = 40, radius = 10 }) {
  return (
    <div style={{
      width: size, height: size, borderRadius: radius,
      background: bg, color,
      display: 'grid', placeItems: 'center',
      fontSize: size * 0.46, fontWeight: 600,
      letterSpacing: '-0.02em',
      flexShrink: 0,
    }}>
      {name.charAt(0)}
    </div>
  );
}

/* The shared chrome: page header, greeting, composer */
function PageShell({ children, greetingSize = 32 }) {
  return (
    <div style={{
      width: '100%', minHeight: '100%',
      background: '#FAFAF9',
      fontFamily: '-apple-system, BlinkMacSystemFont, "PingFang SC", "Helvetica Neue", "Microsoft YaHei", sans-serif',
      color: '#1F1F1E',
      display: 'flex', flexDirection: 'column',
    }}>
      {/* Top bar */}
      <header style={{
        display: 'flex', justifyContent: 'flex-end', alignItems: 'center',
        padding: '20px 28px', gap: 18,
      }}>
        <button style={iconBtnStyle} aria-label="通知">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
            <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/>
            <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>
          </svg>
        </button>
        <div style={{
          width: 28, height: 28, borderRadius: 6,
          display: 'grid', placeItems: 'center',
          fontFamily: 'Georgia, serif', fontStyle: 'italic',
          fontSize: 14, color: '#1F1F1E',
        }}>6∂</div>
      </header>

      {/* Body */}
      <main style={{
        flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center',
        padding: '40px 24px 64px',
      }}>
        <h1 style={{
          fontSize: greetingSize, fontWeight: 400, letterSpacing: '-0.01em',
          margin: '24px 0 32px', color: '#1F1F1E',
        }}>
          早，test-1，今天有什么计划?
        </h1>

        {/* Composer */}
        <div style={{
          width: '100%', maxWidth: 720,
          background: '#fff',
          border: '1px solid #E7E5E1',
          borderRadius: 14,
          padding: '16px 18px 12px',
          boxShadow: '0 1px 2px rgba(20,20,20,0.02)',
        }}>
          <div style={{ color: '#9C9A95', fontSize: 15, padding: '4px 2px 14px' }}>
            今天有什么可以帮助你的?
          </div>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <button style={composerIconBtn} aria-label="附件">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M12 5v14M5 12h14"/></svg>
              </button>
              <span style={{
                display: 'inline-flex', alignItems: 'center', gap: 6,
                padding: '5px 10px 5px 10px',
                background: '#F4F2EE', borderRadius: 6,
                fontSize: 13, color: '#3D3D3A',
              }}>
                双高产教融合评估
                <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M18 6 6 18M6 6l12 12"/></svg>
              </span>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: '#6E6C66', fontSize: 13 }}>
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                deepseek-v4-flash
                <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m6 9 6 6 6-6"/></svg>
              </span>
              <button style={composerIconBtn} aria-label="语音">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><rect x="9" y="2" width="6" height="13" rx="3"/><path d="M5 10v2a7 7 0 0 0 14 0v-2M12 19v3"/></svg>
              </button>
            </div>
          </div>
        </div>

        {/* Directory slot */}
        <div style={{ width: '100%', maxWidth: 720, marginTop: 40 }}>
          {children}
        </div>
      </main>
    </div>
  );
}

const iconBtnStyle = {
  width: 32, height: 32, borderRadius: 8,
  background: 'transparent', border: 'none',
  color: '#3D3D3A', cursor: 'pointer',
  display: 'grid', placeItems: 'center',
};
const composerIconBtn = {
  width: 28, height: 28, borderRadius: 6,
  background: 'transparent', border: 'none',
  color: '#6E6C66', cursor: 'pointer',
  display: 'grid', placeItems: 'center',
};

Object.assign(window, { AGENTS, AgentIcon, PageShell });
