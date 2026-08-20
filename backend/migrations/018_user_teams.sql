-- Team membership, enabling team-level data scope (distinct from department
-- membership). A user can belong to several teams.
CREATE TABLE IF NOT EXISTS user_teams (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, team_id)
);

CREATE INDEX IF NOT EXISTS user_teams_team_idx ON user_teams (team_id, user_id);
