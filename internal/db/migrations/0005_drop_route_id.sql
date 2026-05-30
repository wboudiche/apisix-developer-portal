-- Remove the unused api_products.apisix_route_id column. The APISIX route id is
-- derived deterministically as prod_<id> in the subscriptions service, so the
-- stored column was dead. A real per-product route id (if needed) returns with
-- the Plan 4 admin "publish" flow.
ALTER TABLE api_products DROP COLUMN IF EXISTS apisix_route_id;
