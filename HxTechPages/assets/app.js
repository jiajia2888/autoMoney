'use strict';
const $=s=>document.querySelector(s);
const state={platform:'wx',media:{},accounts:[],videos:[],runs:[],notifications:[],page:1,total:0,showAll:false,view:'videos',editing:null,deleting:null,busy:false,refreshing:false};
const esc=v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const date=v=>v?new Date(v).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false}):'尚未检查';
const num=v=>Number(v).toLocaleString('zh-CN');
const account=id=>state.accounts.find(a=>a.id===id);
const accountStatus=a=>!a.enabled?'已暂停':a.last_status==='error'?'最近检查失败':a.last_status==='running'?'检查中':!a.baseline_at?'待首次成功':'定时检查已启用';
const safeLink=v=>{try{const u=new URL(v);return u.protocol==='https:'&&(u.hostname==='www.douyin.com'||u.hostname==='weixin.qq.com'&&u.pathname.startsWith('/sph/')||u.hostname==='channels.weixin.qq.com'&&u.pathname==='/finder-preview/pages/sph')?esc(u.href):'#'}catch{return '#'}};
const safeCover=v=>{try{const u=new URL(v);return u.protocol==='https:'&&(/(^|\.)(douyinpic\.com|byteimg\.com|douyincdn\.com|qpic\.cn|qlogo\.cn)$/.test(u.hostname))?esc(u.href):''}catch{return ''}};
let toastTimer;
function toast(msg,error=false){$('#toast').textContent=msg;$('#toast').className='toast'+(error?' error':'');$('#toast').hidden=false;clearTimeout(toastTimer);toastTimer=setTimeout(()=>$('#toast').hidden=true,6500)}
async function api(path,method='GET',data){let res;try{res=await fetch('/api/v1'+path,{method,headers:data?{'Content-Type':'application/json'}:{},body:data?JSON.stringify(data):undefined,signal:AbortSignal.timeout(15000)})}catch{throw Error('本地服务未连接或响应超时。请检查服务状态；这不代表登录失效，无需重新配置 Cookie 或反复扫码。')}let body;try{body=await res.json()}catch{throw Error('服务返回了无效响应')}if(!res.ok||body.code!==0)throw Error(body.message||'请求失败');return body.data}
const empty=(title,desc,action='')=>`<div class="empty"><strong>${esc(title)}</strong>${esc(desc)}${action}</div>`;
function switchView(view){state.view=view;const titles={videos:['视频监控','筛选监控结果，获取文案和视频。','视频发现'],accounts:['关注你的下一个灵感来源','管理多个对标账号，让每一次上新和起量，都有迹可循。','对标账号'],notifications:['不错过内容起量的时刻','上新先知道，爆款早发现。每条提醒，都能回到原视频。','监控提醒'],runs:['看见监控背后的每一步','检查频率、覆盖范围与异常原因，清楚记录。','检查记录']};document.querySelectorAll('.view').forEach(el=>el.hidden=el.id!=='view-'+view);document.querySelectorAll('nav button').forEach(el=>el.classList.toggle('active',el.dataset.view===view));const t=titles[view];$('#pageTitle').textContent=t[0];$('#pageDesc').textContent=t[1];$('#breadcrumb').textContent=(state.platform==='wx'?'视频号 · ':'抖音 · ')+t[2];if(state.platform==='wx'){$('#pageDesc').textContent='按大拇指严格大于 10,000 筛选；历史建档与后续新视频分别查看。'}}
function renderAccounts(){
 $('#accountList').innerHTML=state.accounts.length?state.accounts.map(a=>`<article class="account-row"><div class="account-avatar">${esc(a.display_name.slice(0,1))}</div><div class="account-info"><div class="account-name">${a.platform==='wx'?`<span>${esc(a.display_name)}</span>`:`<a href="${safeLink(a.homepage_url)}" target="_blank" rel="noopener noreferrer">${esc(a.display_name)} ↗</a>`}<span class="status-badge ${esc(a.last_status)}">${accountStatus(a)}</span></div><p>${a.douyin_number?'抖音号 '+esc(a.douyin_number)+' · ':''}每 ${a.interval_minutes} 分钟检查 · ${a.platform==='wx'?'大拇指':'点赞'} &gt; ${num(a.threshold)} · 每轮至多 ${a.max_items} 条</p><p>上次：${date(a.last_checked_at)}　${a.enabled?'下次：'+date(a.next_check_at):'自动监控已暂停'}</p>${a.platform==='wx'?`<p>${a.history_complete?'历史作品扫描已到结尾；后续检查近期作品':'历史作品分批扫描中，每批最多 '+a.max_items+' 条；未完时自动续扫'}</p>`:''}${a.last_error?`<p class="account-error">${esc(a.last_error)}</p>`:''}${!a.baseline_at?'<p>首次实时检查会建立历史基线，不发送旧视频提醒。</p>':''}</div><div class="account-actions"><button class="btn small" data-action="check" data-id="${a.id}" ${state.busy?'disabled':''}>↻ 立即检查</button><button class="btn small" data-action="edit" data-id="${a.id}">监控规则</button><button class="btn small" data-action="toggle" data-id="${a.id}">${a.enabled?'暂停':'启用'}</button><button class="btn small" data-action="delete" data-id="${a.id}">删除</button></div></article>`).join(''):empty('从一个对标账号开始',state.platform==='wx'?'点击添加，搜索视频号昵称，选择作者后开始历史过万筛选。':'粘贴主页链接，设置规则，让新内容主动出现在你面前。','<br><button class="btn primary" id="emptyAdd">＋ 添加对标账号</button>');
 const selected=$('#accountFilter').value;$('#accountFilter').innerHTML='<option value="">全部对标账号</option>'+state.accounts.map(a=>`<option value="${a.id}">${esc(a.display_name)}</option>`).join('');$('#accountFilter').value=selected;
}
const dayKey=v=>v?new Intl.DateTimeFormat('sv-SE',{timeZone:'Asia/Shanghai',year:'numeric',month:'2-digit',day:'2-digit'}).format(new Date(v)):'';
function wechatMetric(value){return value===100002?'10万+':num(value)}
function renderVideos(){
 $('#sampleNote').hidden=!state.videos.some(v=>!v.likes_exact);
 const allLocal=state.showAll||state.downloadedOnly;const grouped=$('#sortFilter').value==='over_date';$('#dateNote').hidden=!grouped;
 $('#resultCount').textContent=num(state.total)+' 条内容';$('#pageNumber').textContent=allLocal?`已显示全部 ${num(state.total)} 条`:`第 ${state.page} / ${Math.max(1,Math.ceil(state.total/24))} 页`;
 $('#showAllVideos').hidden=!!state.downloadedOnly;$('#showAllVideos').textContent=state.showAll?'分页显示':'显示全部';$('#showAllVideos').setAttribute('aria-pressed',String(!!state.showAll));
 $('#prevPage').disabled=allLocal||state.page<=1;$('#nextPage').disabled=allLocal||state.page*24>=state.total;
 let previousGroup=null;
 $('#videoGrid').innerHTML=state.videos.length?state.videos.map(v=>{
  const wx=state.platform==='wx',media=state.media?.[v.id],downloaded=media?.status==='success';const a=account(v.account_id),img=downloaded?'/api/v1/videos/'+v.id+'/media/poster':safeCover(v.cover_url),day=dayKey(v.first_over_10k_at);
  let header='';if(grouped&&day!==previousGroup){header=`<div class="day-heading"><strong>${day?esc(day)+' · 发现过万':'发现日期未知'}</strong><span>${day?'以下为该日期在本页的记录':'历史样本或尚未建立精确发现记录'}</span></div>`;previousGroup=day}
  const discoveryNote=v.already_over_10k?'首次收录已过万，实际过万时间未知':'此前未过万，本次监测到过万';
  const metric=wx?wechatMetric(v.likes):(v.likes_exact?num(v.likes):'约 '+esc(v.likes_display));
  const metricNote=wx&&(v.likes===100002||v.hearts===100002)?'10万+为接口高位值，非精确数量':v.likes_exact?'接口数据':'历史样本，浏览器展示约数';
  const source=v.likes_exact?'接口数据':'历史样本';
  const sourceLink=`href="${safeLink(v.url)}" target="_blank" rel="noopener noreferrer" ${wx?`data-wx-share="${v.id}"`:''}`;
  return header+`<article class="video-card video-row"><a ${downloaded?`data-local-play="${v.id}"`:""} class="poster ${img?'':'placeholder'}" ${sourceLink} aria-label="${downloaded?'播放已下载视频':'打开原视频'}：${esc(v.title)}">${img?`<img src="${img}" loading="lazy" referrerpolicy="no-referrer" alt="">`:'<span aria-hidden="true">▶</span>'}</a><div class="row-content"><h3 class="card-title" title="${esc(v.title)}"><a ${sourceLink}>${esc(v.title)}</a></h3><div class="row-subline"><span class="row-author">${esc(a?.display_name||'未知账号')}</span><span class="row-source ${v.likes_exact?'':'sample'}" title="${esc(metricNote)}">${source}</span>${downloaded?'<span class="downloaded-badge">已下载</span>':media?.status==='running'?'<span class="downloaded-badge pending">下载中</span>':''}<span class="row-updated">更新 ${date(v.last_seen_at)}</span></div></div><div class="row-metrics" title="${esc(metricNote)}"><strong class="like-count">${wx?'👍':'♡'} ${metric}</strong><small>${wx?`爱心 ${v.hearts==null?'未知':wechatMetric(v.hearts)} · `:''}评 ${num(v.comments)} · 转 ${num(v.shares)}</small></div><div class="row-dates"><span>发布 ${v.published_at?date(v.published_at):'未知'}</span><small title="${esc(discoveryNote)}">${v.first_over_10k_at?'发现过万 '+date(v.first_over_10k_at):'发现过万日期未知'}${v.already_over_10k?' · 首录已过万':''}</small></div><div class="row-actions"><button class="btn small" data-frames="${v.id}">切片</button>${wx?`<button class="btn small" data-extract="${v.id}">提取文案</button><button class="btn small primary" data-video-download="${v.id}" data-media-platform="wx">${downloaded?'已下载 / 保存':'下载视频'}</button>${downloaded?`<button class="btn small" data-local-play="${v.id}">播放</button>`:''}<button class="btn small" data-wx-share="${v.id}">原视频 / 扫码</button>`:`<button class="btn small" data-extract="${v.id}">提取文案</button><button class="btn small primary" data-video-download="${v.id}">${downloaded?'已下载 / 保存':'下载视频'}</button>${downloaded?`<button class="btn small" data-local-play="${v.id}">播放</button>`:''}<a class="btn small" ${sourceLink}>原视频 ↗</a>`}</div></article>`
 }).join(''):empty('这里会收集值得研究的视频',state.accounts.length?'点击账号的立即检查；视频号默认只显示大拇指过万作品，也可调整日期和视频范围。':'先添加对标账号，开始你的第一次内容追踪。');
 document.querySelectorAll('.poster img').forEach(img=>img.addEventListener('error',()=>{const el=img.closest('.poster');el.classList.add('placeholder');img.remove();const text=document.createElement('span');text.className='poster-word';text.textContent='封面暂不可用';el.prepend(text)},{once:true}));
}
function renderNotifications(){
 $('#notificationList').innerHTML=state.notifications.length?state.notifications.map(n=>`<article class="notification ${n.read?'read':''}"><span class="event-icon">${n.kind==='new'?'＋':'↗'}</span><div><h3>${esc(n.title)}</h3><small>${n.kind==='new'?'新视频':'首次达标'} · ${date(n.created_at)}${n.read?' · 已读':' · 未读'}</small></div>${state.platform==='wx'?`<button class="btn" data-wx-share="${n.video_id}">查看原视频 ↗</button>`:`<a class="event-link" href="${safeLink(n.url)}" target="_blank" rel="noopener noreferrer">查看视频 ↗</a>`}</article>`).join(''):empty('暂时没有新提醒','首次检查会建立历史基线；后续发现上新和首次过门槛的视频时，在这里提示。');
}
function renderRuns(){const names={queued:'排队中',running:'检查中',success:'已完成',error:'检查失败',interrupted:'已中断'};$('#runList').innerHTML=state.runs.length?state.runs.map(r=>`<tr><td>${esc(r.kind==='login'?'连接抖音':account(r.account_id)?.display_name||'未知账号')}</td><td><span class="status-badge ${esc(r.status)}">${names[r.status]||esc(r.status)}</span></td><td>${r.scanned} / ${r.new_count} / ${r.alert_count}</td><td>${date(r.created_at)}</td><td>${esc(r.message)}${['running','queued'].includes(r.status)?`<br><button class="btn small cancel-run" data-id="${r.id}">停止本次任务</button>`:''}${r.status==='success'&&r.kind==='collect'?`<br>${r.complete?'已到作品列表结尾':'本轮为部分范围扫描'}`:''}</td></tr>`).join(''):'<tr><td colspan="5">暂无检查记录。添加账号后可发起第一次检查。</td></tr>'}
let videoRequest=0;
async function loadVideos(){const request=++videoRequest,platform=state.platform;const q=new URLSearchParams({platform,page:state.page,sort:$('#sortFilter').value});if(state.showAll||state.downloadedOnly)q.set('all','true');if(platform==='wx')q.set('min_likes','10000');if($('#cohortFilter').value)q.set('cohort',$('#cohortFilter').value);if($('#publishedFrom').value)q.set('published_from',$('#publishedFrom').value);if($('#publishedTo').value)q.set('published_to',$('#publishedTo').value);if(document.querySelector('[data-mode="hits"]').classList.contains('active'))q.set('over_10k','true');if($('#overDateFrom').value)q.set('over_date_from',$('#overDateFrom').value);if($('#overDateTo').value)q.set('over_date_to',$('#overDateTo').value);if($('#accountFilter').value)q.set('account_id',$('#accountFilter').value);if($('#likeFilter').value!=='')q.set('min_likes',$('#likeFilter').value);if($('#searchInput').value.trim())q.set('q',$('#searchInput').value.trim());if(document.querySelector('[data-mode="exact"]').classList.contains('active'))q.set('exact_only','true');const data=await api('/videos?'+q);if(state.platform!==platform||request!==videoRequest)return;state.videos=data.items||[];state.total=data.total;if(state.videos.length){try{const videos=state.videos;for(let i=0;i<videos.length;i+=500){const media=await api('/media?ids='+videos.slice(i,i+500).map(v=>v.id).join(','));if(state.platform!==platform||request!==videoRequest)return;state.media={...state.media,...media}}}catch(e){toast('本地下载状态读取失败：'+e.message,true)}}if(state.downloadedOnly){state.videos=state.videos.filter(v=>state.media?.[v.id]?.status==='success');state.total=state.videos.length}renderVideos()}
async function refresh(){
 if(state.refreshing)return;state.refreshing=true;const platform=state.platform,p='?platform='+platform;
 try{
 const [status,overview,accounts,notifications,runs,connection]=await Promise.all([api('/status'),api('/overview'+p),api('/accounts'+p),api('/notifications'+p),api('/runs'+p),api(platform==='wx'?'/wechat/connection':'/collector/connection')]);if(state.platform!==platform)return;
 if(platform==='wx')showWechatConnection(connection);else{state.dyConnection=connection;$('#connectBtn').classList.toggle('connected',!!connection.configured);$('#connectBtn').textContent=connection.configured?'抖音已连接':'连接抖音 / Cookie';$('#connectBtn').title='已保存本机 Cookie；有效性以最近检查结果为准'}
 state.busy=status.busy;state.accounts=accounts||[];state.notifications=notifications||[];state.runs=runs||[];
 $('#liveState').dataset.status=status.busy?'busy':'online';$('#liveState').textContent=status.busy?'● 任务执行中':'● 本地服务已连接';
 $('#statAccounts').textContent=overview.accounts;$('#statEnabled').textContent=overview.enabled+' 个已启用定时检查';$('#statVideos').textContent=overview.videos;$('#statHits').textContent=overview.hits;$('#statAlerts').textContent=overview.alerts;$('#videoCount').textContent=overview.videos;$('#alertDot').hidden=!overview.alerts;$('#busyNotice').hidden=!status.busy;$('#browserLoginBtn').disabled=status.busy;
 renderAccounts();renderNotifications();renderRuns();renderMonitorError();await loadVideos();if(state.copyVideo&&$('#copyDialog').open)try{await loadCopy()}catch(e){copyLoadError(e)}
 }catch(e){if(state.platform===platform){$('#liveState').dataset.status='error';$('#liveState').textContent='● 服务连接异常';toast(e.message,true)}}finally{state.loaded=true;state.refreshing=false;if(state.platform!==platform)refresh()}
}
function openAccount(a,author){if(state.platform==='wx'&&!a&&!author){openWxAuthor();return}state.wxSelected=author||null;state.editing=a?.id||null;const f=$('#accountForm');f.reset();$('#formError').textContent='';$('#dialogTitle').textContent=a?'调整账号监控规则':'添加一个值得关注的账号';f.elements.input.disabled=!!a;f.elements.input.value=state.platform==='wx'?(a?.platform_account_id||author?.username||''):(a?.homepage_url||'');f.elements.input.disabled=!!a||state.platform==='wx';$('#accountInputLabel').textContent=state.platform==='wx'?'已选择的视频号作者身份':'账号主页链接';$('#accountInputHelp').textContent=state.platform==='wx'?'已按唯一身份绑定；需要更换作者请重新搜索。':'请粘贴抖音完整主页链接。';$('#accountHelp').textContent=state.platform==='wx'?'首次筛出历史大拇指过万作品，后续持续记录新视频与首次过万日期。':'粘贴抖音主页链接，设定规则。';$('#thresholdLabel').textContent=state.platform==='wx'?'大拇指门槛（严格大于）':'点赞门槛（严格大于）';f.elements.threshold.disabled=state.platform==='wx';if(state.platform==='wx'){f.elements.threshold.value=10000;f.elements.max_items.value=500;}f.elements.display_name.value=a?.display_name||author?.nickname||'';for(const k of ['interval_minutes','threshold','max_items'])if(a)f.elements[k].value=a[k];for(const k of ['enabled','notify_new','notify_threshold'])f.elements[k].checked=a?a[k]:true;$('#accountDialog').showModal()}
function accountData(a){return {display_name:a.display_name,interval_minutes:a.interval_minutes,threshold:a.threshold,max_items:a.max_items,enabled:a.enabled,notify_new:a.notify_new,notify_threshold:a.notify_threshold}}
$('#accountForm').addEventListener('submit',async e=>{e.preventDefault();const f=e.currentTarget;const button=f.querySelector('[type=submit]');button.disabled=true;const data={input:f.elements.input.value.trim(),display_name:f.elements.display_name.value.trim()};for(const k of ['interval_minutes','threshold','max_items'])data[k]=Number(f.elements[k].value);for(const k of ['enabled','notify_new','notify_threshold'])data[k]=f.elements[k].checked;try{const saved=await api((state.editing?'/accounts/'+state.editing:'/accounts')+'?platform='+state.platform,state.editing?'PATCH':'POST',data);if(!state.editing&&state.platform==='wx'){try{await api('/accounts/'+saved.id+'/check','POST')}catch(e){toast('账号已保存；'+e.message,true)}};$('#accountDialog').close();toast(state.editing?'监控规则已更新':'账号已添加；首次检查将建立历史基线');await refresh()}catch(e){$('#formError').textContent=e.message}finally{button.disabled=false}});
$('#accountForm').elements.input.addEventListener('input',e=>{if(e.target.value.includes('80185543890')&&!$('#accountForm').elements.display_name.value)$('#accountForm').elements.display_name.value='徐晶说'});
$('#addBtn').onclick=()=>openAccount();document.querySelectorAll('.close-dialog').forEach(b=>b.onclick=()=>$('#accountDialog').close());
$('#accountList').addEventListener('click',async e=>{if(e.target.id==='emptyAdd'){openAccount();return}const b=e.target.closest('button[data-action]');if(!b)return;const a=account(Number(b.dataset.id));if(!a)return;try{switch(b.dataset.action){case'edit':openAccount(a);return;case'check':b.disabled=true;await api('/accounts/'+a.id+'/check','POST');toast('检查已开始，可在“检查记录”查看结果');break;case'toggle':await api('/accounts/'+a.id,'PATCH',{...accountData(a),enabled:!a.enabled});toast(a.enabled?'自动监控已暂停':'自动监控已启用');break;case'delete':state.deleting=a.id;$('#deleteDialog').showModal();return}await refresh()}catch(e){toast(e.message,true);b.disabled=false}});
$('#cancelDelete').onclick=()=>$('#deleteDialog').close();$('#confirmDelete').onclick=async()=>{try{await api('/accounts/'+state.deleting,'DELETE');$('#deleteDialog').close();state.page=1;toast('账号和本地记录已删除');await refresh()}catch(e){toast(e.message,true)}};
function setConnectionMode(mode){document.querySelectorAll('[data-connection-mode]').forEach(b=>b.classList.toggle('active',b.dataset.connectionMode===mode));$('#browserConnection').hidden=mode!=='browser';$('#connectionForm').hidden=mode!=='cookie'}
function showConnectionStatus(data){state.dyConnection=data;if(state.platform==='dy'){$('#connectBtn').classList.toggle('connected',!!data.configured);$('#connectBtn').textContent=data.configured?'抖音已连接':'连接抖音 / Cookie';}$('#connectionStatus').textContent=data.configured?'已保存手动 Cookie · 更新于 '+date(data.updated_at)+' · 有效性以检查结果为准':'当前使用专用窗口扫码登录；以登录任务和采集结果确认是否成功';setConnectionMode(data.mode)}
async function openConnection(mode){const button=$('#connectBtn');button.disabled=true;try{const data=await api('/collector/connection');$('#connectionForm').reset();$('#connectionError').textContent='';showConnectionStatus(data);if(mode)setConnectionMode(mode);$('#connectionDialog').showModal()}catch(e){toast(e.message,true)}finally{button.disabled=false}}
$('#connectBtn').onclick=()=>{
 if(state.platform!=='wx')return openConnection();
 if(!state.wxConnection?.configured)return openWechatLogin();
 $('#wxIdentity').scrollIntoView({behavior:'smooth',block:'center'});$('#wxVerifyBtn').focus({preventScroll:true});
};
$('#copyConnection').onclick=async()=>{const id=state.copyVideo,wx=state.copyPlatform==='wx';$('#copyDialog').close();if(wx){document.querySelector('[data-video-download="'+id+'"]')?.click()}else await openConnection()};
document.querySelectorAll('[data-connection-mode]').forEach(b=>b.onclick=()=>setConnectionMode(b.dataset.connectionMode));
$('#closeConnection').onclick=$('#cancelConnection').onclick=()=>$('#connectionDialog').close();
$('#connectionDialog').addEventListener('close',()=>{$('#connectionForm').reset();$('#connectionError').textContent=''});
$('#connectionForm').addEventListener('submit',async e=>{e.preventDefault();const f=e.currentTarget,button=f.querySelector('[type=submit]');button.disabled=true;$('#connectionError').textContent='';try{await api('/collector/connection','PUT',{mode:'cookie',cookie:f.elements.cookie.value.trim(),user_agent:f.elements.user_agent.value.trim()});f.reset();$('#connectionDialog').close();toast('Cookie 已保存在本机；后续检查自动复用，请点击“立即检查”验证');await refresh()}catch(e){$('#connectionError').textContent=e.message}finally{button.disabled=false}});
$('#browserLoginBtn').onclick=async()=>{const b=$('#browserLoginBtn');b.disabled=true;try{await api('/collector/connection','PUT',{mode:'browser'});await api('/collector/login','POST');$('#connectionDialog').close();toast('已打开专用 Chrome 窗口，请在该窗口登录抖音');await refresh();switchView('runs')}catch(e){toast(e.message,true)}finally{b.disabled=state.busy}};
$('#markRead').onclick=async()=>{try{await api('/notifications/read?platform='+state.platform,'POST');await refresh()}catch(e){toast(e.message,true)}};
document.querySelectorAll('nav button').forEach(b=>b.onclick=()=>switchView(b.dataset.view));
document.querySelectorAll('[data-mode]').forEach(b=>b.onclick=()=>{document.querySelectorAll('[data-mode]').forEach(x=>x.classList.toggle('active',x===b));state.downloadedOnly=b.dataset.mode==='downloaded';$('#likeFilter').value='';if(b.dataset.mode==='hits')$('#sortFilter').value='over_date';else if($('#sortFilter').value==='over_date')$('#sortFilter').value='likes';applyFilters()});
async function applyFilters(){state.page=1;try{await loadVideos()}catch(e){toast(e.message,true)}}
$('#filterBtn').onclick=applyFilters;$('#searchInput').onkeydown=e=>{if(e.key==='Enter')applyFilters()};$('#accountFilter').onchange=applyFilters;$('#sortFilter').onchange=applyFilters;$('#cohortFilter').onchange=applyFilters;
$('#showAllVideos').onclick=async()=>{const previous=state.showAll;state.showAll=!previous;state.page=1;try{await loadVideos()}catch(e){state.showAll=previous;toast(e.message,true)}};
$('#prevPage').onclick=async()=>{state.page--;await loadVideos();window.scrollTo({top:220,behavior:'smooth'})};$('#nextPage').onclick=async()=>{state.page++;await loadVideos();window.scrollTo({top:220,behavior:'smooth'})};
$('#dateLabel').textContent=new Date().toLocaleDateString('zh-CN',{timeZone:'Asia/Shanghai',year:'numeric',month:'long',day:'numeric',weekday:'long'});
setPlatform('wx');setInterval(refresh,5000);

