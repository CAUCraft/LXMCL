"""Rebuild the original, code-drawn SVG assets for the pixel glass experiment.

No supplied artwork is modified. All icons use an integer 16 x 16 grid.
"""
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "ui"
ICONS = {
    "home": "M7 1h2v2h2v2h2v2h2v2h-2v6H9v-4H7v4H3V9H1V7h2V5h2V3h2z M5 8v5h1V9h4v4h1V8H9V6H7v2z",
    "games": "M4 3h8v2h2v2h1v6h-4v-2H5v2H1V7h1V5h2z M4 6v2H2v1h2v2h1V9h2V8H5V6z M10 7v2h2V7z",
    "server": "M2 1h12v4H2z M3 2v2h2V2z M2 6h12v4H2z M3 7v2h2V7z M2 11h12v4H2z M3 12v2h2v-2z",
    "resources": "M1 1h6v6H1z M3 3v2h2V3z M9 1h6v6H9z M11 3v2h2V3z M1 9h6v6H1z M3 11v2h2v-2z M9 9h6v6H9z M11 11v2h2v-2z",
    "download": "M7 1h2v7h3v2h-2v2H6v-2H4V8h3z M1 11h2v3h10v-3h2v5H1z",
    "account": "M5 1h6v2h2v6h-2v2H5V9H3V3h2z M5 4v4h6V4z M3 12h10v2h2v2H1v-2h2z",
    "settings": "M6 0h4v2h2v2h2v2h2v4h-2v2h-2v2h-2v2H6v-2H4v-2H2v-2H0V6h2V4h2V2h2z M6 5v1H5v4h1v1h4v-1h1V6h-1V5z",
    "diagnostics": "M9 0h2v4h3V1h2v6h-2v2h-4v2H8v2H6v2H2v-4h2V9h2V7h2V2h1z M3 12v1h1v-1z",
    "help": "M4 0h8v2h2v2h2v8h-2v2h-2v2H4v-2H2v-2H0V4h2V2h2z M5 4v2h2V5h2v2H7v3h2V9h2V4z M7 12v2h2v-2z",
    "play": "M3 1h2v2h3v2h3v2h3v2h-3v2H8v2H5v2H3z",
    "search": "M3 1h6v2h2v6H9v2h2v2h2v2h-2v-2H9v-2H3V9H1V3h2z M3 3v6h6V3z",
    "moon": "M6 0h4v2H6v7h2v2h6V9h2v3h-2v2h-2v2H5v-2H3v-2H1V5h2V3h3z",
    "sun": "M7 0h2v3H7z M7 13h2v3H7z M0 7h3v2H0z M13 7h3v2h-3z M2 2h2v2H2z M12 2h2v2h-2z M2 12h2v2H2z M12 12h2v2h-2z M6 4h4v2h2v4h-2v2H6v-2H4V6h2z",
}

icon_dir = ROOT / "icons" / "pixel"
icon_dir.mkdir(parents=True, exist_ok=True)
for name, path in ICONS.items():
    (icon_dir / f"{name}.svg").write_text(
        f'<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="#40558c" fill-rule="evenodd" d="{path}"/></svg>',
        encoding="utf-8",
    )

for dark in (False, True):
    mode = "dark" if dark else "light"
    base, blue, lilac, mint = ("#111c34", "#3a5484", "#544a81", "#236779") if dark else ("#dce8f7", "#9ec7ee", "#c4baf1", "#a6dcd8")
    bg = f'''<svg xmlns="http://www.w3.org/2000/svg" width="1180" height="700" viewBox="0 0 1180 700">
<defs>
 <radialGradient id="b"><stop stop-color="{blue}"/><stop offset="1" stop-color="{blue}" stop-opacity="0"/></radialGradient>
 <radialGradient id="l"><stop stop-color="{lilac}"/><stop offset="1" stop-color="{lilac}" stop-opacity="0"/></radialGradient>
 <radialGradient id="m"><stop stop-color="{mint}"/><stop offset="1" stop-color="{mint}" stop-opacity="0"/></radialGradient>
</defs>
<path fill="{base}" d="M0 0h1180v700H0z"/>
<ellipse cx="450" cy="70" rx="650" ry="500" fill="url(#b)"/>
<ellipse cx="1120" cy="330" rx="680" ry="540" fill="url(#l)"/>
<ellipse cx="430" cy="750" rx="650" ry="430" fill="url(#m)"/>
<path d="M0 540h80v-40h80v80h80v-40h80v80h80v-40h80v80h80v40H0z" fill="{blue}" opacity=".16"/>
<path d="M940 700V580h80V460h80V380h80v320z" fill="{lilac}" opacity=".20"/>
</svg>'''
    (ROOT / "assets" / f"glass-wallpaper-{mode}.svg").write_text(bg, encoding="utf-8")
    fill1, fill2, edge, terrain = ("#4a658c", "#283b63", "#d9edff", "#91b9dc") if dark else ("#ffffff", "#d0e9ff", "#ffffff", "#609bc4")
    hero = f'''<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="360" viewBox="0 0 1200 360">
<defs>
 <linearGradient id="glass" x2=".95" y2="1"><stop stop-color="{fill1}" stop-opacity=".69"/><stop offset=".53" stop-color="{fill2}" stop-opacity=".50"/><stop offset="1" stop-color="{fill1}" stop-opacity=".32"/></linearGradient>
 <linearGradient id="rim" x2=".6" y2="1"><stop stop-color="{edge}" stop-opacity=".95"/><stop offset=".5" stop-color="{edge}" stop-opacity=".16"/><stop offset="1" stop-color="{edge}" stop-opacity=".65"/></linearGradient>
 <linearGradient id="light"><stop stop-color="{edge}" stop-opacity="0"/><stop offset=".6" stop-color="{edge}" stop-opacity=".30"/><stop offset="1" stop-color="{edge}" stop-opacity="0"/></linearGradient>
 <clipPath id="clip"><rect x="2" y="2" width="1196" height="350" rx="28"/></clipPath>
</defs>
<rect x="4" y="8" width="1192" height="350" rx="28" fill="#294378" opacity=".10"/>
<rect x="1" y="1" width="1198" height="350" rx="28" fill="url(#glass)" stroke="url(#rim)" stroke-width="2"/>
<g clip-path="url(#clip)">
 <path d="M780 352v-36h48v-36h48v-36h48v-36h48v36h48v-72h48v-36h48v72h48v-36h48v180z" fill="{terrain}" opacity=".10"/>
 <path d="M780 316h48v-36h48v-36h48v-36h48m48 36v-72h48v-36h48v72h48v-36h48" fill="none" stroke="{edge}" stroke-opacity=".35" stroke-width="2"/>
 <path d="M600 -50h120L450 400H330z" fill="url(#light)" opacity=".6"/>
 <path d="M1070 48h18v18h-18z M1096 74h8v8h-8z M796 96h12v12h-12z M756 228h8v8h-8z" fill="{edge}" opacity=".55"/>
 <path d="M818 60h32v32h-32z M1120 240h40v40h-40z" fill="{edge}" fill-opacity=".13" stroke="{edge}" stroke-opacity=".40"/>
</g>
<path d="M32 4h1136" stroke="{edge}" stroke-opacity=".6" stroke-width="2"/>
<rect x="5" y="5" width="1190" height="342" rx="24" fill="none" stroke="{edge}" stroke-opacity=".13"/>
</svg>'''
    (ROOT / "assets" / f"glass-hero-{mode}.svg").write_text(hero, encoding="utf-8")

print(f"Created {len(ICONS)} pixel icons and 4 glass material SVGs")
