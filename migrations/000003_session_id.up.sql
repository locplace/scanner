-- Migration 003: Add session_id for detecting scanner restarts
-- Session ID tracks the scanner's "epoch" - when it restarts, it gets a new session_id.
-- This allows the coordinator to detect orphaned jobs from previous scanner sessions.

-- Add session_id to scanner_clients (current session for each client)
ALTER TABLE scanner_clients ADD COLUMN session_id UUID;

-- Add session_id to active_scans (which session assigned this job)
ALTER TABLE active_scans ADD COLUMN session_id UUID;

-- Index for efficient orphan detection (jobs where session doesn't match client's current session)
CREATE INDEX idx_active_scans_session ON active_scans(session_id);
