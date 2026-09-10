PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- 人物档案
CREATE TABLE IF NOT EXISTS persons (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    relation    TEXT,
    importance  INTEGER DEFAULT 3,
    notes       TEXT,
    created_at  TEXT DEFAULT (datetime('now')),
    updated_at  TEXT DEFAULT (datetime('now'))
);

-- 事件记录
CREATE TABLE IF NOT EXISTS events (
    id              TEXT PRIMARY KEY,
    person_id       TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    raw_text        TEXT NOT NULL,
    event_date      TEXT NOT NULL,
    summary         TEXT,
    my_feeling      TEXT,
    their_reaction  TEXT,
    promises        TEXT DEFAULT '[]',
    created_at      TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_events_person_date ON events(person_id, event_date DESC);

-- 人物画像
CREATE TABLE IF NOT EXISTS traits (
    id                TEXT PRIMARY KEY,
    person_id         TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    trait_key         TEXT NOT NULL,
    trait_value       TEXT NOT NULL,
    confidence        REAL DEFAULT 0.5,
    source_event_ids  TEXT DEFAULT '[]',
    verified          INTEGER DEFAULT 0,
    updated_at        TEXT DEFAULT (datetime('now')),
    UNIQUE(person_id, trait_key)
);
CREATE INDEX IF NOT EXISTS idx_traits_person ON traits(person_id);