#!/usr/bin/env python3
"""Trace the FoxxyCode fox mark from its raster master into a flat SVG glyph.

    python scripts/brand/trace.py \
        docs/assets/brand/foxxycode-mark-1024.png \
        docs/assets/foxxycode-logo-glyph.svg

The master is a three-colour flat drawing on a transparent background: a navy
outline, an orange head and white markings. Every pixel is snapped to one of
those three, and each colour is traced separately with potrace.

The three layers are painted back to front as *cumulative* masks - navy carries
the whole silhouette, orange carries orange plus white, white carries only
itself - so a lower layer always fills the pixels a higher one rounds away.
Tracing the layers as disjoint regions instead leaves hairline transparent
seams between them, because potrace smooths each mask on its own and two
smoothed curves never meet exactly.

Requires Pillow, numpy and potracer (`pip install potracer`); it is a one-shot
authoring step, not part of the build, and its output is committed.
"""

from __future__ import annotations

import sys
from pathlib import Path

import numpy as np
import potrace
from PIL import Image

# The palette of the master, measured from the artwork itself.
NAVY = (0x20, 0x31, 0x3F)
ORANGE = (0xFC, 0x7E, 0x05)
WHITE = (0xFF, 0xFF, 0xFF)

# Alpha below this counts as background rather than as a faint edge pixel.
ALPHA_FLOOR = 128

# potrace knobs. turdsize drops speckles left by the alpha cut, alphamax is how
# eagerly corners become curves, opttolerance is how far an optimised curve may
# drift from the traced one. Raising opttolerance is what keeps the file small.
TURDSIZE = 6
ALPHAMAX = 1.0
OPTTOLERANCE = 0.6

# Coordinates are emitted in the master's own pixel grid, rounded to this many
# decimals. The glyph therefore has viewBox "0 0 <size> <size>" and callers
# scale it.
PRECISION = 1


def classify(image: Image.Image) -> dict[str, np.ndarray]:
    """Snap every pixel to navy, orange, white or background."""
    rgba = np.asarray(image.convert("RGBA"), dtype=np.int32)
    rgb = rgba[:, :, :3]
    opaque = rgba[:, :, 3] >= ALPHA_FLOOR

    palette = np.array([NAVY, ORANGE, WHITE], dtype=np.int32)
    # Squared distance from each pixel to each palette entry.
    distances = ((rgb[:, :, None, :] - palette[None, None, :, :]) ** 2).sum(axis=3)
    nearest = distances.argmin(axis=2)

    return {
        "navy": opaque & (nearest == 0),
        "orange": opaque & (nearest == 1),
        "white": opaque & (nearest == 2),
    }


def trace(mask: np.ndarray) -> str:
    """Trace one boolean mask into a single SVG path data string."""
    # potracer treats values BELOW blacklevel as ink, so the mask goes in inverted.
    bitmap = potrace.Bitmap(~mask)
    path = bitmap.trace(
        turdsize=TURDSIZE,
        alphamax=ALPHAMAX,
        opttolerance=OPTTOLERANCE,
    )

    def n(value: float) -> str:
        text = f"{value:.{PRECISION}f}".rstrip("0").rstrip(".")
        return text if text not in ("", "-0") else "0"

    parts: list[str] = []
    for curve in path:
        start = curve.start_point
        parts.append(f"M{n(start.x)} {n(start.y)}")
        for segment in curve:
            end = segment.end_point
            if segment.is_corner:
                c = segment.c
                parts.append(f"L{n(c.x)} {n(c.y)}L{n(end.x)} {n(end.y)}")
            else:
                c1, c2 = segment.c1, segment.c2
                parts.append(
                    f"C{n(c1.x)} {n(c1.y)} {n(c2.x)} {n(c2.y)} {n(end.x)} {n(end.y)}"
                )
        parts.append("Z")
    return "".join(parts)


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    source, target = Path(argv[0]), Path(argv[1])

    image = Image.open(source)
    width, height = image.size
    if width != height:
        print(f"trace: expected a square master, got {width}x{height}", file=sys.stderr)
        return 1

    masks = classify(image)
    silhouette = masks["navy"] | masks["orange"] | masks["white"]
    # Cumulative, back to front: see the module docstring.
    # The ids let the generator compose variants: "outline" alone is the
    # silhouette, "outline" plus "markings" under evenodd is the monochrome
    # icon with the muzzle knocked out, all three are the full-colour mark.
    layers = [
        ("outline", "#20313f", silhouette),
        ("fur", "#fc7e05", masks["orange"] | masks["white"]),
        ("markings", "#ffffff", masks["white"]),
    ]

    paths = []
    for name, fill, mask in layers:
        data = trace(mask)
        if data:
            paths.append(f'  <path id="{name}" fill="{fill}" fill-rule="evenodd" d="{data}"/>')

    # A viewBox tight around the drawing, so callers can place the glyph from
    # the viewBox alone instead of carrying the master's transparent margin.
    rows, cols = np.nonzero(silhouette)
    x0, x1 = int(cols.min()), int(cols.max()) + 1
    y0, y1 = int(rows.min()), int(rows.max()) + 1

    svg = "\n".join(
        [
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{x0} {y0} {x1 - x0} {y1 - y0}"'
            ' role="img" aria-label="FoxxyCode">',
            *paths,
            "</svg>",
            "",
        ]
    )
    target.write_text(svg, encoding="utf-8", newline="\n")
    print(f"trace: wrote {target} ({len(svg)} bytes, {len(paths)} layers)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
