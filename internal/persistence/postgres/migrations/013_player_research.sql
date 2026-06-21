-- 013_player_research.sql
-- Phase 3: persist completed research projects per player (gates advanced recipes).
CREATE TABLE IF NOT EXISTS player_research (
    account_id BIGINT      NOT NULL,
    project_id VARCHAR(48) NOT NULL,
    PRIMARY KEY (account_id, project_id)
);
