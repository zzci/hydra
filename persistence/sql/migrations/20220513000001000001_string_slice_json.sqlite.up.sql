-- Migration generated by the command below; DO NOT EDIT.
-- hydra:generate hydra migrate gen


UPDATE hydra_client SET redirect_uris = '["' || REPLACE(redirect_uris,'|','","') || '"]' WHERE redirect_uris <> '[]';
UPDATE hydra_client SET grant_types = '["' || REPLACE(grant_types,'|','","') || '"]' WHERE grant_types <> '[]';
UPDATE hydra_client SET response_types = '["' || REPLACE(response_types,'|','","') || '"]' WHERE response_types <> '[]';
UPDATE hydra_client SET audience = '["' || REPLACE(audience,'|','","') || '"]' WHERE audience <> '[]';
UPDATE hydra_client SET allowed_cors_origins = '["' || REPLACE(allowed_cors_origins,'|','","') || '"]' WHERE allowed_cors_origins <> '[]';
UPDATE hydra_client SET contacts = '["' || REPLACE(contacts,'|','","') || '"]' WHERE contacts <> '[]';
UPDATE hydra_client SET request_uris = '["' || REPLACE(request_uris,'|','","') || '"]' WHERE request_uris <> '[]';
UPDATE hydra_client SET post_logout_redirect_uris = '["' || REPLACE(post_logout_redirect_uris,'|','","') || '"]' WHERE post_logout_redirect_uris <> '[]';
