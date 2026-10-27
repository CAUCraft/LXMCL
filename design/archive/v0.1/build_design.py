"""LXMCL UI concepts: shared editable scene graph for SVG, HTML and Figma."""
from pathlib import Path
import json, html, math

ROOT=Path(__file__).parent
THEMES={
 'glass':dict(name='01 液态玻璃',tag='LIQUID GLASS',bg='#E5EFF9',ink='#20354D',muted='#5B7088',accent='#286CE8',soft='#E3EDFF',card='#FFFFFF',line='#D4E0ED',radius=24,dark=False),
 'classic':dict(name='02 经典桌面',tag='CLASSIC / DESKTOP',bg='#F3F5F8',ink='#263446',muted='#627186',accent='#2566B5',soft='#E7F0FC',card='#FFFFFF',line='#DCE3EC',radius=8,dark=False),
 'terminal':dict(name='03 极简深色',tag='MINIMAL / DARK',bg='#101B25',ink='#ECF4F8',muted='#9FAFBE',accent='#62E2BB',soft='#203F3B',card='#1B2A36',line='#344653',radius=16,dark=True),
}
NAMES={'home':'开始游戏','servers':'社团联机','instances':'我的游戏','resources':'资源中心','character':'账号与装扮','settings':'设置','downloads':'下载任务','join':'加入服务器','search':'聚合搜索','conflict':'安装前确认','diagnostics':'诊断与工具'}
ICONS={
 'home':'M3 11 12 3l9 8v10h-6v-7H9v7H3Z',
 'servers':'M4 3h16v7H4ZM4 14h16v7H4ZM7 6h1M7 17h1M14 6h3M14 17h3',
 'instances':'m12 2 10 5v10l-10 5-10-5V7Zm0 10v10M2 7l10 5 10-5',
 'resources':'M12 3v12m-5-5 5 5 5-5M4 15v6h16v-6',
 'character':'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8ZM4 22v-3c0-6 16-6 16 0v3',
 'settings':'M4 6h16M4 12h16M4 18h16M8 3v6m8 0v6m-6 0v6',
 'downloads':'M12 3v12m-5-5 5 5 5-5M4 18v3h16v-3',
 'search':'M10 17a7 7 0 1 0 0-14 7 7 0 0 0 0 14Zm5-2 6 6',
 'diagnostics':'M3 12h4l3-8 4 16 3-8h4',
 'play':'m7 4 14 8-14 8Z','check':'m4 12 5 5L21 5','plus':'M12 4v16M4 12h16','arrow':'M4 12h16m-6-6 6 6-6 6','help':'M9 8c0-5 8-5 8 0 0 3-5 3-5 6M12 18v1',
}

class Board:
 def __init__(self,theme,key):
  self.theme=theme; self.t=THEMES[theme]; self.key=key; self.nodes=[]; self.width=1440; self.height=900
 def add(self,type,x,y,w,h,**k):
  n=dict(type=type,x=round(x,2),y=round(y,2),w=round(w,2),h=round(h,2),**k); self.nodes.append(n); return n
 def rect(self,x,y,w,h,fill=None,r=None,stroke=None,**k):
  return self.add('rect',x,y,w,h,fill=fill or self.t['card'],r=self.t['radius'] if r is None else r,stroke=stroke,**k)
 def text(self,x,y,txt,size=16,color=None,weight=400,w=700,**k):
  lines=txt.count('\n')+1
  return self.add('text',x,y,w,size*1.5*lines,text=txt,size=size,color=color or self.t['ink'],weight=weight,**k)
 def icon(self,name,x,y,size=20,color=None):
  path=ICONS.get(name,ICONS['instances']); color=color or self.t['muted']
  return self.add('svg',x,y,size,size,svg=f'<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><path d="{path}" fill="none" stroke="{color}" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/></svg>')
 def line(self,x,y,w,color=None): self.rect(x,y,w,1,color or self.t['line'],0)
 def panel(self,x,y,w,h,**k):
  return self.rect(x,y,w,h,stroke='#FFFFFF' if self.theme=='glass' else self.t['line'],opacity=.66 if self.theme=='glass' else 1,glass=self.theme=='glass',**k)
 def btn(self,x,y,w,label,action,primary=False,icon=None,h=44):
  fill=self.t['accent'] if primary else self.t['card']; color=('#132A27' if self.t['dark'] else '#FFFFFF') if primary else self.t['ink']
  self.rect(x,y,w,h,fill,12 if self.theme!='classic' else 6,stroke=None if primary else self.t['line'],action=action,label=label,component='Button/Primary' if primary else 'Button/Secondary')
  off=40 if icon else 16
  if icon:self.icon(icon,x+14,y+(h-20)/2,20,color)
  self.text(x+off,y+(h-24)/2,label,15,color,600,w=w-off-12)
 def chip(self,x,y,label,good=False,w=None,color=None):
  w=w or (len(label)*14+22)
  self.rect(x,y,w,28,self.t['soft'] if not good else ('#1C453B' if self.t['dark'] else '#E5F4EC'),14)
  self.text(x+10,y+4,label,12,color or (('#78E7BD' if self.t['dark'] else '#26724F') if good else self.t['accent']),500,w=w-16)
 def field(self,x,y,w,label,kind='text',value=''):
  self.rect(x,y,w,44,stroke=self.t['line'],r=8)
  self.add('input',x+14,y+9,w-28,26,text=value,placeholder=label,size=14,color=self.t['muted'],kind=kind)
 def row(self,x,y,w,title,sub,action=None,tag=None):
  self.text(x,y,title,16,weight=600,w=w-180)
  self.text(x,y+30,sub,13,self.t['muted'],w=w-60)
  if tag:self.chip(x+w-130,y+3,tag,w=120)
  if action:self.btn(x+w-114,y+6,114,'查看详情',action,h=38)
 def toggle(self,x,y,on=True,action='toggle'):
  self.rect(x,y,42,24,self.t['accent'] if on else self.t['line'],12,action=action,label='关闭' if on else '开启',component='Switch')
  self.rect(x+(21 if on else 3),y+3,18,18,'#FFFFFF',9)
 def art(self,x,y,w,h,variant='world'):
  self.add('svg',x,y,w,h,svg=artwork(variant,self.theme),name='原创矢量 / '+variant)
 def pack(self):return dict(id=f'{self.theme}/{self.key}',theme=self.theme,key=self.key,name=NAMES.get(self.key,self.key),width=self.width,height=self.height,nodes=self.nodes)

