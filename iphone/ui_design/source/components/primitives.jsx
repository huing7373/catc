// primitives.jsx — 通用原子组件（图标、按钮、卡片等）

// SVG 图标集 —— 圆润风格
const Icons = {
  home: (s=24,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 11l9-8 9 8"/><path d="M5 10v9a1 1 0 001 1h3v-6h6v6h3a1 1 0 001-1v-9"/>
    </svg>
  ),
  box: (s=24,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 7l9-4 9 4v10l-9 4-9-4V7z"/><path d="M3 7l9 4 9-4"/><path d="M12 11v10"/>
    </svg>
  ),
  friends: (s=24,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="9" cy="8" r="3.2"/><path d="M3 20c0-3.3 2.7-6 6-6s6 2.7 6 6"/>
      <circle cx="17" cy="7" r="2.4"/><path d="M21 18c0-2.5-1.8-4.5-4-4.5"/>
    </svg>
  ),
  user: (s=24,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="8" r="4"/><path d="M4 21c0-4.4 3.6-8 8-8s8 3.6 8 8"/>
    </svg>
  ),
  paw: (s=20,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill={c}>
      <ellipse cx="6" cy="9" rx="2" ry="2.6"/><ellipse cx="10" cy="6" rx="2" ry="2.6"/>
      <ellipse cx="14" cy="6" rx="2" ry="2.6"/><ellipse cx="18" cy="9" rx="2" ry="2.6"/>
      <path d="M12 11c-3 0-5.5 2-5.5 5 0 2 1.5 3.5 3.5 3.5 1 0 1.5-.5 2-.5s1 .5 2 .5c2 0 3.5-1.5 3.5-3.5 0-3-2.5-5-5.5-5z"/>
    </svg>
  ),
  bowl: (s=22,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 11h18a8 8 0 01-8 8h-2a8 8 0 01-8-8z" fill={c} fillOpacity="0.15"/>
      <path d="M3 11h18"/>
      <path d="M8 7c0-1 1-2 2-2M12 6c0-1 1-2 2-2M16 7c0-1 1-2 2-2"/>
    </svg>
  ),
  heart: (s=22,c='currentColor',filled=false) => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill={filled?c:'none'} stroke={c} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 20s-7-4.3-7-10a4.5 4.5 0 018-3 4.5 4.5 0 018 3c0 5.7-7 10-7 10z"/>
    </svg>
  ),
  ball: (s=22,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="9" fill={c} fillOpacity="0.15"/>
      <path d="M3 12c3-2 6-2 9 0s6 2 9 0"/>
      <path d="M12 3c2 3 2 6 0 9s-2 6 0 9"/>
    </svg>
  ),
  footprint: (s=18,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill={c}>
      <ellipse cx="8" cy="8" rx="2" ry="2.6"/><ellipse cx="14" cy="6" rx="1.8" ry="2.4"/>
      <ellipse cx="18" cy="10" rx="1.8" ry="2.4"/>
      <path d="M13 13c-2 0-4 1.5-4 3.8 0 1.5 1 2.6 2.4 2.6.7 0 1-.3 1.6-.3s.9.3 1.6.3c1.4 0 2.4-1.1 2.4-2.6 0-2.3-2-3.8-4-3.8z"/>
    </svg>
  ),
  plus: (s=20,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2.6" strokeLinecap="round">
      <path d="M12 5v14M5 12h14"/>
    </svg>
  ),
  enter: (s=20,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M15 3h4a2 2 0 012 2v14a2 2 0 01-2 2h-4"/><path d="M10 17l5-5-5-5"/><path d="M15 12H3"/>
    </svg>
  ),
  close: (s=20,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2.4" strokeLinecap="round">
      <path d="M6 6l12 12M18 6l-12 12"/>
    </svg>
  ),
  back: (s=22,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round">
      <path d="M15 6l-6 6 6 6"/>
    </svg>
  ),
  dot: (s=8,c='currentColor') => (<svg width={s} height={s} viewBox="0 0 8 8"><circle cx="4" cy="4" r="4" fill={c}/></svg>),
  copy: (s=18,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 012-2h10"/>
    </svg>
  ),
  check: (s=20,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round">
      <path d="M5 12l5 5L20 7"/>
    </svg>
  ),
  settings: (s=22,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3"/>
      <path d="M19.4 15a1.7 1.7 0 00.3 1.8l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.8-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 01-4 0v-.1a1.7 1.7 0 00-1-1.5 1.7 1.7 0 00-1.8.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.8 1.7 1.7 0 00-1.5-1H3a2 2 0 010-4h.1a1.7 1.7 0 001.5-1 1.7 1.7 0 00-.3-1.8l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.8.3h.1a1.7 1.7 0 001-1.5V3a2 2 0 014 0v.1a1.7 1.7 0 001 1.5 1.7 1.7 0 001.8-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.8v.1a1.7 1.7 0 001.5 1H21a2 2 0 010 4h-.1a1.7 1.7 0 00-1.5 1z"/>
    </svg>
  ),
  sparkle: (s=18,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill={c}>
      <path d="M12 2l1.5 5.5L19 9l-5.5 1.5L12 16l-1.5-5.5L5 9l5.5-1.5L12 2z"/>
    </svg>
  ),
  bell: (s=22,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M6 8a6 6 0 1112 0c0 7 3 7 3 9H3c0-2 3-2 3-9z"/><path d="M10 21a2 2 0 004 0"/>
    </svg>
  ),
  chevronRight: (s=18,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 6l6 6-6 6"/>
    </svg>
  ),
  wechat: (s=20,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill={c}>
      <path d="M8.5 4C4.9 4 2 6.5 2 9.5c0 1.7 1 3.2 2.5 4.2L4 16l2.5-1.3c.6.2 1.3.3 2 .3.2 0 .4 0 .6-.1-.1-.4-.1-.8-.1-1.2 0-3.1 3-5.7 6.7-5.7.3 0 .6 0 .9.1C16 6.1 12.6 4 8.5 4zM6 8.5c.6 0 1 .5 1 1s-.4 1-1 1-1-.5-1-1 .4-1 1-1zm5 0c.6 0 1 .5 1 1s-.4 1-1 1-1-.5-1-1 .4-1 1-1z"/>
      <path d="M22 13.8c0-2.5-2.4-4.5-5.5-4.5s-5.5 2-5.5 4.5 2.4 4.5 5.5 4.5c.6 0 1.2-.1 1.7-.2L20.5 19l-.4-1.5c1.2-.8 1.9-2 1.9-3.7zm-7.5-1c-.4 0-.8-.4-.8-.8s.4-.8.8-.8.8.4.8.8-.4.8-.8.8zm4 0c-.4 0-.8-.4-.8-.8s.4-.8.8-.8.8.4.8.8-.4.8-.8.8z"/>
    </svg>
  ),
  shield: (s=20,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 2l8 3v6c0 5-3.5 9-8 11-4.5-2-8-6-8-11V5l8-3z"/>
    </svg>
  ),
  warn: (s=22,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M10.3 3.7L2 18a2 2 0 001.7 3h16.6a2 2 0 001.7-3L13.7 3.7a2 2 0 00-3.4 0z"/>
      <path d="M12 9v4"/><path d="M12 17h.01"/>
    </svg>
  ),
  diamond: (s=18,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill={c}>
      <path d="M12 2l7 8-7 12L5 10l7-8z" opacity="0.9"/>
    </svg>
  ),
  trophy: (s=22,c='currentColor') => (
    <svg width={s} height={s} viewBox="0 0 24 24" fill="none" stroke={c} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M8 21h8"/><path d="M12 17v4"/>
      <path d="M7 4h10v5a5 5 0 01-10 0V4z"/>
      <path d="M17 5h3v3a3 3 0 01-3 3"/>
      <path d="M7 5H4v3a3 3 0 003 3"/>
    </svg>
  ),
};

