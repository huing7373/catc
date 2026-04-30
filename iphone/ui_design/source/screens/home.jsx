// home.jsx — 首页（养猫主界面）

function HomeScreen({ state, onCreateTeam, onJoinTeamClick, catName, steps }) {
  const [mood, setMood] = React.useState('happy');
  const [hearts, setHearts] = React.useState([]);

  const pulse = (kind) => {
    setMood('happy');
    const id = Date.now();
    setHearts(h => [...h, { id, kind }]);
    setTimeout(() => setHearts(h => h.filter(x => x.id !== id)), 1400);
  };

  return (
    <div style={{
      height:'100%', padding:'68px 20px 100px',
      display:'flex', flexDirection:'column', gap: 14,
      background:'linear-gradient(180deg, var(--accent-soft) 0%, var(--page-bg) 38%)',
      overflow: 'auto',
    }}>
      {/* 顶部状态条 */}
      <div style={{display:'flex', justifyContent:'space-between', alignItems:'center', paddingTop: 4}}>
        <div>
          <div style={{fontSize: 12, color:'var(--ink-soft)', fontWeight:600}}>今天 · 晴</div>
          <div style={{fontSize: 22, fontWeight:800, color:'var(--ink)', fontFamily:'var(--app-font)'}}>
            {catName} 想你啦 ♥
          </div>
        </div>
        <div style={{
          display:'flex', alignItems:'center', gap: 6,
          background:'var(--surface)', padding:'8px 14px', borderRadius: 20,
          boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)',
        }}>
          {Icons.footprint(18, 'var(--coin)')}
          <span style={{fontWeight:800, color:'var(--ink)', fontSize: 15}}>{steps.toLocaleString()}</span>
          <span style={{fontSize: 11, color:'var(--ink-soft)', fontWeight:600}}>步</span>
        </div>
      </div>

      {/* 小猫舞台 */}
      <div style={{
        position:'relative', borderRadius: 28, padding: 20,
        background:'var(--surface)',
        boxShadow:'var(--shadow-md)', border:'1px solid var(--border)',
        overflow:'hidden',
      }}>
        {/* 背景斑点 */}
        <div style={{position:'absolute', top: 20, right: 30, width: 50, height: 50, borderRadius:'50%', background:'var(--accent-soft)', opacity:0.5}}/>
        <div style={{position:'absolute', bottom: 40, left: 20, width: 30, height: 30, borderRadius:'50%', background:'var(--accent-soft)', opacity:0.4}}/>

        <div style={{display:'flex', justifyContent:'center', padding:'12px 0 20px', position:'relative'}}>
          <CatPlaceholder size={220} mood={mood} label="猫 3D 模型"/>
          {/* 飘心 */}
          {hearts.map(h => (
            <div key={h.id} style={{
              position:'absolute', top: 60, left:'50%',
              animation: 'floatUp 1.4s ease-out forwards',
              pointerEvents:'none', fontSize: 28,
            }}>
              {h.kind==='feed' ? '🍥' : h.kind==='pet' ? '💕' : '⭐'}
            </div>
          ))}
        </div>

        {/* 猫的名字标签 */}
        <div style={{
          position:'absolute', top: 16, left: 16,
          background:'var(--accent)', color:'white',
          padding:'4px 12px', borderRadius: 12,
          fontSize: 12, fontWeight: 700,
        }}>
          Lv.8 · {catName}
        </div>

        {/* 状态条 */}
        <div style={{display:'flex', gap: 10, padding:'4px 4px 8px'}}>
          <StatusBar label="饱食" value={72} color="var(--warn)" icon={Icons.bowl(14,'white')}/>
          <StatusBar label="心情" value={88} color="var(--accent)" icon={Icons.heart(14,'white',true)}/>
          <StatusBar label="活力" value={65} color="var(--success)" icon={Icons.ball(14,'white')}/>
        </div>
      </div>

      {/* 互动按钮 */}
      <div style={{display:'flex', gap: 10}}>
        <ActionButton label="喂食"  icon={Icons.bowl(24,'var(--accent-deep)')}  onClick={()=>pulse('feed')}/>
        <ActionButton label="抚摸"  icon={Icons.heart(24,'var(--accent-deep)',true)} onClick={()=>pulse('pet')}/>
        <ActionButton label="玩耍"  icon={Icons.ball(24,'var(--accent-deep)')}   onClick={()=>pulse('play')}/>
      </div>

      {/* 底部——根据是否有队伍显示不同 */}
      {state === 'idle' ? (
        <TeamIdleCard onCreate={onCreateTeam} onJoin={onJoinTeamClick}/>
      ) : null}

      <style>{`
        @keyframes floatUp {
          0%   { transform: translate(-50%, 0) scale(0.5); opacity: 0; }
          25%  { transform: translate(-50%, -20px) scale(1.2); opacity: 1; }
          100% { transform: translate(-50%, -110px) scale(0.8); opacity: 0; }
        }
        @keyframes fadeIn {
          from { opacity: 0; transform: translateY(8px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  );
}

function StatusBar({ label, value, color, icon }) {
  return (
    <div style={{flex: 1}}>
      <div style={{display:'flex', justifyContent:'space-between', alignItems:'center', marginBottom: 4}}>
        <div style={{display:'flex', alignItems:'center', gap: 4}}>
          <div style={{width:20, height:20, borderRadius:10, background: color, display:'flex', alignItems:'center', justifyContent:'center'}}>
            {icon}
          </div>
          <span style={{fontSize: 11, color:'var(--ink-soft)', fontWeight: 700}}>{label}</span>
        </div>
        <span style={{fontSize: 11, fontWeight: 800, color:'var(--ink)'}}>{value}</span>
      </div>
      <div style={{height: 6, borderRadius: 3, background:'var(--surface-2)', overflow:'hidden'}}>
        <div style={{height:'100%', width:`${value}%`, background: color, borderRadius: 3, transition:'width 0.4s'}}/>
      </div>
    </div>
  );
}

function ActionButton({ label, icon, onClick }) {
  return (
    <button onClick={onClick} style={{
      flex: 1, height: 72, border:'none', cursor:'pointer',
      background:'var(--surface)', borderRadius: 20,
      boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)',
      display:'flex', flexDirection:'column', alignItems:'center', justifyContent:'center', gap: 4,
      transition:'transform 0.1s',
    }}
    onMouseDown={(e)=>e.currentTarget.style.transform='translateY(2px)'}
    onMouseUp={(e)=>e.currentTarget.style.transform=''}
    onMouseLeave={(e)=>e.currentTarget.style.transform=''}>
      {icon}
      <span style={{fontSize: 12, fontWeight: 700, color:'var(--ink)', fontFamily:'var(--app-font)'}}>{label}</span>
    </button>
  );
}

function TeamIdleCard({ onCreate, onJoin }) {
  return (
    <div style={{
      background:'linear-gradient(135deg, var(--accent) 0%, var(--accent-deep) 100%)',
      borderRadius: 24, padding: 18, color:'white',
      boxShadow:'var(--shadow-md)', position:'relative', overflow:'hidden',
    }}>
      <div style={{position:'absolute', right:-20, top:-20, width: 100, height: 100, borderRadius:'50%', background:'rgba(255,255,255,0.1)'}}/>
      <div style={{position:'absolute', right: 20, bottom: -30, width: 70, height: 70, borderRadius:'50%', background:'rgba(255,255,255,0.08)'}}/>

      <div style={{display:'flex', alignItems:'center', gap: 10, marginBottom: 6}}>
        {Icons.paw(22, 'white')}
        <div style={{fontSize: 16, fontWeight: 800}}>和好友一起玩耍</div>
      </div>
      <div style={{fontSize: 13, opacity: 0.88, marginBottom: 14}}>
        创建一个小屋，或用房间代码加入好友的队伍
      </div>

      <div style={{display:'flex', gap: 10, position:'relative', zIndex: 1}}>
        <button onClick={onCreate} style={{
          flex: 1, height: 46, border:'none', cursor:'pointer',
          background:'white', color:'var(--accent-deep)',
          borderRadius: 23, fontWeight: 800, fontSize: 14,
          display:'inline-flex', alignItems:'center', justifyContent:'center', gap: 6,
          fontFamily:'var(--app-font)',
        }}>
          {Icons.plus(18, 'var(--accent-deep)')} 创建队伍
        </button>
        <button onClick={onJoin} style={{
          flex: 1, height: 46, cursor:'pointer',
          background:'rgba(255,255,255,0.22)', color:'white',
          border:'1.5px solid rgba(255,255,255,0.5)',
          borderRadius: 23, fontWeight: 800, fontSize: 14,
          display:'inline-flex', alignItems:'center', justifyContent:'center', gap: 6,
          fontFamily:'var(--app-font)',
        }}>
          {Icons.enter(18, 'white')} 加入队伍
        </button>
      </div>
    </div>
  );
}

Object.assign(window, { HomeScreen });
