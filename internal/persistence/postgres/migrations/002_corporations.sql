CREATE TABLE IF NOT EXISTS corporations (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    wallet BIGINT NOT NULL DEFAULT 0,
    founder_id BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS corporation_members (
    corp_id INT NOT NULL REFERENCES corporations(id) ON DELETE CASCADE,
    account_id BIGINT PRIMARY KEY,
    role VARCHAR(50) NOT NULL
);

-- Indexing for speed
CREATE INDEX IF NOT EXISTS idx_corp_members_corp_id ON corporation_members(corp_id);
