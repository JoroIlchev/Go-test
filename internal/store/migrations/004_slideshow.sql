-- A rendered slideshow keeps the recipe it was made from, so an interrupted
-- render resumes after a restart and the editor can reopen it.
ALTER TABLE media ADD COLUMN recipe TEXT NOT NULL DEFAULT '';
