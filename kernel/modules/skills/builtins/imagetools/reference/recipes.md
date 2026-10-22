# image-tools recipes

The helper (`scripts/img.py`) covers info/resize/convert/crop/thumb/rotate/
grayscale/annotate. For anything else, write Pillow directly in `code_exec`.

## Shrink a screenshot for sharing

```sh
python scripts/img.py '{"op":"resize","path":"shot.png","w":1200,"out":"shot-sm.png"}'
```

## Batch convert a folder to JPEG thumbnails

```python
import glob, os
from PIL import Image
os.makedirs("thumbs", exist_ok=True)
for p in glob.glob("imgs/*.png"):
    im = Image.open(p).convert("RGB"); im.thumbnail((256, 256))
    im.save(f"thumbs/{os.path.splitext(os.path.basename(p))[0]}.jpg", quality=82)
```

## OCR — read text from an image

Pillow doesn't OCR. Install tesseract via the computer-use skill, then:
```sh
pip install pytesseract
python -c "import pytesseract; from PIL import Image; print(pytesseract.image_to_string(Image.open('shot.png')))"
```

## Strip EXIF (privacy) before sharing a photo

```python
from PIL import Image
im = Image.open("photo.jpg")
data = list(im.getdata()); clean = Image.new(im.mode, im.size); clean.putdata(data)
clean.save("photo-clean.jpg", quality=90)   # no EXIF copied
```

## Compose / watermark

```python
from PIL import Image
base = Image.open("base.png").convert("RGBA")
logo = Image.open("logo.png").convert("RGBA")
base.alpha_composite(logo, (base.width - logo.width - 12, base.height - logo.height - 12))
base.convert("RGB").save("branded.jpg", quality=88)
```

## The visual pipeline

image-tools composes with the other built-ins:
- **browser-use** takes screenshots → crop/resize them here.
- **pdf-tools** renders PDF pages to PNG → thumbnail/annotate them here.
- **data-analysis** saves chart PNGs → resize for a report here.
- Save any result with the `artifacts` tool so it appears in the Files view.