$('#runList').addEventListener('click',async e=>{const b=e.target.closest('.cancel-run');if(!b)return;try{await api('/runs/'+b.dataset.id+'/cancel','POST');toast('已请求停止任务');await refresh()}catch(e){toast(e.message,true)}});

function copyLoadError(e){$('#copyState').textContent='暂时无法读取提取状态';$('#copyError').textContent=e.message;$('#copyStart').disabled=false;$('#copyStart').textContent='重新连接并读取';$('#copyCancel').hidden=true;$('#copyStart').hidden=false}
async function loadCopy(){
 const id=state.copyVideo;if(!id)return;
 const data=await api('/videos/'+id+'/transcript');if(state.copyVideo!==id||!$('#copyDialog').open)return;
 state.copyData=data;const job=data.job,active=['queued','running'].includes(job.status),done=job.status==='success';
 $('#copyConnection').textContent=state.copyPlatform==='wx'?'视频号下载 / 登录':'抖音连接设置';
 $('#copyState').textContent=job.status==='not_started'?'尚未提取视频口播':job.stage||job.status;
 $('#copyError').textContent=job.error||data.tools_error||'';if(job.error)showActionError(job.error);
 $('#copyStart').disabled=active||!data.tools_ready;$('#copyStart').textContent=done?'已提取，结果已保存在本机':job.status==='error'||job.status==='interrupted'?'重新提取口播':'提取视频口播';if(done)$('#copyStart').disabled=true;
 $('#copyCancel').hidden=!active;$('#copyStart').hidden=done||active;$('#copyConnection').hidden=!(job.error||data.tools_error);
 if($('#transcriptText').value!==job.text)$('#transcriptText').value=job.text||'';
 $('#copyTranscript').disabled=!done;
 for(const [selector,format]of [['#downloadTranscript','txt'],['#downloadSubtitle','srt']]){const link=$(selector);link.hidden=!done||(format==='srt'&&!job.srt);link.href='/api/v1/videos/'+id+'/transcript/download?format='+format;link.setAttribute('download','transcript-'+id+'.'+format)}
 return data;
}
async function startCopy(){if(!state.copyVideo)return;$('#copyStart').disabled=true;$('#copyError').textContent='';try{await api('/videos/'+state.copyVideo+'/transcript','POST');await loadCopy()}catch(e){$('#copyError').textContent=e.message;showActionError(e.message);$('#copyStart').disabled=false}}
$('#videoGrid').addEventListener('click',async e=>{const b=e.target.closest('[data-extract]');if(!b)return;state.copyVideo=Number(b.dataset.extract);state.copyPlatform=state.platform;state.copyData=null;$('#transcriptText').value='';$('#copyState').textContent='正在读取…';$('#copyError').textContent='';$('#copyStart').disabled=true;$('#copyStart').hidden=false;$('#copyConnection').hidden=true;$('#copyCancel').hidden=true;$('#downloadTranscript').hidden=true;$('#downloadSubtitle').hidden=true;$('#copyDialog').showModal();try{const data=await loadCopy();if(data?.job.status==='not_started'&&data.tools_ready)await startCopy()}catch(e){copyLoadError(e)}});
$('#closeCopy').onclick=()=>$('#copyDialog').close();$('#copyDialog').addEventListener('close',()=>{state.copyVideo=null;state.copyData=null});
$('#copyStart').onclick=async()=>{if($('#copyStart').textContent==='重新连接并读取'){try{await loadCopy()}catch(e){copyLoadError(e)}}else await startCopy()};
$('#copyCancel').onclick=async()=>{try{await api('/videos/'+state.copyVideo+'/transcript/cancel','POST');toast('已请求停止文案提取');await loadCopy()}catch(e){$('#copyError').textContent=e.message}};
async function copyText(selector){const field=$(selector);if(!field.value)return;try{await navigator.clipboard.writeText(field.value);toast('已复制')}catch{field.select();toast('请按 ⌘C 或 Ctrl+C 复制选中的文案')}}
$('#copyTranscript').onclick=()=>copyText('#transcriptText');

