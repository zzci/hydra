-- Migration generated by the command below; DO NOT EDIT.
-- hydra:generate hydra migrate gen

UPDATE hydra_oauth2_logout_request SET nid = (SELECT id FROM networks LIMIT 1);
ALTER TABLE hydra_oauth2_logout_request MODIFY `nid` char(36) NOT NULL;



