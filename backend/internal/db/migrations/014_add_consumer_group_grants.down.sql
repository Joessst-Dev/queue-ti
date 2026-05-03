DROP INDEX IF EXISTS uq_user_grants_consume;

ALTER TABLE user_grants DROP COLUMN IF EXISTS consumer_group;

ALTER TABLE user_grants
    DROP CONSTRAINT user_grants_action_check,
    ADD  CONSTRAINT user_grants_action_check
         CHECK (action IN ('read', 'write', 'admin'));