function setPlatform(platform){
 state.platform=platform;state.page=1;state.showAll=false;state.accounts=[];state.videos=[];state.notifications=[];state.runs=[];$('#accountFilter').innerHTML='<option value="">全部对标账号</option>';$('#cohortFilter').value='';$('#publishedFrom').value=$('#publishedTo').value=$('#overDateFrom').value=$('#overDateTo').value=$('#likeFilter').value='';$('#videoGrid').innerHTML='正在读取当前平台…';$('#accountList').innerHTML='正在读取当前平台…';$('#notificationList').innerHTML='';$('#runList').innerHTML='';
 document.querySelectorAll('[data-platform]').forEach(b=>{const selected=b.dataset.platform===platform;b.classList.toggle('active',selected);b.setAttribute('aria-selected',String(selected))});
 const wx=platform==='wx';$('#metricFilterLabel').textContent=wx?'大拇指 >':'点赞 >';$('#metricSortOption').textContent=wx?'大拇指最多':'点赞最多';$('#connectBtn').classList.toggle('connected',!!(wx?state.wxConnection:state.dyConnection)?.configured);$('#wechatWorkspace').hidden=!wx;$('#douyinWorkspace').hidden=false;$('#addBtn').hidden=false;$('#videoCount').hidden=false;$('#alertDot').hidden=true;
 $('#connectBtn').textContent=wx?(state.wxConnection?.configured?'视频号已连接 · '+(state.wxConnection.identity?.nickname||'投手账号'):'连接视频号'):(state.dyConnection?.configured?'抖音已连接':'连接抖音 / Cookie');
 $('#pageTitle').textContent=wx?'从视频号，发现下一个好内容':'发现值得研究的好内容';$('#pageDesc').textContent=wx?'先连接微信投手账号，为视频号对标监控做好准备。':'把关注变成洞察，让每一条有价值的内容，都被看见。';
 switchView(state.view);refresh();
}
document.querySelectorAll('[data-platform]').forEach(b=>b.onclick=()=>setPlatform(b.dataset.platform));
function showWechatConnection(data){
 state.wxConnection=data;
 if(state.platform==='wx'){
  $('#connectBtn').classList.toggle('connected',!!data.configured);
  $('#connectBtn').textContent=data.configured?'视频号已连接 · '+(data.identity?.nickname||'投手账号'):'连接视频号';
  $('#connectBtn').title=data.configured?'已保存本机会话，点击查看连接信息；可用“核验会话”确认当前有效性':'扫码连接微信投手账号';
 }
 $('#wxIdentity').textContent=data.configured?(data.identity.nickname||'视频号投手账号'):'尚未连接视频号';
 $('#wxConnectionState').textContent=data.configured?'本机已保存会话 · 可点击“核验会话”确认当前有效性':'请先扫码连接微信投手账号。';
 $('#wxVerifiedAt').textContent=data.configured?'上次身份核验：'+date(data.verified_at)+' · '+data.identity.username:'';
 $('#wxLoginBtn').textContent=data.configured?'重新扫码':'微信扫码登录 →';$('#wxVerifyBtn').hidden=$('#wxDisconnectBtn').hidden=!data.configured;
}
async function refreshWechat(){
 if(state.wxRefreshing)return;state.wxRefreshing=true;
 try{const data=await api('/wechat/connection');if(state.platform!=='wx')return;showWechatConnection(data);$('#liveState').dataset.status='online';$('#liveState').textContent='● 本地服务已连接';$('#wxPageError').textContent=''}catch(e){if(state.platform==='wx'){$('#wxPageError').textContent=e.message;$('#liveState').dataset.status='error';$('#liveState').textContent='● 服务连接异常'}}finally{state.wxRefreshing=false}
}
let wxLoginEpoch=0,wxLoginID=null,wxPollTimer;
const wxLoginMessages={waiting:'等待微信扫码',scanned:'已扫码，请在手机上确认登录',success:'连接成功，投手身份已核验并保存在本机',expired:'二维码已过期，请刷新二维码',unknown:'微信返回了未确认的状态，请稍候或刷新二维码'};
async function generateWechatQR(){
 const epoch=++wxLoginEpoch;clearTimeout(wxPollTimer);const previous=wxLoginID;wxLoginID=null;
 $('#wxQRImage').hidden=true;$('#wxQRImage').removeAttribute('src');$('#wxLoginError').textContent='';$('#wxQRExpiry').textContent='';$('#wxRetryPoll').hidden=true;$('#wxLoginState').textContent='正在获取二维码…';$('#wxRefreshQR').disabled=true;
 try{if(previous)await api('/wechat/login/cancel','POST',{id:previous});const data=await api('/wechat/login','POST');if(epoch!==wxLoginEpoch||!$('#wxLoginDialog').open){await api('/wechat/login/cancel','POST',{id:data.id});return}
 wxLoginID=data.id;$('#wxQRImage').src=data.qr_image;$('#wxQRImage').hidden=false;$('#wxLoginState').textContent=wxLoginMessages.waiting;$('#wxQRExpiry').textContent='本次二维码有效至 '+date(data.expires_at)+'，以微信返回状态为准。';wxPollTimer=setTimeout(pollWechatLogin,2000);
 }catch(e){if(epoch===wxLoginEpoch){$('#wxLoginState').textContent='二维码获取失败，请重试';$('#wxLoginError').textContent=e.message}}finally{if(epoch===wxLoginEpoch)$('#wxRefreshQR').disabled=false}
}
function openWechatLogin(){if($('#wxLoginDialog').open)return;$('#wxLoginDialog').showModal();generateWechatQR()}
async function pollWechatLogin(){
 clearTimeout(wxPollTimer);if(!wxLoginID||!$('#wxLoginDialog').open)return;const epoch=wxLoginEpoch,id=wxLoginID;$('#wxRetryPoll').hidden=true;
 try{const data=await api('/wechat/login/poll','POST',{id});if(epoch!==wxLoginEpoch||!$('#wxLoginDialog').open)return;
 $('#wxLoginError').textContent='';$('#wxLoginState').textContent=wxLoginMessages[data.state]||wxLoginMessages.unknown;
 if(data.state==='success'||data.state==='expired'){$('#wxQRImage').hidden=true;$('#wxQRImage').removeAttribute('src');$('#wxQRExpiry').textContent='';if(data.state==='success'){wxLoginID=null;if(state.actionError?.platform==='wx')state.actionError=null;$('#wxLoginDialog').close();toast('视频号连接成功');await refreshWechat();renderMonitorError()}return}
 wxPollTimer=setTimeout(pollWechatLogin,2000);
 }catch(e){if(epoch===wxLoginEpoch&&$('#wxLoginDialog').open){$('#wxLoginError').textContent=e.message;$('#wxRetryPoll').hidden=false}}
}
$('#wxLoginBtn').onclick=openWechatLogin;$('#wxRefreshQR').onclick=generateWechatQR;$('#wxRetryPoll').onclick=pollWechatLogin;$('#wxCloseLogin').onclick=()=>$('#wxLoginDialog').close();
$('#wxLoginDialog').addEventListener('close',()=>{++wxLoginEpoch;clearTimeout(wxPollTimer);const id=wxLoginID;wxLoginID=null;$('#wxQRImage').removeAttribute('src');$('#wxQRImage').hidden=true;$('#wxRefreshQR').disabled=false;if(id)api('/wechat/login/cancel','POST',{id}).catch(()=>{})});
$('#wxVerifyBtn').onclick=async()=>{const b=$('#wxVerifyBtn');b.disabled=true;$('#wxPageError').textContent='';try{const data=await api('/wechat/connection/verify','POST');showWechatConnection(data);$('#wxConnectionState').textContent='会话有效，微信投手身份核验通过'}catch(e){await refreshWechat();$('#wxPageError').textContent=e.message}finally{b.disabled=false}};
$('#wxDisconnectBtn').onclick=async()=>{const b=$('#wxDisconnectBtn');b.disabled=true;try{await api('/wechat/connection','DELETE');showWechatConnection({configured:false});$('#wxPageError').textContent='';toast('视频号会话已从本机移除')}catch(e){$('#wxPageError').textContent=e.message}finally{b.disabled=false}};

