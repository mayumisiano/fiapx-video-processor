-- Denormalized from the JWT claim at request creation time so the worker
-- never needs to query Identity's database to send a notification email
-- (see docs/adr/0007). DEFAULT '' only unblocks this ALTER on a table that
-- may already have rows; it is not a real backfill strategy for existing
-- requests created before this column existed.
ALTER TABLE processing_requests ADD COLUMN user_email TEXT NOT NULL DEFAULT '';
