-- Migration generated by the command below; DO NOT EDIT.
-- hydra:generate hydra migrate gen

ALTER TABLE hydra_oauth2_jti_blacklist ALTER nid SET NOT NULL;