let wxAuthors=[],wxAuthorGroups=[],wxAuthorSelected=new Map(),wxAuthorEpoch=0,wxAuthorBusy=false;
function parseWxAuthorQueries(value){
 const queries=[...new Set(value.split(/[\n\r,，;；]+/).map(s=>s.trim()).filter(Boolean))];
 if(!queries.length)throw Error('请输入作者昵称');
 if(queries.length>20)throw Error('每批最多20个昵称');
 if(queries.some(s=>[...s].length>80))throw Error('每个昵称最多80个字');
 return queries;
}
function wxAuthorExists(id){return state.accounts.some(a=>a.platform==='wx'&&a.platform_account_id===id)}
function updateWxAuthorSelection(){
 const choices=wxAuthors.filter(a=>!wxAuthorExists(a.username));
 const selected=choices.filter(a=>wxAuthorSelected.has(a.username)).length;
 $('#wxAuthorAdd').textContent='添加选中作者（'+wxAuthorSelected.size+'）';
 $('#wxAuthorAdd').disabled=wxAuthorBusy||!wxAuthorSelected.size;
 $('#wxAuthorSelectAll').checked=!!choices.length&&selected===choices.length;
 $('#wxAuthorSelectAll').indeterminate=selected>0&&selected<choices.length;
 $('#wxAuthorSelectAll').disabled=wxAuthorBusy||!choices.length;
}
function renderWxAuthors(){
 wxAuthors=[...new Map(wxAuthorGroups.flatMap(g=>g.items).map(a=>[a.username,a])).values()];
 $('#wxAuthorResults').innerHTML=wxAuthorGroups.map((g,i)=>`<section class="wx-author-group"><h3>${esc(g.query)}</h3>${g.error?`<p class="form-error">${esc(g.error)}</p>`:''}${g.items.map(a=>{const exists=wxAuthorExists(a.username);return `<label class="account-row wx-author-choice"><input type="checkbox" data-wx-author-id="${esc(a.username)}" ${wxAuthorSelected.has(a.username)?'checked':''} ${exists||wxAuthorBusy?'disabled':''}>${safeCover(a.avatar)?`<img src="${safeCover(a.avatar)}" width="40" height="40" referrerpolicy="no-referrer" alt="作者头像">`:''}<span class="account-info"><strong>${esc(a.nickname)}${exists?' · 已添加':''}</strong><span title="${esc(a.signature||'暂无简介')}">${esc(a.signature||'暂无简介')}</span><small class="finder-id" title="${esc(a.username)}">${esc(a.username)}</small></span></label>`}).join('')}${!g.items.length&&!g.error?(g.page?'<p>未找到作者</p>':'<p>等待搜索结果…</p>'):''}${g.error||(g.page<50&&g.page*5<g.total)?`<button class="btn small" data-wx-author-more="${i}" ${wxAuthorBusy?'disabled':''}>${g.error?'重试搜索':'更多结果'}</button>`:''}</section>`).join('');
 $('#wxAuthorBatch').hidden=!wxAuthors.length;updateWxAuthorSelection();
}
function setWxAuthorBusy(value){wxAuthorBusy=value;$('#wxAuthorSearch').disabled=value;$('#wxAuthorInterval').disabled=value;renderWxAuthors()}
function openWxAuthor(){
 ++wxAuthorEpoch;wxAuthorGroups=[];wxAuthors=[];wxAuthorSelected.clear();wxAuthorBusy=false;
 $('#wxAuthorError').textContent='';$('#wxAuthorProgress').textContent='';$('#wxAuthorInterval').value=60;
 setWxAuthorBusy(false);$('#wxAuthorDialog').showModal();$('#wxAuthorQuery').focus();
}
async function fetchWxAuthorGroup(group,page){
 try{const data=await api('/wechat/authors/search','POST',{query:group.query,page});group.items=[...new Map([...group.items,...(data.items||[])].map(a=>[a.username,a])).values()];group.page=page;group.total=data.total;group.error=''}
 catch(e){group.error=e.message}
}
async function searchWxAuthors(){
 if(wxAuthorBusy)return;let queries;try{queries=parseWxAuthorQueries($('#wxAuthorQuery').value)}catch(e){$('#wxAuthorError').textContent=e.message;return}
 const epoch=++wxAuthorEpoch;wxAuthorGroups=queries.map(query=>({query,items:[],page:0,total:0,error:''}));wxAuthorSelected.clear();$('#wxAuthorError').textContent='';setWxAuthorBusy(true);
 try{for(let i=0;i<wxAuthorGroups.length;i++){const group=wxAuthorGroups[i];$('#wxAuthorProgress').textContent='正在搜索 '+(i+1)+' / '+queries.length+'：'+group.query;await fetchWxAuthorGroup(group,1);if(epoch!==wxAuthorEpoch)return;renderWxAuthors()}
 $('#wxAuthorProgress').textContent='搜索完成：'+wxAuthors.length+' 位作者，请勾选要监控的账号。'}
 finally{if(epoch===wxAuthorEpoch)setWxAuthorBusy(false)}
}
async function addWxAuthors(){
 if(wxAuthorBusy)return;const interval=Number($('#wxAuthorInterval').value);
 if(!Number.isInteger(interval)||interval<5||interval>10080){$('#wxAuthorError').textContent='检查间隔为5至10080分钟';return}
 const authors=[...wxAuthorSelected.values()];if(!authors.length)return;
 const epoch=wxAuthorEpoch;let count=0;const errors=[];$('#wxAuthorError').textContent='';setWxAuthorBusy(true);
 try{for(const a of authors){if(epoch!==wxAuthorEpoch)break;
 $('#wxAuthorProgress').textContent='正在添加：'+a.nickname;
 try{const saved=await api('/accounts?platform=wx','POST',{input:a.username,display_name:a.nickname,interval_minutes:interval,threshold:10000,max_items:500,enabled:true,notify_new:true,notify_threshold:true});if(state.platform==='wx'&&!state.accounts.some(x=>x.id===saved.id))state.accounts.push(saved);wxAuthorSelected.delete(a.username);count++}
 catch(e){errors.push(a.nickname+'：'+e.message)}
 if(epoch===wxAuthorEpoch)renderWxAuthors();}
 if(epoch===wxAuthorEpoch){$('#wxAuthorError').textContent=errors.join('\n');$('#wxAuthorProgress').textContent='已添加 '+count+' 位作者'+(errors.length?'，'+errors.length+' 位失败，可重试。':'，已开启自动监控。');if(!errors.length){$('#wxAuthorDialog').close();toast('已添加 '+count+' 位作者，已开启自动监控')}}
 }finally{if(epoch===wxAuthorEpoch)setWxAuthorBusy(false);await refresh()}
}
$('#wxAuthorForm').onsubmit=e=>{e.preventDefault();searchWxAuthors()};
$('#closeWxAuthor').onclick=()=>$('#wxAuthorDialog').close();
$('#wxAuthorDialog').addEventListener('close',()=>{++wxAuthorEpoch;wxAuthorBusy=false});
$('#wxAuthorResults').addEventListener('change',e=>{
 const id=e.target.dataset.wxAuthorId;if(!id||wxAuthorBusy)return;
 const author=wxAuthors.find(a=>a.username===id);if(!author||wxAuthorExists(id))return;
 if(e.target.checked)wxAuthorSelected.set(id,author);else wxAuthorSelected.delete(id);renderWxAuthors();
});
$('#wxAuthorResults').addEventListener('click',async e=>{
 const b=e.target.closest('[data-wx-author-more]');if(!b||wxAuthorBusy)return;
 const group=wxAuthorGroups[Number(b.dataset.wxAuthorMore)],epoch=wxAuthorEpoch;setWxAuthorBusy(true);
 try{await fetchWxAuthorGroup(group,group.page+1)}finally{if(epoch===wxAuthorEpoch)setWxAuthorBusy(false)}
});
$('#wxAuthorSelectAll').onchange=e=>{if(wxAuthorBusy)return;for(const a of wxAuthors){if(wxAuthorExists(a.username))continue;if(e.target.checked)wxAuthorSelected.set(a.username,a);else wxAuthorSelected.delete(a.username)}renderWxAuthors()};
$('#wxAuthorAdd').onclick=addWxAuthors;
let wxShareEpoch=0;
async function resolveWechatShare(e){
 const b=e.target.closest('[data-wx-share]');if(!b)return;e.preventDefault();const epoch=++wxShareEpoch;
 $('#wxShareError').textContent='';$('#wxShareStatus').textContent='正在取得微信官方分享链接…';$('#wxShareQR').hidden=true;$('#wxShareQR').removeAttribute('src');$('#wxShareURL').value='';$('#wxShareOpen').hidden=true;$('#wxShareCopy').disabled=true;
 if(!$('#wxShareDialog').open)$('#wxShareDialog').showModal();
 try{const data=await api('/wechat/videos/'+b.dataset.wxShare+'/share','POST');if(epoch!==wxShareEpoch||!$('#wxShareDialog').open)return;const link=safeLink(data.url);if(link==='#')throw Error('分享链接格式无效');$('#wxShareURL').value=data.url;$('#wxShareOpen').href=data.direct_url&&safeLink(data.direct_url)!=='#'?data.direct_url:data.url;$('#wxShareOpen').hidden=false;$('#wxShareCopy').disabled=false;$('#wxShareQR').src=data.qr_image;$('#wxShareQR').hidden=!data.qr_image;$('#wxShareStatus').textContent='分享链接已取得，可在电脑打开，或用手机微信扫一扫。'}catch(e){if(epoch===wxShareEpoch){$('#wxShareStatus').textContent='分享链接获取失败';$('#wxShareError').textContent=e.message}}
}
$('#wxShareClose').onclick=()=>$('#wxShareDialog').close();$('#wxShareDialog').addEventListener('close',()=>{++wxShareEpoch;$('#wxShareQR').removeAttribute('src');$('#wxShareURL').value=''});$('#wxShareCopy').onclick=()=>copyText('#wxShareURL');
$('#videoGrid').addEventListener('click',resolveWechatShare);$('#notificationList').addEventListener('click',resolveWechatShare);

