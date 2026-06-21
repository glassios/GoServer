-- 014_space_bases.sql
-- Phase 5: persist player-built space bases so they survive worldnode restarts.
CREATE TABLE IF NOT EXISTS space_bases (
    id         BIGINT  PRIMARY KEY,
    system_id  INT     NOT NULL,
    owner_id   BIGINT  NOT NULL,
    x          REAL    NOT NULL,
    y          REAL    NOT NULL,
    level      INT     NOT NULL DEFAULT 1,
    health     INT     NOT NULL DEFAULT 500,
    max_health INT     NOT NULL DEFAULT 500
);
CREATE INDEX IF NOT EXISTS idx_space_bases_system ON space_bases (system_id);
