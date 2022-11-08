-- The contents of this file were moved into 20190300000003_testdata.sql due to
-- conflicting requirements:
--
-- 1. The test cases whose data used to be in this file (used to) require the
-- ability to load a consent challenge with NULL login_challenge (the column is
-- added in migration 20190300000003).
--
-- 2. Hydra 2.x requires login_challenge not to be NULL. ***The 2.x migrations
-- delete consent challenges with NULL login_challenge.***
--
-- Instead of completely removing the test cases in this file, we decided to populate
-- the login_challenge column and move the test cases into 20190300000003.