let mediaVideo=null,mediaTimer,mediaEpoch=0,mediaPlatform="dy",yuanbaoTimer,yuanbaoQRShown=false;
function renderMedia(job){
 if(mediaVideo){state.media[mediaVideo]=job;renderVideos()}
 const active=job.status==='running',done=job.status==='success';
 if(mediaPlatform==='wx'&&job.error&&/元宝.*(登录|会话|凭据)|登录元宝/.test(job.error)){$('#yuanbaoLogin').hidden=false;$('#yuanbaoState').hidden=false}
 $('#mediaStatus').textContent=done?'视频已保存到本机，可下载MP4文件'+(job.bytes?'（'+(job.bytes/1024/1024).toFixed(1)+' MB）':''):active?'正在下载视频，关闭弹窗后仍会继续…':'尚未下载';
 $('#mediaError').textContent=job.error||'';if(job.error)showActionError(job.error);$('#mediaRetry').hidden=active||done;$('#mediaRetry').textContent=job.status==='error'?'重试下载':'开始下载';$('#mediaSave').hidden=!done;
 if(done){$('#yuanbaoLogin').hidden=true;$('#yuanbaoState').hidden=true;$('#yuanbaoCancel').hidden=true;$('#yuanbaoQRPanel').hidden=true;clearTimeout(yuanbaoTimer);$('#mediaSave').href='/api/v1/videos/'+mediaVideo+'/media/file';$('#mediaSave').setAttribute('download',job.filename)}
}
async function loadMedia(start=false){
 const id=mediaVideo,epoch=mediaEpoch;if(!id||!$('#mediaDialog').open)return;
 try{let job=await api('/videos/'+id+'/media',start?'POST':'GET');if(mediaVideo!==id||epoch!==mediaEpoch||!$('#mediaDialog').open)return;renderMedia(job);if(job.status==='running'){clearTimeout(mediaTimer);mediaTimer=setTimeout(()=>loadMedia(),1500)}}catch(e){if(mediaVideo===id&&epoch===mediaEpoch&&$('#mediaDialog').open){$('#mediaStatus').textContent='下载未完成';$('#mediaError').textContent=e.message;showActionError(e.message);$('#mediaRetry').hidden=false;$('#mediaRetry').textContent='重试下载'}}
}
document.addEventListener('click',async e=>{
 const b=e.target.closest('[data-video-download]');if(!b)return;
 ++mediaEpoch;mediaPlatform=b.dataset.mediaPlatform||'dy';
 $('#mediaTitle').textContent=mediaPlatform==='wx'?'下载视频号视频':'下载抖音视频';
 $('#mediaHint').textContent=mediaPlatform==='wx'?'通过元宝网页版解析分享链接，无需安装元宝电脑 App；已保存会话会自动复用。完成后点“保存MP4”，支持150MB、30分钟以内普通视频。':'使用已保存的抖音登录状态。下载完成后点“保存MP4”，支持150MB、30分钟以内视频。';
 $('#yuanbaoLogin').hidden=true;$('#yuanbaoState').hidden=true;$('#yuanbaoState').textContent='';$('#yuanbaoCancel').hidden=true;$('#yuanbaoQRPanel').hidden=true;$('#yuanbaoQR').hidden=true;$('#yuanbaoQR').removeAttribute('src');yuanbaoQRShown=false;
 mediaVideo=b.dataset.videoDownload;const openEpoch=mediaEpoch;clearTimeout(mediaTimer);clearTimeout(yuanbaoTimer);$('#yuanbaoLogin').disabled=false;$('#mediaStatus').textContent='正在检查本机视频…';$('#mediaError').textContent='';$('#mediaSave').hidden=true;$('#mediaRetry').hidden=true;$('#mediaDialog').showModal();if(state.media?.[mediaVideo]?.status==='success'){await loadMedia();return}if(mediaPlatform==='wx'){
  try{const c=await api('/yuanbao/connection');if(openEpoch!==mediaEpoch||!$('#mediaDialog').open)return;await showYuanbao(c);if(!c.configured&&state.media?.[mediaVideo]?.status!=='success'){$('#mediaStatus').textContent='需要先登录元宝下载会话';await showYuanbao(c);return}}catch(e){$('#mediaError').textContent=e.message;return}
 }
 await loadMedia(true);
});
$('#mediaClose').onclick=()=>$('#mediaDialog').close();$('#mediaDialog').addEventListener('close',()=>{++mediaEpoch;clearTimeout(mediaTimer);clearTimeout(yuanbaoTimer);mediaVideo=null;$('#yuanbaoQR').removeAttribute('src');yuanbaoQRShown=false});$('#mediaRetry').onclick=()=>loadMedia(true);