def artwork(variant,theme):
 dark=theme=='terminal'; p=[]
 if variant=='avatar':
  colors=['#916345','#AF805B','#E3B18A','#193D50','#36758A','#59A6B4','#253B50']
  p=['<ellipse cx="200" cy="330" rx="96" ry="20" fill="#50819A" opacity=".15"/>']
  for x,y,w,h,c in [(153,42,94,88,0),(160,68,80,62,2),(145,130,110,108,4),(107,132,37,106,2),(256,132,37,106,2),(148,238,49,88,6),(202,238,49,88,6),(161,46,80,25,0),(176,87,12,10,6),(218,87,12,10,6),(194,109,15,6,0),(188,132,24,30,5)]:p.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" fill="{colors[c]}"/>')
  return '<svg xmlns="http://www.w3.org/2000/svg" width="400" height="360" viewBox="0 0 400 360">'+''.join(p)+'</svg>'
 # Original block landscape: grass, soil, water and trees.
 p=['<ellipse cx="302" cy="350" rx="242" ry="40" fill="#173A38" opacity=".12"/>']
 def tile(x,y,w,d,h,top,left,right):
  return f'<path d="M{x} {y}l{w} {-w/2} {d} {d/2} {-w} {w/2}Z" fill="{top}"/><path d="M{x} {y}v{h}l{d} {d/2}v{-h}Z" fill="{left}"/><path d="M{x+d} {y+d/2}l{w} {-w/2}v{h}l{-w} {w/2}Z" fill="{right}"/>'
 grass=('#6EBD9C','#416B58','#518A70') if dark else ('#A8D9A3','#78A46E','#8DBD7C')
 p.append(tile(60,230,240,240,65,'#6A8C64','#8C705C','#AF8A68'))
 p.append(tile(60,220,240,240,14,*grass))
 p.append(tile(246,220,110,88,4,'#86CDD1','#539AA6','#66B0BC'))
 p.append(tile(148,181,85,75,45,*grass))
 p.append(tile(241,152,70,65,52,*grass))
 p.append(tile(213,213,31,28,21,'#CAD2CD','#8D9B98','#A9B6AE'))
 p.append(tile(398,241,27,28,17,'#CAD2CD','#8D9B98','#A9B6AE'))
 for x,y in [(139,197),(289,140),(414,217)]:
  p.append(tile(x,y,14,14,57,'#A9815C','#735C45','#96704E'))
  p.append(tile(x-29,y-38,46,46,48,'#72B886','#3C825B','#52996A'))
  p.append(tile(x-16,y-61,31,31,27,'#91CCA0','#519668','#6DB17D'))
 for x,y in [(148,270),(183,288),(363,288),(404,266)]:
  p.append(f'<path d="M{x} {y}v-9m-4 5 4 4 4-4" stroke="#58894B" stroke-width="2"/>')
 p.append(tile(132,266,13,12,10,'#F1E4BA','#C5AA79','#DEC99B'))
 return '<svg xmlns="http://www.w3.org/2000/svg" width="600" height="430" viewBox="0 0 600 430">'+''.join(p)+'</svg>'


