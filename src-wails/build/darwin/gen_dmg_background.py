#!/usr/bin/env python3
"""DevPortal DMG background: 540x380 at 2x for the Finder window."""

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

WIDTH, HEIGHT = 540, 380
OUT = Path(__file__).with_name("dmg-background.png")

BG = (243, 240, 228)
BG_MID = (239, 233, 216)
LINE = (210, 204, 182)
PHOSPHOR = (61, 107, 16)
PHOSPHOR_DIM = (90, 138, 34)
MUTED = (106, 101, 86)
INK = (27, 25, 19)


def load_font(size: int):
    for path in (
        "/System/Library/Fonts/Hiragino Sans GB.ttc",
        "/System/Library/Fonts/ヒラギノ角ゴシック W4.ttc",
        "/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
        "/Library/Fonts/Arial Unicode.ttf",
        "/System/Library/Fonts/Helvetica.ttc",
    ):
        try:
            return ImageFont.truetype(path, size=size, index=0)
        except OSError:
            continue
    return ImageFont.load_default()


def main() -> None:
    img = Image.new("RGB", (WIDTH, HEIGHT), BG)
    draw = ImageDraw.Draw(img)

    for y in range(HEIGHT):
        t = y / (HEIGHT - 1)
        r = int(BG[0] * (1 - t) + BG_MID[0] * t)
        g = int(BG[1] * (1 - t) + BG_MID[1] * t)
        b = int(BG[2] * (1 - t) + BG_MID[2] * t)
        draw.line([(0, y), (WIDTH, y)], fill=(r, g, b))

    step = 24
    for x in range(0, WIDTH, step):
        draw.line([(x, 0), (x, HEIGHT)], fill=LINE)
    for y in range(0, HEIGHT, step):
        draw.line([(0, y), (WIDTH, y)], fill=LINE)

    # Arrow between the app icon (left) and Applications (right).
    y = 165
    x1, x2 = 215, 325
    draw.line([(x1, y), (x2, y)], fill=PHOSPHOR, width=5)
    draw.polygon(
        [(x2 + 4, y), (x2 - 18, y - 14), (x2 - 18, y + 14)],
        fill=PHOSPHOR,
    )
    draw.ellipse([x1 - 7, y - 7, x1 + 7, y + 7], fill=PHOSPHOR_DIM)

    kicker = load_font(14)
    title = load_font(22)
    hint = load_font(13)
    draw.text((32, 24), "LOCAL LAUNCH BOARD", fill=PHOSPHOR, font=kicker)
    draw.text((32, 44), "DEVPORTAL", fill=INK, font=title)
    draw.text(
        (WIDTH // 2, 340),
        "Applications フォルダへドラッグしてインストール",
        fill=MUTED,
        font=hint,
        anchor="mm",
    )

    img.save(OUT, "PNG")
    print(f"wrote {OUT} ({WIDTH}x{HEIGHT})")


if __name__ == "__main__":
    main()
