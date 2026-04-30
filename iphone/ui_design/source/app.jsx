// app.jsx — 根应用

const { useState, useEffect } = React;

const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "theme": "candy",
  "darkMode": false,
  "font": "rounded",
  "catName": "奶团",
  "hasTeam": false,
  "friendsOnline": 4,
  "wechatBound": false
}/*EDITMODE-END*/;

const ALL_FRIENDS = [
  {id:'f1', name:'小桃', color:'#ffb3c1', online:true,  status:'inRoom', statusText:'在房间 7K3-P2 中玩耍'},
  {id:'f2', name:'阿糖', color:'#ffd6a5', online:true,  status:'online', statusText:'在线 · 正在装扮小猫'},
  {id:'f3', name:'喵喵', color:'#caffbf', online:true,  status:'inRoom', statusText:'在房间 9X2-L8 中'},
  {id:'f4', name:'奶盖', color:'#bdb2ff', online:true,  status:'online', statusText:'在线 · 刚开始游戏'},
  {id:'f5', name:'小鱼干', color:'#a0c4ff', online:false, status:'offline', statusText:'上次在线 2 小时前'},
  {id:'f6', name:'布丁', color:'#ffc8dd', online:false, status:'offline', statusText:'上次在线 昨天'},
  {id:'f7', name:'芝士',  color:'#b8e0d2', online:false, status:'offline', statusText:'上次在线 3 天前'},
];

