"""Deploy committed BillFlow code to server (sync + restart + verify).

Auth: BF_PASS env var (no hardcoded password). Run from project root:
  BF_PASS=xxx python scripts/deploy.py
"""
import paramiko, sys, io, os, time, tarfile

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

HOST = "192.168.2.109"
USER = "bosscatdog"
PASS = os.environ.get("BF_PASS")
ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
REMOTE = "/home/bosscatdog/billflow"

if not PASS:
    print("ERROR: BF_PASS env var required", file=sys.stderr)
    sys.exit(1)

# Files to sync (matches the commit)
FILES = [
    "backend/internal/handlers/bills.go",
    "backend/internal/handlers/shopee_import.go",
]


def run(client, cmd, label=None, timeout=900):
    if label:
        print(f"\n========== {label} ==========")
    print(f"$ {cmd}")
    si, so, se = client.exec_command(cmd, timeout=timeout, get_pty=True)
    out = so.read().decode("utf-8", errors="replace").rstrip()
    err = se.read().decode("utf-8", errors="replace").rstrip()
    if out:
        print(out)
    if err:
        print(f"[stderr] {err}")
    return so.channel.recv_exit_status()


def main():
    print(f"Connecting to {USER}@{HOST}...")
    c = paramiko.SSHClient()
    c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(HOST, username=USER, password=PASS, timeout=10,
              allow_agent=False, look_for_keys=False)
    print("✓ connected")

    # Build tar of changed files in memory
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tar:
        for f in FILES:
            local = os.path.join(ROOT, f)
            if not os.path.exists(local):
                print(f"⚠️  missing: {f}")
                continue
            tar.add(local, arcname=f.replace("\\", "/"))
    buf.seek(0)
    print(f"\ntar: {len(buf.getvalue()):,} bytes")

    # Upload + extract
    sftp = c.open_sftp()
    sftp.putfo(buf, "/tmp/billflow-deploy.tar.gz")
    sftp.close()
    print("✓ uploaded")

    run(c, f"cd {REMOTE} && tar -xzf /tmp/billflow-deploy.tar.gz && rm /tmp/billflow-deploy.tar.gz",
        label="extract")
    run(c, f"mkdir -p {REMOTE}/backups", label="ensure backups dir")

    # Rebuild only what's needed based on file paths
    has_backend_change = any(f.startswith("backend/") for f in FILES)
    has_frontend_change = any(f.startswith("frontend/") for f in FILES)
    services = []
    if has_backend_change: services.append("backend")
    if has_frontend_change: services.append("frontend")
    if not services:
        services = ["backend", "frontend"]

    rc = run(c, f"cd {REMOTE} && docker compose build {' '.join(services)}",
             timeout=900, label=f"build {' '.join(services)}")
    if rc != 0:
        print(f"❌ build failed (exit {rc})")
        c.close(); sys.exit(1)

    run(c, f"cd {REMOTE} && docker compose up -d {' '.join(services)}", label="restart")

    print("\n... waiting 8s ...")
    time.sleep(8)

    # Verify
    run(c, "docker ps --format 'table {{.Names}}\t{{.Status}}' | grep -E 'billflow|NAME'", label="containers")

    # Health check — must return non-empty body containing "ok", else fail loud.
    si, so, se = c.exec_command("curl -s -m 5 http://localhost:8090/health", timeout=15)
    health = so.read().decode("utf-8", errors="replace").strip()
    print(f"\n========== health ==========\n{health!r}")
    if '"status":"ok"' not in health:
        print("❌ health check failed — backend may be crashing. Recent fatal logs:")
        si, so, se = c.exec_command(
            "docker logs billflow-backend 2>&1 | grep -i 'fatal\\|panic\\|error.:\\|migration' | tail -20",
            timeout=15,
        )
        print(so.read().decode("utf-8", errors="replace"))
        c.close()
        sys.exit(2)

    run(c, "docker logs billflow-backend 2>&1 | grep -i 'migration applied' | tail -10", label="migrations applied")

    c.close()
    print("\n✅ deploy complete")


if __name__ == "__main__":
    main()
