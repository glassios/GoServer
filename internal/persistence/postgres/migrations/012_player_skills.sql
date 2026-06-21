-- 012_player_skills.sql
-- Phase 3: persist per-player skill progression (mining / engineering / combat).
CREATE TABLE IF NOT EXISTS player_skills (
    account_id BIGINT      NOT NULL,
    skill      VARCHAR(16) NOT NULL,
    level      INT         NOT NULL DEFAULT 1,
    xp         INT         NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, skill)
);
