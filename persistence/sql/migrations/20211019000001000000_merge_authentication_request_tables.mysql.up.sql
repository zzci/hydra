-- Migration generated by the command below; DO NOT EDIT.
-- hydra:generate hydra migrate gen
CREATE TABLE hydra_oauth2_flow
(
    `login_challenge` varchar(40) NOT NULL,
    `requested_scope` text NOT NULL DEFAULT ('[]'),
    `login_verifier` varchar(40) NOT NULL,
    `login_csrf` varchar(40) NOT NULL,
    `subject` varchar(255) NOT NULL,
    `request_url` text NOT NULL,
    `login_skip` tinyint(1) NOT NULL,
    `client_id` varchar(255) NOT NULL,
    `requested_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `login_initialized_at` timestamp NULL DEFAULT NULL,
    `oidc_context` json NOT NULL DEFAULT (('{}')),
    `login_session_id` varchar(40) NULL,
    `requested_at_audience` text NULL DEFAULT ('[]'),

    `state` smallint NOT NULL,

    `login_remember` tinyint(1) NOT NULL DEFAULT false,
    `login_remember_for` int(11) NOT NULL,
    `login_error` text NULL,
    `acr` text  NOT NULL DEFAULT (''),
    `login_authenticated_at` timestamp NULL DEFAULT NULL,
    `login_was_used` tinyint(1) NOT NULL DEFAULT false,
    `forced_subject_identifier` varchar(255) NOT NULL DEFAULT '',
    `context` json NOT NULL DEFAULT ('{}'),
    `amr` text NOT NULL DEFAULT ('[]'),

    `consent_challenge_id` varchar(40) NULL,
    `consent_skip` tinyint(1) NOT NULL DEFAULT 0,
    `consent_verifier` varchar(40) NULL,
    `consent_csrf` varchar(40) NULL,

    `granted_scope` text NOT NULL DEFAULT ('[]'),
    `granted_at_audience` text NOT NULL DEFAULT ('[]'),
    `consent_remember` tinyint(1) NOT NULL DEFAULT false,
    `consent_remember_for` int(11) NULL,
    `consent_handled_at` timestamp NULL DEFAULT NULL,
    `consent_error` TEXT NULL,
    `session_access_token` json DEFAULT ('{}') NOT NULL,
    `session_id_token` json DEFAULT ('{}') NOT NULL,
    `consent_was_used` tinyint(1),

    PRIMARY KEY (`login_challenge`),
    UNIQUE KEY `hydra_oauth2_flow_login_verifier_idx` (`login_verifier`),
    KEY `hydra_oauth2_flow_cid_idx` (`client_id`),
    KEY `hydra_oauth2_flow_sub_idx` (`subject`),
    KEY `hydra_oauth2_flow_login_session_id_idx` (`login_session_id`),
    CONSTRAINT `hydra_oauth2_flow_client_id_fk` FOREIGN KEY (`client_id`) REFERENCES `hydra_client` (`id`) ON DELETE CASCADE,
    CONSTRAINT `hydra_oauth2_flow_login_session_id_fk` FOREIGN KEY (`login_session_id`) REFERENCES `hydra_oauth2_authentication_session` (`id`) ON DELETE CASCADE,

    UNIQUE KEY `hydra_oauth2_flow_consent_challenge_idx` (`consent_challenge_id`),
    KEY `hydra_oauth2_flow_consent_verifier_idx` (`consent_verifier`),
    KEY `hydra_oauth2_flow_client_id_subject_idx` (`client_id`,`subject`)
);

ALTER TABLE hydra_oauth2_flow ADD CONSTRAINT hydra_oauth2_flow_chk CHECK (
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
  );