async function showYuanbao(c){
 if(mediaPlatform!=='wx'||!$('#mediaDialog').open)return;
 const active=c.state==='starting'||c.state==='waiting';
 $('#yuanbaoLogin').hidden=!!c.configured&&!active;$('#yuanbaoState').hidden=false;
 $('#yuanbaoState').textContent=c.error||(c.state==='starting'?'正在从元宝官方获取登录二维码，请稍候…':c.state==='waiting'?'请直接用微信扫描下面的元宝官方二维码，并在手机确认。保存会话后自动尝试下载。':c.configured?'本机已保存元宝会话，有效性以实际下载结果为准。':'点击“获取元宝登录二维码”，直接在这里扫码，无需找其他窗口。');
 $('#yuanbaoLogin').disabled=active;$('#yuanbaoLogin').textContent=c.configured?'重新获取元宝二维码':'获取元宝登录二维码';$('#yuanbaoCancel').hidden=!active;
 $('#yuanbaoQRPanel').hidden=!c.qr_ready;$('#yuanbaoQR').hidden=!c.qr_ready;
 if(c.qr_ready&&!yuanbaoQRShown){$('#yuanbaoQR').src='/api/v1/yuanbao/login/qr?t='+Date.now();yuanbaoQRShown=true}
 if(!c.qr_ready){$('#yuanbaoQR').removeAttribute('src');yuanbaoQRShown=false}
 if(active){clearTimeout(yuanbaoTimer);yuanbaoTimer=setTimeout(pollYuanbao,1500)}
}
async function pollYuanbao(){
 const epoch=mediaEpoch;if(!$('#mediaDialog').open||mediaPlatform!=='wx')return;
 try{const c=await api('/yuanbao/connection');if(epoch!==mediaEpoch)return;await showYuanbao(c);if(c.state==='saved'&&c.configured)await loadMedia(true)}catch(e){$('#mediaError').textContent=e.message;$('#yuanbaoLogin').disabled=false}
}
$('#yuanbaoLogin').onclick=async()=>{const epoch=mediaEpoch;$('#yuanbaoLogin').disabled=true;try{yuanbaoQRShown=false;$('#yuanbaoQR').removeAttribute('src');const c=await api('/yuanbao/login','POST');if(epoch===mediaEpoch)await showYuanbao(c)}catch(e){$('#mediaError').textContent=e.message;$('#yuanbaoLogin').disabled=false}};
$('#yuanbaoCancel').onclick=async()=>{clearTimeout(yuanbaoTimer);try{await api('/yuanbao/login/cancel','POST');await pollYuanbao()}catch(e){$('#mediaError').textContent=e.message}};

