PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS organizations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS persons (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    relation TEXT,
    importance INTEGER DEFAULT 3,
    notes TEXT,
    org_id TEXT REFERENCES organizations(id) ON DELETE SET NULL,
    position TEXT,
    gender TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    person_id TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    raw_text TEXT NOT NULL,
    event_date TEXT NOT NULL,
    summary TEXT,
    my_feeling TEXT,
    their_reaction TEXT,
    promises TEXT DEFAULT '[]',
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_events_person_date ON events(person_id, event_date DESC);
CREATE TABLE IF NOT EXISTS traits (
    id TEXT PRIMARY KEY,
    person_id TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    trait_key TEXT NOT NULL,
    trait_value TEXT NOT NULL,
    confidence REAL DEFAULT 0.5,
    source_event_ids TEXT DEFAULT '[]',
    verified INTEGER DEFAULT 0,
    updated_at TEXT DEFAULT (datetime('now')),
    UNIQUE(person_id, trait_key)
);
CREATE INDEX IF NOT EXISTS idx_traits_person ON traits(person_id);

CREATE TABLE IF NOT EXISTS follow_ups (
    id TEXT PRIMARY KEY,
    person_id TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    due_date TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now')),
    completed_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_follow_ups_person_status_due ON follow_ups(person_id, status, due_date);
