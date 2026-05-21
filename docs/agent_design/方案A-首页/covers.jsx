/* Dribbble-style cover illustrations — layered, dimensional, modern.
   Each: a tinted backdrop, soft decorative shapes, a thematic centerpiece, tiny accents. */

/* Util: derive a darker / lighter shade in oklch space, fallback string mixing */
const mix = (c, ratio) => `color-mix(in oklab, ${c}, white ${ratio}%)`;
const deeper = (c, ratio) => `color-mix(in oklab, ${c}, black ${ratio}%)`;

/* ── 1. 双高产教融合评估 — floating dashboard + chart card */
function Cover_pj({ color, bg }) {
  return (
    <svg viewBox="0 0 280 120" preserveAspectRatio="xMidYMid slice"
         style={{ width: '100%', height: '100%', display: 'block' }}>
      <defs>
        <linearGradient id="pj-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={bg} />
          <stop offset="1" stopColor={mix(color, 88)} />
        </linearGradient>
        <filter id="pj-shadow" x="-20%" y="-20%" width="140%" height="140%">
          <feDropShadow dx="0" dy="2" stdDeviation="3" floodColor={color} floodOpacity="0.18"/>
        </filter>
      </defs>
      <rect width="280" height="120" fill="url(#pj-bg)"/>
      {/* big soft blob */}
      <circle cx="240" cy="20" r="55" fill={color} opacity="0.16"/>
      <circle cx="40"  cy="110" r="40" fill={color} opacity="0.12"/>

      {/* dashboard card */}
      <g transform="translate(60 22)" filter="url(#pj-shadow)">
        <rect width="160" height="74" rx="8" fill="#fff"/>
        {/* card top row */}
        <circle cx="14" cy="14" r="4" fill={color} opacity="0.35"/>
        <rect x="24" y="11" width="60" height="6" rx="3" fill={color} opacity="0.18"/>
        <rect x="130" y="9" width="22" height="10" rx="2" fill={color} opacity="0.18"/>
        {/* chart bars */}
        <rect x="14" y="48" width="14" height="16" rx="2" fill={color} opacity="0.35"/>
        <rect x="32" y="38" width="14" height="26" rx="2" fill={color} opacity="0.55"/>
        <rect x="50" y="28" width="14" height="36" rx="2" fill={color}/>
        <rect x="68" y="44" width="14" height="20" rx="2" fill={color} opacity="0.4"/>
        <rect x="86" y="32" width="14" height="32" rx="2" fill={color} opacity="0.7"/>
        {/* trend line */}
        <path d="M110 56 L 124 44 L 140 50 L 152 32" stroke={color} strokeWidth="2" fill="none" strokeLinecap="round" strokeLinejoin="round"/>
        <circle cx="152" cy="32" r="2.5" fill={color}/>
      </g>

      {/* floating accent */}
      <g transform="translate(228 78)">
        <circle r="14" fill="#fff" opacity="0.9"/>
        <path d="M-6 0 L-2 4 L6 -4" stroke={color} strokeWidth="2.2" fill="none" strokeLinecap="round" strokeLinejoin="round"/>
      </g>
      <circle cx="36" cy="32" r="3" fill={color}/>
      <circle cx="48" cy="22" r="1.5" fill={color} opacity="0.6"/>
    </svg>
  );
}

