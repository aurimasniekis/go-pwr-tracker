-- Timestamps are UTC, stored as fixed-width RFC 3339 text
-- (YYYY-MM-DDTHH:MM:SS.sssZ) so lexical order equals chronological order.

CREATE TABLE artists (
    id         INTEGER PRIMARY KEY,
    name_key   TEXT NOT NULL UNIQUE,  -- normalize.Key(name)
    name       TEXT NOT NULL,         -- display value as first observed
    created_at TEXT NOT NULL
);

CREATE TABLE tracks (
    id                INTEGER PRIMARY KEY,
    identity_key      TEXT NOT NULL UNIQUE,      -- normalize.TrackKey(artist, title)
    artist_id         INTEGER REFERENCES artists(id), -- NULL for station segments
    title             TEXT NOT NULL,
    title_key         TEXT NOT NULL,
    kind              TEXT NOT NULL CHECK (kind IN ('music', 'segment')),
    first_observed_at TEXT NOT NULL,
    last_observed_at  TEXT NOT NULL
);
CREATE INDEX tracks_artist_id ON tracks(artist_id);
CREATE INDEX tracks_title_key ON tracks(title_key);

-- One row per distinct airplay event (observed via upstream uid, or inferred).
CREATE TABLE plays (
    id                  INTEGER PRIMARY KEY,
    track_id            INTEGER NOT NULL REFERENCES tracks(id),
    upstream_uid        TEXT,
    upstream_start_at   TEXT,  -- upstream "date", converted to UTC, when plausible
    start_at            TEXT NOT NULL, -- upstream_start_at if present, else first_observed_at
    start_authoritative INTEGER NOT NULL CHECK (start_authoritative IN (0, 1)),
    first_observed_at   TEXT NOT NULL,
    last_observed_at    TEXT NOT NULL,
    observation_count   INTEGER NOT NULL DEFAULT 1,
    reason              TEXT NOT NULL CHECK (reason IN
                          ('upstream_event', 'observed_transition', 'initial_observation', 'continuity_gap')),
    uncertain           INTEGER NOT NULL CHECK (uncertain IN (0, 1))
);
CREATE UNIQUE INDEX plays_upstream_uid ON plays(upstream_uid) WHERE upstream_uid IS NOT NULL;
CREATE INDEX plays_start_at_id ON plays(start_at, id);
CREATE INDEX plays_track_start ON plays(track_id, start_at);

-- One row per successful poll. Pruned by OBSERVATION_RETENTION; plays are kept.
CREATE TABLE observations (
    id            INTEGER PRIMARY KEY,
    observed_at   TEXT NOT NULL,
    track_id      INTEGER NOT NULL REFERENCES tracks(id),
    play_id       INTEGER REFERENCES plays(id) ON DELETE SET NULL,
    upstream_uid  TEXT,
    upstream_date TEXT,      -- raw upstream "date" string
    program       TEXT,
    fetch_mode    TEXT NOT NULL,
    raw_payload   TEXT       -- only when STORE_RAW_PAYLOADS=true; cleared by RAW_PAYLOAD_RETENTION
);
CREATE INDEX observations_observed_at ON observations(observed_at);
CREATE INDEX observations_play_id ON observations(play_id);

-- Single-row collector state, persisted so restarts continue the current play.
CREATE TABLE collector_state (
    id                   INTEGER PRIMARY KEY CHECK (id = 1),
    current_play_id      INTEGER REFERENCES plays(id),
    first_success_at     TEXT,
    last_attempt_at      TEXT,
    last_success_at      TEXT,
    last_error           TEXT,
    last_error_at        TEXT,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    fetch_mode           TEXT
);
INSERT INTO collector_state (id) VALUES (1);
