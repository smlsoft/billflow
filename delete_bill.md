cd c:/DEV/billflow && BF_PASS=boss123456 python -c "
import paramiko, os
c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('192.168.2.109', username='bosscatdog', password=os.environ['BF_PASS'])
def sh(cmd):
    _, so, _ = c.exec_command(cmd)
    print(so.read().decode())
sh('docker exec billflow-postgres psql -U billflow -d billflow -c \"DELETE FROM bill_artifacts; DELETE FROM bill_items; DELETE FROM audit_logs WHERE target_id IS NOT NULL; DELETE FROM bills;\"')
sh('docker exec billflow-backend sh -c \"rm -rf /app/artifacts/*\"')
sh('docker exec billflow-postgres psql -U billflow -d billflow -c \"SELECT (SELECT COUNT(*) FROM bills) bills, (SELECT COUNT(*) FROM bill_artifacts) artifacts;\"')
"