/* ── 2. 岗位能力模型 — orbiting skill nodes around a center */
function Cover_gw({ color, bg }) {
  return (
    <svg viewBox="0 0 280 120" preserveAspectRatio="xMidYMid slice"
         style={{ width: '100%', height: '100%', display: 'block' }}>
      <defs>
        <linearGradient id="gw-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={bg} />
          <stop offset="1" stopColor={mix(color, 86)} />
        </linearGradient>
        <radialGradient id="gw-core" cx="0.3" cy="0.3">
          <stop offset="0" stopColor={mix(color, 30)}/>
          <stop offset="1" stopColor={color}/>
        </radialGradient>
      </defs>
      <rect width="280" height="120" fill="url(#gw-bg)"/>
      <circle cx="40" cy="20"  r="48" fill={color} opacity="0.12"/>
      <circle cx="250" cy="110" r="40" fill={color} opacity="0.1"/>

      {/* orbit rings */}
      <g transform="translate(140 60)">
        <ellipse rx="64" ry="34" fill="none" stroke={color} strokeOpacity="0.25" strokeWidth="1"/>
        <ellipse rx="46" ry="22" fill="none" stroke={color} strokeOpacity="0.35" strokeWidth="1" transform="rotate(-18)"/>

        {/* nodes */}
        <g>
          <circle cx="-64" cy="0"  r="6" fill="#fff" stroke={color} strokeWidth="1.5"/>
          <circle cx="48"  cy="-18" r="7" fill={color} opacity="0.85"/>
          <circle cx="58"  cy="14"  r="5" fill="#fff" stroke={color} strokeWidth="1.5"/>
          <circle cx="-30" cy="-26" r="5" fill={color} opacity="0.55"/>
          <circle cx="-44" cy="20"  r="4" fill="#fff" stroke={color} strokeWidth="1.5"/>
        </g>

        {/* center node */}
        <circle r="18" fill="url(#gw-core)"/>
        <circle r="22" fill="none" stroke="#fff" strokeOpacity="0.5" strokeWidth="2"/>
        {/* tiny person glyph */}
        <circle cy="-4" r="4" fill="#fff"/>
        <path d="M-7 10 Q 0 2 7 10 Z" fill="#fff"/>
      </g>

      <circle cx="208" cy="30" r="2" fill={color}/>
      <circle cx="72"  cy="92" r="2.5" fill={color} opacity="0.7"/>
    </svg>
  );
}

/* ── 3. 产业学院 — isometric building */
function Cover_cy({ color, bg }) {
  const c = color;
  const light = mix(color, 35);
  const dark = deeper(color, 15);
  return (
    <svg viewBox="0 0 280 120" preserveAspectRatio="xMidYMid slice"
         style={{ width: '100%', height: '100%', display: 'block' }}>
      <defs>
        <linearGradient id="cy-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={bg} />
          <stop offset="1" stopColor={mix(color, 88)} />
        </linearGradient>
      </defs>
      <rect width="280" height="120" fill="url(#cy-bg)"/>
      <circle cx="240" cy="20"  r="46" fill={c} opacity="0.14"/>
      <circle cx="30"  cy="100" r="34" fill={c} opacity="0.1"/>

      {/* iso building */}
      <g transform="translate(110 28)">
        {/* ground shadow */}
        <ellipse cx="44" cy="82" rx="56" ry="6" fill={c} opacity="0.18"/>

        {/* tower (back, taller) */}
        <g>
          {/* left face */}
          <path d="M 12 22 L 12 70 L 42 84 L 42 36 Z" fill={c}/>
          {/* right face */}
          <path d="M 42 36 L 42 84 L 70 70 L 70 22 Z" fill={dark}/>
          {/* top */}
          <path d="M 12 22 L 42 8 L 70 22 L 42 36 Z" fill={light}/>
          {/* windows left */}
          {[0,1,2,3].map(i => (
            <rect key={i} x="18" y={32 + i*10} width="6" height="5" fill="#fff" opacity="0.55"/>
          ))}
          {[0,1,2,3].map(i => (
            <rect key={i} x="28" y={37 + i*10} width="6" height="5" fill="#fff" opacity="0.55"/>
          ))}
          {/* windows right */}
          {[0,1,2,3].map(i => (
            <rect key={i} x="50" y={36 + i*10} width="6" height="5" fill="#fff" opacity="0.4"/>
          ))}
          {[0,1,2,3].map(i => (
            <rect key={i} x="60" y={31 + i*10} width="6" height="5" fill="#fff" opacity="0.4"/>
          ))}
        </g>

        {/* annex (front, shorter) */}
        <g transform="translate(56 24)">
          <path d="M 0 24 L 0 58 L 28 70 L 28 38 Z" fill={light}/>
          <path d="M 28 38 L 28 70 L 50 60 L 50 28 Z" fill={c}/>
          <path d="M 0 24 L 28 10 L 50 28 L 28 38 Z" fill="#fff" opacity="0.92"/>
          {/* door */}
          <path d="M 12 50 L 12 60 L 18 62 L 18 52 Z" fill={dark} opacity="0.7"/>
          <rect x="34" y="42" width="6" height="5" fill="#fff" opacity="0.6"/>
          <rect x="42" y="38" width="6" height="5" fill="#fff" opacity="0.6"/>
        </g>
      </g>

      {/* floating elements */}
      <circle cx="60" cy="34" r="3" fill={c}/>
      <circle cx="76" cy="46" r="1.6" fill={c} opacity="0.6"/>
      <path d="M222 90 q 6 -6 12 0" stroke={c} strokeWidth="1.5" fill="none" opacity="0.5"/>
    </svg>
  );
}