function App() {
  const [tweaks, setTweak] = useTweaks(TWEAK_DEFAULTS);

  const [tab, setTab] = useState('home');
  const [inTeam, setInTeam] = useState(tweaks.hasTeam);
  const [roomCode, setRoomCode] = useState('7K3-P2');
  const [members, setMembers] = useState([
    {id:'me', name: tweaks.catName, level: 8, status:'等待好友加入', isHost: true},
  ]);
  const [modal, setModal] = useState(null); // 'join' | 'toast' | null
  const [joinInput, setJoinInput] = useState('');
  const [toast, setToast] = useState(null);

  // 同步 tweaks 的 hasTeam
  useEffect(() => {
    setInTeam(tweaks.hasTeam);
  }, [tweaks.hasTeam]);

  // 应用主题
  useEffect(() => {
    document.body.className =
      `theme-${tweaks.theme} font-${tweaks.font}` + (tweaks.darkMode ? ' dark-mode' : '');
  }, [tweaks.theme, tweaks.font, tweaks.darkMode]);

  const friendsVisible = ALL_FRIENDS.slice(0, Math.max(1, tweaks.friendsOnline + 3));

  const flashToast = (text) => {
    setToast(text);
    setTimeout(() => setToast(null), 1800);
  };

  const createTeam = () => {
    const code = Array.from({length: 3}, () => String.fromCharCode(65 + Math.floor(Math.random()*26))).join('') + '-' + Math.floor(Math.random()*90+10);
    setRoomCode(code);
    setMembers([{id:'me', name: tweaks.catName, level: 8, status:'等待好友加入', isHost: true}]);
    setInTeam(true);
    setTweak('hasTeam', true);
  };

  const joinTeam = (code) => {
    setRoomCode(code || '9X2-L8');
    setMembers([
      {id:'host', name:'小桃', level: 12, status:'队长', isHost: true},
      {id:'me',   name: tweaks.catName, level: 8, status:'刚刚加入', isHost: false},
      {id:'b',    name:'奶盖', level: 5, status:'准备就绪', isHost: false},
    ]);
    setInTeam(true);
    setTweak('hasTeam', true);
    setModal(null);
    setJoinInput('');
  };

  const leaveRoom = () => {
    setInTeam(false);
    setTweak('hasTeam', false);
  };

  const inviteFriend = (f) => {
    if (!inTeam) {
      createTeam();
      flashToast(`已创建房间并邀请 ${f.name}`);
    } else {
      flashToast(`已邀请 ${f.name} 加入房间`);
    }
  };

  const joinFriendRoom = (f) => {
    const code = f.statusText.match(/[A-Z0-9]+-[A-Z0-9]+/)?.[0] || '9X2-L8';
    joinTeam(code);
    setTab('home');
    flashToast(`已加入 ${f.name} 的房间`);
  };

  let mainContent;
  if (tab === 'home') {
    mainContent = inTeam ? (
      <RoomScreen roomCode={roomCode} members={members} onLeave={leaveRoom} catName={tweaks.catName}/>
    ) : (
      <HomeScreen
        state="idle"
        catName={tweaks.catName}
        steps={8426}
        onCreateTeam={createTeam}
        onJoinTeamClick={()=>setModal('join')}
      />
    );
  } else if (tab === 'wardrobe') {
    mainContent = <WardrobeScreen catName={tweaks.catName}/>;
  } else if (tab === 'friends') {
    mainContent = <FriendsScreen
      friends={friendsVisible}
      onInvite={inviteFriend}
      onJoinFriend={joinFriendRoom}
      myRoomCode={inTeam ? roomCode : null}
    />;
  } else if (tab === 'profile') {
    mainContent = <ProfileScreen
      user={{
        name:'主人', id:'82947103', title:'见习铲屎官',
        joinedAt:'2024年3月15日',
      }}
      wechatBound={tweaks.wechatBound}
      onBindWechat={()=>{ setTweak('wechatBound', true); flashToast('微信绑定成功、数据已受保护'); }}
    />;
  }

  return (
    <div style={{
      width:'100%', height:'100%',
      display:'flex', alignItems:'center', justifyContent:'center', gap: 40,
      padding: 20, overflow:'hidden',
    }}>
      <IOSDevice width={402} height={874} dark={tweaks.darkMode}>
        <div style={{position:'relative', height:'100%', overflow:'hidden', background:'var(--page-bg)'}}>
          <FadeIn keyProp={tab + (inTeam ? '-room' : '-home')}>
            {mainContent}
          </FadeIn>

          <TabBar active={tab} onChange={setTab}/>

          {/* 加入队伍弹窗 */}
          {modal === 'join' && (
            <JoinRoomModal
              value={joinInput}
              onChange={setJoinInput}
              onClose={()=>{setModal(null); setJoinInput('');}}
              onConfirm={()=>joinTeam(joinInput || '9X2-L8')}
            />
          )}

          {/* Toast */}
          {toast && (
            <div style={{
              position:'absolute', bottom: 100, left:'50%', transform:'translateX(-50%)',
              background:'var(--ink)', color:'var(--surface)',
              padding:'10px 18px', borderRadius: 20,
              fontSize: 13, fontWeight: 700, zIndex: 20,
              boxShadow: 'var(--shadow-lg)',
              animation: 'toastIn 0.3s ease',
              whiteSpace:'nowrap',
            }}>{toast}</div>
          )}
        </div>
      </IOSDevice>

      {/* 旁边的说明卡片（仅桌面） */}
      <SideInfo inTeam={inTeam} tab={tab}/>

      {/* Tweaks */}
      <TweaksPanel title="Tweaks">
        <TweakSection label="主题"/>
        <TweakRadio label="主题色" value={tweaks.theme}
          options={['candy','matcha','sky']}
          onChange={(v)=>setTweak('theme', v)}/>
        <TweakToggle label="深色模式" value={tweaks.darkMode}
          onChange={(v)=>setTweak('darkMode', v)}/>
        <TweakRadio label="字体" value={tweaks.font}
          options={['rounded','baloo','kai']}
          onChange={(v)=>setTweak('font', v)}/>

        <TweakSection label="状态"/>
        <TweakText label="小猫名字" value={tweaks.catName}
          onChange={(v)=>setTweak('catName', v)}/>
        <TweakToggle label="已加入队伍" value={tweaks.hasTeam}
          onChange={(v)=>setTweak('hasTeam', v)}/>
        <TweakToggle label="微信已绑定" value={tweaks.wechatBound}
          onChange={(v)=>setTweak('wechatBound', v)}/>
        <TweakSlider label="在线好友" value={tweaks.friendsOnline}
          min={0} max={4} step={1}
          onChange={(v)=>setTweak('friendsOnline', v)}/>
      </TweaksPanel>

      <style>{`
        @keyframes toastIn {
          from { opacity: 0; transform: translate(-50%, 10px); }
          to   { opacity: 1; transform: translate(-50%, 0); }
        }
        @keyframes modalIn {
          from { opacity: 0; transform: translateY(20px); }
          to   { opacity: 1; transform: translateY(0); }
        }
        @keyframes overlayIn {
          from { opacity: 0; }
          to   { opacity: 1; }
        }
      `}</style>
    </div>
  );
}

