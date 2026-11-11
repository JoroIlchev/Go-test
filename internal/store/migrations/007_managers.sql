-- Managers are the hub operator's staff: like super admins they belong to no
-- client (client_id NULL), but they work only the clients in
-- manager_clients, or all of them. Billing and logs are opt-in.
ALTER TABLE users ADD COLUMN is_manager INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN all_clients INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN can_billing INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN can_logs INTEGER NOT NULL DEFAULT 0;

CREATE TABLE manager_clients (
    user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, client_id)
);