/* ── 4. 现场工程师 — big gear + circuit grid */
function Cover_xc({ color, bg }) {
  const c = color;
  const teeth = 12;
  const cx = 90, cy = 60, rOuter = 40, rInner = 32;
  let d = '';
  for (let i=0; i<teeth*2; i++){
    const a = (i/(teeth*2)) * Math.PI*2 - Math.PI/2;
    const rr = i%2===0 ? rOuter : rInner;
    const x = cx + Math.cos(a)*rr;
    const y = cy + Math.sin(a)*rr;
    d += (i===0?'M':'L') + x.toFixed(1) + ' ' + y.toFixed(1) + ' ';
  }
  d += 'Z';
  return (
    <svg viewBox="0 0 280 120" preserveAspectRatio="xMidYMid slice"
         style={{ width: '100%', height: '100%', display: 'block' }}>
      <defs>
        <linearGradient id="xc-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={bg} />
          <stop offset="1" stopColor={mix(color, 88)} />
        </linearGradient>
        <radialGradient id="xc-gear" cx="0.35" cy="0.35">
          <stop offset="0" stopColor={mix(color, 25)}/>
          <stop offset="1" stopColor={c}/>
        </radialGradient>
        <pattern id="xc-grid" width="14" height="14" patternUnits="userSpaceOnUse">
          <path d="M 14 0 L 0 0 0 14" fill="none" stroke={c} strokeOpacity="0.18" strokeWidth="1"/>
        </pattern>
      </defs>
      <rect width="280" height="120" fill="url(#xc-bg)"/>
      {/* circuit grid right half */}
      <rect x="150" y="0" width="130" height="120" fill="url(#xc-grid)"/>

      <circle cx="245" cy="20" r="40" fill={c} opacity="0.12"/>

      {/* big gear */}
      <g>
        <path d={d} fill="url(#xc-gear)"/>
        <circle cx={cx} cy={cy} r="12" fill={bg}/>
        <circle cx={cx} cy={cy} r="5" fill={c}/>
      </g>

      {/* small gear */}
      <g transform="translate(60 100)">
        {(() => {
          const t = 8, ro = 12, ri = 9;
          let dd = '';
          for (let i=0;i<t*2;i++){
            const a = (i/(t*2))*Math.PI*2 - Math.PI/2;
            const rr = i%2===0?ro:ri;
            dd += (i===0?'M':'L') + (Math.cos(a)*rr).toFixed(1) + ' ' + (Math.sin(a)*rr).toFixed(1) + ' ';
          }
          dd += 'Z';
          return <path d={dd} fill={c} opacity="0.55"/>;
        })()}
        <circle r="4" fill={bg}/>
      </g>

      {/* circuit lines on right */}
      <g stroke={c} strokeWidth="2" fill="none" strokeLinecap="round" strokeLinejoin="round">
        <path d="M154 32 H 188 V 18 H 232"/>
        <path d="M154 60 H 210"/>
        <path d="M154 88 H 188 V 102 H 244"/>
      </g>
      <g fill={c}>
        <circle cx="232" cy="18" r="3.5"/>
        <circle cx="210" cy="60" r="3.5"/>
        <circle cx="244" cy="102" r="3.5"/>
      </g>
      <circle cx="232" cy="18"  r="6" fill="none" stroke={c} strokeOpacity="0.4" strokeWidth="1"/>
      <circle cx="244" cy="102" r="6" fill="none" stroke={c} strokeOpacity="0.4" strokeWidth="1"/>
    </svg>
  );
}

