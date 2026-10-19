#!/usr/bin/env python3
"""Render the Miro-style dashboard mapping flowchart.

Usage:
    python3 scripts/gen-dashboard-mapping.py docs/dashboard-mapping.png
    SS=6 OUT=2 python3 scripts/gen-dashboard-mapping.py docs/dashboard-mapping@2x.png

SS  = internal supersampling factor (antialiasing quality)
OUT = output scale multiplier (1 = 1600x900, 2 = 3200x1800)

Requires Pillow and DejaVu fonts (both present on the project CI image).
"""

import sys
from PIL import Image, ImageDraw, ImageFont

import os
SS = float(os.environ.get("SS", "3"))
OUT = float(os.environ.get("OUT", "1"))
W, H = 1600, 900

BG = (244, 244, 244)
INK = (43, 43, 43)
LINE = (74, 74, 74)

DARK = (46, 46, 46)
YELLOW = (247, 208, 69)
PURPLE = (143, 140, 230)
CREAM = (240, 234, 216)
GREEN = (182, 232, 120)
RED = (239, 139, 134)

FONT_DIR = "/usr/share/fonts/truetype/dejavu/"
F_REG = FONT_DIR + "DejaVuSans.ttf"
F_BOLD = FONT_DIR + "DejaVuSans-Bold.ttf"


def px(v):
    return int(round(v * SS))


def font(path, size):
    return ImageFont.truetype(path, px(size))


_TITLE = font(F_BOLD, 24)
_SUB = font(F_REG, 13)
_L1 = font(F_BOLD, 13.5)
_L2 = font(F_REG, 10.8)
_L1S = font(F_BOLD, 12.5)
_L2S = font(F_REG, 10.2)
_LEG = font(F_REG, 11)

img = Image.new("RGB", (px(W), px(H)), BG)
d = ImageDraw.Draw(img)


def text(x, y, s, fnt, fill, anchor="mm"):
    d.text((px(x), px(y)), s, font=fnt, fill=fill, anchor=anchor)


def fit(s, path, size, max_w):
    """Shrink font size until the string fits max_w px."""
    while size > 6:
        f = font(path, size)
        if d.textlength(s, font=f) / SS <= max_w:
            return f
        size -= 0.4
    return font(path, 6)


def box(x0, y0, w, h, fill, l1, l2, dark_text_ok=True):
    d.rectangle([px(x0), px(y0), px(x0 + w), px(y0 + h)], fill=fill, outline=INK, width=px(1.4))
    cx = x0 + w / 2
    cy = y0 + h / 2
    if dark_text_ok:
        c1, c2 = (24, 24, 24), (60, 60, 60)
    else:
        c1, c2 = (255, 255, 255), (206, 206, 206)
    pad = w - 22
    if l2:
        text(cx, cy - 8, l1, fit(l1, F_BOLD, 13.5, pad), c1)
        text(cx, cy + 9, l2, fit(l2, F_REG, 10.8, pad), c2)
    else:
        text(cx, cy, l1, fit(l1, F_BOLD, 13.5, pad), c1)
    return (x0, y0, x0 + w, y0 + h)


def vline(x, y_from, y_to):
    d.line([px(x), px(y_from), px(x), px(y_to)], fill=LINE, width=px(1.5), joint="curve")


def hline(x_from, x_to, y, arrow=True):
    end = x_to - 1
    d.line([px(x_from), px(y), px(end), px(y)], fill=LINE, width=px(1.5))
    if arrow:
        a = 5.5
        d.polygon(
            [(px(end - a), px(y - 3.4)), (px(end), px(y)), (px(end - a), px(y + 3.4))],
            fill=LINE,
        )


# ---------------------------------------------------------------- header
text(70, 62, "ChitChat Bot — Dashboard Mapping", _TITLE, INK, anchor="lm")
text(70, 92, "Wails v2 desktop app · data path, bridge surface and panel map", _SUB, (128, 128, 128), anchor="lm")

# ---------------------------------------------------------------- geometry
CX = 430  # shared centre line

DARK_X, DARK_W, DARK_H = 110, 210, 68
dark_centres = [280, 380, 480, 580]

YEL_X, YEL_Y, YEL_W, YEL_H = 380, 392, 190, 76

PUR_X, PUR_W, PUR_H = 655, 245, 76
purple_centres = [365, 495]

CRM_X, CRM_Y, CRM_W, CRM_H = 935, 390, 215, 80

GRN_X, GRN_W, GRN_H = 1195, 215, 64
green_centres = [242, 336, 430, 524, 618]

RED_X, RED_Y, RED_W, RED_H = 1440, 393, 145, 74

MERGE1 = 345
SPLIT1 = 605
MERGE2 = 915
SPLIT2 = 1168
MERGE3 = 1422

