#!/usr/bin/env python3
"""Draw every icon the app ships from the brand's stacked symbol
(launch/…/brand/poiesis-assets/svg/app-icon.svg gives the geometry) in the green palette:
signal yellow on a Forest rounded square.

Writes, next to this script: icon.png (Mac, 1024 canvas with the 824 square macOS expects),
Poiesis.icns (via iconutil), tray-template.png (menu bar, black with alpha, macOS tints it),
tray.png (Windows/Linux tray), icon.ico (Windows). Needs Pillow; iconutil is on every Mac.

    python3 assets/make-icons.py
"""
import os
import shutil
import subprocess
import tempfile

from PIL import Image, ImageDraw

HERE = os.path.dirname(os.path.abspath(__file__))
# The green palette of the 22 September colour study (brand/palette-study/README.md):
# Forest is the ground, Sage "remains the softer colour for the logo and large graphic surfaces".
FOREST = (25, 59, 45, 255)     # #193B2D
SAGE = (198, 205, 174, 255)    # #C6CDAE, the study's logo colour on large surfaces
SIGNAL = (239, 255, 0, 255)    # #EFFF00, the study's signal yellow; chosen for the icon's mark
GROUND, MARK = FOREST, SIGNAL
BLACK = (0, 0, 0, 255)

# The symbol, as drawn in the brand's SVG (a 250.07 × 200 box): two stacked forward blocks.
SYMBOL = [
    (83.356, 0.0), (250.068, 0.0), (250.068, 56.0), (166.712, 100.0), (250.068, 100.0),
    (250.068, 156.0), (166.712, 200.0), (0.0, 200.0), (0.0, 144.0), (83.356, 100.0),
    (0.0, 100.0), (0.0, 44.0),
]
SYMBOL_W, SYMBOL_H = 250.068, 200.0
SYMBOL_SHARE = 0.72             # the symbol's width as a share of the square's side (app-icon.svg has 0.45; larger by request)
CORNER = 112.64 / 512.0         # corner radius as a share of the side
SS = 8                          # supersampling: drawn at eight times the size, then scaled down, for crisp edges


def symbol_points(cx, cy, width):
    scale = width / SYMBOL_W
    w, h = SYMBOL_W * scale, SYMBOL_H * scale
    return [(cx - w / 2 + x * scale, cy - h / 2 + y * scale) for x, y in SYMBOL]


def draw_square(canvas, side, offset, ground=GROUND, mark=MARK):
    """A rounded square of `side` px at `offset` on a transparent canvas, the mark centred."""
    big = Image.new("RGBA", (canvas * SS, canvas * SS), (0, 0, 0, 0))
    d = ImageDraw.Draw(big)
    o, s = offset * SS, side * SS
    d.rounded_rectangle([o, o, o + s - 1, o + s - 1], radius=CORNER * s, fill=ground)
    d.polygon(symbol_points(o + s / 2, o + s / 2, SYMBOL_SHARE * s), fill=mark)
    return big.resize((canvas, canvas), Image.LANCZOS)


def draw_symbol(w, h, mark_h, colour):
    """The bare symbol, `mark_h` px tall, centred on a transparent w × h canvas."""
    big = Image.new("RGBA", (w * SS, h * SS), (0, 0, 0, 0))
    d = ImageDraw.Draw(big)
    width = mark_h * SYMBOL_W / SYMBOL_H
    d.polygon(symbol_points(w * SS / 2, h * SS / 2, width * SS), fill=colour)
    return big.resize((w, h), Image.LANCZOS)


def write_icns(png1024, out):
    sizes = [16, 32, 128, 256, 512]
    tmp = tempfile.mkdtemp()
    iconset = os.path.join(tmp, "Poiesis.iconset")
    os.mkdir(iconset)
    for s in sizes:
        png1024.resize((s, s), Image.LANCZOS).save(os.path.join(iconset, f"icon_{s}x{s}.png"))
        png1024.resize((s * 2, s * 2), Image.LANCZOS).save(os.path.join(iconset, f"icon_{s}x{s}@2x.png"))
    subprocess.run(["iconutil", "-c", "icns", iconset, "-o", out], check=True)
    shutil.rmtree(tmp)


def main():
    # Mac: a 1024 canvas, the square 824 wide with transparent margins, as macOS lays out app icons.
    mac = draw_square(1024, 824, 100)
    mac.save(os.path.join(HERE, "icon.png"))
    write_icns(mac, os.path.join(HERE, "Poiesis.icns"))
    # Menu bar: a black-with-alpha template that macOS tints; the app shows it at 17 × 17 points,
    # so the canvas is square (34 px for Retina) and the symbol sits 13 points tall inside it.
    draw_symbol(34, 34, 26, BLACK).save(os.path.join(HERE, "tray-template.png"))
    # Windows and Linux trays, and the Windows icon: the square fills the canvas.
    full = draw_square(64, 64, 0)
    full.save(os.path.join(HERE, "tray.png"))
    draw_square(256, 256, 0).save(
        os.path.join(HERE, "icon.ico"),
        sizes=[(256, 256), (128, 128), (64, 64), (48, 48), (32, 32), (16, 16)],
    )
    print("icon.png, Poiesis.icns, tray-template.png, tray.png, icon.ico written")


if __name__ == "__main__":
    main()