function JoinRoomModal({ value, onChange, onClose, onConfirm }) {
  return (
    <>
      <div onClick={onClose} style={{
        position:'absolute', inset: 0, background:'rgba(0,0,0,0.45)',
        zIndex: 30, animation:'overlayIn 0.2s ease',
      }}/>
      <div style={{
        position:'absolute', left: 20, right: 20, top:'50%', transform:'translateY(-50%)',
        background:'var(--surface)', borderRadius: 28, padding: 22,
        zIndex: 31, boxShadow:'var(--shadow-lg)', animation:'modalIn 0.3s ease',
      }}>
        <div style={{display:'flex', justifyContent:'space-between', alignItems:'center', marginBottom: 14}}>
          <div style={{fontSize: 18, fontWeight: 800, color:'var(--ink)'}}>加入队伍</div>
          <button onClick={onClose} style={{
            width: 32, height: 32, borderRadius: 16, border:'none',
            background:'var(--surface-2)', cursor:'pointer',
            display:'flex', alignItems:'center', justifyContent:'center',
          }}>{Icons.close(18,'var(--ink-soft)')}</button>
        </div>

        <div style={{fontSize: 13, color:'var(--ink-soft)', marginBottom: 14, lineHeight: 1.5}}>
          输入好友分享给你的房间代码，加入他们的小屋一起玩耍～
        </div>

        <div style={{
          background:'var(--surface-2)', borderRadius: 18, padding: '14px 16px',
          display:'flex', alignItems:'center', gap: 10, marginBottom: 4,
          border:'2px solid var(--accent-soft)',
        }}>
          {Icons.paw(20, 'var(--accent)')}
          <input
            value={value}
            onChange={(e)=>onChange(e.target.value.toUpperCase())}
            placeholder="例如 9X2-L8"
            maxLength={8}
            style={{
              flex: 1, border:'none', outline:'none', background:'transparent',
              fontSize: 20, fontWeight: 800, color:'var(--ink)',
              fontFamily:'SF Mono, Menlo, monospace', letterSpacing:'3px',
            }}
          />
        </div>
        <div style={{fontSize: 11, color:'var(--ink-mute)', marginBottom: 18, padding:'0 4px'}}>
          房间代码格式：3 个字母 - 2 位数字
        </div>

        <div style={{display:'flex', gap: 10}}>
          <PrimaryButton variant="secondary" onClick={onClose} fullWidth>取消</PrimaryButton>
          <PrimaryButton onClick={onConfirm} fullWidth disabled={value.length < 3}>
            确定加入
          </PrimaryButton>
        </div>
      </div>
    </>
  );
}

function SideInfo({ inTeam, tab }) {
  return (
    <div style={{
      maxWidth: 300, color:'var(--ink-soft)', fontSize: 13, lineHeight: 1.6,
      display:'flex', flexDirection:'column', gap: 14, opacity: 0.85,
    }}>
      <div>
        <div style={{fontSize: 11, letterSpacing:'2px', color:'var(--ink-mute)', fontWeight: 800, marginBottom: 4}}>
          小猫 APP 原型
        </div>
        <div style={{fontSize: 22, fontWeight: 800, color:'var(--ink)', lineHeight: 1.3}}>
          糖果风的养猫 · 组队 · 装扮 App
        </div>
      </div>
      <div style={{padding: 14, background:'var(--surface)', borderRadius: 16, border:'1px solid var(--border)'}}>
        <div style={{fontWeight:800, color:'var(--ink)', marginBottom: 6}}>当前状态</div>
        <div>Tab: <b>{ {home:'家',wardrobe:'仓库',friends:'好友',profile:'我的'}[tab] }</b></div>
        <div>首页: <b>{inTeam ? '已在队伍房间' : '未加入队伍（显示创建/加入按钮）'}</b></div>
      </div>
      <div style={{fontSize: 12, color:'var(--ink-mute)'}}>
        提示：打开右上角 Tweaks 切换主题色、深色模式、字体，或快速切换队伍状态测试互斥界面。
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App/>);