def shell(b,active):
 t=b.t; b.rect(0,0,1440,900,t['bg'],0)
 if b.theme=='glass':
  b.add('ellipse',650,-280,1000,1000,fill='#B4CCFA',opacity=.7,glow=True)
  b.add('ellipse',-200,400,1000,800,fill='#A9DCD8',opacity=.65,glow=True)
  b.add('ellipse',690,450,1000,700,fill='#CEBDEE',opacity=.6,glow=True)
 b.text(28,14,'LXMCL',13,t['muted'],600,w=130); b.text(1110,14,'中国民航 MC 启动器',12,t['muted'],w=200)
 b.text(1304,10,'—    □    ×',17,t['muted'],w=130)
 nav=[('home','开始游戏'),('servers','社团联机'),('instances','我的游戏'),('resources','资源中心')]
 if b.theme=='classic':
  b.panel(24,54,1392,76)
  b.icon('instances',46,77,27,t['accent']); b.text(82,73,'LXMCL',25,weight=700,w=140)
  for i,(k,label) in enumerate(nav):
   x=243+i*153
   if active==k:b.rect(x-12,68,140,48,t['soft'],7)
   b.icon(k,x,82,20,t['accent'] if active==k else t['muted']); b.text(x+31,80,label,16,t['accent'] if active==k else t['ink'],600,w=95)
   b.add('hit',x-12,68,140,48,action='route:'+k,label=label)
  b.btn(876,70,128,'账号装扮','route:character',active=='character',icon='character')
  b.btn(1016,70,94,'设置','route:settings',active=='settings',icon='settings')
  b.btn(1122,70,124,'搜索','route:search',icon='search')
  b.btn(1258,70,136,'下载 · 2','route:downloads',active=='downloads',icon='downloads')
  b.x,b.y,b.w=44,160,1352
 else:
  if b.theme=='glass':b.panel(24,62,214,806)
  else:b.rect(0,44,216,856,'#14232F',0);b.line(216,44,1)
  b.icon('instances',49,94,28,t['accent']);b.text(90,88,'LXMCL',25,weight=700,w=140)
  b.text(49,132,'你的游戏，在这里',12,t['muted'],w=180)
  for i,(k,label) in enumerate(nav):
   y=198+i*60
   if active==k:b.rect(38,y-10,184,48,t['soft'],13 if b.theme=='glass' else 7)
   b.icon(k,53,y+2,21,t['accent'] if active==k else t['muted']);b.text(89,y,label,16,t['accent'] if active==k else t['ink'],600 if active==k else 400,w=120)
   b.add('hit',38,y-10,184,48,action='route:'+k,label=label)
  b.line(48,454,164)
  for i,(k,label) in enumerate([('character','账号与装扮'),('downloads','下载任务'),('diagnostics','诊断与工具')]):
   y=482+i*52
   if active==k:b.rect(38,y-10,184,44,t['soft'],12)
   b.icon(k,53,y+2,20,t['accent'] if active==k else t['muted']);b.text(89,y,label,15,t['accent'] if active==k else t['ink'],w=126);b.add('hit',38,y-10,184,44,action='route:'+k,label=label)
  b.icon('help',53,668,20);b.text(89,666,'帮助中心',15,w=126);b.add('hit',38,656,184,44,action='help',label='帮助中心')
  if active=='settings':b.rect(38,710,184,44,t['soft'],12)
  b.icon('settings',53,722,20);b.text(89,720,'设置',15,w=120);b.add('hit',38,710,184,44,action='route:settings',label='设置')
  b.rect(44,784,172,62,t['soft'],14);b.rect(57,796,34,34,t['accent'],10);b.text(66,801,'L',18,'#FFFFFF' if not t['dark'] else '#16342F',700,w=25)
  b.text(101,793,'LingXiao',14,weight=600,w=100);b.text(101,816,'民航社团账号',11,t['muted'],w=104);b.add('hit',44,784,172,62,action='route:character',label='切换账号')
  b.x,b.y,b.w=274 if b.theme=='glass' else 250,144,1128 if b.theme=='glass' else 1152
  b.rect(b.x,66,600,45,t['card'],14,stroke=t['line'],opacity=.7 if b.theme=='glass' else 1)
  b.icon('search',b.x+16,79,18);b.text(b.x+46,77,'搜索服务器、游戏、资源和设置',14,t['muted'],w=400);b.text(b.x+521,78,'Ctrl K',12,t['muted'],w=65);b.add('hit',b.x,66,600,45,action='route:search',label='打开聚合搜索')
  b.chip(1250,75,'服务正常',True,w=112)
 b.text(b.x,874,'LXMCL  /  UI CONCEPT     ·     示例数据',10,t['muted'],w=700)
 b.text(1296,874,'帮助中心  ↗',11,t['muted'],w=100);b.add('hit',1284,862,122,36,action='help',label='帮助中心')
 return b.x,b.y,b.w

def heading(b,title,sub,action=None):
 x,y,w=b.x,b.y,b.w;b.text(x,y,title,29,weight=700,w=w-260);b.text(x,y+48,sub,14,b.t['muted'],w=w-260)
 if action:b.btn(x+w-164,y+10,164,action[0],action[1],True,icon='plus')

def home(b):
 x,y,w=shell(b,'home');t=b.t
 heading(b,'准备好，开启新世界。','欢迎回来，凌霄。和社团伙伴一起，把想象变成方块。')
 y+=100;left=w-354
 b.panel(x,y,left,338)
 b.chip(x+26,y+24,'社团推荐',w=100)
 b.text(x+28,y+76,'一起探索，\n一起创造。',38,weight=700,w=350)
 b.text(x+29,y+206,'民航社团服  ·  1.21.1 / Fabric',14,t['muted'],w=380)
 b.chip(x+28,y+244,'在线 32 / 100',True,w=126)
 b.art(x+left-380,y+52,382,275,'world')
 b.btn(x+28,y+286,170,'进入社团服','route:join',True,icon='play')
 b.btn(x+212,y+286,128,'服务器详情','route:servers')
 rx=x+left+22;b.panel(rx,y,332,338)
 b.text(rx+24,y+25,'当前启动配置',17,weight=600,w=260)
 b.rect(rx+24,y+70,54,54,t['soft'],12);b.icon('instances',rx+40,y+86,22,t['accent'])
 b.text(rx+94,y+73,'社团生存 · 1.21.1',17,weight=600,w=215);b.text(rx+94,y+104,'Fabric  ·  最近游玩',12,t['muted'],w=210)
 b.line(rx+24,y+147,284)
 for i,(a,c) in enumerate([('游戏账号','LingXiao · 社团'),('运行环境','Java 21 · 已就绪'),('分配内存','自动 · 6 GB')]):
  b.text(rx+24,y+165+i*34,a,13,t['muted'],w=90);b.text(rx+132,y+165+i*34,c,13,w=180)
 b.btn(rx+24,y+282,178,'启动游戏','launch',True,icon='play');b.btn(rx+214,y+282,94,'切换','route:instances')
 yy=y+364;b.text(x,yy,'最近的世界',20,weight=600,w=400);b.text(x+w-115,yy+3,'全部游戏  →',14,t['accent'],w=115);b.add('hit',x+w-128,yy-2,128,38,action='route:instances',label='全部游戏')
 cw=(w-40)/3
 for i,(name,desc,color) in enumerate([('社团生存','1.21.1  ·  Fabric  ·  今天','#81B9A8'),('创造与建筑','1.21.1  ·  原版  ·  昨天','#83ADCE'),('机械动力','1.20.1  ·  Forge  ·  3 天前','#D0AB78')]):
  xx=x+i*(cw+20);b.panel(xx,yy+47,cw,122);b.rect(xx+20,yy+70,58,58,color,13);b.icon('instances',xx+36,yy+86,25,'#FFFFFF');b.text(xx+95,yy+72,name,17,weight=600,w=cw-120);b.text(xx+95,yy+105,desc,12,t['muted'],w=cw-110);b.add('hit',xx,yy+47,cw,122,action='route:instances',label=name)
 b.panel(x,yy+180,w,64);b.icon('help',x+22,yy+202,22,t['accent']);b.text(x+61,yy+200,'第一次玩？从账号登录到进入服务器，跟着指引就能完成。',14,w=w-270);b.btn(x+w-170,yy+191,148,'开始新手引导','route:join',h=42)

