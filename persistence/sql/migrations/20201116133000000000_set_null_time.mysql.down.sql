ALTER TABLE hydra_oauth2_authentication_session MODIFY authenticated_at timestamp NOT NULL DEFAULT NOW();
