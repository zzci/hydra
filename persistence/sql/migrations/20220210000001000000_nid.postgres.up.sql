-- Migration generated by the command below; DO NOT EDIT.
-- hydra:generate hydra migrate gen
-- hydra_client
ALTER TABLE hydra_client ADD COLUMN nid UUID;
ALTER TABLE hydra_client ADD CONSTRAINT hydra_client_nid_fk_idx FOREIGN KEY (nid) REFERENCES networks (id) ON UPDATE RESTRICT ON DELETE CASCADE;
