-- Managers who also run the hub operator's own ads ("Моята реклама"), which
-- play on the screens of every client. Opt-in, like billing and logs.
ALTER TABLE users ADD COLUMN can_house INTEGER NOT NULL DEFAULT 0;
