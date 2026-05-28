"""Deploy committed BillFlow code to server (sync + restart + verify).

Auth: BF_PASS env var (no hardcoded password). Run from project root:
  BF_PASS=xxx python scripts/deploy.py                # deploy main
  BF_PASS=xxx python scripts/deploy.py --instance henna
  BF_PASS=xxx python scripts/deploy.py --instance thaisunsport

Instances:
  main         /home/bosscatdog/billflow          backend:8090  frontend:3010
  henna        /home/bosscatdog/billflow-henna    backend:8110  frontend:3030
  thaisunsport /home/bosscatdog/billflow-thaisun… backend:8100  frontend:3020
"""
import paramiko, sys, io, os, time, tarfile, argparse

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

HOST = "192.168.2.109"
USER = "bosscatdog"
PASS = os.environ.get("BF_PASS")
ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))

INSTANCES = {
    "main": {
        "remote":       "/home/bosscatdog/billflow",
        "backend_port": 8090,
        "container":    "billflow",
    },
    "henna": {
        "remote":       "/home/bosscatdog/billflow-henna",
        "backend_port": 8110,
        "container":    "billflow-henna",
    },
    "thaisunsport": {
        "remote":       "/home/bosscatdog/billflow-thaisunsport",
        "backend_port": 8100,
        "container":    "billflow-thaisunsport",
    },
}

FILES = ["frontend", "backend"]


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
    parser = argparse.ArgumentParser(description="Deploy BillFlow to server")
    parser.add_argument(
        "--instance", "-i",
        choices=list(INSTANCES.keys()),
        default="main",
        help="Which instance to deploy (default: main)",
    )
    args = parser.parse_args()

    if not PASS:
        print("ERROR: BF_PASS env var required", file=sys.stderr)
        sys.exit(1)

    inst = INSTANCES[args.instance]
    REMOTE       = inst["remote"]
    BACKEND_PORT = inst["backend_port"]
    CONTAINER    = inst["container"]

    print(f"Instance  : {args.instance}")
    print(f"Remote    : {REMOTE}")
    print(f"Health    : http://localhost:{BACKEND_PORT}/health")
    print(f"Container : {CONTAINER}-backend")

    print(f"\nConnecting to {USER}@{HOST}...")
    c = paramiko.SSHClient()
    c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    c.connect(HOST, username=USER, password=PASS, timeout=10,
              allow_agent=False, look_for_keys=False)
    print("✓ connected")

    # Build tar of changed files in memory.
    SKIP = {"node_modules", "dist", ".git", "backups", "artifacts", "__pycache__"}

    def tar_filter(info):
        parts = set(info.name.replace("\\", "/").split("/"))
        return None if parts & SKIP else info

    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tar:
        for f in FILES:
            local = os.path.join(ROOT, f)
            if not os.path.exists(local):
                print(f"⚠️  missing: {f}")
                continue
            tar.add(local, arcname=f.replace("\\", "/"), filter=tar_filter)
    buf.seek(0)
    print(f"\ntar: {len(buf.getvalue()):,} bytes")

    sftp = c.open_sftp()
    sftp.putfo(buf, "/tmp/billflow-deploy.tar.gz")
    sftp.close()
    print("✓ uploaded")

    # Remove legacy files that have been deleted locally (tar only adds, never removes)
    LEGACY_FILES = [
        "frontend/src/components/Layout.css",
        "frontend/src/components/StatsCard.css",
        "frontend/src/components/InsightCard.css",
        "frontend/src/components/LearningProgress.css",
        "frontend/src/components/BillStatusBadge.css",
        "frontend/src/components/BillTable.css",
        "frontend/src/pages/Login.css",
        "frontend/src/pages/Dashboard.css",
        "frontend/src/pages/Bills.css",
        "frontend/src/pages/Mappings.css",
        "frontend/src/pages/Logs.css",
        "frontend/src/pages/Import.css",
        "frontend/src/pages/ShopeeImport.css",
        "frontend/src/pages/Settings.css",
        "frontend/src/pages/CatalogSettings.css",
        "frontend/src/pages/BillDetail.tsx",
        "frontend/src/pages/BillDetail.css",
        "backend/internal/jobs/email_poller.go",
    ]
    rm_cmd = " && ".join(f"rm -f {REMOTE}/{p}" for p in LEGACY_FILES)
    run(c, rm_cmd, label="remove orphan legacy files")

    run(c, f"cd {REMOTE} && tar -xzf /tmp/billflow-deploy.tar.gz && rm /tmp/billflow-deploy.tar.gz",
        label="extract")
    run(c, f"mkdir -p {REMOTE}/backups {REMOTE}/artifacts", label="ensure dirs")

    has_backend  = any(f == "backend"  or f.startswith("backend/")  for f in FILES)
    has_frontend = any(f == "frontend" or f.startswith("frontend/") for f in FILES)
    services = []
    if has_backend:  services.append("backend")
    if has_frontend: services.append("frontend")
    if not services: services = ["backend", "frontend"]

    rc = run(c, f"cd {REMOTE} && docker compose build {' '.join(services)}",
             timeout=900, label=f"build {' '.join(services)}")
    if rc != 0:
        print(f"❌ build failed (exit {rc})")
        c.close(); sys.exit(1)

    run(c, f"cd {REMOTE} && docker compose up -d {' '.join(services)}", label="restart")

    print("\n... waiting 8s ...")
    time.sleep(8)

    run(c, "docker ps --format 'table {{.Names}}\t{{.Status}}' | grep -E 'billflow|NAME'",
        label="containers")

    si, so, se = c.exec_command(f"curl -s -m 5 http://localhost:{BACKEND_PORT}/health", timeout=15)
    health = so.read().decode("utf-8", errors="replace").strip()
    print(f"\n========== health ==========\n{health!r}")
    if '"status":"ok"' not in health:
        print("❌ health check failed — backend may be crashing. Recent fatal logs:")
        si, so, se = c.exec_command(
            f"docker logs {CONTAINER}-backend 2>&1 | grep -i 'fatal\\|panic\\|error.:\\|migration' | tail -20",
            timeout=15,
        )
        print(so.read().decode("utf-8", errors="replace"))
        c.close()
        sys.exit(2)

    run(c, f"docker logs {CONTAINER}-backend 2>&1 | grep -i 'migration applied' | tail -10",
        label="migrations applied")

    c.close()
    print(f"\n✅ deploy complete → {args.instance} ({REMOTE})")


if __name__ == "__main__":
    main()
