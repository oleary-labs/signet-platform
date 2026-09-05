-- Environments collapse to development and production.
--
-- Staging existed as a middle tier where a developer could assemble a real
-- operator set before paying. That distinction now lives inside development
-- instead: the platform only *deploys* a development group on its own
-- operators, but the developer is free to invite others into it, and does so
-- on the way to production. One fewer concept for the same capability.
UPDATE apps SET environment = 'development' WHERE environment = 'staging';

ALTER TABLE apps DROP CONSTRAINT IF EXISTS apps_environment_check;
ALTER TABLE apps ADD CONSTRAINT apps_environment_check
    CHECK (environment IN ('development', 'production'));
