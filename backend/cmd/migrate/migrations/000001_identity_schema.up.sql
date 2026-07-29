CREATE SCHEMA IF NOT EXISTS identity;

CREATE TABLE identity.user_profiles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    keycloak_id UUID NOT NULL UNIQUE,
    username    TEXT NOT NULL UNIQUE,
    avatar_url  TEXT NOT NULL DEFAULT '',
    bio         TEXT NOT NULL DEFAULT '',
    suspended   BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
