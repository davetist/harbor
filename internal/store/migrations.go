package store

const schema = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL);
INSERT INTO schema_version SELECT 1 WHERE NOT EXISTS(SELECT 1 FROM schema_version);
CREATE TABLE IF NOT EXISTS ships (id TEXT PRIMARY KEY, pier_path TEXT NOT NULL UNIQUE, payload TEXT NOT NULL CHECK(json_valid(payload)));
CREATE TABLE IF NOT EXISTS jobs (id TEXT PRIMARY KEY, ship_id TEXT NOT NULL, kind TEXT NOT NULL, trigger TEXT NOT NULL CHECK(trigger IN ('manual','system','scheduled')), phase TEXT NOT NULL, schedule_id TEXT, occurrence_id TEXT UNIQUE, payload TEXT NOT NULL CHECK(json_valid(payload)), sequence INTEGER UNIQUE,
 FOREIGN KEY(occurrence_id) REFERENCES occurrences(id) DEFERRABLE INITIALLY DEFERRED);
CREATE TABLE IF NOT EXISTS processes (id TEXT PRIMARY KEY, ship_id TEXT NOT NULL, phase TEXT NOT NULL, payload TEXT NOT NULL CHECK(json_valid(payload)));
CREATE TABLE IF NOT EXISTS schedules (id TEXT PRIMARY KEY, ship_id TEXT NOT NULL, next_due TEXT NOT NULL, enabled INTEGER NOT NULL, payload TEXT NOT NULL CHECK(json_valid(payload)), FOREIGN KEY(ship_id) REFERENCES ships(id));
CREATE TABLE IF NOT EXISTS occurrences (id TEXT PRIMARY KEY, schedule_id TEXT NOT NULL, scheduled_for TEXT NOT NULL, local_date TEXT NOT NULL, job_id TEXT NOT NULL UNIQUE,
 UNIQUE(schedule_id, scheduled_for), FOREIGN KEY(schedule_id) REFERENCES schedules(id), FOREIGN KEY(job_id) REFERENCES jobs(id) DEFERRABLE INITIALLY DEFERRED);
CREATE TABLE IF NOT EXISTS backups (id TEXT PRIMARY KEY,ship_id TEXT NOT NULL,archive_path TEXT NOT NULL UNIQUE,created_at TIMESTAMP NOT NULL,verified INTEGER NOT NULL,FOREIGN KEY(ship_id) REFERENCES ships(id));
CREATE TABLE IF NOT EXISTS pending_registrations (id TEXT PRIMARY KEY,job_id TEXT NOT NULL UNIQUE,payload TEXT NOT NULL CHECK(json_valid(payload)));
CREATE INDEX IF NOT EXISTS jobs_dispatch ON jobs(phase,sequence);
CREATE TABLE IF NOT EXISTS job_log_chunks (job_id TEXT NOT NULL,output BLOB NOT NULL,FOREIGN KEY(job_id) REFERENCES jobs(id));
CREATE INDEX IF NOT EXISTS job_log_lookup ON job_log_chunks(job_id);
CREATE INDEX IF NOT EXISTS due_schedules ON schedules(enabled,next_due);
`
