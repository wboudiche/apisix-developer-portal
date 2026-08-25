-- The demo seed data (0002_seed.sql) was authored in French with no
-- localization mechanism behind it, so product descriptions/tags stayed
-- French regardless of the selected UI language (#4). Re-seed it in English;
-- migrations are append-only, so this updates the existing rows rather than
-- editing 0002_seed.sql in place.
UPDATE api_products SET description = 'On-page audits, rank tracking, and on-demand backlink analysis.', tags = '{seo,marketing,real-time}' WHERE slug = 'seoapi';
UPDATE api_products SET description = 'Collect and aggregate customer reviews from multiple sources.', tags = '{reviews,marketing}' WHERE slug = 'reviewsapi';
UPDATE api_products SET description = 'Stock quotes, technical indicators, and real-time signals.', tags = '{finance,real-time}' WHERE slug = 'stockanalysisapi';
UPDATE api_products SET description = 'Sandbox for validating your integrations before going to production.', tags = '{sandbox,internal}' WHERE slug = 'testapi';
UPDATE api_products SET description = 'Search volume, difficulty, and keyword suggestions.', tags = '{seo,keywords}' WHERE slug = 'keywordresearchapi';
UPDATE api_products SET description = 'Directory, roles, and user provisioning for the organization.', tags = '{identity,admin}' WHERE slug = 'peopleapi';
UPDATE api_products SET description = 'Up-to-date exchange rates and instant multi-currency conversion.', tags = '{finance,currency}' WHERE slug = 'currencyconverterapi';
UPDATE api_products SET description = 'Phone number verification and OTP codes via SMS.', tags = '{otp,identity}' WHERE slug = 'phoneverification';
UPDATE api_products SET description = 'Ordering, delivery tracking, and menu — the demo API.', tags = '{pizza,demo}' WHERE slug = 'pizzashackapi';