def servers(b):
 x,y,w=shell(b,'servers');t=b.t;heading(b,'社团联机','服务器、兼容游戏与入服准备，都在这里。')
 yy=y+99;b.panel(x,yy,w,270);b.art(x+w-430,yy-15,420,270,'world')
 b.chip(x+26,yy+24,'社团官方',w=100);b.text(x+28,yy+70,'民航社团 · 生存服务器',30,weight=700,w=640)
 b.text(x+29,yy+121,'一起建设校园，也一起探索更远的世界。',15,t['muted'],w=610)
 b.chip(x+28,yy+163,'在线 32 / 100',True,w=125);b.chip(x+166,yy+163,'延迟 28 ms',True,w=116);b.chip(x+296,yy+163,'1.21.1 · Fabric',w=142)
 b.btn(x+28,yy+214,182,'准备并进入','route:join',True,icon='play');b.btn(x+224,yy+214,124,'查看公告','notice')
 yy+=294;left=w*.59;b.panel(x,yy,left,270);b.text(x+24,yy+25,'入服检查',19,weight=600,w=400)
 for i,(title,detail) in enumerate([('社团账号已登录','LingXiao · 民航社团认证'),('找到兼容游戏','社团生存 · Minecraft 1.21.1 / Fabric'),('需要更新 2 项资源','约 24 MB · 保留本地存档与个人设置')]):
  ry=yy+72+i*62;b.icon('check' if i<2 else 'downloads',x+26,ry+3,20,t['accent']);b.text(x+61,ry,title,15,weight=600,w=left-80);b.text(x+61,ry+26,detail,12,t['muted'],w=left-80)
 rx=x+left+20;b.panel(rx,yy,w-left-20,270);b.text(rx+24,yy+25,'和朋友一起玩',19,weight=600,w=340)
 b.text(rx+24,yy+71,'创建临时联机房间，或加入好友的房间。',14,t['muted'],w=w-left-58)
 b.btn(rx+24,yy+117,140,'创建房间','room');b.btn(rx+177,yy+117,140,'加入房间','room')
 b.line(rx+24,yy+184,w-left-68);b.text(rx+24,yy+207,'联机组件  Terracotta',13,t['muted'],w=270)
 b.text(x,yy+293,'提示：服务器维护时会显示维护公告，并提供重试与更换服务器入口。',12,t['muted'],w=w)

def instances(b):
 x,y,w=shell(b,'instances');t=b.t;heading(b,'我的游戏','每个游戏独立保存模组、世界和设置。',('创建 / 导入','create'))
 yy=y+98;lw=280;b.panel(x,yy,lw,590);b.field(x+16,yy+18,lw-32,'搜索本地游戏',kind='instances')
 for i,(name,info) in enumerate([('社团生存','1.21.1 · Fabric · 社团'),('创造与建筑','1.21.1 · 原版'),('机械动力','1.20.1 · Forge')]):
  ry=yy+85+i*85
  if i==0:b.rect(x+10,ry-8,lw-20,76,t['soft'],10)
  b.icon('instances',x+23,ry+13,26,t['accent']);b.text(x+67,ry+3,name,16,weight=600,w=200);b.text(x+67,ry+33,info,12,t['muted'],w=200);b.add('hit',x+10,ry-8,lw-20,76,action='select:'+name,label=name)
 b.btn(x+18,yy+523,lw-36,'导入整合包 / 文件','import',icon='plus')
 rx=x+lw+20;rw=w-lw-20;b.panel(rx,yy,rw,590)
 b.text(rx+24,yy+23,'社团生存',23,weight=700,w=300);b.text(rx+24,yy+62,'1.21.1  ·  Fabric 0.16.10  ·  独立游戏目录',13,t['muted'],w=rw-240)
 b.btn(rx+rw-169,yy+25,145,'启动游戏','launch',True,icon='play')
 tabs=['概览','模组 36','资源包','光影','世界','设置']
 for i,label in enumerate(tabs):
  tx=rx+24+i*(rw-48)/6;b.text(tx,yy+112,label,14,t['accent'] if i==1 else t['muted'],600 if i==1 else 400,w=95);b.add('hit',tx-5,yy+100,(rw-48)/6,40,action='tab:'+label,label=label)
 b.line(rx+24,yy+146,rw-48);b.rect(rx+24+(rw-48)/6,yy+143,68,3,t['accent'],0)
 b.field(rx+24,yy+166,rw-300,'搜索当前游戏的模组',kind='mods');b.btn(rx+rw-258,yy+166,108,'导入文件','import');b.btn(rx+rw-138,yy+166,114,'获取模组','route:resources',True)
 b.text(rx+24,yy+235,'名称 / 来源',12,t['muted'],w=300);b.text(rx+rw-220,yy+235,'版本',12,t['muted'],w=80);b.text(rx+rw-82,yy+235,'启用',12,t['muted'],w=60)
 mods=[('Sodium','Modrinth · 可在线更新','0.6.5',True),('Fabric API','Modrinth · 已是最新','0.115.1',True),('建筑辅助.jar','用户自行导入 · 不支持在线更新','未知',True),('Iris Shaders','Modrinth · 与当前配置冲突','1.8.0',False)]
 for i,(name,detail,ver,on) in enumerate(mods):
  ry=yy+271+i*67;b.line(rx+24,ry-9,rw-48);b.text(rx+24,ry,name,15,weight=600,w=rw-270);b.text(rx+24,ry+26,detail,12,('#E4B67D' if t['dark'] else '#8C551E') if i==2 else t['muted'],w=rw-260);b.text(rx+rw-220,ry+9,ver,13,t['muted'],w=110);b.toggle(rx+rw-80,ry+10,on,'togglemod:'+name);b.add('hit',rx+20,ry-4,rw-125,59,action='mod:'+name,label='管理 '+name)
 b.text(rx+24,yy+555,'选择条目后可更新、删除或查看来源。',12,t['muted'],w=rw-40)

