INSERT INTO hydra_oauth2_access (signature, request_id, requested_at, client_id, scope, granted_scope, form_data, session_data, subject, active, requested_audience, granted_audience, challenge_id)
SELECT 'sig-0010', 'req-0010', now(), hc.id, 'scope-0010', 'granted_scope-0010', 'form_data-0010', 'session-0010', 'subject-0010', false, 'requested_audience-0010', 'granted_audience-0010', crh.challenge
FROM hydra_client hc, hydra_oauth2_consent_request_handled crh
ORDER BY hc.pk, crh.challenge DESC
LIMIT 1;

INSERT INTO hydra_oauth2_refresh (signature, request_id, requested_at, client_id, scope, granted_scope, form_data, session_data, subject, active, requested_audience, granted_audience, challenge_id)
SELECT 'sig-0010', 'req-0010', now(), hc.id, 'scope-0010', 'granted_scope-0010', 'form_data-0010', 'session-0010', 'subject-0010', false, 'requested_audience-0010', 'granted_audience-0010', crh.challenge
FROM hydra_client hc, hydra_oauth2_consent_request_handled crh
ORDER BY hc.pk, crh.challenge DESC
LIMIT 1;

INSERT INTO hydra_oauth2_code (signature, request_id, requested_at, client_id, scope, granted_scope, form_data, session_data, subject, active, requested_audience, granted_audience, challenge_id)
SELECT 'sig-0010', 'req-0010', now(), hc.id, 'scope-0010', 'granted_scope-0010', 'form_data-0010', 'session-0010', 'subject-0010', false, 'requested_audience-0010', 'granted_audience-0010', crh.challenge
FROM hydra_client hc, hydra_oauth2_consent_request_handled crh
ORDER BY hc.pk, crh.challenge DESC
LIMIT 1;

INSERT INTO hydra_oauth2_oidc (signature, request_id, requested_at, client_id, scope, granted_scope, form_data, session_data, subject, active, requested_audience, granted_audience, challenge_id)
SELECT 'sig-0010', 'req-0010', now(), hc.id, 'scope-0010', 'granted_scope-0010', 'form_data-0010', 'session-0010', 'subject-0010', false, 'requested_audience-0010', 'granted_audience-0010', crh.challenge
FROM hydra_client hc, hydra_oauth2_consent_request_handled crh
ORDER BY hc.pk, crh.challenge DESC
LIMIT 1;

INSERT INTO hydra_oauth2_pkce (signature, request_id, requested_at, client_id, scope, granted_scope, form_data, session_data, subject, active, requested_audience, granted_audience, challenge_id)
SELECT 'sig-0010', 'req-0010', now(), hc.id, 'scope-0010', 'granted_scope-0010', 'form_data-0010', 'session-0010', 'subject-0010', false, 'requested_audience-0010', 'granted_audience-0010', crh.challenge
FROM hydra_client hc, hydra_oauth2_consent_request_handled crh
ORDER BY hc.pk, crh.challenge DESC
LIMIT 1;
