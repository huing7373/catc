// cat-placeholder.jsx — 3D 猫模型占位符（条纹纹理 + 标注）

function CatPlaceholder({ size=220, label='猫 3D 模型', mood='happy', accessories=[] }) {
  // 条纹纹理表示"资源占位"
  const stripes = (
    <pattern id="cat-stripes" patternUnits="userSpaceOnUse" width="8" height="8" patternTransform="rotate(45)">
      <rect width="8" height="8" fill="var(--surface-2)"/>
      <line x1="0" y1="0" x2="0" y2="8" stroke="var(--ink-mute)" strokeWidth="1.5" strokeOpacity="0.35"/>
    </pattern>
  );

  // 简化几何小猫轮廓（用 CSS 形状勾勒）
  return (
    <div style={{
      position:'relative', width: size, height: size,
      display:'flex', alignItems:'center', justifyContent:'center',
    }}>
      <svg viewBox="0 0 220 220" width={size} height={size}>
        <defs>{stripes}</defs>
        {/* 地面阴影 */}
        <ellipse cx="110" cy="195" rx="68" ry="10" fill="rgba(0,0,0,0.08)"/>

        {/* 身体 */}
        <ellipse cx="110" cy="150" rx="58" ry="44" fill="url(#cat-stripes)" stroke="var(--ink-mute)" strokeWidth="1.2" strokeDasharray="4 3"/>
        {/* 尾巴 */}
        <path d="M160,140 Q185,120 180,95" fill="none" stroke="var(--ink-mute)" strokeWidth="12" strokeDasharray="4 3" strokeLinecap="round" opacity="0.7"/>
        {/* 头 */}
        <circle cx="110" cy="90" r="48" fill="url(#cat-stripes)" stroke="var(--ink-mute)" strokeWidth="1.2" strokeDasharray="4 3"/>
        {/* 耳朵 */}
        <path d="M75,60 L70,30 L95,50 Z" fill="url(#cat-stripes)" stroke="var(--ink-mute)" strokeWidth="1.2" strokeDasharray="4 3"/>
        <path d="M145,60 L150,30 L125,50 Z" fill="url(#cat-stripes)" stroke="var(--ink-mute)" strokeWidth="1.2" strokeDasharray="4 3"/>
        {/* 脸 */}
        <circle cx="95" cy="88" r="3.5" fill="var(--ink)"/>
        <circle cx="125" cy="88" r="3.5" fill="var(--ink)"/>
        <path d={mood==='happy'?"M102,105 Q110,112 118,105":"M102,108 Q110,104 118,108"} stroke="var(--ink)" strokeWidth="2" fill="none" strokeLinecap="round"/>
        <path d="M108,98 L112,98 L110,101 Z" fill="var(--ink)"/>

        {/* 配饰（发夹/领结等） */}
        {accessories.includes('bow') && (
          <g transform="translate(110, 55)">
            <path d="M-16,0 L-6,-6 L-6,6 Z" fill="var(--accent)"/>
            <path d="M16,0 L6,-6 L6,6 Z" fill="var(--accent)"/>
            <circle r="4" fill="var(--accent-deep)"/>
          </g>
        )}
        {accessories.includes('hat') && (
          <g transform="translate(110, 42)">
            <rect x="-20" y="0" width="40" height="5" rx="2" fill="var(--accent-deep)"/>
            <path d="M-14,-18 L-14,0 L14,0 L14,-18 Z" fill="var(--accent)"/>
            <circle cx="0" cy="-18" r="5" fill="var(--accent-soft)"/>
          </g>
        )}
        {accessories.includes('scarf') && (
          <g>
            <ellipse cx="110" cy="132" rx="40" ry="8" fill="var(--accent)"/>
            <path d="M142,130 L152,160 L138,156 Z" fill="var(--accent-deep)"/>
          </g>
        )}

        {/* 四角角标标识这是占位符 */}
        <g opacity="0.45">
          <path d="M12 12 L28 12 L12 28 Z" fill="var(--ink-mute)"/>
          <path d="M208 12 L192 12 L208 28 Z" fill="var(--ink-mute)"/>
        </g>
      </svg>
      {label && (
        <div style={{
          position:'absolute', bottom: -6, left:'50%', transform:'translateX(-50%)',
          fontFamily: 'SF Mono, Menlo, monospace', fontSize: 10,
          background:'rgba(0,0,0,0.6)', color:'white', padding:'3px 10px', borderRadius: 999,
          whiteSpace:'nowrap', letterSpacing:'0.5px',
        }}>{label}</div>
      )}
    </div>
  );
}

// 小版本的猫（用于房间多只猫）
function MiniCat({ size=80, color='var(--accent-soft)', name }) {
  return (
    <div style={{position:'relative', width:size, height:size, display:'flex', flexDirection:'column', alignItems:'center'}}>
      <svg viewBox="0 0 100 100" width={size} height={size*0.9}>
        <ellipse cx="50" cy="85" rx="22" ry="4" fill="rgba(0,0,0,0.08)"/>
        <ellipse cx="50" cy="68" rx="25" ry="18" fill={color} stroke="var(--ink-mute)" strokeWidth="1" strokeDasharray="2 2"/>
        <circle cx="50" cy="42" r="22" fill={color} stroke="var(--ink-mute)" strokeWidth="1" strokeDasharray="2 2"/>
        <path d="M35,28 L32,14 L44,23 Z" fill={color} stroke="var(--ink-mute)" strokeWidth="1" strokeDasharray="2 2"/>
        <path d="M65,28 L68,14 L56,23 Z" fill={color} stroke="var(--ink-mute)" strokeWidth="1" strokeDasharray="2 2"/>
        <circle cx="42" cy="41" r="1.8" fill="var(--ink)"/>
        <circle cx="58" cy="41" r="1.8" fill="var(--ink)"/>
        <path d="M46,50 Q50,53 54,50" stroke="var(--ink)" strokeWidth="1.4" fill="none" strokeLinecap="round"/>
      </svg>
      {name && <div style={{fontSize:11, color:'var(--ink-soft)', fontWeight:600, marginTop:-4}}>{name}</div>}
    </div>
  );
}

Object.assign(window, { CatPlaceholder, MiniCat });