// Published descriptions are available locally; they are separate from speech transcription.
$('#videoGrid').addEventListener('click',e=>{
 const button=e.target.closest('[data-caption]');if(!button)return;
 const video=state.videos.find(v=>v.id===Number(button.dataset.caption));if(!video)return;
 $('#publishedCaptionTitle').textContent='发布文案 · '+(account(video.account_id)?.display_name||'未知账号');
 $('#publishedCaptionText').value=video.title||'';
 $('#publishedCaptionDialog').showModal();
});
$('#closePublishedCaption').onclick=()=>$('#publishedCaptionDialog').close();
$('#copyPublishedCaption').onclick=()=>copyText('#publishedCaptionText');

function showActionError(message){
 state.actionError={platform:state.platform,message};renderMonitorError();
}
function renderMonitorError(){
 const isAuthError=message=>/(会话.*失效|登录.*失效|重新扫码|尚未.*登录|尚未连接|Cookie.*无效|登录凭据|验证提示)/.test(message);
 const verified=state.platform==='wx'&&state.wxConnection?.configured?Date.parse(state.wxConnection.verified_at):NaN;
 const failed=state.accounts.find(a=>a.last_status==='error'&&a.last_error&&!(isAuthError(a.last_error)&&Date.parse(a.last_checked_at)<verified));
 const action=state.actionError?.platform===state.platform?state.actionError.message:'';
 const message=action||(failed?failed.display_name+'：'+failed.last_error:'');
 const panel=$('#monitorError');panel.hidden=!message;
 $('#monitorErrorText').textContent=message?((state.platform==='wx'?'视频号':'抖音')+'异常：'+message):'';
 const authError=/(会话.*失效|登录.*失效|重新扫码|尚未.*登录|尚未连接|Cookie.*无效|登录凭据|验证提示)/.test(message);
 $('#monitorErrorConnection').textContent=authError?'重新登录':'连接信息';
 if(message&&authError){
  $('#connectBtn').classList.remove('connected');$('#connectBtn').textContent=(state.platform==='wx'?'视频号':'抖音')+'登录需检查';
 }
}
$('#monitorErrorConnection').onclick=()=>{if(state.platform==='wx'&&state.actionError?.message.includes('元宝'))document.querySelector('[data-video-download]')?.click();else if(state.platform==='wx')openWechatLogin();else openConnection()};
$('#monitorErrorRuns').onclick=()=>switchView('runs');
$('#monitorErrorDismiss').onclick=()=>{state.actionError=null;$('#monitorError').hidden=true};
document.addEventListener('click',async e=>{
 const button=e.target.closest('[data-local-play]');if(!button)return;e.preventDefault();e.stopPropagation();
 const id=Number(button.dataset.localPlay);const video=state.videos.find(v=>v.id===id);if(!video)return;
 $('#localPlayerTitle').textContent=video.title;$('#localPlayerError').textContent='';
 const player=$('#localPlayer');player.src='/api/v1/videos/'+id+'/media/play';player.poster='/api/v1/videos/'+id+'/media/poster';
 $('#localPlayerSave').href='/api/v1/videos/'+id+'/media/file';$('#localPlayerDialog').showModal();
 try{await player.play()}catch{$('#localPlayerError').textContent='点击播放按钮开始播放；如加载失败，请重新检查本机文件。'}
},true);
$('#closeLocalPlayer').onclick=()=>$('#localPlayerDialog').close();
$('#localPlayerDialog').addEventListener('close',()=>{const player=$('#localPlayer');player.pause();player.removeAttribute('src');player.load()});
$('#localPlayer').addEventListener('error',()=>{$('#localPlayerError').textContent='本机视频加载失败，请检查视频文件或重新下载。'});

