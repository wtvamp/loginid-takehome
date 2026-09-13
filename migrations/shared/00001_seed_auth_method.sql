-- +goose Up
-- auth_method is a lookup table, not a native enum — new methods are
-- inserted as data, never a migration (contract §6.7, checklist item 19).
-- Idempotent so a re-run never duplicates the seed row (checklist item 18).
INSERT INTO auth_method (name, requires_secret, is_active)
VALUES ('password', true, true)
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DELETE FROM auth_method WHERE name = 'password';
