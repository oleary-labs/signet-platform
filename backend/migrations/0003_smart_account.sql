-- Every user gets a Signet key and a smart wallet controlled by it.
--
-- The key lives in the platform's own signing group and is provisioned at
-- signup regardless of how the developer authenticated: a wallet signature is
-- an auth guard that gets them to the key, not the thing that signs afterwards.
-- The smart wallet is derived CREATE2 from the group public key and deployed
-- lazily by the first UserOperation that needs it.
ALTER TABLE users ADD COLUMN IF NOT EXISTS smart_account_address TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS signet_key_id TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS signet_key_address TEXT;

-- One smart wallet per group public key; two users cannot end up sharing one.
CREATE UNIQUE INDEX IF NOT EXISTS users_smart_account_idx
    ON users (lower(smart_account_address))
    WHERE smart_account_address IS NOT NULL;
