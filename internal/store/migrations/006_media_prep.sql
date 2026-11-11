-- How an upload is to be prepared for the players (JSON of media.Prep),
-- chosen in the media library; '' = the hub settings.
ALTER TABLE media ADD COLUMN prep TEXT NOT NULL DEFAULT '';
