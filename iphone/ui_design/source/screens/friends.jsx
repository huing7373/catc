// friends.jsx — 好友界面

function FriendsScreen({ friends, onInvite, onJoinFriend, myRoomCode }) {
  const [tab, setTab] = React.useState('online');
  const filter = tab === 'online' ? friends.filter(f => f.online) : friends;

  return (
    <div style={{height:'100%', display:'flex', flexDirection:'column', background:'var(--page-bg)'}}>
      <div style={{padding:'68px 20px 8px', display:'flex', justifyContent:'space-between', alignItems:'center'}}>
        <div>
          <div style={{fontSize: 12, color:'var(--ink-soft)', fontWeight: 700}}>
            {friends.filter(f=>f.online).length} 位在线 · 共 {friends.length} 位
          </div>
          <div style={{fontSize: 22, fontWeight: 800, color:'var(--ink)'}}>好友</div>
        </div>
        <button style={{
          width: 40, height: 40, borderRadius: 20, border:'none', cursor:'pointer',
          background:'var(--surface)', boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)',
          display:'flex', alignItems:'center', justifyContent:'center',
        }}>
          {Icons.plus(20, 'var(--ink)')}
        </button>
      </div>

      {/* 我的房间提示 */}
      {myRoomCode && (
        <div style={{
          margin:'4px 20px 8px', padding: '10px 14px',
          background:'linear-gradient(90deg, var(--accent-soft), transparent)',
          borderRadius: 16, display:'flex', alignItems:'center', gap: 10,
          border:'1px solid var(--border)',
        }}>
          <div style={{
            width: 36, height: 36, borderRadius: 18, background:'var(--accent)',
            display:'flex', alignItems:'center', justifyContent:'center',
          }}>{Icons.paw(18,'white')}</div>
          <div style={{flex:1}}>
            <div style={{fontSize: 11, color:'var(--ink-soft)', fontWeight:700}}>你的房间</div>
            <div style={{fontSize: 14, fontWeight:800, color:'var(--ink)'}}>
              代码 <span style={{color:'var(--accent-deep)', letterSpacing:'2px', fontFamily:'SF Mono, Menlo, monospace'}}>{myRoomCode}</span>
            </div>
          </div>
        </div>
      )}

      {/* Tab */}
      <div style={{display:'flex', gap: 6, padding:'6px 20px'}}>
        {[{id:'online',label:'在线'},{id:'all',label:'全部'}].map(t => (
          <button key={t.id} onClick={()=>setTab(t.id)} style={{
            padding:'7px 18px', border:'none', cursor:'pointer',
            background: tab===t.id ? 'var(--ink)' : 'var(--surface)',
            color: tab===t.id ? 'var(--surface)' : 'var(--ink-soft)',
            borderRadius: 14, fontWeight: 800, fontSize: 12,
            border: tab===t.id ? 'none' : '1px solid var(--border)',
            fontFamily:'var(--app-font)',
          }}>{t.label}</button>
        ))}
      </div>

      {/* 好友列表 */}
      <div style={{flex:1, overflow:'auto', padding:'8px 20px 100px'}}>
        <div style={{display:'flex', flexDirection:'column', gap: 8}}>
          {filter.map(f => <FriendRow key={f.id} f={f} onInvite={()=>onInvite(f)} onJoin={()=>onJoinFriend(f)}/>)}
          {filter.length === 0 && (
            <div style={{
              padding: 40, textAlign:'center', color:'var(--ink-mute)',
              fontSize: 13, fontWeight: 600,
            }}>
              暂无好友在线～
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function FriendRow({ f, onInvite, onJoin }) {
  return (
    <div style={{
      display:'flex', alignItems:'center', gap: 12,
      background:'var(--surface)', padding: 12, borderRadius: 18,
      boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)',
    }}>
      <Avatar name={f.name} size={48} online={f.online} color={f.color}/>
      <div style={{flex: 1, minWidth: 0}}>
        <div style={{fontSize: 14, fontWeight: 800, color:'var(--ink)', display:'flex', alignItems:'center', gap: 4}}>
          {f.name}
          {f.status==='inRoom' && (
            <span style={{
              fontSize: 9, padding:'2px 6px', borderRadius: 6,
              background:'var(--accent-soft)', color:'var(--accent-deep)', fontWeight: 800,
            }}>房间中</span>
          )}
        </div>
        <div style={{fontSize: 11, color: f.online?'var(--ink-soft)':'var(--ink-mute)', fontWeight: 600, marginTop: 2}}>
          {f.statusText}
        </div>
      </div>
      {f.status === 'inRoom' ? (
        <button onClick={onJoin} style={{
          padding:'8px 14px', border:'none', cursor:'pointer',
          background:'var(--accent)', color:'white',
          borderRadius: 14, fontWeight: 800, fontSize: 12,
          display:'inline-flex', alignItems:'center', gap: 4,
          fontFamily:'var(--app-font)',
        }}>
          {Icons.enter(14,'white')} 加入
        </button>
      ) : f.online ? (
        <button onClick={onInvite} style={{
          padding:'8px 14px', border:'1.5px solid var(--accent)', cursor:'pointer',
          background:'transparent', color:'var(--accent-deep)',
          borderRadius: 14, fontWeight: 800, fontSize: 12,
          fontFamily:'var(--app-font)',
        }}>
          邀请
        </button>
      ) : (
        <div style={{fontSize: 11, color:'var(--ink-mute)', fontWeight: 700, padding:'0 8px'}}>
          离线
        </div>
      )}
    </div>
  );
}

Object.assign(window, { FriendsScreen });
