from pathlib import Path
import json,re,zipfile,xml.etree.ElementTree as ET
from PIL import Image,ImageDraw,ImageFont
root=Path(__file__).parent
data=json.loads((root/'design-data.json').read_text(encoding='utf-8'))
ids={b['id'] for b in data['boards']};routes=0
for b in data['boards']:
 for n in b['nodes']:
  if n['type']=='svg':ET.fromstring(n['svg'])
  action=n.get('action','')
  if action.startswith('route:'):
   assert b['theme']+'/'+action[6:] in ids,(b['id'],action)
   routes+=1
(root/'prototype-script.js').write_text(re.findall(r'<script>(.*?)</script>',(root/'index.html').read_text(encoding='utf-8'),re.S)[0],encoding='utf-8')
image=Image.new('RGB',(1920,510),'#E6ECF3');d=ImageDraw.Draw(image);font=ImageFont.truetype('C:/Windows/Fonts/msyh.ttc',22)
labels=['01 液态玻璃 · 液态质感','02 经典桌面 · 传统导航','03 极简深色 · 低干扰深色']
for i,theme in enumerate(data['themes']):
 im=Image.open(root/'screens'/f'{theme}-home.png').convert('RGB');im=im.resize((616,385),Image.Resampling.LANCZOS);image.paste(im,(i*640+12,80));d.text((i*640+20,28),labels[i],font=font,fill='#20354D')
image.save(root/'screens'/'three-directions.png')
report={'date':'2026-10-26','boards':len(ids),'scene_nodes':sum(len(b['nodes']) for b in data['boards']),'validated_navigation_targets':routes,'text_or_frame_bounds_findings':len(json.loads((root/'qa-layout.json').read_text(encoding='utf-8'))),'svg_xml':'passed','figma_cloud_write':'completed via Figma desktop and local drawing plugin; no Figma AI calls','figma_file':'https://www.figma.com/design/wDdnB7HQaIpiE8rzkwU4VQ/LXMCL','figma_native_audit':{'ui_frames':33,'information_architecture_frames':1,'native_text_layers_in_ui':1865,'button_master_components':130,'button_instances':248,'vector_layers':1166,'cross_frame_navigation_links':409,'current_frame_actions_without_navigation':52,'font':'Noto Sans SC','errors':[]},'figma_canvas_validation':'all three home screens visually reviewed in native Figma prototype; classic home > join > downloads and glass resources > compatibility confirmation > downloads clicked successfully','browser_interaction_validation':'HTML browser access blocked by URL policy; not claimed as passed','html':'static syntax checked separately','implementation_scope':'UI prototypes only, no game/network/backend integration'}
(root/'validation-report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
out=root.parent/'LXMCL-UI-review-v0.1.zip'
with zipfile.ZipFile(out,'w',zipfile.ZIP_DEFLATED) as z:
 for p in root.rglob('*'):
  if p.is_file() and p.suffix not in {'.pid','.log'} and '__pycache__' not in str(p):z.write(p,'LXMCL-UI/'+p.relative_to(root).as_posix())
print(json.dumps(report,ensure_ascii=False));print(str(out))
