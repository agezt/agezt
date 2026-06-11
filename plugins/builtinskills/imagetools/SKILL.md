---
name: image-tools
description: Work with images — read their size/format/EXIF, resize, convert between formats, crop, make thumbnails, rotate, grayscale, and stamp text on them — when a task hands you a photo, screenshot, or chart instead of clean data
triggers: [image, photo, screenshot, png, jpg, jpeg, resize, thumbnail, crop, convert, exif, watermark, rotate]
tools: [code_exec, shell, artifacts]
---

# image-tools — manipulate images programmatically

When a task involves an image — a screenshot to crop, a photo to resize, a batch
to convert, a chart to thumbnail — don't hand-wave it. Load it with Pillow and
transform it. This skill runs through the `code_exec` tool (python).

## One-time setup

Run `scripts/setup.sh` once (via code_exec or shell): it installs `Pillow`. Use
`skill op=files image-tools` to find the bundle directory.

## The helper

`scripts/img.py` takes a JSON spec with an `op` and prints JSON. Ops:

```sh
# Metadata (size, mode, format, EXIF):
python scripts/img.py '{"op":"info","path":"photo.jpg"}'

# Resize (keeps aspect if only one of w/h given):
python scripts/img.py '{"op":"resize","path":"photo.jpg","w":800,"out":"small.jpg"}'

# Convert format (inferred from out extension):
python scripts/img.py '{"op":"convert","path":"photo.png","out":"photo.jpg","quality":85}'

# Crop a box [left,top,right,bottom]:
python scripts/img.py '{"op":"crop","path":"shot.png","box":[0,0,400,300],"out":"crop.png"}'

# Thumbnail (longest side <= max):
python scripts/img.py '{"op":"thumb","path":"big.png","max":256,"out":"thumb.png"}'

# Rotate / grayscale / stamp text:
python scripts/img.py '{"op":"rotate","path":"a.png","deg":90,"out":"r.png"}'
python scripts/img.py '{"op":"grayscale","path":"a.png","out":"g.png"}'
python scripts/img.py '{"op":"annotate","path":"a.png","text":"DRAFT","xy":[12,12],"out":"a2.png"}'
```

### Spec fields
- `op` (required) — `info` | `resize` | `convert` | `crop` | `thumb` | `rotate` |
  `grayscale` | `annotate`.
- `path` (required) — the source image.
- `out` — output path (format inferred from its extension). Defaults vary by op.
- `w` / `h` (resize), `box` (crop), `max` (thumb), `deg` (rotate),
  `text` + `xy` + `size` (annotate), `quality` (convert/resize JPEG).

### Output (JSON on stdout)
```
{ "ok": true, "op": "resize", "out": "small.jpg", "w": 800, "h": 600 }
{ "ok": true, "op": "info", "w": 1920, "h": 1080, "mode": "RGB",
  "format": "JPEG", "exif": {...} }
```

## OCR (text in an image)

Pillow doesn't read text. To pull text out of a screenshot/scan, install
`tesseract` via the computer-use skill, then `pytesseract.image_to_string(img)`.
For PDFs, render pages first with the pdf-tools / pymupdf path.

## Going further

The helper is a fast start, not a cage — for compositing, filters, drawing, or
EXIF stripping, write Pillow directly in `code_exec`. Save any produced image with
the `artifacts` tool so it shows in the Files view. Pairs with browser-use
(screenshots), pdf-tools (rendered pages), and data-analysis (chart PNGs). See
`reference/recipes.md`.
