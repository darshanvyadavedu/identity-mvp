-- PostgreSQL Schema for identification database
-- Generated: 2026-04-14
-- Database: identification (postgresql://localhost:5432/identification)

-- =============================================================================
-- EXTENSIONS
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


-- =============================================================================
-- TABLES
-- =============================================================================

CREATE TABLE public.users (
    user_id          uuid DEFAULT gen_random_uuid() NOT NULL,
    custom_username  character varying(100)         NOT NULL,
    username_hash    character varying(255)         NOT NULL,
    password_hash    character varying(255)         NOT NULL,
    email            character varying(255)         NOT NULL,
    id_verified      boolean                        DEFAULT false,
    retry_count      integer                        DEFAULT 0,
    locked_until     timestamp with time zone,
    created_at       timestamp with time zone       DEFAULT now(),
    updated_at       timestamp with time zone       DEFAULT now()
);

CREATE TABLE public.verification_sessions (
    session_id           uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id              uuid                           NOT NULL,
    module_type          character varying(50)          DEFAULT 'ID',
    status               character varying(50)          DEFAULT 'pending',
    decision_status      character varying(50)          DEFAULT 'pending',
    provider             character varying(50),
    provider_session_id  character varying(255),
    retry_count          integer                        DEFAULT 0,
    expires_at           timestamp with time zone,
    created_at           timestamp with time zone       DEFAULT now(),
    updated_at           timestamp with time zone       DEFAULT now()
);

CREATE TABLE public.biometric_checks (
    check_id        uuid DEFAULT gen_random_uuid() NOT NULL,
    session_id      uuid                           NOT NULL,
    user_id         uuid                           NOT NULL,
    entity_type     character varying(50)          NOT NULL,
    status          character varying(50)          DEFAULT 'pending',
    attempted_at    timestamp with time zone,
    attempt_number  integer                        DEFAULT 1,
    entity_value    jsonb,
    reference_image text,
    raw_response    jsonb,
    created_at      timestamp with time zone       DEFAULT now(),
    updated_at      timestamp with time zone       DEFAULT now()
);

CREATE TABLE public.identity_hashes (
    hash_id    uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id    uuid                           NOT NULL,
    field_name character varying(50)          NOT NULL,
    hash_value character varying(255)         NOT NULL,
    hash_algo  character varying(50)          NOT NULL,
    created_at timestamp with time zone       DEFAULT now(),
    updated_at timestamp with time zone       DEFAULT now()
);

CREATE TABLE public.consent_records (
    consent_id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id    uuid,
    session_id uuid,
    field_name character varying(100),
    consented  boolean,
    created_at timestamp with time zone       DEFAULT now(),
    updated_at timestamp with time zone       DEFAULT now()
);

CREATE TABLE public.verified_data (
    data_id         uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id         uuid,
    session_id      uuid,
    consent_id      uuid,
    field_name      character varying(100),
    encrypted_value text,
    encryption_iv   character varying(255),
    created_at      timestamp with time zone       DEFAULT now(),
    updated_at      timestamp with time zone       DEFAULT now()
);

CREATE TABLE public.audit_logs (
    log_id     uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id    uuid,
    action     character varying(255)         NOT NULL,
    session_id uuid,
    details    jsonb,
    created_at timestamp with time zone       DEFAULT now()
);


-- =============================================================================
-- PRIMARY KEYS & UNIQUE CONSTRAINTS
-- =============================================================================

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (user_id);
ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_custom_username_key UNIQUE (custom_username);

ALTER TABLE ONLY public.verification_sessions
    ADD CONSTRAINT verification_sessions_pkey PRIMARY KEY (session_id);

ALTER TABLE ONLY public.biometric_checks
    ADD CONSTRAINT biometric_checks_pkey PRIMARY KEY (check_id);


ALTER TABLE ONLY public.identity_hashes
    ADD CONSTRAINT identity_hashes_pkey PRIMARY KEY (hash_id);
ALTER TABLE ONLY public.identity_hashes
    ADD CONSTRAINT identity_hashes_user_id_field_name_hash_value_key UNIQUE (user_id, field_name, hash_value);

ALTER TABLE ONLY public.consent_records
    ADD CONSTRAINT consent_records_pkey PRIMARY KEY (consent_id);

ALTER TABLE ONLY public.verified_data
    ADD CONSTRAINT verified_data_pkey PRIMARY KEY (data_id);

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (log_id);


-- =============================================================================
-- INDEXES
-- =============================================================================

CREATE INDEX idx_sessions_user_id  ON public.verification_sessions USING btree (user_id);
CREATE INDEX idx_sessions_status   ON public.verification_sessions USING btree (status);
CREATE INDEX idx_checks_user_id    ON public.biometric_checks      USING btree (user_id);
CREATE INDEX idx_ihashes_lookup    ON public.identity_hashes        USING btree (field_name, hash_value);


-- =============================================================================
-- FOREIGN KEYS
-- =============================================================================

-- verification_sessions → users
ALTER TABLE ONLY public.verification_sessions
    ADD CONSTRAINT verification_sessions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;

-- biometric_checks → verification_sessions, users
ALTER TABLE ONLY public.biometric_checks
    ADD CONSTRAINT biometric_checks_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES public.verification_sessions(session_id) ON DELETE CASCADE;
ALTER TABLE ONLY public.biometric_checks
    ADD CONSTRAINT biometric_checks_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;


-- identity_hashes → users
ALTER TABLE ONLY public.identity_hashes
    ADD CONSTRAINT identity_hashes_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;

-- consent_records → users, verification_sessions
ALTER TABLE ONLY public.consent_records
    ADD CONSTRAINT consent_records_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;
ALTER TABLE ONLY public.consent_records
    ADD CONSTRAINT consent_records_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES public.verification_sessions(session_id) ON DELETE CASCADE;

-- verified_data → users, verification_sessions, consent_records
ALTER TABLE ONLY public.verified_data
    ADD CONSTRAINT verified_data_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;
ALTER TABLE ONLY public.verified_data
    ADD CONSTRAINT verified_data_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES public.verification_sessions(session_id) ON DELETE CASCADE;
ALTER TABLE ONLY public.verified_data
    ADD CONSTRAINT verified_data_consent_id_fkey
    FOREIGN KEY (consent_id) REFERENCES public.consent_records(consent_id) ON DELETE CASCADE;
