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
    ".env.example",
    "backend/cmd/server/main.go",
    "backend/internal/config/config.go",
    "backend/internal/database/migrations/004_shopee_shipped.sql",
    "backend/internal/handlers/bills.go",
    "backend/internal/handlers/email.go",
    "backend/internal/handlers/shipped_email.go",
    "backend/internal/repository/bill_repo.go",
    "backend/internal/services/email/imap.go",
    "backend/internal/services/sml/purchaseorder_client.go",
    "frontend/src/hooks/useBills.ts",
    "frontend/src/pages/BillDetail.tsx",
    "frontend/src/pages/Bills.tsx",
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

    # Update IMAP_FILTER_SUBJECT and Shipped SML config in .env if missing
    run(c, f"cd {REMOTE} && grep -q 'ถูกจัดส่งแล้ว' .env || sed -i 's/^IMAP_FILTER_SUBJECT=.*/&,ถูกจัดส่งแล้ว/' .env",
        label="add ถูกจัดส่งแล้ว to IMAP_FILTER_SUBJECT")
    run(c, f"cd {REMOTE} && grep -q '^SHIPPED_SML_DOC_FORMAT=' .env || echo 'SHIPPED_SML_DOC_FORMAT=PO' >> .env",
        label="add SHIPPED_SML_DOC_FORMAT")
    run(c, f"cd {REMOTE} && grep -q '^SHIPPED_SML_CUST_CODE=' .env || echo 'SHIPPED_SML_CUST_CODE=' >> .env",
        label="add SHIPPED_SML_CUST_CODE")

    # Rebuild backend + frontend
    rc = run(c, f"cd {REMOTE} && docker compose build backend frontend",
             timeout=900, label="build backend + frontend")
    if rc != 0:
        print(f"❌ build failed (exit {rc})")
        c.close(); sys.exit(1)

    run(c, f"cd {REMOTE} && docker compose up -d backend frontend", label="restart")

    print("\n... waiting 8s ...")
    time.sleep(8)

    # Verify
    run(c, "docker ps --format 'table {{.Names}}\t{{.Status}}' | grep -E 'billflow|NAME'", label="containers")
    run(c, "curl -s http://localhost:8090/health", label="health")
    run(c, "docker logs billflow-backend 2>&1 | grep -i 'migration applied'", label="migrations")
    run(c, "ls -la ~/billflow/backups/", label="backups dir")

    c.close()
    print("\n✅ deploy complete")


if __name__ == "__main__":
    main()
