const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const source = fs.readFileSync('HxTechPages/assets/app.js', 'utf8');
test('backend disconnect is not reported as expired Douyin credentials', async () => {
 const ctx = vm.createContext({fetch: async () => {throw new TypeError('Failed to fetch')}, AbortSignal});
 vm.runInContext(source.slice(source.indexOf('async function api('), source.indexOf('const empty=')), ctx);
 await assert.rejects(ctx.api('/videos/1/transcript'), /本地服务未连接.*无需重新配置 Cookie/);
});
test('failed modal load leaves reading state and provides reconnection', () => {
 const fields = {};
 const ctx = vm.createContext({$: s => fields[s] ||= {}});
 vm.runInContext(source.slice(source.indexOf('function copyLoadError('), source.indexOf('async function loadCopy(')), ctx);
 ctx.copyLoadError(new Error('service disconnected'));
 assert.equal(fields['#copyState'].textContent, '暂时无法读取提取状态');
 assert.equal(fields['#copyStart'].disabled, false);
 assert.equal(fields['#copyStart'].textContent, '重新连接并读取');
});
test('platform switch clears old platform data and exposes shared monitor views', () => {
 const elements = {};
 const wx = {dataset:{platform:'wx'},classList:{toggle(){}},setAttribute(){}};
 const dy = {dataset:{platform:'dy'},classList:{toggle(){}},setAttribute(){}};
 const ctx = vm.createContext({state:{platform:'dy',page:4,view:'videos'},document:{querySelectorAll:()=>[wx,dy]},$:s=>elements[s] ||= {classList:{toggle(){}}},switchView(){},refresh(){}});
 const start=source.indexOf('function setPlatform(');
 vm.runInContext(source.slice(start,source.indexOf("document.querySelectorAll('[data-platform]').forEach(b=>b.onclick",start)),ctx);
 ctx.setPlatform('wx');
 assert.equal(ctx.state.platform,'wx');assert.equal(elements['#douyinWorkspace'].hidden,false);assert.equal(elements['#wechatWorkspace'].hidden,false);assert.equal(elements['#addBtn'].hidden,false);
 ctx.setPlatform('dy');assert.equal(elements['#douyinWorkspace'].hidden,false);assert.equal(elements['#wechatWorkspace'].hidden,true);
});
test('WeChat upper-range counter does not render as an exact 100,002', () => {
 const elements = {'#sortFilter':{value:'likes'}};
 const ctx=vm.createContext({state:{platform:'wx',videos:[{id:1,platform_id:'x',account_id:2,title:'测试作品',likes:100002,hearts:100002,likes_exact:true,comments:1,shares:2}],accounts:[],page:1,total:1},$:s=>elements[s]||={value:'',setAttribute(){}},account:()=>({display_name:'作者'}),num:v=>Number(v).toLocaleString('en-US'),esc:String,safeCover:()=>'',safeLink:()=>'',dayKey:()=>'',date:()=>'',empty:()=>'',document:{querySelectorAll:()=>[]}});
 const start=source.indexOf('function renderVideos(');
 const helper=source.indexOf('function wechatMetric(');
 if(helper>=0)vm.runInContext(source.slice(helper,source.indexOf('function renderVideos(',helper)),ctx);
 vm.runInContext(source.slice(start,source.indexOf('function renderNotifications(',start)),ctx);
 ctx.renderVideos();
 assert.match(elements['#videoGrid'].innerHTML,/10万\+/);
 assert.doesNotMatch(elements['#videoGrid'].innerHTML,/精确大拇指/);
 assert.match(elements['#videoGrid'].innerHTML,/data-video-download="1" data-media-platform="wx"/);
});
test('authentication errors stay visible and network failures keep their own meaning', () => {
 for (const platform of ['wx','dy']) {
  for (const [message,expired] of [['登录会话已失效，请重新扫码',true],['接口网络失败或超时，请稍后重试',false]]) {
   const elements={};let removed=false;
   const ctx=vm.createContext({state:{platform,accounts:[{display_name:'作者',last_status:'error',last_error:message}]},$:s=>elements[s]||={classList:{remove(){removed=true}}}});
   const start=source.indexOf('function renderMonitorError(');
   vm.runInContext(source.slice(start,source.indexOf("$('#monitorErrorConnection').onclick",start)),ctx);
   ctx.renderMonitorError();
   assert.equal(elements['#monitorError'].hidden,false);
   assert.match(elements['#monitorErrorText'].textContent,new RegExp(message));
   assert.equal(removed,expired);
   assert.equal(elements['#monitorErrorConnection'].textContent,expired?'重新登录':'连接信息');
  }
 }
});
test('successful WeChat login closes dialog and refreshes connection', async () => {
 const fields={};let closed=false,refreshed=false;
 const ctx=vm.createContext({wxLoginID:'job',wxLoginEpoch:1,wxPollTimer:0,wxLoginMessages:{success:'成功'},state:{},clearTimeout(){},setTimeout(){throw Error('must stop polling')},$:s=>fields[s]||={open:true,removeAttribute(){},close(){closed=true;this.open=false}},api:async()=>({state:'success'}),refreshWechat:async()=>{refreshed=true},renderMonitorError(){},toast(){}});
 const start=source.indexOf('async function pollWechatLogin(');
 vm.runInContext(source.slice(start,source.indexOf("$('#wxLoginBtn').onclick",start)),ctx);
 await ctx.pollWechatLogin();assert.equal(refreshed,true);assert.equal(closed,true);
});
test('fresh verified WeChat connection is not overwritten by older authentication failure', () => {
 const elements={};let removed=false;
 const ctx=vm.createContext({state:{platform:'wx',wxConnection:{configured:true,verified_at:'2026-11-10T14:10:00+08:00'},accounts:[{display_name:'作者',last_status:'error',last_error:'视频号会话已失效，请重新扫码',last_checked_at:'2026-11-10T13:00:00+08:00'}]},$:s=>elements[s]||={classList:{remove(){removed=true}}}});
 const start=source.indexOf('function renderMonitorError(');vm.runInContext(source.slice(start,source.indexOf("$('#monitorErrorConnection').onclick",start)),ctx);
 ctx.renderMonitorError();assert.equal(removed,false);assert.equal(elements['#monitorError'].hidden,true);
});
test('batch author input splits and deduplicates with bounded requests', () => {
 const ctx=vm.createContext({});const start=source.indexOf('function parseWxAuthorQueries(');
 vm.runInContext(source.slice(start,source.indexOf('function wxAuthorExists(',start)),ctx);
 assert.deepEqual(Array.from(ctx.parseWxAuthorQueries('甲\n乙，甲;丙；丁')),['甲','乙','丙','丁']);
 assert.throws(()=>ctx.parseWxAuthorQueries(' '),/请输入/);
 assert.throws(()=>ctx.parseWxAuthorQueries(Array.from({length:21},(_,i)=>'作者'+i).join('\n')),/最多20/);
});
test('batch add saves only selected identities and retries only failures', async () => {
 const elements={};const calls=[];let fail=true,closed=false;
 const ctx=vm.createContext({state:{platform:'wx',accounts:[]},$:s=>elements[s]||={value:'60',close(){closed=true}},api:async(path,method,data)=>{calls.push(data.input);if(data.input==='b'&&fail)throw Error('接口超时');return {id:data.input,platform:'wx',platform_account_id:data.input}},refresh:async()=>{},toast(){}});
 const start=source.indexOf('let wxAuthors=');
 vm.runInContext(source.slice(start,source.indexOf("$('#wxAuthorForm').onsubmit",start)),ctx);
 vm.runInContext("setWxAuthorBusy=v=>{wxAuthorBusy=v};renderWxAuthors=()=>{};wxAuthorSelected.set('a',{username:'a',nickname:'甲'});wxAuthorSelected.set('b',{username:'b',nickname:'乙'})",ctx);
 await ctx.addWxAuthors();assert.deepEqual(calls,['a','b']);assert.equal(closed,false);assert.match(elements['#wxAuthorError'].textContent,/乙：接口超时/);
 fail=false;await ctx.addWxAuthors();assert.deepEqual(calls,['a','b','b']);assert.equal(closed,true);assert.equal(ctx.state.accounts.length,2);
});

test('saved Yuanbao session hides login unless an authentication error needs recovery', async () => {
 const fields={};const ctx=vm.createContext({$:s=>fields[s]||={open:true,removeAttribute(){}},mediaPlatform:'wx',yuanbaoQRShown:false,clearTimeout(){},setTimeout(){},yuanbaoTimer:null,pollYuanbao(){}});
 vm.runInContext(source.slice(source.indexOf('async function showYuanbao('),source.indexOf('async function pollYuanbao(')),ctx);
 await ctx.showYuanbao({configured:true,state:'idle',qr_ready:false});
 assert.equal(fields['#yuanbaoLogin'].hidden,true);
});
