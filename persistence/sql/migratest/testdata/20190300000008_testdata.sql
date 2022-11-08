INSERT INTO hydra_oauth2_authentication_session
(id, authenticated_at, subject)
VALUES
('auth_session-0008', now(), 'subject-0008');

-- using the most lately added client as a foreign key
INSERT INTO hydra_oauth2_authentication_request (challenge, verifier, client_id, subject, request_url, skip, requested_scope, csrf, authenticated_at, requested_at, oidc_context, requested_at_audience, login_session_id)
SELECT 'challenge-0008', 'verifier-0008', hydra_client.id, 'subject-0008', 'http://request/0008', true, 'requested_scope-0008_1', 'csrf-0008', now(), now(), '{"display": "display-0008"}', 'requested_audience-0008_1', 'auth_session-0008'
FROM hydra_client
ORDER BY hydra_client.pk DESC
LIMIT 1;

INSERT INTO hydra_oauth2_consent_request (challenge, verifier, client_id, subject, request_url, skip, requested_scope, csrf, authenticated_at, requested_at, oidc_context, forced_subject_identifier, login_session_id, login_challenge, requested_at_audience, acr, context)
SELECT 'challenge-0008', 'verifier-0008', hydra_client.id, 'subject-0008', 'http://request/0008', true, 'requested_scope-0008_1', 'csrf-0008', now(), now(), '{"display": "display-0008"}', 'force_subject_id-0008', 'auth_session-0008', 'challenge-0008', 'requested_audience-0008_1', 'acr-0008', '{"context": "0008"}'
FROM hydra_client
ORDER BY hydra_client.pk DESC
LIMIT 1;

INSERT INTO hydra_oauth2_consent_request_handled
(challenge, granted_scope, remember, remember_for, error, requested_at, session_access_token, session_id_token, authenticated_at, was_used, granted_at_audience)
VALUES
('challenge-0008', 'granted_scope-0008_1', true, 0008, '{}', now(), '{"session_access_token-0008": "0008"}', '{"session_id_token-0008": "0008"}', now(), true, 'granted_audience-0008_1');

INSERT INTO hydra_oauth2_authentication_request_handled
(challenge, subject, remember, remember_for, error, acr, requested_at, authenticated_at, was_used, forced_subject_identifier, context)
VALUES
('challenge-0008', 'subject-0008', true, 0008, '{}', 'acr-0008', now(), now(), true, 'force_subject_id-0008', '{"context": "0008"}');

INSERT INTO hydra_oauth2_obfuscated_authentication_session (client_id, subject, subject_obfuscated)
SELECT hydra_client.id, 'subject-0008', 'subject_obfuscated-0008'
FROM hydra_client
ORDER BY hydra_client.pk DESC
LIMIT 1;
