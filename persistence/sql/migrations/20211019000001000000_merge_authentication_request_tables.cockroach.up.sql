-- Migration generated by the command below; DO NOT EDIT.
-- hydra:generate hydra migrate gen
CREATE TABLE hydra_oauth2_flow
(
    login_challenge           character varying(40) NOT NULL,
    requested_scope           text NOT NULL DEFAULT '[]',
    login_verifier            character varying(40) NOT NULL,
    login_csrf                character varying(40) NOT NULL,
    subject                   character varying(255) NOT NULL,
    request_url               text NOT NULL,
    login_skip                boolean NOT NULL,
    client_id                 character varying(255) NOT NULL,
    requested_at              timestamp without time zone DEFAULT now() NOT NULL,
    login_initialized_at      timestamp without time zone NULL DEFAULT NULL,
    oidc_context              jsonb NOT NULL DEFAULT '{}',
    login_session_id          character varying(40) NULL,
    requested_at_audience     text NULL DEFAULT '[]',

    state                     INTEGER      NOT NULL,

    login_remember boolean NOT NULL DEFAULT false,
    login_remember_for integer NOT NULL,
    login_error text NULL,
    acr text  NOT NULL DEFAULT '',
    login_authenticated_at timestamp without time zone NULL DEFAULT NULL,
    login_was_used boolean NOT NULL DEFAULT false,
    forced_subject_identifier character varying(255) NOT NULL DEFAULT ''::character varying,
    context jsonb DEFAULT '{}',
    amr text DEFAULT '[]',

    consent_challenge_id character varying(40) NULL,
    consent_skip boolean DEFAULT false NOT NULL,
    consent_verifier character varying(40) NULL,
    consent_csrf character varying(40) NULL,

    granted_scope text NOT NULL DEFAULT '[]',
    granted_at_audience text NOT NULL DEFAULT '[]',
    consent_remember boolean DEFAULT false NOT NULL,
    consent_remember_for integer NULL,
    consent_handled_at TIMESTAMP WITHOUT TIME ZONE NULL,
    consent_error TEXT NULL,
    session_access_token jsonb DEFAULT '{}' NOT NULL,
    session_id_token jsonb DEFAULT '{}' NOT NULL,
    consent_was_used boolean DEFAULT false NOT NULL,

    CHECK (
        state = 128 OR
        state = 129 OR
        state = 1 OR
        (state = 2 AND (
            login_remember IS NOT NULL AND
            login_remember_for IS NOT NULL AND
            login_error IS NOT NULL AND
            acr IS NOT NULL AND
            login_was_used IS NOT NULL AND
            context IS NOT NULL AND
            amr IS NOT NULL
        )) OR
        (state = 3 AND (
            login_remember IS NOT NULL AND
            login_remember_for IS NOT NULL AND
            login_error IS NOT NULL AND
            acr IS NOT NULL AND
            login_was_used IS NOT NULL AND
            context IS NOT NULL AND
            amr IS NOT NULL
        )) OR
        (state = 4 AND (
            login_remember IS NOT NULL AND
            login_remember_for IS NOT NULL AND
            login_error IS NOT NULL AND
            acr IS NOT NULL AND
            login_was_used IS NOT NULL AND
            context IS NOT NULL AND
            amr IS NOT NULL AND

            consent_challenge_id IS NOT NULL AND
            consent_verifier IS NOT NULL AND
            consent_skip IS NOT NULL AND
            consent_csrf IS NOT NULL
        )) OR
        (state = 5 AND (
            login_remember IS NOT NULL AND
            login_remember_for IS NOT NULL AND
            login_error IS NOT NULL AND
            acr IS NOT NULL AND
            login_was_used IS NOT NULL AND
            context IS NOT NULL AND
            amr IS NOT NULL AND

            consent_challenge_id IS NOT NULL AND
            consent_verifier IS NOT NULL AND
            consent_skip IS NOT NULL AND
            consent_csrf IS NOT NULL
        )) OR
        (state = 6 AND (
            login_remember IS NOT NULL AND
            login_remember_for IS NOT NULL AND
            login_error IS NOT NULL AND
            acr IS NOT NULL AND
            login_was_used IS NOT NULL AND
            context IS NOT NULL AND
            amr IS NOT NULL AND

            consent_challenge_id IS NOT NULL AND
            consent_verifier IS NOT NULL AND
            consent_skip IS NOT NULL AND
            consent_csrf IS NOT NULL AND

            granted_scope IS NOT NULL AND
            consent_remember IS NOT NULL AND
            consent_remember_for IS NOT NULL AND
            consent_error IS NOT NULL AND
            session_access_token IS NOT NULL AND
            session_id_token IS NOT NULL AND
            consent_was_used IS NOT NULL
        ))
    )
);
