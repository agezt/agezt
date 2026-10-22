#!/usr/bin/env python3
"""ssh-remote helper — run commands and move files on a remote host over SSH.

Usage:  python ssh.py '<json-spec>'   (or pipe the JSON on stdin)
Spec:   { "op":"run|put|get|ls", "host":"...", "port":22, "user":"...",
          "key_path":"~/.ssh/id_ed25519" | "password":"...", "key_pass":"...",
          "cmd":"...", "local":"...", "remote":"...", "timeout":30 }
Output: one JSON object on stdout.

Prefer key auth; a password passed in is never echoed back. A fast start, not a
cage: for interactive shells, port forwarding, or jump hosts, use paramiko.
"""
import json
import os
import sys


def read_spec():
    if len(sys.argv) > 1 and sys.argv[1].strip():
        return json.loads(sys.argv[1])
    data = sys.stdin.read()
    if data.strip():
        return json.loads(data)
    raise ValueError("no spec: pass a JSON spec as argv[1] or on stdin")


def connect(spec):
    import paramiko

    host = spec.get("host")
    user = spec.get("user")
    if not host or not user:
        raise ValueError("needs host and user")
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    kwargs = {
        "hostname": host,
        "port": int(spec.get("port", 22)),
        "username": user,
        "timeout": float(spec.get("timeout", 30)),
    }
    key_path = spec.get("key_path")
    if key_path:
        kwargs["key_filename"] = os.path.expanduser(key_path)
        if spec.get("key_pass"):
            kwargs["passphrase"] = spec["key_pass"]
    elif spec.get("password"):
        kwargs["password"] = spec["password"]
    client.connect(**kwargs)
    return client


def op_run(client, spec):
    cmd = spec.get("cmd")
    if not cmd:
        raise ValueError("run needs cmd")
    stdin, stdout, stderr = client.exec_command(cmd, timeout=float(spec.get("timeout", 30)))
    out = stdout.read().decode("utf-8", errors="replace")
    err = stderr.read().decode("utf-8", errors="replace")
    code = stdout.channel.recv_exit_status()
    mc = int(spec.get("max_chars", 8000))
    return {
        "exit_code": code,
        "stdout": out[:mc] + (" …" if len(out) > mc else ""),
        "stderr": err[:mc] + (" …" if len(err) > mc else ""),
    }


def op_put(client, spec):
    local, remote = spec.get("local"), spec.get("remote")
    if not local or not remote:
        raise ValueError("put needs local and remote")
    sftp = client.open_sftp()
    try:
        sftp.put(os.path.expanduser(local), remote)
    finally:
        sftp.close()
    return {"local": local, "remote": remote}


def op_get(client, spec):
    local, remote = spec.get("local"), spec.get("remote")
    if not local or not remote:
        raise ValueError("get needs local and remote")
    sftp = client.open_sftp()
    try:
        sftp.get(remote, os.path.expanduser(local))
    finally:
        sftp.close()
    return {"remote": remote, "local": local}


def op_ls(client, spec):
    remote = spec.get("remote", ".")
    sftp = client.open_sftp()
    try:
        import stat as statmod

        entries = []
        for attr in sftp.listdir_attr(remote):
            name = attr.filename
            if statmod.S_ISDIR(attr.st_mode):
                name += "/"
            entries.append(name)
        entries.sort()
    finally:
        sftp.close()
    return {"remote": remote, "entries": entries}


OPS = {"run": op_run, "put": op_put, "get": op_get, "ls": op_ls}


def run(spec):
    op = spec.get("op")
    if op not in OPS:
        raise ValueError("spec.op must be one of: " + ", ".join(OPS))
    client = connect(spec)
    try:
        result = OPS[op](client, spec)
    finally:
        client.close()
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
