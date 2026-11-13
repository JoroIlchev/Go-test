-- The hub operator's own ads ("Моята реклама"): one client node marked house.
-- Its campaigns may target any client's nodes and only super admins see it.
ALTER TABLE nodes ADD COLUMN house INTEGER NOT NULL DEFAULT 0;
