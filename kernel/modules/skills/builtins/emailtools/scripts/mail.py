#!/usr/bin/env python3
"""email-tools helper — send (SMTP) and read (IMAP) email. Standard library only.

Usage:  python mail.py '<json-spec>'   (or pipe the JSON on stdin)
Ops:
  send {smtp:{host,port,user,pass,starttls,ssl}, from, to, cc?, subject, body,
        html?, attachments?[paths]}                -> {to}
  list {imap:{host,port,user,pass}, folder, search, limit}
                                                    -> {messages:[{uid,from,subject,date}], count}
  read {imap:{...}, folder, uid}                    -> {from,subject,date,text}

Use an app password (not the main account password). The password is never echoed
back. A fast start, not a cage: for OAuth2/calendar invites, use smtplib/imaplib.
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


def _as_list(v):
    if v is None:
        return []
    return v if isinstance(v, list) else [v]


def op_send(spec):
    import os
    import smtplib
    from email.message import EmailMessage

    smtp = spec.get("smtp") or {}
    host = smtp.get("host")
    if not host:
        raise ValueError("send needs smtp.host")
    to = _as_list(spec.get("to"))
    if not to:
        raise ValueError("send needs to")
    cc = _as_list(spec.get("cc"))

    msg = EmailMessage()
    msg["From"] = spec.get("from") or smtp.get("user", "")
    msg["To"] = ", ".join(to)
    if cc:
        msg["Cc"] = ", ".join(cc)
    msg["Subject"] = spec.get("subject", "")
    msg.set_content(spec.get("body", ""))
    if spec.get("html"):
        msg.add_alternative(spec["html"], subtype="html")

    for path in _as_list(spec.get("attachments")):
        with open(path, "rb") as fh:
            data = fh.read()
        msg.add_attachment(
            data, maintype="application", subtype="octet-stream", filename=os.path.basename(path)
        )

    port = int(smtp.get("port", 587))
    user, pw = smtp.get("user"), smtp.get("pass")
    if smtp.get("ssl"):
        with smtplib.SMTP_SSL(host, port) as s:
            if user:
                s.login(user, pw)
            s.send_message(msg, to_addrs=to + cc)
    else:
        with smtplib.SMTP(host, port) as s:
            if smtp.get("starttls", True):
                s.starttls()
            if user:
                s.login(user, pw)
            s.send_message(msg, to_addrs=to + cc)
    return {"to": to, "cc": cc}


def _imap_login(spec):
    import imaplib

    im = spec.get("imap") or {}
    host = im.get("host")
    if not host:
        raise ValueError("needs imap.host")
    m = imaplib.IMAP4_SSL(host, int(im.get("port", 993)))
    m.login(im.get("user"), im.get("pass"))
    return m


def _decode(s):
    from email.header import decode_header

    if not s:
        return ""
    parts = []
    for text, enc in decode_header(s):
        if isinstance(text, bytes):
            parts.append(text.decode(enc or "utf-8", errors="replace"))
        else:
            parts.append(text)
    return "".join(parts)


def op_list(spec):
    m = _imap_login(spec)
    try:
        m.select(spec.get("folder", "INBOX"), readonly=True)
        typ, data = m.uid("search", None, spec.get("search", "ALL"))
        uids = data[0].split() if data and data[0] else []
        limit = int(spec.get("limit", 20))
        uids = uids[-limit:][::-1]  # newest first
        out = []
        for uid in uids:
            typ, md = m.uid("fetch", uid, "(BODY.PEEK[HEADER.FIELDS (FROM SUBJECT DATE)])")
            raw = md[0][1] if md and md[0] else b""
            import email

            hdr = email.message_from_bytes(raw)
            out.append(
                {
                    "uid": uid.decode(),
                    "from": _decode(hdr.get("From")),
                    "subject": _decode(hdr.get("Subject")),
                    "date": hdr.get("Date", ""),
                }
            )
        return {"messages": out, "count": len(out)}
    finally:
        try:
            m.logout()
        except Exception:  # noqa: BLE001
            pass


def op_read(spec):
    import email

    uid = spec.get("uid")
    if not uid:
        raise ValueError("read needs uid")
    m = _imap_login(spec)
    try:
        m.select(spec.get("folder", "INBOX"), readonly=True)
        typ, md = m.uid("fetch", str(uid), "(RFC822)")
        raw = md[0][1] if md and md[0] else b""
        msg = email.message_from_bytes(raw)
        text = ""
        if msg.is_multipart():
            for part in msg.walk():
                if part.get_content_type() == "text/plain" and "attachment" not in str(
                    part.get("Content-Disposition", "")
                ):
                    text = part.get_payload(decode=True).decode(
                        part.get_content_charset() or "utf-8", errors="replace"
                    )
                    break
        else:
            text = msg.get_payload(decode=True).decode(
                msg.get_content_charset() or "utf-8", errors="replace"
            )
        mc = int(spec.get("max_chars", 8000))
        return {
            "from": _decode(msg.get("From")),
            "subject": _decode(msg.get("Subject")),
            "date": msg.get("Date", ""),
            "text": text[:mc] + (" …" if len(text) > mc else ""),
        }
    finally:
        try:
            m.logout()
        except Exception:  # noqa: BLE001
            pass


OPS = {"send": op_send, "list": op_list, "read": op_read}


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