# ---------------------------------------------------------------- left column
box(DARK_X, dark_centres[0] - DARK_H / 2, DARK_W, DARK_H, DARK, "Badger settings", "uiStore · 4 keys", False)
box(DARK_X, dark_centres[1] - DARK_H / 2, DARK_W, DARK_H, DARK, "bot_state.json", "counts · seen · step", False)
box(DARK_X, dark_centres[2] - DARK_H / 2, DARK_W, DARK_H, DARK, "login_info*.json", "1 file = 1 worker", False)
box(DARK_X, dark_centres[3] - DARK_H / 2, DARK_W, DARK_H, DARK, "logs.txt · eva_chat", "diagnostics · TXT pack", False)

for cy in dark_centres:
    hline(DARK_X + DARK_W, MERGE1, cy, arrow=False)
vline(MERGE1, dark_centres[0], dark_centres[-1])
hline(MERGE1, YEL_X, CX, arrow=True)

# ---------------------------------------------------------------- bridge
box(YEL_X, YEL_Y, YEL_W, YEL_H, YELLOW, "Wails Bridge", "app.go · 17 methods")

hline(YEL_X + YEL_W, SPLIT1, CX, arrow=False)
vline(SPLIT1, purple_centres[0], purple_centres[1])
hline(SPLIT1, PUR_X, purple_centres[0])
hline(SPLIT1, PUR_X, purple_centres[1])

# ---------------------------------------------------------------- read / write
box(PUR_X, purple_centres[0] - PUR_H / 2, PUR_W, PUR_H, PURPLE, "Read APIs", "GetInitialState · ChatSnapshot")
box(PUR_X, purple_centres[1] - PUR_H / 2, PUR_W, PUR_H, PURPLE, "Write & action APIs", "Save* · Start/Stop · RunBrowser · Test*")

for cy in purple_centres:
    hline(PUR_X + PUR_W, MERGE2, cy, arrow=False)
vline(MERGE2, purple_centres[0], purple_centres[-1])
hline(MERGE2, CRM_X, CX)

# ---------------------------------------------------------------- core
box(CRM_X, CRM_Y, CRM_W, CRM_H, CREAM, "internal core", "workerplan · bot · browser · proxy")

hline(CRM_X + CRM_W, SPLIT2, CX, arrow=False)
vline(SPLIT2, green_centres[0], green_centres[-1])
for cy in green_centres:
    hline(SPLIT2, GRN_X, cy)

# ---------------------------------------------------------------- panels
box(GRN_X, green_centres[0] - GRN_H / 2, GRN_W, GRN_H, GREEN, "Home", "8 stats · live chat")
box(GRN_X, green_centres[1] - GRN_H / 2, GRN_W, GRN_H, GREEN, "Message", "single · folder · eva flow")
box(GRN_X, green_centres[2] - GRN_H / 2, GRN_W, GRN_H, GREEN, "Browser", "rod · chromedp · show mode")
box(GRN_X, green_centres[3] - GRN_H / 2, GRN_W, GRN_H, GREEN, "Proxy", "parse · test · rotation")
box(GRN_X, green_centres[4] - GRN_H / 2, GRN_W, GRN_H, GREEN, "Setting", "5 timing pairs")

for cy in green_centres:
    hline(GRN_X + GRN_W, MERGE3, cy, arrow=False)
vline(MERGE3, green_centres[0], green_centres[-1])
hline(MERGE3, RED_X, CX)

# ---------------------------------------------------------------- end
box(RED_X, RED_Y, RED_W, RED_H, RED, "Operator", "dashboard user")

# ---------------------------------------------------------------- legend
legend_y = 800
items = [
    (DARK, "source of truth"),
    (YELLOW, "bridge"),
    (PURPLE, "API surface"),
    (CREAM, "core package"),
    (GREEN, "dashboard panel"),
    (RED, "end user"),
]
x = 110
for color, label in items:
    d.rectangle([px(x), px(legend_y - 9), px(x + 26), px(legend_y + 9)], fill=color, outline=INK, width=px(1.1))
    text(x + 36, legend_y, label, _LEG, (70, 70, 70), anchor="lm")
    x += 36 + d.textlength(label, font=_LEG) / SS + 40

text(70, 852, "Read path: GetInitialState / GetLiveChatSnapshot   ·   Write path: Save* / StartBot / RunBrowser   ·   3 s stats poll · 1 s transcript poll",
     _LEG, (138, 138, 138), anchor="lm")

out = img.resize((int(W * OUT), int(H * OUT)), Image.LANCZOS)
dest = sys.argv[1] if len(sys.argv) > 1 else "dashboard-mapping.png"
out.save(dest, "PNG", optimize=True)
print("wrote", dest, out.size)
