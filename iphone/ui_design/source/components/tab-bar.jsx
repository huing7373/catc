// tab-bar.jsx — 底部 4 Tab 导航栏

function TabBar({ active, onChange }) {
  const tabs = [
    { id: 'home',     label: '家',   icon: Icons.home },
    { id: 'wardrobe', label: '仓库', icon: Icons.box },
    { id: 'friends',  label: '好友', icon: Icons.friends },
    { id: 'profile',  label: '我的', icon: Icons.user },
  ];
  return (
    <div style={{
      position:'absolute', left: 12, right: 12, bottom: 14,
      height: 72, borderRadius: 36,
      background: 'var(--surface)',
      boxShadow: 'var(--shadow-md), 0 0 0 1px var(--border)',
      display:'flex', alignItems:'center', justifyContent:'space-around',
      padding: '0 8px', zIndex: 5,
      backdropFilter: 'blur(20px)',
    }}>
      {tabs.map(t => {
        const a = active === t.id;
        return (
          <button key={t.id} onClick={()=>onChange(t.id)} style={{
            border:'none', background:'transparent', cursor:'pointer',
            display:'flex', flexDirection:'column', alignItems:'center', gap: 3,
            padding: '6px 10px', borderRadius: 20,
            color: a ? 'var(--accent-deep)' : 'var(--ink-mute)',
            transform: a ? 'translateY(-2px)' : 'none',
            transition: 'all 0.2s ease',
            position: 'relative',
          }}>
            <div style={{
              padding: '6px 14px', borderRadius: 18,
              background: a ? 'var(--accent-soft)' : 'transparent',
              transition: 'all 0.2s',
            }}>
              {t.icon(22, 'currentColor')}
            </div>
            <span style={{fontSize: 11, fontWeight: 700, fontFamily:'var(--app-font)'}}>{t.label}</span>
          </button>
        );
      })}
    </div>
  );
}

Object.assign(window, { TabBar });
