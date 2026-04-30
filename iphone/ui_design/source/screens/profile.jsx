// profile.jsx — 我的界面

function ProfileScreen({ user, wechatBound, onBindWechat }) {
  const [showBindModal, setShowBindModal] = React.useState(false);

  // 进入页面后 1.2s 自动弹出绑定提醒（仅当未绑定）
  React.useEffect(() => {
    if (!wechatBound) {
      const t = setTimeout(() => setShowBindModal(true), 1200);
      return () => clearTimeout(t);
    }
  }, [wechatBound]);

  const handleBind = () => {
    setShowBindModal(false);
    onBindWechat?.();
  };

  return (
    <div style={{height:'100%', overflow:'auto', background:'var(--page-bg)', paddingBottom: 100, position:'relative'}}>
      {/* 顶部渐变头图 */}
      <div style={{
        position:'relative', padding:'68px 20px 50px',
        background:'linear-gradient(180deg, var(--accent-soft) 0%, var(--accent) 100%)',
      }}>
        <div style={{display:'flex', justifyContent:'space-between', alignItems:'center'}}>
          <div style={{fontSize: 22, fontWeight: 800, color:'white'}}>我的</div>
          <div style={{display:'flex', gap: 8}}>
            <button style={{
              width: 36, height: 36, borderRadius: 18, border:'none', cursor:'pointer',
              background:'rgba(255,255,255,0.3)', display:'flex', alignItems:'center', justifyContent:'center',
            }}>{Icons.bell(18, 'white')}</button>
            <button style={{
              width: 36, height: 36, borderRadius: 18, border:'none', cursor:'pointer',
              background:'rgba(255,255,255,0.3)', display:'flex', alignItems:'center', justifyContent:'center',
            }}>{Icons.settings(18, 'white')}</button>
          </div>
        </div>

        <div style={{display:'flex', alignItems:'center', gap: 14, marginTop: 14}}>
          <Avatar name={user.name} size={72} color="#fff1e8" ring/>
          <div style={{flex:1}}>
            <div style={{fontSize: 22, fontWeight: 800, color:'white'}}>{user.name}</div>
            <div style={{fontSize: 12, color:'rgba(255,255,255,0.85)', fontWeight:700, marginTop: 2}}>
              ID: {user.id} · {user.title}
            </div>
            <div style={{
              display:'inline-flex', alignItems:'center', gap: 4, marginTop: 6,
              background:'rgba(255,255,255,0.25)', padding:'3px 10px', borderRadius: 10,
              fontSize: 11, fontWeight: 700, color:'white',
            }}>
              {Icons.sparkle(12,'white')} 加入于 {user.joinedAt}
            </div>
          </div>
        </div>
      </div>

      {/* 统计卡片（覆盖在渐变上） */}
      <div style={{padding: '0 20px', marginTop: -34}}>
        <div style={{
          background:'var(--surface)', borderRadius: 22, padding: 16,
          boxShadow:'var(--shadow-md)', border:'1px solid var(--border)',
          display:'flex', justifyContent:'space-around',
        }}>
          <Stat label="收藏品" value="36" icon={Icons.diamond(18,'var(--accent)')}/>
          <Divider/>
          <Stat label="好友" value="12" icon={Icons.friends(18,'var(--accent)')}/>
          <Divider/>
          <Stat label="小猫等级" value="Lv.8" icon={Icons.paw(18,'var(--accent)')}/>
          <Divider/>
          <Stat label="成就" value="15" icon={Icons.trophy(18,'var(--accent)')}/>
        </div>
      </div>

      {/* 绑定微信卡片 */}
      <div style={{padding: '14px 20px 0'}}>
        {wechatBound ? (
          <div style={{
            background:'var(--surface)', borderRadius: 18, padding:'12px 14px',
            boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)',
            display:'flex', alignItems:'center', gap: 12,
          }}>
            <div style={{
              width: 40, height: 40, borderRadius: 12,
              background:'#e8f7e0', display:'flex', alignItems:'center', justifyContent:'center',
            }}>{Icons.wechat(22, '#1aad19')}</div>
            <div style={{flex:1}}>
              <div style={{fontSize: 14, fontWeight: 800, color:'var(--ink)', display:'flex', alignItems:'center', gap: 6}}>
                微信已绑定
                <span style={{
                  fontSize: 9, padding:'2px 6px', borderRadius: 6,
                  background:'#1aad19', color:'white', fontWeight:800,
                }}>已保护</span>
              </div>
              <div style={{fontSize: 11, color:'var(--ink-soft)', fontWeight: 700, marginTop: 2}}>
                数据已同步至云端，卸载重装不会丢失
              </div>
            </div>
            {Icons.shield(20, '#1aad19')}
          </div>
        ) : (
          <button onClick={()=>setShowBindModal(true)} style={{
            width:'100%', border:'none', cursor:'pointer', textAlign:'left',
            background:'linear-gradient(135deg, #fff8e1 0%, #ffe8b5 100%)',
            borderRadius: 18, padding:'12px 14px',
            boxShadow:'var(--shadow-sm)',
            border:'1.5px solid #ffc94c',
            display:'flex', alignItems:'center', gap: 12,
            fontFamily:'var(--app-font)',
          }}>
            <div style={{
              width: 40, height: 40, borderRadius: 12, flexShrink: 0,
              background:'white', display:'flex', alignItems:'center', justifyContent:'center',
              boxShadow:'0 2px 6px rgba(255,180,0,0.3)',
            }}>{Icons.warn(22, '#e89400')}</div>
            <div style={{flex:1, minWidth: 0}}>
              <div style={{fontSize: 14, fontWeight: 800, color:'#7a4f00', display:'flex', alignItems:'center', gap: 6}}>
                绑定微信，保护小猫数据
              </div>
              <div style={{fontSize: 11, color:'#a06b00', fontWeight: 700, marginTop: 2, lineHeight: 1.4}}>
                未绑定时卸载 App 将丢失全部数据
              </div>
            </div>
            <div style={{
              padding:'7px 12px', borderRadius: 14,
              background:'#1aad19', color:'white',
              fontSize: 12, fontWeight: 800,
              display:'inline-flex', alignItems:'center', gap: 4, flexShrink: 0,
            }}>
              {Icons.wechat(14, 'white')} 立即绑定
            </div>
          </button>
        )}
      </div>

      {/* 收藏预览 */}
      <div style={{padding: '18px 20px 0'}}>
        <SectionHeader title="最近收藏" more="查看全部"/>
        <div style={{display:'flex', gap: 10, overflowX:'auto', paddingBottom: 4, marginTop: 10}}>
          {[
            {n:'樱花发饰',r:'SR',i:'🎀'},
            {n:'贝雷帽',r:'R',i:'🎩'},
            {n:'骑士披风',r:'SR',i:'🧣'},
            {n:'水手服',r:'R',i:'👘'},
            {n:'樱花树下',r:'SR',i:'🏞️'},
          ].map((c,i) => (
            <div key={i} style={{
              flexShrink: 0, width: 88,
              background:'var(--surface)', borderRadius: 16, padding: 10,
              boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)',
              display:'flex', flexDirection:'column', alignItems:'center', gap: 4,
            }}>
              <div style={{
                width: 60, height: 60, borderRadius: 12, fontSize: 32,
                background:'var(--surface-2)',
                display:'flex', alignItems:'center', justifyContent:'center',
              }}>{c.i}</div>
              <div style={{fontSize: 11, fontWeight: 800, color:'var(--ink)', textAlign:'center'}}>{c.n}</div>
            </div>
          ))}
        </div>
      </div>

      {/* 菜单 */}
      <div style={{padding:'18px 20px 0'}}>
        <SectionHeader title="更多"/>
        <div style={{
          marginTop: 10, background:'var(--surface)', borderRadius: 20,
          boxShadow:'var(--shadow-sm)', border:'1px solid var(--border)', overflow:'hidden',
        }}>
          {[
            {icon: Icons.trophy(20,'var(--accent-deep)'), label:'成就徽章', extra:'15/40'},
            {icon: Icons.bell(20,'var(--accent-deep)'),   label:'消息通知', extra:'3 条未读'},
            {icon: Icons.heart(20,'var(--accent-deep)',true), label:'喜欢的道具', extra:''},
            {icon: Icons.settings(20,'var(--accent-deep)'), label:'设置', extra:''},
          ].map((r,i,arr) => (
            <div key={i} style={{
              padding:'14px 16px', display:'flex', alignItems:'center', gap: 12,
              borderBottom: i < arr.length-1 ? '1px solid var(--border)' : 'none',
              cursor:'pointer',
            }}>
              <div style={{
                width: 36, height: 36, borderRadius: 12, background:'var(--accent-soft)',
                display:'flex', alignItems:'center', justifyContent:'center',
              }}>{r.icon}</div>
              <div style={{flex: 1, fontSize: 14, fontWeight: 700, color:'var(--ink)'}}>{r.label}</div>
              {r.extra && <div style={{fontSize: 11, color:'var(--ink-soft)', fontWeight:700}}>{r.extra}</div>}
              {Icons.chevronRight(18,'var(--ink-mute)')}
            </div>
          ))}
        </div>
      </div>

      {/* 绑定微信浮窗 */}
      {showBindModal && (
        <BindWechatModal
          onClose={()=>setShowBindModal(false)}
          onBind={handleBind}
        />
      )}
    </div>
  );
}

