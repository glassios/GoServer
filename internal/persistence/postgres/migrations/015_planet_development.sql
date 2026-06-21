-- 015_planet_development.sql
-- Phase 5: persist planet ownership + development level (planets themselves are seeded in-code
-- with stable IDs; only their mutable state is stored).
CREATE TABLE IF NOT EXISTS planet_development (
    planet_id BIGINT PRIMARY KEY,
    system_id INT    NOT NULL,
    owner_id  BIGINT NOT NULL DEFAULT 0,
    level     INT    NOT NULL DEFAULT 0
);
