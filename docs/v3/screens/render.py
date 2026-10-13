import re, sys, glob
from PIL import Image, ImageDraw, ImageFont
from fontTools.ttLib import TTFont

REG = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
BLD = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"
FALL = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
SIZE = 15
f_reg, f_bld, f_fall = (ImageFont.truetype(p, SIZE) for p in (REG, BLD, FALL))
cmap = TTFont(REG).getBestCmap()
CW = round(f_reg.getlength("M"))
CH = round(SIZE * 1.45)
BG = (22, 22, 30)
DEFAULT_FG = (230, 230, 239)
SGR = re.compile(r"\x1b\[([0-9;]*)m")

def parse(line):
    cells, fg, bg, bold = [], None, None, False
    pos = 0
    for m in SGR.finditer(line):
        for ch in line[pos:m.start()]:
            cells.append((ch, fg, bg, bold))
        pos = m.end()
        codes = [int(c) if c else 0 for c in m.group(1).split(";")] or [0]
        i = 0
        while i < len(codes):
            c = codes[i]
            if c == 0: fg, bg, bold = None, None, False
            elif c == 1: bold = True
            elif c == 22: bold = False
            elif c == 39: fg = None
            elif c == 49: bg = None
            elif c in (38, 48) and i + 4 < len(codes) and codes[i+1] == 2:
                rgb = tuple(codes[i+2:i+5])
                if c == 38: fg = rgb
                else: bg = rgb
                i += 4
            i += 1
    for ch in line[pos:]:
        cells.append((ch, fg, bg, bold))
    return cells

# box-drawing characters drawn as lines: (left, right, up, down, thick, rounded)
BOX = {
    "─": (1,1,0,0,0,0), "━": (1,1,0,0,1,0), "│": (0,0,1,1,0,0), "┃": (0,0,1,1,1,0),
    "╭": (0,1,0,1,0,1), "╮": (1,0,0,1,0,1), "╰": (0,1,1,0,0,1), "╯": (1,0,1,0,0,1),
    "┏": (0,1,0,1,1,0), "┓": (1,0,0,1,1,0), "┗": (0,1,1,0,1,0), "┛": (1,0,1,0,1,0),
    "┌": (0,1,0,1,0,0), "┐": (1,0,0,1,0,0), "└": (0,1,1,0,0,0), "┘": (1,0,1,0,0,0),
}

def draw_box(d, ch, x, y, col):
    l, r, u, dn, thick, rounded = BOX[ch]
    wdt = 3 if thick else 1
    cx, cy = x + CW // 2, y + CH // 2
    if rounded:
        rad = CW // 2
        if r and dn:
            d.arc([cx, cy, cx + 2 * rad, cy + 2 * rad], 180, 270, fill=col, width=wdt)
            d.line([cx + rad, cy, x + CW, cy], fill=col, width=wdt); d.line([cx, cy + rad, cx, y + CH], fill=col, width=wdt)
        elif l and dn:
            d.arc([cx - 2 * rad, cy, cx, cy + 2 * rad], 270, 360, fill=col, width=wdt)
            d.line([x, cy, cx - rad, cy], fill=col, width=wdt); d.line([cx, cy + rad, cx, y + CH], fill=col, width=wdt)
        elif r and u:
            d.arc([cx, cy - 2 * rad, cx + 2 * rad, cy], 90, 180, fill=col, width=wdt)
            d.line([cx + rad, cy, x + CW, cy], fill=col, width=wdt); d.line([cx, y, cx, cy - rad], fill=col, width=wdt)
        elif l and u:
            d.arc([cx - 2 * rad, cy - 2 * rad, cx, cy], 0, 90, fill=col, width=wdt)
            d.line([x, cy, cx - rad, cy], fill=col, width=wdt); d.line([cx, y, cx, cy - rad], fill=col, width=wdt)
        return
    if l: d.line([x, cy, cx, cy], fill=col, width=wdt)
    if r: d.line([cx, cy, x + CW, cy], fill=col, width=wdt)
    if u: d.line([cx, y, cx, cy], fill=col, width=wdt)
    if dn: d.line([cx, cy, cx, y + CH], fill=col, width=wdt)

def render(path, out, title):
    lines = [parse(l) for l in open(path, encoding="utf-8").read().split("\n")]
    cols = max(len(l) for l in lines)
    pad, top = 22, 38
    w, h = cols * CW + 2 * pad, len(lines) * CH + top + pad
    img = Image.new("RGB", (w + 40, h + 40), (236, 236, 242))
    win = Image.new("RGB", (w, h), BG)
    d = ImageDraw.Draw(win)
    d.rectangle([0, 0, w, 28], fill=(38, 38, 50))
    for i, c in enumerate([(255, 95, 86), (255, 189, 46), (39, 201, 63)]):
        d.ellipse([14 + i * 20, 9, 26 + i * 20, 21], fill=c)
    tw = f_fall.getlength(title)
    d.text(((w - tw) / 2, 6), title, font=f_fall, fill=(170, 170, 185))
    for r, line in enumerate(lines):
        y = top + r * CH
        for c, (ch, fg, bg, bold) in enumerate(line):
            x = pad + c * CW
            if bg: d.rectangle([x, y, x + CW, y + CH], fill=bg)
            if ch == " ": continue
            font = (f_bld if bold else f_reg) if ord(ch) in cmap else f_fall
            if ch in BOX:
                draw_box(d, ch, x, y, fg or DEFAULT_FG)
                continue
            d.text((x, y + 2), ch, font=font, fill=fg or DEFAULT_FG)
    mask = Image.new("L", (w, h), 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, w - 1, h - 1], 10, fill=255)
    shadow = Image.new("RGB", (w, h), (200, 200, 210))
    img.paste(shadow, (24, 26), mask)
    img.paste(win, (20, 20), mask)
    img.save(out)
    print(out, img.size)

titles = {
    "01-default": "huddle — default: Activity takes all the space",
    "02-companion": "huddle — companion open: it takes half of Activity",
    "03-menu": "huddle — all three open: 1/4 Activity, 1/2 Companion, 1/4 menu",
    "04-menu-filter": "huddle — /s filters the menu",
    "05-paused": "huddle — paused: /resume appears, /pause disappears",
    "06-status": "huddle — /status shown in the Activity slot until esc",
    "07-activity-select": "huddle — selecting the 22:52 line (same Activity slot)",
    "08-log": "huddle — raw log at 22:52 in the Activity slot; the companion knows what you see",
    "09-settings": "huddle — /settings, read-only",
    "10-decision": "huddle — a decision waits; you keep typing in the text field",
    "10b-decision-focused": "huddle — after /decision: focus on the card, arrows choose, enter confirms",
    "11-wizard-pm": "huddle setup — PM agent: CLI, model and effort",
    "12-wizard-companion": "huddle setup — companion: its own model and effort",
    "13-wizard-autonomy": "huddle setup — autonomy level",
    "14-start-menu": "huddle — start menu",
    "15-pick-milestone": "huddle — running an existing milestone",
}
for p in sorted(glob.glob("*.ans")):
    name = p[:-4]
    render(p, name + ".png", titles[name])