function BindWechatModal({ onClose, onBind }) {
  return (
    <>
      <div onClick={onClose} style={{
        position:'absolute', inset: 0, background:'rgba(0,0,0,0.5)',
        zIndex: 30, animation:'overlayIn 0.25s ease',
      }}/>
      <div style={{
        position:'absolute', left: 24, right: 24, top:'50%', transform:'translateY(-50%)',
        background:'var(--surface)', borderRadius: 28, padding: 24,
        zIndex: 31, boxShadow:'var(--shadow-lg)', animation:'modalIn 0.32s ease',
      }}>
        {/* 警告插画区 */}
        <div style={{
          margin:'0 auto 14px', width: 88, height: 88, borderRadius: 44,
          background:'linear-gradient(135deg, #fff3d6 0%, #ffd97a 100%)',
          display:'flex', alignItems:'center', justifyContent:'center',
          position:'relative',
        }}>
          <div style={{position:'absolute', inset:-6, borderRadius:50, border:'2px dashed #ffc94c', opacity:0.6, animation:'spin 18s linear infinite'}}/>
          {Icons.warn(46, '#e89400')}
        </div>

        <div style={{
          fontSize: 19, fontWeight: 800, color:'var(--ink)',
          textAlign:'center', marginBottom: 8,
        }}>
          数据可能丢失！
        </div>

        <div style={{
          fontSize: 13, color:'var(--ink-soft)', textAlign:'center',
          marginBottom: 16, lineHeight: 1.6, fontWeight: 600,
        }}>
          您还未绑定微信账号，
          <span style={{color:'#e15f7c', fontWeight: 800}}>
            一旦卸载本 App，您的小猫、收藏品、好友关系等所有数据都将被永久删除，无法恢复。
          </span>
        </div>

        {/* 数据风险列表 */}
        <div style={{
          background:'#fff5f5', borderRadius: 16, padding:'12px 14px',
          marginBottom: 18, border:'1px solid #ffe0e0',
        }}>
          <DataLossRow icon="🐱" text="小猫 Lv.8 · 奶团"/>
          <DataLossRow icon="💎" text="36 件收藏品 · 价值 248 钻石"/>
          <DataLossRow icon="🏆" text="15 个成就徽章"/>
          <DataLossRow icon="👥" text="12 位好友关系" last/>
        </div>

        <div style={{display:'flex', flexDirection:'column', gap: 10}}>
          <button onClick={onBind} style={{
            height: 52, border:'none', cursor:'pointer',
            background:'#1aad19', color:'white',
            borderRadius: 26, fontWeight: 800, fontSize: 15,
            boxShadow: '0 4px 0 #138a12',
            display:'inline-flex', alignItems:'center', justifyContent:'center', gap: 8,
            fontFamily:'var(--app-font)',
          }}>
            {Icons.wechat(20, 'white')} 绑定微信，保护数据
          </button>
          <button onClick={onClose} style={{
            height: 40, border:'none', cursor:'pointer',
            background:'transparent', color:'var(--ink-mute)',
            borderRadius: 20, fontWeight: 700, fontSize: 12,
            fontFamily:'var(--app-font)',
          }}>
            稍后再说（数据将不受保护）
          </button>
        </div>

        <style>{`
          @keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
        `}</style>
      </div>
    </>
  );
}

