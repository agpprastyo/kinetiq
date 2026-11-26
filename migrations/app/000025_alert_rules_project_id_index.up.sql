-- alert_rules is always queried by project_id (alerting repository, intelligence sweep).
CREATE INDEX IF NOT EXISTS idx_alert_rules_project_id ON alert_rules (project_id);
