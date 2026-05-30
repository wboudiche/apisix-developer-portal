-- Give the seeded demo products a working upstream so they can be subscribed and
-- called end-to-end against the compose `echo` service. Real per-product upstreams
-- become an admin "publish" concern in Plan 4; this default makes the demo usable now.
UPDATE api_products SET upstream_url = 'echo:8080' WHERE upstream_url = '';
