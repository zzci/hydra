-- Migration generated by the command below; DO NOT EDIT.
-- hydra:generate hydra migrate gen
ALTER TABLE hydra_client ADD COLUMN redirect_uris_json json DEFAULT ('[]') NOT NULL;
ALTER TABLE hydra_client ADD COLUMN grant_types_json json DEFAULT ('[]') NOT NULL;
ALTER TABLE hydra_client ADD COLUMN response_types_json json DEFAULT ('[]') NOT NULL;
ALTER TABLE hydra_client ADD COLUMN audience_json json DEFAULT ('[]') NOT NULL;
ALTER TABLE hydra_client ADD COLUMN allowed_cors_origins_json json DEFAULT ('[]') NOT NULL;
ALTER TABLE hydra_client ADD COLUMN contacts_json json DEFAULT ('[]') NOT NULL;
ALTER TABLE hydra_client ADD COLUMN request_uris_json json DEFAULT ('[]') NOT NULL;
ALTER TABLE hydra_client ADD COLUMN post_logout_redirect_uris_json json DEFAULT ('[]') NOT NULL;