// 圆润主按钮
function PrimaryButton({ children, onClick, variant='primary', icon, fullWidth, style={}, disabled=false }) {
  const styles = {
    primary: { bg: 'var(--accent)', fg: 'white', shadow: '0 4px 0 var(--accent-deep)' },
    secondary:{ bg: 'var(--surface)', fg: 'var(--ink)', shadow: '0 4px 0 rgba(0,0,0,0.08)', border:'1.5px solid var(--border)' },
    ghost:    { bg: 'var(--accent-soft)', fg: 'var(--accent-deep)', shadow: '0 3px 0 rgba(0,0,0,0.06)' },
  }[variant];
  return (
    <button onClick={onClick} disabled={disabled} style={{
      height: 52, padding: '0 22px', borderRadius: 26,
      border: styles.border || 'none',
      background: styles.bg, color: styles.fg,
      fontFamily: 'var(--app-font)', fontSize: 16, fontWeight: 700,
      boxShadow: styles.shadow, cursor: disabled?'default':'pointer',
      display:'inline-flex', alignItems:'center', justifyContent:'center', gap: 8,
      width: fullWidth ? '100%' : 'auto', opacity: disabled?0.5:1,
      transition: 'transform 0.1s ease',
      ...style,
    }}
    onMouseDown={(e)=>!disabled && (e.currentTarget.style.transform='translateY(2px)')}
    onMouseUp={(e)=>e.currentTarget.style.transform=''}
    onMouseLeave={(e)=>e.currentTarget.style.transform=''}
    >
      {icon}{children}
    </button>
  );
}

// 卡片
function Card({ children, style={}, onClick }) {
  return (
    <div onClick={onClick} style={{
      background: 'var(--surface)', borderRadius: 24,
      padding: 16, boxShadow: 'var(--shadow-sm)',
      border: '1px solid var(--border)',
      cursor: onClick ? 'pointer' : 'default',
      ...style,
    }}>{children}</div>
  );
}

// 头像（带占位圆）
function Avatar({ name, size=44, color, online, ring }) {
  const palette = ['#ffb3c1','#ffd6a5','#caffbf','#bdb2ff','#a0c4ff','#ffc8dd','#b8e0d2'];
  const hash = (name||'').split('').reduce((a,c)=>a+c.charCodeAt(0),0);
  const bg = color || palette[hash % palette.length];
  const initial = (name||'?').charAt(0).toUpperCase();
  return (
    <div style={{
      position:'relative', width: size, height: size, borderRadius: '50%',
      background: bg, display:'flex', alignItems:'center', justifyContent:'center',
      color: 'rgba(0,0,0,0.55)', fontWeight: 800,
      fontSize: size*0.4, flexShrink: 0,
      boxShadow: ring ? '0 0 0 3px var(--surface), 0 0 0 5px var(--accent)' : 'inset 0 -2px 0 rgba(0,0,0,0.08)',
    }}>
      {initial}
      {online !== undefined && (
        <div style={{
          position:'absolute', right: 0, bottom: 0,
          width: size*0.28, height: size*0.28, borderRadius:'50%',
          background: online?'var(--success)':'#c3bdb9',
          border: '2.5px solid var(--surface)',
        }}/>
      )}
    </div>
  );
}

// Tab 页面切换时的淡入
function FadeIn({ children, keyProp }) {
  return <div key={keyProp} style={{animation:'fadeIn 0.28s ease both', height:'100%'}}>{children}</div>;
}

Object.assign(window, { Icons, PrimaryButton, Card, Avatar, FadeIn });
