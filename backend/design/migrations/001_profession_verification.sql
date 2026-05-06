-- Migration: 001_profession_verification
-- Adds profession verification tables to the existing identity schema.
-- Run once against an existing database that already has the base schema applied.

-- =============================================================================
-- TABLES
-- =============================================================================

CREATE TABLE public.profession_verifications (
    verification_id   uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id           uuid                           NOT NULL,
    profession_type   character varying(50)          NOT NULL,  -- corporate|freelance|regulated
    profession_title  character varying(255),                   -- stored only when consent_store_data = true
    employer_name     character varying(255),                   -- stored only when consent_store_data = true
    status            character varying(50)          DEFAULT 'in_progress',
    confidence_score  integer                        DEFAULT 0,
    confidence_level  character varying(20)          DEFAULT 'none',
    consent_store_data boolean                       DEFAULT false,
    expires_at        timestamp with time zone,
    created_at        timestamp with time zone       DEFAULT now(),
    updated_at        timestamp with time zone       DEFAULT now()
);

-- Stores individual pieces of evidence; sensitive document bytes are never persisted.
-- Only non-sensitive metadata (domain, platform, doc type) is kept.
CREATE TABLE public.profession_evidence (
    evidence_id        uuid DEFAULT gen_random_uuid() NOT NULL,
    verification_id    uuid                           NOT NULL,
    user_id            uuid                           NOT NULL,
    method             character varying(50)          NOT NULL,  -- work_email|linkedin_oauth|document_upload|portfolio_social
    status             character varying(50)          DEFAULT 'pending',
    score_contribution integer                        DEFAULT 0,
    metadata           jsonb,
    email_otp_hash     character varying(255),   -- HMAC(otp, HMAC_SECRET); work_email only
    email_otp_expires  timestamp with time zone, -- OTP TTL; work_email only
    created_at         timestamp with time zone  DEFAULT now(),
    updated_at         timestamp with time zone  DEFAULT now()
);


-- =============================================================================
-- PRIMARY KEYS
-- =============================================================================

ALTER TABLE ONLY public.profession_verifications
    ADD CONSTRAINT profession_verifications_pkey PRIMARY KEY (verification_id);

ALTER TABLE ONLY public.profession_evidence
    ADD CONSTRAINT profession_evidence_pkey PRIMARY KEY (evidence_id);

-- One active evidence entry per method per verification.
ALTER TABLE ONLY public.profession_evidence
    ADD CONSTRAINT profession_evidence_verification_method_key
    UNIQUE (verification_id, method);


-- =============================================================================
-- INDEXES
-- =============================================================================

CREATE INDEX idx_pverif_user_id  ON public.profession_verifications USING btree (user_id);
CREATE INDEX idx_pverif_status   ON public.profession_verifications USING btree (status);
CREATE INDEX idx_pevid_verif_id  ON public.profession_evidence      USING btree (verification_id);
CREATE INDEX idx_pevid_user_id   ON public.profession_evidence      USING btree (user_id);


-- =============================================================================
-- FOREIGN KEYS
-- =============================================================================

ALTER TABLE ONLY public.profession_verifications
    ADD CONSTRAINT profession_verifications_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.profession_evidence
    ADD CONSTRAINT profession_evidence_verification_id_fkey
    FOREIGN KEY (verification_id) REFERENCES public.profession_verifications(verification_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.profession_evidence
    ADD CONSTRAINT profession_evidence_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;
