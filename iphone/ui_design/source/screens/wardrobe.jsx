// wardrobe.jsx — 仓库/试衣间（左侧分类 + 右侧预览）

function WardrobeScreen({ catName }) {
  const [cat, setCat] = React.useState('hat');
  const [equipped, setEquipped] = React.useState({ bow: null, hat: null, scarf: null });
  const [selected, setSelected] = React.useState(null);

  const categories = [
    { id: 'hat',     label: '帽子', icon: '🎩', count: 8 },
    { id: 'bow',     label: '饰品', icon: '🎀', count: 12 },
    { id: 'scarf',   label: '围巾', icon: '🧣', count: 5 },
    { id: 'outfit',  label: '服装', icon: '👘', count: 7 },
    { id: 'bg',      label: '背景', icon: '🏞️', count: 4 },
  ];

  const items = {
    hat:   [{id:'h1',name:'贝雷帽',rarity:'R',owned:true},{id:'h2',name:'草帽',rarity:'N',owned:true},{id:'h3',name:'皇冠',rarity:'SR',owned:true,equip:'hat'},{id:'h4',name:'魔法帽',rarity:'SSR',owned:false},{id:'h5',name:'蝴蝶帽',rarity:'R',owned:true},{id:'h6',name:'警官帽',rarity:'R',owned:false}],
    bow:   [{id:'b1',name:'粉色蝴蝶结',rarity:'N',owned:true,equip:'bow'},{id:'b2',name:'星星发夹',rarity:'R',owned:true},{id:'b3',name:'樱花发饰',rarity:'SR',owned:true},{id:'b4',name:'彩虹丝带',rarity:'SSR',owned:false}],
    scarf: [{id:'s1',name:'毛线围巾',rarity:'N',owned:true},{id:'s2',name:'骑士披风',rarity:'SR',owned:true,equip:'scarf'},{id:'s3',name:'太空斗篷',rarity:'SSR',owned:false}],
    outfit:[{id:'o1',name:'水手服',rarity:'R',owned:true},{id:'o2',name:'和服',rarity:'SR',owned:false}],
    bg:    [{id:'g1',name:'粉色房间',rarity:'N',owned:true},{id:'g2',name:'樱花树下',rarity:'SR',owned:true},{id:'g3',name:'星空',rarity:'SSR',owned:false}],
  };

  const currentItems = items[cat] || [];
  const activeItem = selected || currentItems.find(i => i.equip === cat) || currentItems[0];

  const toggleEquip = (item) => {
    if (!item.owned) return;
    setEquipped(prev => ({
      ...prev,
      [cat]: prev[cat] === item.id ? null : item.id,
    }));
  };
  const isEquipped = (item) => equipped[cat] === item.id || item.equip === cat;

  return (
    <div style={{height:'100%', display:'flex', flexDirection:'column', background:'var(--page-bg)'}}>
      {/* 顶部 */}
      <div style={{padding:'68px 20px 8px', display:'flex', justifyContent:'space-between', alignItems:'center'}}>
        <div>
          <div style={{fontSize: 12, color:'var(--ink-soft)', fontWeight: 700}}>收藏 · 36/53</div>
          <div style={{fontSize: 22, fontWeight: 800, color:'var(--ink)'}}>{catName} 的衣柜</div>
        </div>
        <div style={{display:'flex', alignItems:'center', gap: 4,
          background:'var(--surface)', padding:'6px 12px', borderRadius: 16,
          boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)',
        }}>
          {Icons.diamond(16, 'var(--accent)')}
          <span style={{fontWeight: 800, fontSize: 13, color:'var(--ink)'}}>248</span>
        </div>
      </div>

      {/* 3D 预览区 */}
      <div style={{
        margin: '4px 20px', borderRadius: 24, padding: 14,
        background:'linear-gradient(180deg, var(--accent-soft) 0%, var(--surface) 100%)',
        boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)',
        display:'flex', alignItems:'center', gap: 12, position:'relative', overflow:'hidden',
      }}>
        <div style={{
          position:'absolute', inset: 0,
          backgroundImage:'radial-gradient(circle at 20% 80%, var(--accent-soft) 0%, transparent 60%)',
          opacity: 0.6, pointerEvents:'none',
        }}/>
        <div style={{position:'relative'}}>
          <CatPlaceholder
            size={140}
            label="猫 3D 预览"
            accessories={[
              equipped.bow ? 'bow' : null,
              equipped.hat ? 'hat' : null,
              equipped.scarf ? 'scarf' : null,
            ].filter(Boolean)}
          />
        </div>
        <div style={{flex: 1, position:'relative', zIndex: 1}}>
          <div style={{fontSize: 11, color:'var(--ink-soft)', fontWeight:700, marginBottom: 4, letterSpacing:'0.5px'}}>当前预览</div>
          <div style={{fontSize: 17, fontWeight: 800, color:'var(--ink)', marginBottom: 8}}>
            {activeItem?.name || '未选择'}
          </div>
          <div style={{display:'flex', gap: 6, marginBottom: 10, flexWrap:'wrap'}}>
            {activeItem && <RarityTag rarity={activeItem.rarity}/>}
            {activeItem?.owned ? (
              <span style={{fontSize: 10, padding:'3px 8px', borderRadius: 8, background:'var(--success)', color:'white', fontWeight:800}}>已拥有</span>
            ) : (
              <span style={{fontSize: 10, padding:'3px 8px', borderRadius: 8, background:'var(--ink-mute)', color:'white', fontWeight:800}}>未解锁</span>
            )}
          </div>
          <button
            onClick={()=>activeItem && toggleEquip(activeItem)}
            disabled={!activeItem?.owned}
            style={{
              width:'100%', height: 36, border:'none',
              cursor: activeItem?.owned ? 'pointer' : 'default',
              background: isEquipped(activeItem||{}) ? 'var(--ink)' : 'var(--accent)',
              color:'white', borderRadius: 18,
              fontWeight: 800, fontSize: 12,
              opacity: activeItem?.owned ? 1 : 0.5,
              fontFamily:'var(--app-font)',
            }}>
            {isEquipped(activeItem||{}) ? '✓ 已装备 (点击卸下)' : '装备'}
          </button>
        </div>
      </div>

      {/* 分类 tab */}
      <div style={{display:'flex', gap: 6, padding:'6px 20px', overflowX:'auto'}}>
        {categories.map(c => (
          <button key={c.id} onClick={()=>{setCat(c.id); setSelected(null);}} style={{
            flexShrink: 0, padding:'8px 14px', border:'none', cursor:'pointer',
            background: cat===c.id ? 'var(--accent)' : 'var(--surface)',
            color: cat===c.id ? 'white' : 'var(--ink)',
            borderRadius: 16, fontWeight: 700, fontSize: 12,
            boxShadow: cat===c.id ? 'var(--shadow-sm)' : 'none',
            border: cat===c.id ? 'none' : '1px solid var(--border)',
            display:'inline-flex', alignItems:'center', gap: 6,
            fontFamily:'var(--app-font)',
          }}>
            <span>{c.icon}</span>
            <span>{c.label}</span>
            <span style={{fontSize: 10, opacity: 0.7}}>{c.count}</span>
          </button>
        ))}
      </div>

      {/* 网格 */}
      <div style={{flex: 1, overflow:'auto', padding:'8px 20px 100px'}}>
        <div style={{display:'grid', gridTemplateColumns:'repeat(3, 1fr)', gap: 10}}>
          {currentItems.map(item => (
            <button key={item.id}
              onClick={()=>setSelected(item)}
              style={{
                padding: 10, border:'none', cursor:'pointer',
                background:'var(--surface)', borderRadius: 16,
                boxShadow: selected?.id === item.id ? '0 0 0 2.5px var(--accent), var(--shadow-sm)' : 'var(--shadow-sm)',
                border:'1px solid var(--border)',
                display:'flex', flexDirection:'column', alignItems:'center', gap: 6,
                position:'relative', opacity: item.owned ? 1 : 0.55,
              }}>
              <div style={{
                width: 60, height: 60, borderRadius: 12,
                background:'var(--surface-2)',
                display:'flex', alignItems:'center', justifyContent:'center',
                position:'relative',
                backgroundImage:'repeating-linear-gradient(45deg, transparent 0 6px, rgba(0,0,0,0.04) 6px 7px)',
              }}>
                <div style={{fontSize: 28}}>{categories.find(c=>c.id===cat)?.icon}</div>
                {!item.owned && (
                  <div style={{position:'absolute', top: 3, right: 3, fontSize: 12}}>🔒</div>
                )}
                {isEquipped(item) && (
                  <div style={{
                    position:'absolute', top: -4, right: -4, width: 20, height: 20, borderRadius:'50%',
                    background:'var(--success)', display:'flex', alignItems:'center', justifyContent:'center',
                    border:'2px solid var(--surface)',
                  }}>{Icons.check(12,'white')}</div>
                )}
              </div>
              <div style={{fontSize: 11, fontWeight: 800, color:'var(--ink)', textAlign:'center'}}>{item.name}</div>
              <RarityTag rarity={item.rarity} small/>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

function RarityTag({ rarity, small }) {
  const map = {
    N:  { bg:'#b0b0b0', fg:'white' },
    R:  { bg:'#7db3e8', fg:'white' },
    SR: { bg:'#c58ae8', fg:'white' },
    SSR:{ bg:'linear-gradient(90deg,#ffd166,#ef476f)', fg:'white' },
  };
  const s = map[rarity] || map.N;
  return (
    <span style={{
      fontSize: small ? 9 : 10, padding: small ? '2px 6px' : '3px 8px',
      borderRadius: 6, background: s.bg, color: s.fg,
      fontWeight: 800, letterSpacing:'0.5px',
    }}>{rarity}</span>
  );
}

Object.assign(window, { WardrobeScreen });
