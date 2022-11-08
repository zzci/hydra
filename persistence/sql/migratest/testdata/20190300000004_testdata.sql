INSERT INTO hydra_oauth2_authentication_session
(id, authenticated_at, subject)
VALUES
('auth_session-0004', now(), 'subject-0004');

-- using the most lately added client as a foreign key
INSERT INTO hydra_oauth2_authentication_request (challenge, verifier, client_id, subject, request_url, skip, requested_scope, csrf, authenticated_at, requested_at, oidc_context, requested_at_audience, login_session_id)
SELECT 'challenge-0004', 'verifier-0004', hydra_client.id, 'subject-0004', 'http://request/0004', true, 'requested_scope-0004_1', 'csrf-0004', now(), now(), '{"display": "display-0004"}', 'requested_audience-0004_1', 'auth_session-0004'
FROM hydra_client
ORDER BY hydra_client.pk DESC
LIMIT 1;

INSERT INTO hydra_oauth2_consent_request (challenge, verifier, client_id, subject, request_url, skip, requested_scope, csrf, authenticated_at, requested_at, oidc_context, forced_subject_identifier, login_session_id, login_challenge, requested_at_audience)
SELECT 'challenge-0004', 'verifier-0004', hydra_client.id, 'subject-0004', 'http://request/0004', true, 'requested_scope-0004_1', 'csrf-0004', now(), now(), '{"display": "display-0004"}', 'force_subject_id-0004', 'auth_session-0004', 'challenge-0004', 'requested_audience-0004_1'
FROM hydra_client
ORDER BY hydra_client.pk DESC
LIMIT 1;

INSERT INTO hydra_oauth2_consent_request_handled
(challenge, granted_scope, remember, remember_for, error, requested_at, session_access_token, session_id_token, authenticated_at, was_used, granted_at_audience)
VALUES
('challenge-0004', 'granted_scope-0004_1', true, 0004, '{}', now(), '{"session_access_token-0004": "0004"}', '{"session_id_token-0004": "0004"}', now(), true, 'granted_audience-0004_1');

INSERT INTO hydra_oauth2_authentication_request_handled
(challenge, subject, remember, remember_for, error, acr, requested_at, authenticated_at, was_used, forced_subject_identifier)
VALUES
('challenge-0004', 'subject-0004', true, 0004, '{}', 'acr-0004', now(), now(), true, 'force_subject_id-0004');

INSERT INTO hydra_oauth2_obfuscated_authentication_session (client_id, subject, subject_obfuscated)
SELECT hydra_client.id, 'subject-0004', 'subject_obfuscated-0004'
FROM hydra_client
ORDER BY hydra_client.pk DESC
LIMIT 1;
