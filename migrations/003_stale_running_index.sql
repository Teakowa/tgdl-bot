CREATE INDEX IF NOT EXISTS idx_tasks_status_started_at
  ON tasks(status, started_at);
