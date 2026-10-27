from pathlib import Path
import json, math, sys
import cairosvg
from PIL import Image,ImageDraw,ImageFont
root=Path(__file__).parent
data=json.loads((root/'design-data.json').read_text(encoding='utf-8'))
issues=[]
for b in data['boards']:
 for n in b['nodes']:
  if n['type']=='text':
   f=ImageFont.truetype('C:/Windows/Fonts/msyhbd.ttc' if n.get('weight',400)>=600 else 'C:/Windows/Fonts/msyh.ttc',n['size'])
   measure=max(f.getlength(s) for s in n['text'].split('\n'))
   if measure>n['w']+4:issues.append((b['id'],n['text'],round(measure),n['w']))
  if n['type'] not in ['ellipse','svg'] and (n['x']<0 or n['y']<0 or n['x']+n['w']>1445 or n['y']+n['h']>901):issues.append((b['id'],'BOUNDS '+n.get('text',n.get('label',n['type'])),n['x'],n['y'],n['w'],n['h']))
print('Text / bounds findings:',len(issues))
for issue in issues:print(issue)
(root/'qa-layout.json').write_text(json.dumps(issues,ensure_ascii=False,indent=2),encoding='utf-8')
for b in data['boards']:
 path=root/'screens'/b['id'].replace('/','-')
 cairosvg.svg2png(url=str(path.with_suffix('.svg')),write_to=str(path.with_suffix('.png')))
font=ImageFont.truetype('C:/Windows/Fonts/msyh.ttc',20)
for theme in data['themes']:
 boards=[b for b in data['boards'] if b['theme']==theme]
 contact=Image.new('RGB',(1440,math.ceil(len(boards)/3)*340),'#DFE5ED');d=ImageDraw.Draw(contact)
 for i,b in enumerate(boards):
  im=Image.open(root/'screens'/f'{theme}-{b["key"]}.png').convert('RGB');im.thumbnail((456,285));x=12+(i%3)*480;y=12+(i//3)*340;contact.paste(im,(x,y));d.text((x,y+290),b['name'],font=font,fill='#22364D')
 contact.save(root/'screens'/f'{theme}-overview.jpg',quality=92)
# Self-contained plugin bundle.
(root/'figma-plugin'/'code.js').write_text((root/'figma-plugin'/'data.js').read_text(encoding='utf-8')+(root/'figma-plugin'/'runtime.js').read_text(encoding='utf-8'),encoding='utf-8')
print('Rendered 33 PNGs, 3 contact sheets and bundled Figma plugin.')
