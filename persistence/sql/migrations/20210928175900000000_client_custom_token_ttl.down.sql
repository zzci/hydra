ALTER TABLE hydra_client DROP COLUMN authorization_code_grant_access_token_lifespan;
ALTER TABLE hydra_client DROP COLUMN authorization_code_grant_id_token_lifespan;
ALTER TABLE hydra_client DROP COLUMN authorization_code_grant_refresh_token_lifespan;
ALTER TABLE hydra_client DROP COLUMN client_credentials_grant_access_token_lifespan;
ALTER TABLE hydra_client DROP COLUMN implicit_grant_access_token_lifespan;
ALTER TABLE hydra_client DROP COLUMN implicit_grant_id_token_lifespan;
ALTER TABLE hydra_client DROP COLUMN jwt_bearer_grant_access_token_lifespan;
ALTER TABLE hydra_client DROP COLUMN password_grant_access_token_lifespan;
ALTER TABLE hydra_client DROP COLUMN password_grant_refresh_token_lifespan;
ALTER TABLE hydra_client DROP COLUMN refresh_token_grant_id_token_lifespan;
ALTER TABLE hydra_client DROP COLUMN refresh_token_grant_access_token_lifespan;
ALTER TABLE hydra_client DROP COLUMN refresh_token_grant_refresh_token_lifespan;