/* ── 5. 学习辅导 — floating book + graduation spark */
function Cover_xx({ color, bg }) {
  const c = color;
  const light = mix(color, 40);
  const dark = deeper(color, 10);
  return (
    <svg viewBox="0 0 280 120" preserveAspectRatio="xMidYMid slice"
         style={{ width: '100%', height: '100%', display: 'block' }}>
      <defs>
        <linearGradient id="xx-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={bg} />
          <stop offset="1" stopColor={mix(color, 88)} />
        </linearGradient>
        <filter id="xx-shadow" x="-20%" y="-20%" width="140%" height="140%">
          <feDropShadow dx="0" dy="3" stdDeviation="4" floodColor={color} floodOpacity="0.22"/>
        </filter>
      </defs>
      <rect width="280" height="120" fill="url(#xx-bg)"/>
      <circle cx="40" cy="20"  r="44" fill={c} opacity="0.14"/>
      <circle cx="240" cy="100" r="36" fill={c} opacity="0.1"/>

      {/* open book */}
      <g transform="translate(90 30) rotate(-6 50 30)" filter="url(#xx-shadow)">
        {/* left page */}
        <path d="M 4 6 Q 50 -2 50 8 L 50 60 Q 50 50 4 58 Z" fill="#fff"/>
        {/* right page */}
        <path d="M 96 6 Q 50 -2 50 8 L 50 60 Q 50 50 96 58 Z" fill="#fff"/>
        {/* binding */}
        <path d="M 50 8 L 50 60" stroke={c} strokeOpacity="0.25" strokeWidth="1"/>
        {/* text lines */}
        <g stroke={c} strokeOpacity="0.45" strokeWidth="1.4" strokeLinecap="round">
          <line x1="12" y1="20" x2="42" y2="14"/>
          <line x1="12" y1="28" x2="42" y2="22"/>
          <line x1="12" y1="36" x2="36" y2="30"/>
          <line x1="58" y1="14" x2="88" y2="20"/>
          <line x1="58" y1="22" x2="88" y2="28"/>
          <line x1="58" y1="30" x2="82" y2="36"/>
        </g>
        {/* page corners colored */}
        <path d="M 4 6 Q 28 2 50 6 L 50 14 Q 28 12 4 16 Z" fill={light}/>
        <path d="M 96 6 Q 72 2 50 6 L 50 14 Q 72 12 96 16 Z" fill={c} opacity="0.7"/>
      </g>

      {/* graduation cap floating */}
      <g transform="translate(210 32) rotate(15)">
        <path d="M -16 0 L 0 -8 L 16 0 L 0 8 Z" fill={dark}/>
        <path d="M -10 4 L -10 12 Q 0 16 10 12 L 10 4" fill={c} opacity="0.85"/>
        <circle cx="14" cy="-2" r="1.5" fill={c}/>
        <path d="M 14 -2 Q 18 4 16 8" stroke={c} strokeWidth="1.2" fill="none"/>
      </g>

      {/* sparkles */}
      <g fill={c}>
        <path d="M44 80 L46 85 L51 87 L46 89 L44 94 L42 89 L37 87 L42 85 Z" opacity="0.75"/>
        <circle cx="230" cy="80" r="2"/>
        <circle cx="60" cy="40" r="1.6" opacity="0.7"/>
      </g>
    </svg>
  );
}

const COVER_MAP = { pj: Cover_pj, gw: Cover_gw, cy: Cover_cy, xc: Cover_xc, xx: Cover_xx };
window.COVER_MAP = COVER_MAP;
