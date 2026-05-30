-- Subscriptions now require admin approval before provisioning (Plan 4c), so a
-- newly inserted subscription must start 'pending', not 'active'. SaveSubscription
-- always writes the status explicitly, but flipping the column default closes the
-- gap where any future bare INSERT (test, script, migration) would otherwise land
-- as 'active' and silently bypass the approval gate. Existing rows are untouched.
ALTER TABLE subscriptions ALTER COLUMN status SET DEFAULT 'pending';