def resources(b):
 x,y,w=shell(b,'resources');t=b.t;heading(b,'发现新的可能。','为你的游戏寻找模组、整合包与视觉资源。')
 yy=y+99;b.field(x,yy,w-372,'搜索资源名称，支持中文别名',kind='resources');b.btn(x+w-354,yy,170,'目标：社团生存','choose-instance');b.btn(x+w-170,yy,170,'来源：全部','source')
 cats=['模组','整合包','资源包','光影','世界']
 for i,c in enumerate(cats):
  xx=x+i*118;b.rect(xx,yy+65,104,38,t['soft'] if i==0 else t['card'],19 if b.theme=='glass' else 6,stroke=t['line']);b.text(xx+27,yy+73,c,14,t['accent'] if i==0 else t['muted'],600,w=80);b.add('hit',xx,yy+65,104,38,action='category:'+c,label=c)
 b.chip(x+w-291,yy+70,'1.21.1',w=90);b.chip(x+w-190,yy+70,'Fabric',w=86);b.text(x+w-92,yy+74,'兼容优先',13,t['muted'],w=100)
 cw=(w-40)/3;items=[('S','Sodium','钠 · 渲染性能优化','改善渲染性能，让方块世界更流畅。','性能优化','#DAEFE4'),('I','Iris Shaders','Iris · 光影加载','让你喜欢的光影融入当前世界。','画面增强','#E8DEF4'),('F','Fabric API','Fabric 基础接口','许多 Fabric 模组需要的基础组件。','基础组件','#EDE7DC'),('L','Lithium','锂 · 游戏逻辑优化','面向游戏逻辑的性能优化模组。','性能优化','#DDE8F7'),('J','JEI','物品管理器','查看合成配方，探索更多玩法。','实用工具','#F1E0D3'),('M','MiniHUD','轻量信息显示','把你需要的信息放在视线之内。','实用工具','#DAECEE')]
 for i,(letter,name,cn,desc,tag,c) in enumerate(items):
  xx=x+(i%3)*(cw+20);ry=yy+128+(i//3)*231;b.panel(xx,ry,cw,210)
  b.rect(xx+20,ry+20,48,48,c,12);b.text(xx+34,ry+26,letter,25,'#35665E',700,w=30)
  b.text(xx+82,ry+20,name,18,weight=700,w=cw-100);b.text(xx+82,ry+49,cn,12,t['muted'],w=cw-100)
  b.text(xx+20,ry+86,desc,13,t['muted'],w=cw-40);b.chip(xx+20,ry+119,tag,w=94);b.text(xx+126,ry+124,'Modrinth',12,t['muted'],w=120)
  b.btn(xx+20,ry+161,cw-40,'查看并安装','route:conflict',h=34)

def character(b):
 x,y,w=shell(b,'character');t=b.t;heading(b,'账号与装扮','切换身份，也让角色拥有自己的风格。',('添加账号','account'))
 yy=y+98;left=310;b.panel(x,yy,left,590);b.text(x+22,yy+22,'我的账号',18,weight=600,w=250)
 for i,(name,sub) in enumerate([('LingXiao','民航社团 · 当前使用'),('CloudPilot','Microsoft · 已登录'),('本地玩家','离线账号 · 仅本地使用')]):
  ry=yy+76+i*94
  if i==0:b.rect(x+12,ry-9,left-24,82,t['soft'],12)
  b.rect(x+24,ry+9,40,40,t['accent'] if i==0 else t['line'],10);b.text(x+37,ry+14,name[0],20,'#FFFFFF',600,w=30)
  b.text(x+79,ry+5,name,16,weight=600,w=210);b.text(x+79,ry+36,sub,12,t['muted'],w=210);b.add('hit',x+12,ry-9,left-24,82,action='account:'+name,label='切换到 '+name)
 b.text(x+23,yy+450,'登录状态异常？',14,weight=600,w=250);b.text(x+23,yy+481,'重新登录后可继续使用当前角色。',12,t['muted'],w=270);b.btn(x+23,yy+519,left-46,'管理认证服务','auth')
 rx=x+left+20;rw=w-left-20;b.panel(rx,yy,rw,590)
 b.text(rx+26,yy+25,'LingXiao 的衣橱',22,weight=600,w=450);b.chip(rx+rw-147,yy+26,'民航皮肤站',w=123)
 b.rect(rx+25,yy+85,rw*.48,394,t['soft'],20);b.art(rx+30,yy+94,rw*.46,352,'avatar');b.text(rx+73,yy+448,'拖动旋转  ·  滚轮缩放',12,t['muted'],w=260)
 tx=rx+rw*.52+25;b.text(tx,yy+102,'皮肤',17,weight=600,w=260);b.text(tx,yy+135,'校园飞行员 · 经典模型',13,t['muted'],w=270)
 b.btn(tx,yy+181,rw*.48-48,'从皮肤站选择','skin');b.btn(tx,yy+237,rw*.48-48,'上传本地皮肤','skin-upload')
 b.text(tx,yy+316,'披风',17,weight=600,w=260);b.btn(tx,yy+359,rw*.48-48,'选择披风','cape')
 b.text(tx,yy+424,'装扮能力取决于当前账号和认证服务。',12,t['muted'],w=rw*.48-42)
 b.line(rx+26,yy+505,rw-52);b.btn(rx+rw-206,yy+527,180,'保存当前装扮','save-skin',True)

def settings(b):
 x,y,w=shell(b,'settings');t=b.t;heading(b,'设置','全局配置会用于新游戏；单个游戏可以单独覆盖。')
 yy=y+99;left=248;b.panel(x,yy,left,590)
 groups=['常规与启动','外观与无障碍','游戏默认配置','Java 与存储','下载与网络','扩展与自动化','关于与更新']
 for i,s in enumerate(groups):
  ry=yy+25+i*55
  if i==3:b.rect(x+12,ry-9,left-24,44,t['soft'],9)
  b.text(x+25,ry,s,15,t['accent'] if i==3 else t['ink'],600 if i==3 else 400,w=220);b.add('hit',x+12,ry-9,left-24,44,action='setting:'+s,label=s)
 b.line(x+24,yy+447,left-48);b.text(x+25,yy+477,'遇到问题？',14,weight=600,w=220);b.btn(x+24,yy+518,left-48,'打开诊断与工具','route:diagnostics')
 rx=x+left+20;rw=w-left-20;b.panel(rx,yy,rw,590);b.text(rx+25,yy+24,'Java 与存储',22,weight=700,w=400)
 b.text(rx+25,yy+67,'自动选择兼容版本，保留手动控制。',13,t['muted'],w=500)
 b.text(rx+25,yy+119,'自动匹配 Java',16,weight=600,w=500);b.text(rx+25,yy+150,'创建游戏或启动时，根据游戏版本检查环境。',13,t['muted'],w=rw-100);b.toggle(rx+rw-70,yy+122,True,'toggle:auto-java')
 b.line(rx+25,yy+192,rw-50);b.text(rx+25,yy+214,'已安装的 Java',17,weight=600,w=500);b.btn(rx+rw-253,yy+206,104,'重新检测','scan-java',h=38);b.btn(rx+rw-137,yy+206,112,'添加 Java','java',h=38)
 for i,(name,path) in enumerate([('Zulu OpenJDK 21','D:\\LXMCL\\runtime\\zulu-21'),('Zulu OpenJDK 17','D:\\LXMCL\\runtime\\zulu-17')]):
  ry=yy+269+i*84;b.rect(rx+25,ry,rw-50,70,t['soft'],9);b.text(rx+41,ry+10,name,15,weight=600,w=400);b.text(rx+41,ry+39,path,12,t['muted'],w=rw-90)
 b.text(rx+25,yy+451,'新环境的默认安装位置',15,weight=600,w=500);b.field(rx+25,yy+485,rw-185,'Java 安装文件夹',value='D:\\LXMCL\\runtime');b.btn(rx+rw-147,yy+485,122,'更改位置','folder')
 b.text(rx+25,yy+550,'缺失 Java 时会先显示版本、大小与路径，由你确认后下载。',12,t['muted'],w=rw-50)

def downloads(b):
 x,y,w=shell(b,'downloads');t=b.t;heading(b,'下载任务','关闭窗口后保留任务；重新打开可以继续。')
 yy=y+99;b.panel(x,yy,w,136);b.text(x+26,yy+24,'正在准备社团生存',21,weight=700,w=700);b.text(x+26,yy+65,'2 项任务  ·  已下载 15.6 / 24 MB  ·  3.2 MB/s',14,t['muted'],w=700)
 b.rect(x+26,yy+105,w-52,6,t['line'],3);b.rect(x+26,yy+105,(w-52)*.65,6,t['accent'],3);b.btn(x+w-170,yy+26,144,'全部暂停','pause-all')
 yy+=158;b.panel(x,yy,w,328)
 items=[('Sodium 0.6.5','Modrinth · 正在下载','65%','暂停'),('Fabric API 0.115.1','等待前置任务完成','排队中','取消'),('Minecraft 1.21.1','已完成 · 校验通过','已完成','打开游戏')]
 for i,(title,sub,status,btn) in enumerate(items):
  ry=yy+25+i*99
  if i:b.line(x+24,ry-12,w-48)
  b.icon('check' if i==2 else 'downloads',x+27,ry+12,24,t['accent']);b.text(x+71,ry+4,title,17,weight=600,w=w-360);b.text(x+71,ry+37,sub,13,t['muted'],w=w-360);b.text(x+w-265,ry+16,status,14,t['muted'],w=110);b.btn(x+w-139,ry+7,114,btn,'route:instances' if i==2 else 'task:'+str(i),h=38)
 b.panel(x,yy+352,w,82);b.text(x+24,yy+369,'下载慢或失败？',15,weight=600,w=400);b.text(x+24,yy+397,'测试连接、切换备用源，或检查系统代理。',13,t['muted'],w=w-220);b.btn(x+w-170,yy+372,145,'网络诊断','route:diagnostics')

def diagnostics(b):
 x,y,w=shell(b,'diagnostics');t=b.t;heading(b,'诊断与工具','先找到问题，再给出可执行的下一步。')
 yy=y+100;left=w*.57;b.panel(x,yy,left,405);b.text(x+24,yy+22,'网络连通性',20,weight=600,w=400);b.btn(x+left-143,yy+17,118,'重新测试','network',h=40)
 for i,(name,state) in enumerate([('民航认证与服务器','可连接 · 28 ms'),('BMCLAPI 镜像','可连接 · 42 ms'),('Modrinth','可连接 · 96 ms'),('CurseForge','连接超时 · 重试'),('常用服务与 API','8 / 9 项正常')]):
  ry=yy+86+i*59;b.text(x+24,ry,name,15,w=320);b.text(x+left-202,ry,state,13,('#E4B67D' if t['dark'] else '#9C5E24') if i==3 else t['muted'],w=190);b.line(x+24,ry+39,left-48)
 rx=x+left+20;rw=w-left-20;b.panel(rx,yy,rw,405);b.text(rx+24,yy+22,'游戏诊断',20,weight=600,w=330)
 for i,(title,sub) in enumerate([('启动失败的处理建议','查看原因、修复入口和完整日志'),('日志与诊断包','筛选日志 / 调整字体 / 导出诊断包'),('游戏完整性检查','定位缺失依赖并重新下载')]):
  ry=yy+87+i*99;b.text(rx+24,ry,title,16,weight=600,w=rw-48);b.text(rx+24,ry+30,sub,12,t['muted'],w=rw-48);b.add('hit',rx+20,ry-10,rw-40,82,action='diagnostic:'+title,label=title)
 b.panel(x,yy+429,w,128);b.text(x+24,yy+451,'系统工具',17,weight=600,w=200)
 for i,(label,act) in enumerate([('内存与性能','memory'),('网络测速 / IP','network'),('图形缓存','cache'),('高级日志工具','logs')]):b.btn(x+24+i*(w-48)/4,yy+492,(w-72)/4,label,act,h=40)

def join(b):
 home(b);t=b.t;b.rect(0,44,1440,856,'#112A43',0,opacity=.3)
 x,y,w=360,144,760;b.rect(x,y,w,612,t['card'],24 if b.theme!='classic' else 10,stroke=t['line'],shadow=True)
 b.text(x+32,y+25,'进入民航社团服',25,weight=700,w=650);b.btn(x+w-73,y+23,40,'×','route:servers',h=36)
 b.text(x+32,y+73,'确认以下配置，就可以和伙伴汇合。',14,t['muted'],w=660)
 for i,(num,txt) in enumerate([('1','账号'),('2','游戏环境'),('3','下载并进入')]):
  xx=x+33+i*233;b.chip(xx,y+118,num+'  '+txt,good=i<2,w=207)
 b.rect(x+32,y+173,w-64,76,t['soft'],12);b.icon('character',x+49,y+194,24,t['accent']);b.text(x+91,y+186,'LingXiao',17,weight=600,w=400);b.text(x+91,y+216,'民航社团账号 · 已登录',12,t['muted'],w=420)
 b.text(x+32,y+276,'推荐游戏配置',16,weight=600,w=600);b.text(x+32,y+311,'社团生存 · 1.21.1 / Fabric · Java 21',15,w=650)
 b.text(x+32,y+352,'需要更新 2 项资源，共 24 MB。',14,t['muted'],w=660)
 b.text(x+32,y+394,'安装位置',13,t['muted'],w=160);b.field(x+32,y+421,w-204,'选择安装位置',value='D:\\LXMCL\\instances\\社团生存');b.btn(x+w-155,y+421,123,'更改位置','folder')
 b.text(x+32,y+488,'下载完成后自动启动并进入此服务器',14,w=600);b.toggle(x+w-80,y+487,True,'toggle:auto-join')
 b.line(x+32,y+533,w-64);b.btn(x+32,y+551,125,'稍后再说','route:servers');b.btn(x+w-244,y+551,212,'下载并进入服务器','route:downloads',True,icon='play')

def search(b):
 home(b);t=b.t;b.rect(0,44,1440,856,'#112A43',0,opacity=.3);x,y,w=365,148,740;b.rect(x,y,w,588,t['card'],20,stroke=t['line'],shadow=True)
 b.icon('search',x+24,y+26,23,t['accent']);b.field(x+61,y+15,w-140,'搜索服务器、游戏、资源或设置',kind='global',value='社团');b.btn(x+w-64,y+19,40,'×','route:home',h=36)
 b.line(x+24,y+77,w-48)
 sections=[('服务器','民航社团服','在线 32 人 · 兼容：社团生存','route:join','准备进入'),('我的游戏','社团生存','1.21.1 · Fabric · 已安装','route:instances','打开游戏'),('账号','LingXiao · 民航社团','当前使用的社团认证账号','route:character','管理账号'),('帮助与设置','社团账号登录指南','如何登录、选择角色与进入服务器','help','查看指南')]
 for i,(section,title,sub,action,verb) in enumerate(sections):
  ry=y+94+i*111;b.text(x+24,ry,section,11,t['muted'],600,w=600)
  if i==0:b.rect(x+16,ry+24,w-32,71,t['soft'],10)
  b.text(x+30,ry+35,title,16,weight=600,w=w-220);b.text(x+30,ry+64,sub,12,t['muted'],w=w-220);b.text(x+w-126,ry+47,verb+'  ↵',12,t['accent'],w=110);b.add('hit',x+16,ry+24,w-32,71,action=action,label=title,searchResult=True)
 b.line(x+24,y+548,w-48);b.text(x+24,y+562,'↑ ↓ 选择      Enter 执行      Esc 关闭     ·     本地结果优先',11,t['muted'],w=650)

def conflict(b):
 resources(b);t=b.t;b.rect(0,44,1440,856,'#112A43',0,opacity=.3);x,y,w=350,153,780;b.rect(x,y,w,600,t['card'],20,stroke=t['line'],shadow=True)
 b.text(x+32,y+26,'安装前，确认兼容性',25,weight=700,w=650);b.btn(x+w-72,y+23,40,'×','route:resources',h=36)
 b.text(x+32,y+73,'将 Iris Shaders 安装到「社团生存」。',14,t['muted'],w=650)
 b.rect(x+32,y+119,w-64,85,'#3E3427' if t['dark'] else '#FFF2DE',12);b.text(x+50,y+136,'发现可能冲突的模组',17,'#E6BD84' if t['dark'] else '#8D571C',600,w=660);b.text(x+50,y+167,'当前游戏包含 OptiFine。建议先禁用，再继续安装。',13,'#D3BA9B' if t['dark'] else '#895F30',w=670)
 b.text(x+32,y+235,'本次变更',17,weight=600,w=600)
 for i,(a,c) in enumerate([('安装','Iris Shaders 1.8.0'),('补充依赖','Sodium 0.6.5'),('禁用冲突项','OptiFine · 保留文件，可恢复')]):
  ry=y+284+i*57;b.text(x+32,ry,a,13,t['muted'],w=180);b.text(x+188,ry,c,14,w=520);b.line(x+32,ry+39,w-64)
 b.text(x+32,y+469,'没有可确认的兼容版本时，保留资源详情并提供更换目标游戏入口。',12,t['muted'],w=710)
 b.btn(x+32,y+531,136,'取消','route:resources');b.btn(x+w-259,y+531,227,'禁用冲突项并安装','install',True)

BUILDERS=dict(home=home,servers=servers,instances=instances,resources=resources,character=character,settings=settings,downloads=downloads,diagnostics=diagnostics,join=join,search=search,conflict=conflict)

def svg_board(board):
 out=[f'<svg xmlns="http://www.w3.org/2000/svg" width="1440" height="900" viewBox="0 0 1440 900">']
 out.append('<defs><filter id="shadow" x="-20%" y="-30%" width="140%" height="160%"><feDropShadow dx="0" dy="12" stdDeviation="18" flood-color="#244863" flood-opacity=".12"/></filter></defs>')
 for n in board['nodes']:
  k=n['type'];x=n['x'];y=n['y'];w=n['w'];h=n['h'];op=n.get('opacity',1)
  if k=='rect':out.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{n.get("r",0)}" fill="{n["fill"]}" fill-opacity="{op}" stroke="{n.get("stroke") or "none"}"'+(' filter="url(#shadow)"' if n.get('shadow') else '')+'/>')
  elif k=='ellipse':
   color=n['fill'];gid='g'+str(len(out))
   if n.get('glow'):out.append(f'<defs><radialGradient id="{gid}"><stop stop-color="{color}"/><stop offset="1" stop-color="{color}" stop-opacity="0"/></radialGradient></defs>');color=f'url(#{gid})'
   out.append(f'<ellipse cx="{x+w/2}" cy="{y+h/2}" rx="{w/2}" ry="{h/2}" fill="{color}" opacity="{op}"/>')
  elif k in ('text','input'):
   text=n.get('text') or n.get('placeholder','');sz=n.get('size',14)
   for i,line in enumerate(text.split('\n')):out.append(f'<text x="{x}" y="{y+sz*1.13+i*sz*1.5}" font-family="Microsoft YaHei, Noto Sans CJK SC, sans-serif" font-size="{sz}" font-weight="{n.get("weight",400)}" fill="{n["color"]}">{html.escape(line)}</text>')
  elif k=='svg':
   s=n['svg'].replace('<svg ',f'<svg x="{x}" y="{y}" ',1)
   import re
   s=re.sub(r'width="\d+"',f'width="{w}"',s,count=1);s=re.sub(r'height="\d+"',f'height="{h}"',s,count=1);out.append(s)
 out.append('</svg>');return ''.join(out)

def main():
 boards=[]
 for theme in THEMES:
  for key,builder in BUILDERS.items():
   b=Board(theme,key);builder(b);data=b.pack();boards.append(data)
   (ROOT/'screens'/f'{theme}-{key}.svg').write_text(svg_board(data),encoding='utf-8')
 data=dict(version='1.0',product='LXMCL',themes=THEMES,boards=boards)
 (ROOT/'design-data.json').write_text(json.dumps(data,ensure_ascii=False),encoding='utf-8')
 (ROOT/'design-data.js').write_text('window.LXMCL_DATA='+json.dumps(data,ensure_ascii=False)+';',encoding='utf-8')
 (ROOT/'figma-plugin'/'data.js').write_text('const DESIGN_DATA='+json.dumps(data,ensure_ascii=False)+';\n',encoding='utf-8')
 print(f'{len(boards)} boards, {sum(len(b["nodes"]) for b in boards)} editable nodes generated')
if __name__=='__main__':main()
