-- What Marquee Player reports about its device (JSON, see DeviceInfo):
-- model, Android version, screen, video decoder limits, free storage.
-- Empty for other players, which report nothing beyond their User-Agent.
ALTER TABLE devices ADD COLUMN info TEXT NOT NULL DEFAULT '';
