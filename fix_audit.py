import re

with open('internal/server/server.go', 'r', encoding='utf-8') as f:
    c = f.read()

c = c.replace('s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "export_bundle", user.Email, "Exported T4 Signed Bundle")', 's.logAuditHelper(user.Email, "export_bundle", "bundle", "system", "Exported T4 Signed Bundle")')
c = c.replace('s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "configure_webhook", user.Email, "Configured webhook for "+payload.Event)', 's.logAuditHelper(user.Email, "configure_webhook", "webhook", "system", "Configured webhook for "+payload.Event)')
c = c.replace('s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "public_vote", identity, "Voted for "+projectID)', 's.logAuditHelper(identity, "public_vote", "project", projectID, "Voted for "+projectID)')
c = c.replace('s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "add_comment", identity, "Commented on "+projectID)', 's.logAuditHelper(identity, "add_comment", "project", projectID, "Commented on "+projectID)')
c = c.replace('s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "update_project", user.Email, "Updated project "+projectID)', 's.logAuditHelper(user.Email, "update_project", "project", projectID, "Updated project "+projectID)')
c = c.replace('s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "delete_project", user.Email, "Deleted project "+projectID)', 's.logAuditHelper(user.Email, "delete_project", "project", projectID, "Deleted project "+projectID)')
c = c.replace('s.db.Exec("INSERT INTO audit_logs (action, identity, details) VALUES (?, ?, ?)", "bulk_import", user.Email, "Imported projects")', 's.logAuditHelper(user.Email, "bulk_import", "project", "all", "Imported projects")')

with open('internal/server/server.go', 'w', encoding='utf-8') as f:
    f.write(c)

print('Done!')
