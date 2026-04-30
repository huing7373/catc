// room.jsx — 队伍房间界面（与 Home 互斥）

function RoomScreen({ roomCode, members, onLeave, catName }) {
  const [copied, setCopied] = React.useState(false);
  const copy = () => {
    navigator.clipboard?.writeText(roomCode);
    setCopied(true);
    setTimeout(()=>setCopied(false), 1200);
  };

  return (
    <div style={{
      height:'100%', padding:'68px 20px 100px',
      display:'flex', flexDirection:'column', gap: 14,
      background:'linear-gradient(180deg, var(--accent-soft) 0%, var(--page-bg) 45%)',
      overflow:'auto',
    }}>
      {/* 顶部 */}
      <div style={{display:'flex', alignItems:'center', justifyContent:'space-between', paddingTop: 4}}>
        <button onClick={onLeave} style={{
          border:'none', background:'var(--surface)', cursor:'pointer',
          width: 40, height: 40, borderRadius: 20,
          display:'flex', alignItems:'center', justifyContent:'center',
          boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)', color:'var(--ink)',
        }}>{Icons.back(20,'var(--ink)')}</button>
        <div style={{textAlign:'center'}}>
          <div style={{fontSize: 11, color:'var(--ink-soft)', fontWeight: 700, letterSpacing:'0.5px'}}>队伍房间</div>
          <div style={{fontSize: 18, fontWeight: 800, color:'var(--ink)'}}>{catName}的小屋</div>
        </div>
        <div style={{width: 40}}/>
      </div>

      {/* 房间代码卡片 */}
      <div style={{
        background:'var(--surface)', borderRadius: 22, padding: 14,
        boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)',
        display:'flex', alignItems:'center', justifyContent:'space-between',
      }}>
        <div>
          <div style={{fontSize: 11, color:'var(--ink-soft)', fontWeight: 700}}>房间代码</div>
          <div style={{
            fontSize: 22, fontWeight: 800, color:'var(--accent-deep)',
            letterSpacing: '3px', fontFamily:'SF Mono, Menlo, monospace',
          }}>{roomCode}</div>
        </div>
        <button onClick={copy} style={{
          padding:'10px 14px', border:'none', cursor:'pointer',
          background: copied ? 'var(--success)' : 'var(--accent-soft)',
          color: copied ? 'white' : 'var(--accent-deep)',
          borderRadius: 16, fontWeight: 800, fontSize: 12,
          display:'inline-flex', alignItems:'center', gap: 6, transition:'all 0.2s',
        }}>
          {copied ? Icons.check(16, 'white') : Icons.copy(16, 'var(--accent-deep)')}
          {copied ? '已复制' : '复制'}
        </button>
      </div>

      {/* 房间舞台 */}
      <div style={{
        background:'linear-gradient(180deg, #fff2e0 0%, #ffe0e9 100%)',
        borderRadius: 28, padding: '16px 14px',
        boxShadow:'var(--shadow-md)',
        border:'1px solid var(--border)',
        position:'relative', overflow:'hidden', minHeight: 260,
      }}>
        {/* 房间装饰 */}
        <div style={{
          position:'absolute', left: 0, right: 0, bottom: 0, height: 50,
          background:'linear-gradient(180deg, rgba(218,165,132,0) 0%, rgba(218,165,132,0.25) 100%)',
        }}/>
        {/* 猫玩具/家具元素 */}
        <div style={{position:'absolute', right: 18, bottom: 40, fontSize: 32}}>🧶</div>
        <div style={{position:'absolute', left: 20, bottom: 44, fontSize: 28}}>🐟</div>
        <div style={{position:'absolute', top: 14, right: 20, fontSize: 22, opacity: 0.6}}>☁️</div>
        <div style={{position:'absolute', top: 34, left: 30, fontSize: 18, opacity: 0.5}}>☁️</div>

        <div style={{
          fontSize: 11, color:'var(--ink-soft)', fontWeight: 700,
          background:'rgba(255,255,255,0.7)', display:'inline-block',
          padding:'4px 10px', borderRadius: 10, marginBottom: 10,
        }}>
          {members.length} 只小猫在玩耍
        </div>

        {/* 小猫群 */}
        <div style={{
          display:'flex', justifyContent:'space-around', alignItems:'flex-end',
          padding: '14px 0 20px', flexWrap:'wrap', gap: 8,
        }}>
          {members.map((m, i) => (
            <div key={m.id} style={{
              animation: `bounce 2.2s ${i * 0.2}s ease-in-out infinite`,
              display:'flex', flexDirection:'column', alignItems:'center',
            }}>
              <MiniCat size={68} name={m.name}
                color={['#ffd6df','#dfe8c8','#cfe2f2','#f5d4a6'][i % 4]}/>
            </div>
          ))}
        </div>
      </div>

      {/* 成员列表 */}
      <div>
        <div style={{fontSize: 13, fontWeight: 800, color:'var(--ink)', marginBottom: 8, padding:'0 4px'}}>
          成员 ({members.length}/4)
        </div>
        <div style={{display:'flex', flexDirection:'column', gap: 8}}>
          {members.map(m => (
            <div key={m.id} style={{
              display:'flex', alignItems:'center', gap: 12,
              background:'var(--surface)', padding: 10, borderRadius: 16,
              boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)',
            }}>
              <Avatar name={m.name} size={40}/>
              <div style={{flex: 1}}>
                <div style={{fontSize: 14, fontWeight: 800, color:'var(--ink)'}}>
                  {m.name} {m.isHost && <span style={{fontSize:10, padding:'2px 6px', background:'var(--accent-soft)', color:'var(--accent-deep)', borderRadius: 8, marginLeft: 4, fontWeight: 800}}>队长</span>}
                </div>
                <div style={{fontSize: 11, color:'var(--ink-soft)', fontWeight: 600}}>
                  小猫 Lv.{m.level} · {m.status}
                </div>
              </div>
              {Icons.paw(16,'var(--accent)')}
            </div>
          ))}
          {/* 空位 */}
          {Array.from({length: 4 - members.length}).map((_, i) => (
            <div key={'e'+i} style={{
              height: 60, border:'2px dashed var(--border)', borderRadius: 16,
              display:'flex', alignItems:'center', justifyContent:'center',
              color:'var(--ink-mute)', fontSize: 13, fontWeight: 700,
            }}>
              + 等待好友加入
            </div>
          ))}
        </div>
      </div>

      {/* 离开按钮 */}
      <button onClick={onLeave} style={{
        height: 48, border:'1.5px solid var(--border)', cursor:'pointer',
        background:'var(--surface)', color:'var(--ink-soft)',
        borderRadius: 24, fontWeight: 800, fontSize: 14, marginTop: 4,
        fontFamily:'var(--app-font)',
      }}>
        离开房间
      </button>

      <style>{`
        @keyframes bounce {
          0%, 100% { transform: translateY(0); }
          50%      { transform: translateY(-6px); }
        }
      `}</style>
    </div>
  );
}

Object.assign(window, { RoomScreen });
