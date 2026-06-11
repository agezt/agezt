#!/usr/bin/env python3
"""image-tools helper — inspect and transform images with Pillow.

Usage:  python img.py '<json-spec>'   (or pipe the JSON on stdin)
Ops:
  info      {path}                                  -> {w,h,mode,format,exif}
  resize    {path, w?, h?, out, quality?}           -> {out,w,h}   (keeps aspect if one of w/h)
  convert   {path, out, quality?}                   -> {out,format}
  crop      {path, box:[l,t,r,b], out}              -> {out,w,h}
  thumb     {path, max, out}                        -> {out,w,h}   (longest side <= max)
  rotate    {path, deg, out}                        -> {out}
  grayscale {path, out}                             -> {out}
  annotate  {path, text, xy?, size?, out}           -> {out}       (stamp text on the image)

A fast start, not a cage: for compositing/filters/drawing, use Pillow directly
in code_exec. Output format is inferred from `out`'s extension.
"""
import json
import sys


def read_spec():
    if len(sys.argv) > 1 and sys.argv[1].strip():
        return json.loads(sys.argv[1])
    data = sys.stdin.read()
    if data.strip():
        return json.loads(data)
    raise ValueError("no spec: pass a JSON spec as argv[1] or on stdin")


def _open(spec):
    from PIL import Image

    if not spec.get("path"):
        raise ValueError("spec.path is required")
    return Image.open(spec["path"])


def _save(img, out, quality=None):
    params = {}
    if quality is not None:
        params["quality"] = int(quality)
    img.save(out, **params)


def op_info(spec):
    img = _open(spec)
    exif = {}
    try:
        raw = img.getexif()
        from PIL.ExifTags import TAGS

        exif = {TAGS.get(k, str(k)): str(v) for k, v in raw.items()}
    except Exception:  # noqa: BLE001 — no/again-unreadable EXIF is fine
        exif = {}
    return {"w": img.width, "h": img.height, "mode": img.mode, "format": img.format, "exif": exif}


def op_resize(spec):
    img = _open(spec)
    w, h = spec.get("w"), spec.get("h")
    if not w and not h:
        raise ValueError("resize needs w and/or h")
    if w and not h:
        h = round(img.height * (int(w) / img.width))
    elif h and not w:
        w = round(img.width * (int(h) / img.height))
    img = img.resize((int(w), int(h)))
    out = spec.get("out", "resized.png")
    _save(img, out, spec.get("quality"))
    return {"out": out, "w": img.width, "h": img.height}


def op_convert(spec):
    img = _open(spec)
    out = spec.get("out")
    if not out:
        raise ValueError("convert needs out (its extension picks the format)")
    if out.lower().endswith((".jpg", ".jpeg")) and img.mode in ("RGBA", "P"):
        img = img.convert("RGB")  # JPEG has no alpha
    _save(img, out, spec.get("quality"))
    return {"out": out, "format": (img.format or out.rsplit(".", 1)[-1].upper())}


def op_crop(spec):
    box = spec.get("box")
    if not box or len(box) != 4:
        raise ValueError("crop needs box:[left,top,right,bottom]")
    img = _open(spec).crop(tuple(int(x) for x in box))
    out = spec.get("out", "crop.png")
    _save(img, out)
    return {"out": out, "w": img.width, "h": img.height}


def op_thumb(spec):
    img = _open(spec)
    m = int(spec.get("max", 256))
    img.thumbnail((m, m))
    out = spec.get("out", "thumb.png")
    _save(img, out)
    return {"out": out, "w": img.width, "h": img.height}


def op_rotate(spec):
    img = _open(spec).rotate(float(spec.get("deg", 90)), expand=True)
    out = spec.get("out", "rotated.png")
    _save(img, out)
    return {"out": out, "w": img.width, "h": img.height}


def op_grayscale(spec):
    img = _open(spec).convert("L")
    out = spec.get("out", "gray.png")
    _save(img, out)
    return {"out": out}


def op_annotate(spec):
    from PIL import ImageDraw

    text = spec.get("text")
    if not text:
        raise ValueError("annotate needs text")
    img = _open(spec).convert("RGBA")
    draw = ImageDraw.Draw(img)
    xy = spec.get("xy", [10, 10])
    draw.text((int(xy[0]), int(xy[1])), str(text), fill=(255, 0, 0, 255))
    out = spec.get("out", "annotated.png")
    if out.lower().endswith((".jpg", ".jpeg")):
        img = img.convert("RGB")
    _save(img, out)
    return {"out": out}


OPS = {
    "info": op_info,
    "resize": op_resize,
    "convert": op_convert,
    "crop": op_crop,
    "thumb": op_thumb,
    "rotate": op_rotate,
    "grayscale": op_grayscale,
    "annotate": op_annotate,
}


def run(spec):
    op = spec.get("op")
    if op not in OPS:
        raise ValueError("spec.op must be one of: " + ", ".join(OPS))
    result = OPS[op](spec)
    result.update({"ok": True, "op": op})
    return result


def main():
    try:
        print(json.dumps(run(read_spec()), default=str))
    except Exception as e:  # noqa: BLE001
        print(json.dumps({"ok": False, "error": str(e)}))
        sys.exit(1)


if __name__ == "__main__":
    main()