let framesID=0,framesPage=1,framesJob=null,framesTimer=null,framesEpoch=0;
function framesQuery(){return '?'+new URLSearchParams({mode:$('#framesMode').value,count:$('#framesCount').value,interval:$('#framesInterval').value})}
function frameSettingsChanged(){framesEpoch++;clearTimeout(framesTimer);framesJob=null;framesPage=1;renderFrames();loadFrames()}
function renderFrames(){
 const j=framesJob||{status:'not_started',count:0};const running=j.status==='running',success=j.status==='success';
 $('#framesStatus').textContent=running?`正在下载、切片或打包 · 已提取 ${num(j.count)} 张`:success?`已完成 ${num(j.count)} 张 · ${(j.bytes/1048576).toFixed(1)} MB 图片`:'设置数量或时间间隔后，点击开始提取。';
 $('#framesError').textContent=j.error||'';$('#startFrames').hidden=running||success;$('#startFrames').disabled=false;
 $('#startFrames').textContent=j.status==='error'?'重新切片':'开始提取';$('#cancelFrames').hidden=!running;$('#saveFrames').hidden=!success;
 $('#saveFrames').href='/api/v1/videos/'+framesID+'/frames/archive'+framesQuery();$('#framesPager').hidden=!success;
 $('#framesMode').disabled=running;$('#framesCount').disabled=running;$('#framesInterval').disabled=running;$('#framesCountLabel').hidden=$('#framesMode').value!=='count';$('#framesIntervalLabel').hidden=$('#framesMode').value!=='interval';
 const pages=Math.max(1,Math.ceil(j.count/48));framesPage=Math.min(framesPage,pages);
 $('#framesPage').textContent=`${framesPage} / ${pages} 页`;$('#framesPrev').disabled=framesPage<=1;$('#framesNext').disabled=framesPage>=pages;
 $('#framesGrid').innerHTML=success?Array.from({length:Math.min(48,j.count-(framesPage-1)*48)},(_,i)=>{
  const n=(framesPage-1)*48+i+1,url='/api/v1/videos/'+framesID+'/frames/images/'+n+framesQuery();
  return `<a href="${url}" target="_blank" rel="noopener" title="打开原尺寸图片"><img loading="lazy" src="${url}" alt="第 ${n} 张"><span>第 ${num(n)} 张</span></a>`;
 }).join(''):'';
}
async function loadFrames(method='GET',suffix=''){
 clearTimeout(framesTimer);const epoch=framesEpoch,id=framesID;
 try{
  const j=await api('/videos/'+id+'/frames'+suffix+framesQuery(),method);if(epoch!==framesEpoch)return;
  framesJob=j;renderFrames();if(j.status==='running'&&$('#framesDialog').open)framesTimer=setTimeout(()=>loadFrames(),1500);
 }catch(e){if(epoch!==framesEpoch)return;$('#framesError').textContent=e.message;$('#startFrames').disabled=false;$('#startFrames').hidden=false;$('#startFrames').textContent='重新读取 / 开始';}
}
document.addEventListener('click',e=>{
 const b=e.target.closest('[data-frames]');if(!b)return;
 $('#framesMode').value='count';$('#framesCount').value='30';$('#framesInterval').value='5';framesEpoch++;framesID=Number(b.dataset.frames);framesPage=1;framesJob=null;clearTimeout(framesTimer);
 $('#framesTitle').textContent='视频切片 · '+(state.videos.find(v=>v.id===framesID)?.title||'视频');
 renderFrames();$('#framesDialog').showModal();loadFrames();
});
$('#startFrames').onclick=()=>{$('#startFrames').disabled=true;loadFrames('POST')};
$('#cancelFrames').onclick=()=>loadFrames('POST','/cancel');
$('#closeFrames').onclick=()=>$('#framesDialog').close();
$('#framesDialog').addEventListener('close',()=>{framesEpoch++;clearTimeout(framesTimer)});
$('#framesPrev').onclick=()=>{framesPage--;renderFrames()};$('#framesNext').onclick=()=>{framesPage++;renderFrames()};

for(const selector of ["#framesMode","#framesCount","#framesInterval"])$(selector).onchange=frameSettingsChanged;
