-- Language of the admin for each user: "bg" (default) or "en".
ALTER TABLE users ADD COLUMN lang TEXT NOT NULL DEFAULT 'bg';
