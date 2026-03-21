CREATE TABLE IF NOT EXISTS tasks (
  task_id TEXT PRIMARY KEY,
  chat_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  url TEXT NOT NULL,
  status TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  retry_count INTEGER NOT NULL DEFAULT 0,
  lease_id TEXT,
  download_dir TEXT,
  output_summary TEXT,
  error_message TEXT,
  exit_code INTEGER,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  started_at DATETIME,
  finished_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_tasks_user_created_at
  ON tasks(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_tasks_idempotency_key
  ON tasks(idempotency_key);

CREATE INDEX IF NOT EXISTS idx_tasks_status
  ON tasks(status);