function DataLossRow({ icon, text, last }) {
  return (
    <div style={{
      display:'flex', alignItems:'center', gap: 8,
      padding:'5px 0', borderBottom: last ? 'none' : '1px dashed #ffd0d0',
    }}>
      <span style={{fontSize: 16}}>{icon}</span>
      <span style={{fontSize: 12, color:'#7a3a3a', fontWeight: 700}}>{text}</span>
      <span style={{marginLeft:'auto', fontSize: 10, color:'#e15f7c', fontWeight: 800}}>将丢失</span>
    </div>
  );
}

function Stat({ label, value, icon }) {
  return (
    <div style={{textAlign:'center', flex: 1}}>
      <div style={{display:'flex', justifyContent:'center', marginBottom: 4}}>{icon}</div>
      <div style={{fontSize: 17, fontWeight: 800, color:'var(--ink)'}}>{value}</div>
      <div style={{fontSize: 10, color:'var(--ink-soft)', fontWeight: 700, marginTop: 2}}>{label}</div>
    </div>
  );
}
function Divider() {
  return <div style={{width: 1, background:'var(--border)', margin:'6px 0'}}/>;
}
function SectionHeader({ title, more }) {
  return (
    <div style={{display:'flex', justifyContent:'space-between', alignItems:'center'}}>
      <div style={{fontSize: 15, fontWeight: 800, color:'var(--ink)'}}>{title}</div>
      {more && <button style={{
        background:'none', border:'none', cursor:'pointer',
        color:'var(--accent-deep)', fontSize: 12, fontWeight: 700,
        display:'inline-flex', alignItems:'center',
      }}>{more} {Icons.chevronRight(14,'var(--accent-deep)')}</button>}
    </div>
  );
}

Object.assign(window, { ProfileScreen });
