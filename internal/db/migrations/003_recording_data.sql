-- 003_recording_data.sql
-- Adds data column to shell_recordings for storing asciicast v2 recording content.

ALTER TABLE shell_recordings ADD COLUMN data BLOB;
