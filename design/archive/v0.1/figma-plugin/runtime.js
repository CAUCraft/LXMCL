/* No network, no external libraries. All artwork and content are included. */
figma.showUI(__html__, {width:440,height:580,themeColors:true});
const colors = h => {const c=parseInt(h.slice(1),16);return {r:(c>>16)/255,g:((c>>8)&255)/255,b:(c&255)/255}};
const solid=(h,opacity=1)=>({type:'SOLID',color:colors(h),opacity});
let fontRegular,fontBold,masterCache={},createdPages=[],pendingReactions=[],warnings=[];
async function fonts(){
 const all=await figma.listAvailableFontsAsync();
 const family=['Noto Sans SC','Microsoft YaHei','Microsoft YaHei UI','PingFang SC','Noto Sans CJK SC','Source Han Sans CN','Inter'].find(f=>all.some(a=>a.fontName.family===f));
 if(!family)throw Error('找不到可用字体，请启用 Noto Sans SC 后重试。');
 const list=all.filter(a=>a.fontName.family===family);fontRegular=(list.find(a=>/Regular|Normal|Book/.test(a.fontName.style))||list[0]).fontName;fontBold=(list.find(a=>/Bold|Semibold|SemiBold|Medium/.test(a.fontName.style))||list[0]).fontName;
 await Promise.all([figma.loadFontAsync(fontRegular),figma.loadFontAsync(fontBold)]);
 if(family==='Inter')warnings.push('仅找到 Inter；请检查中文字体替代情况。');
}
function textNode(n){const f=figma.createText();f.fontName=n.weight>=600?fontBold:fontRegular;f.fontSize=n.size||14;f.lineHeight={value:150,unit:'PERCENT'};f.characters=n.text||n.placeholder||'';f.fills=[solid(n.color||'#20354D')];f.resize(Math.max(n.w,1),Math.max(n.h,1));f.textAutoResize='HEIGHT';f.name=f.characters.slice(0,48);return f}
function primitive(n){
 let f;if(n.type==='text'||n.type==='input')f=textNode(n);
 else if(n.type==='svg'){f=figma.createFrame();f.resize(n.w,n.h);f.fills=[];f.clipsContent=false;const v=figma.createNodeFromSvg(n.svg);const scale=Math.min(n.w/v.width,n.h/v.height);v.rescale(scale);f.appendChild(v);v.x=(n.w-v.width)/2;v.y=(n.h-v.height)/2;f.name=n.name||'Icon';}
 else if(n.type==='ellipse'){f=figma.createEllipse();f.resize(n.w,n.h);f.fills=n.glow?[{type:'GRADIENT_RADIAL',gradientTransform:[[1,0,0],[0,1,0]],gradientStops:[{position:0,color:{...colors(n.fill),a:n.opacity||1}},{position:1,color:{...colors(n.fill),a:0}}]}]:[solid(n.fill,n.opacity??1)];f.name='Atmosphere';}
 else{f=figma.createRectangle();f.resize(Math.max(1,n.w),Math.max(1,n.h));f.cornerRadius=n.r||0;f.fills=[solid(n.fill||'#FFFFFF',n.type==='hit'?0.001:(n.opacity??1))];if(n.stroke){f.strokes=[solid(n.stroke)];f.strokeWeight=1;}f.name=n.label||n.component||'Surface';
  if(n.glass)f.effects=[{type:'BACKGROUND_BLUR',radius:24,visible:true},{type:'DROP_SHADOW',color:{r:.2,g:.35,b:.5,a:.06},offset:{x:0,y:8},radius:24,spread:0,visible:true,blendMode:'NORMAL'}];
  if(n.shadow)f.effects=[{type:'DROP_SHADOW',color:{r:.05,g:.15,b:.25,a:.20},offset:{x:0,y:16},radius:48,spread:0,visible:true,blendMode:'NORMAL'}];
 }
 f.x=n.x;f.y=n.y;if(n.action){pendingReactions.push({node:f,action:n.action,theme:n._theme});}return f;
}
function buttonMaster(n,next,theme,parent,offset){
 const icon=next&&next.type==='svg'?next:null;const key=theme+'|'+n.component+'|'+n.label+'|'+Boolean(icon)+'|'+n.h;
 if(masterCache[key])return masterCache[key];
 const c=figma.createComponent();c.name=THEME_NAME(theme)+' / '+n.component+' / '+n.label;c.resize(n.w,n.h);c.cornerRadius=n.r;c.fills=[solid(n.fill)];if(n.stroke)c.strokes=[solid(n.stroke)];
 c.layoutMode='HORIZONTAL';c.primaryAxisSizingMode='FIXED';c.counterAxisSizingMode='FIXED';c.primaryAxisAlignItems='MIN';c.counterAxisAlignItems='CENTER';c.paddingLeft=16;c.paddingRight=12;c.itemSpacing=8;
 if(icon){const v=figma.createNodeFromSvg(icon.svg);v.rescale(20/v.width);c.appendChild(v);}
 const label=textNode({text:n.label,size:15,weight:600,w:n.w-32,color:n.component==='Button/Primary'?(THEMES_NATIVE[theme].dark?'#132A27':'#FFFFFF'):THEMES_NATIVE[theme].ink,h:24});c.appendChild(label);label.layoutGrow=1;
 const property=c.addComponentProperty('Label','TEXT',n.label);label.componentPropertyReferences={characters:property};c.description='可复用按钮 / Auto Layout / 标签可覆盖。焦点与状态规范见信息架构文档。';parent.appendChild(c);const index=Object.keys(masterCache).filter(k=>k.startsWith(theme+'|')).length;c.x=offset.x+(index%6)*260;c.y=offset.y+Math.floor(index/6)*74;masterCache[key]=c;return c;
}
const THEMES_NATIVE=DESIGN_DATA.themes;
function THEME_NAME(k){return THEMES_NATIVE[k].name}
function addText(parent,s,x,y,size=20,color='#20354D',w=1400){const n=textNode({text:s,x,y,w,h:200,size,color,weight:size>=20?700:400});parent.appendChild(n);n.x=x;n.y=y;return n}
async function build(msg){
 warnings=[];await fonts();masterCache={};pendingReactions=[];const startPage=figma.currentPage;
 const bbox=startPage.children.map(n=>n.absoluteBoundingBox).filter(Boolean);const originY=bbox.length?Math.max(...bbox.map(n=>n.y+n.height))+300:0;
 const stamp='LXMCL '+new Date().toISOString().slice(0,10);const frames={};let sectionIndex=0,firstFrame=null,layerCount=0;
 for(const theme of msg.themes){
  const p=msg.newpages?figma.createPage():startPage;if(msg.newpages){p.name=stamp+' / '+THEME_NAME(theme);await figma.setCurrentPageAsync(p);createdPages.push(p);}
  const baseY=msg.newpages?0:originY+sectionIndex*4350;sectionIndex++;
  addText(p,THEME_NAME(theme)+'  /  '+THEMES_NATIVE[theme].tag,0,baseY,48,THEMES_NATIVE[theme].dark?'#3D8F78':'#20354D');
  addText(p,'UI CONCEPT · 示例数据 · 11 个主要页面与状态 · 1440 × 900',0,baseY+80,18,'#66788F');
  const boards=DESIGN_DATA.boards.filter(b=>b.theme===theme);let bi=0;
  for(const b of boards){
   figma.ui.postMessage({text:'生成 '+THEME_NAME(theme)+' / '+b.name+'…'});
   const frame=figma.createFrame();frame.name=b.name+' / '+b.key;frame.resize(b.width,b.height);frame.fills=[solid(THEMES_NATIVE[theme].bg)];frame.clipsContent=true;p.appendChild(frame);frame.x=(bi%4)*1520;frame.y=baseY+160+Math.floor(bi/4)*1000;bi++;frames[b.id]=frame;if(!firstFrame)firstFrame=frame;
   for(let i=0;i<b.nodes.length;i++){
    const n={...b.nodes[i],_theme:theme};let f;
    if(n.component&&n.component.startsWith('Button/')){
     const next=b.nodes[i+1];const master=buttonMaster(n,next,theme,p,{x:0,y:baseY+3280});f=master.createInstance();f.resize(n.w,n.h);f.name=n.label;f.x=n.x;f.y=n.y;pendingReactions.push({node:f,action:n.action,theme});
     i+=next&&next.type==='svg'?2:1;
    }else f=primitive(n);
    frame.appendChild(f);f.x=n.x;f.y=n.y;layerCount++;
   }
   // Fixed desktop canvas. Native child text, vectors and component instances remain editable.
   frame.exportSettings=[{format:'PNG',suffix:'',constraint:{type:'SCALE',value:1}}];
  }
  addText(p,'COMPONENTS / 原生可复用按钮',0,baseY+3190,28);
  for(const [key,value] of Object.entries(THEMES_NATIVE[theme]))if(typeof value==='string'&&/^#[0-9a-f]{6}$/i.test(value)){const s=figma.createPaintStyle();s.name='LXMCL/'+theme+'/'+key;s.paints=[solid(value)];}
 }
 let wired=0;
 for(const r of pendingReactions){let dest=r.action.startsWith('route:')?frames[r.theme+'/'+r.action.slice(6)]:null;
  if(r.action==='install')dest=frames[r.theme+'/downloads'];
  if(dest===r.node.parent)continue;
  if(dest&&r.node.setReactionsAsync){try{await r.node.setReactionsAsync([{trigger:{type:'ON_CLICK'},actions:[{type:'NODE',destinationId:dest.id,navigation:'NAVIGATE',transition:null,preserveScrollPosition:false}]}]);wired++}catch(err){warnings.push('导航连接失败：'+r.action)}}
 }
 await figma.setCurrentPageAsync(firstFrame.parent);figma.currentPage.selection=[firstFrame];figma.viewport.scrollAndZoomIntoView([firstFrame]);
 figma.notify('已生成 '+Object.keys(frames).length+' 个可编辑画板');
 figma.ui.postMessage({done:true,text:`完成：${Object.keys(frames).length} 个画板，${layerCount} 个顶层图层，${Object.keys(masterCache).length} 个按钮组件，${wired} 条导航连线。\n非导航操作的交互演示见随附 HTML 原型。`+(warnings.length?'\n'+[...new Set(warnings)].join('\n'):'')});
}
function existingBoards(){
 const frames={};
 for(const key of Object.keys(DESIGN_DATA.themes)){
  const ti=Object.keys(DESIGN_DATA.themes).indexOf(key);
  for(const b of DESIGN_DATA.boards.filter(v=>v.theme===key)){
   const candidates=figma.currentPage.children.filter(n=>n.type==='FRAME'&&n.name===b.name+' / '+b.key&&n.width===1440).sort((a,b)=>a.y-b.y);
   if(candidates[ti])frames[b.id]=candidates[ti];
  }
 }
 return frames;
}
async function inspectBoards(){
 const frames=existingBoards();let wired=0,stationary=0;const errors=[];let textCount=0,instances=0,vectorCount=0;const fontsUsed=new Set();
 for(const b of DESIGN_DATA.boards){
  const f=frames[b.id];if(!f){errors.push('缺少画板 '+b.id);continue;}
  for(const n of f.findAll()){
   if(n.type==='TEXT'){textCount++;if(n.fontName!==figma.mixed)fontsUsed.add(n.fontName.family);}
   if(n.type==='INSTANCE')instances++;if(n.type==='VECTOR')vectorCount++;
  }
  for(const item of b.nodes.filter(n=>n.action)){
   const action=item.action;const dest=action.startsWith('route:')?frames[b.theme+'/'+action.slice(6)]:action==='install'?frames[b.theme+'/downloads']:null;
   if(!dest)continue;if(dest===f){stationary++;continue;}
   const node=f.children.find(n=>n.name===item.label&&Math.abs(n.x-item.x)<1&&Math.abs(n.y-item.y)<1);
   if(!node){errors.push(b.id+' / 未找到 '+item.label);continue;}
   if(node.reactions&&node.reactions.some(r=>r.actions&&r.actions.some(a=>a.destinationId===dest.id))){wired++;continue;}
   try{await node.setReactionsAsync([{trigger:{type:'ON_CLICK'},actions:[{type:'NODE',destinationId:dest.id,navigation:'NAVIGATE',transition:null,preserveScrollPosition:false}]}]);wired++;}catch(e){errors.push(b.id+' / '+item.label+' / '+e.message);}
  }
 }
 const report={boards:Object.keys(frames).length,textLayers:textCount,buttonInstances:instances,vectorLayers:vectorCount,navigationLinks:wired,currentPageActions:stationary,fonts:[...fontsUsed],errors,homes:Object.fromEntries(Object.keys(DESIGN_DATA.themes).map(k=>[k,frames[k+'/home']?.id]))};
 figma.ui.postMessage({done:true,text:JSON.stringify(report,null,2)});
}
async function guide(){
 await fonts();let f=figma.currentPage.children.find(n=>n.name==='00 功能分类与交互路径');
 if(!f){
  f=figma.createFrame();f.name='00 功能分类与交互路径';f.resize(1520,1230);f.x=0;f.y=-1510;f.fills=[solid('#F3F5F8')];
  addText(f,'LXMCL / 功能分类与交互路径',48,36,38,'#20354D',1424);
  addText(f,'按用户任务组织功能；高级选项在所属任务中展开。服务于中国民航大学，视觉围绕 Minecraft 本身。',48,103,18,'#627186',1424);
  const groups=[
   ['01 开始游戏','快速继续上次游玩','当前游戏 · 账号 · 运行环境\n一键启动 · 社团服快捷入口\n新手引导与启动状态'],
   ['02 社团联机','找到服务器并进入','服务器列表 · 公告 · 延迟\n兼容游戏匹配 · 入服准备\n临时联机房间'],
   ['03 我的游戏','管理已经安装的内容','创建 / 导入 · 多实例隔离\n模组 / 资源包 / 光影 / 世界\n版本组件 · 当前游戏设置'],
   ['04 资源中心','发现并安装新的内容','模组 / 整合包 / 资源包 / 光影\nModrinth / CurseForge 与筛选\n目标游戏 · 兼容检查 · 安装预览'],
   ['05 账号与装扮','管理身份、皮肤与披风','社团 / Microsoft / 离线 / 第三方\n账号切换 · 认证状态\n皮肤站衣橱 · 预览 · 本地导入'],
   ['06 下载任务','跟进下载并从失败中恢复','进行中 / 已完成 / 失败\n依赖与任务分组 · 暂停 / 继续\n断点续传 · 重试 · 备用源'],
   ['07 设置','修改默认行为与运行环境','外观 / 字体 / 主题 · 常规行为\nJava 与存储 · 下载源与代理\n游戏默认配置 · 扩展与更新'],
   ['08 诊断与工具','解决启动、资源与网络问题','运行日志 · 崩溃建议 · 诊断包\n连接测试 · 内存与图形缓存\n就近提供修复与教程入口'],
   ['09 帮助中心','按任务学习操作','登录账号 · 首次入服 · 安装游戏\n资源管理 · 常见问题\n全局搜索与上下文帮助']
  ];
  groups.forEach((g,i)=>{const x=48+i%3*480,y=172+Math.floor(i/3)*250;const r=figma.createRectangle();f.appendChild(r);r.x=x;r.y=y;r.resize(464,230);r.cornerRadius=14;r.fills=[solid('#FFFFFF')];r.strokes=[solid('#DCE3EC')];addText(f,g[0],x+24,y+20,23,'#20354D',416);addText(f,g[1],x+24,y+62,15,'#2566B5',416);addText(f,g[2],x+24,y+104,16,'#627186',416);});
  addText(f,'常用路径',48,941,25,'#20354D',400);
  addText(f,'首次入服：社团联机 → 登录 → 匹配游戏与 Java → 确认下载 → 启动进入\n安装资源：资源中心 → 选择目标游戏 → 兼容与变更预览 → 下载 → 我的游戏\n全局搜索：Ctrl K → 分组结果 → 准备进入 / 打开游戏 / 管理账号 / 前往设置',48,989,18,'#20354D',1424);
  addText(f,'边界：线上资源与已安装资源分开；皮肤归账号；用户自行导入不支持在线更新；全局默认与实例覆盖分别标注。',48,1121,15,'#627186',1424);
 }
 figma.currentPage.name='LXMCL / 三版 UI 与功能地图';figma.currentPage.selection=[f];figma.viewport.scrollAndZoomIntoView([f]);
 figma.ui.postMessage({done:true,text:'功能地图已添加。点击“检查图层与导航”查看原生图层和连接结果。'});
}
figma.ui.onmessage=msg=>{
 let p;if(msg.type==='build')p=build(msg);
 if(msg.type==='inspect')p=inspectBoards();
 if(msg.type==='guide')p=guide();
 if(msg.type==='show'){const f=existingBoards()[msg.theme+'/'+msg.key];if(f){figma.currentPage.selection=[f];figma.viewport.scrollAndZoomIntoView([f]);figma.closePlugin();}return;}
 if(p)p.catch(e=>{figma.ui.postMessage({error:true,text:'操作未完成：'+String(e.message||e)});figma.notify('请查看插件提示',{error:true});});
};
