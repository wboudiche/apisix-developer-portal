-- The demo seed data (0002_seed.sql) was authored in French with no
-- localization mechanism behind it, so product descriptions/tags stayed
-- French regardless of the selected UI language (#4). Re-seed it in English;
-- migrations are append-only, so this updates the existing rows rather than
-- editing 0002_seed.sql in place.
--
-- Each UPDATE is guarded on description matching the exact original French
-- seed text (mirroring the WHERE upstream_url = '' guard in
-- 0004_seed_upstream.sql), so an admin who already customized a seeded
-- product's description before this migration runs keeps their own text
-- instead of it being silently overwritten.
UPDATE api_products SET description = 'On-page audits, rank tracking, and on-demand backlink analysis.', tags = '{seo,marketing,real-time}' WHERE slug = 'seoapi' AND description = 'Audit on-page, suivi de positions et analyse de backlinks à la demande.';
UPDATE api_products SET description = 'Collect and aggregate customer reviews from multiple sources.', tags = '{reviews,marketing}' WHERE slug = 'reviewsapi' AND description = 'Collecte et agrégation d''avis clients depuis plusieurs sources.';
UPDATE api_products SET description = 'Stock quotes, technical indicators, and real-time signals.', tags = '{finance,real-time}' WHERE slug = 'stockanalysisapi' AND description = 'Cours boursiers, indicateurs techniques et signaux en temps réel.';
UPDATE api_products SET description = 'Sandbox for validating your integrations before going to production.', tags = '{sandbox,internal}' WHERE slug = 'testapi' AND description = 'Bac à sable pour valider vos intégrations avant la mise en production.';
UPDATE api_products SET description = 'Search volume, difficulty, and keyword suggestions.', tags = '{seo,keywords}' WHERE slug = 'keywordresearchapi' AND description = 'Volume de recherche, difficulté et suggestions de mots-clés.';
UPDATE api_products SET description = 'Directory, roles, and user provisioning for the organization.', tags = '{identity,admin}' WHERE slug = 'peopleapi' AND description = 'Annuaire, rôles et provisioning des utilisateurs de l''organisation.';
UPDATE api_products SET description = 'Up-to-date exchange rates and instant multi-currency conversion.', tags = '{finance,currency}' WHERE slug = 'currencyconverterapi' AND description = 'Taux de change actualisés et conversion multidevise instantanée.';
UPDATE api_products SET description = 'Phone number verification and OTP codes via SMS.', tags = '{otp,identity}' WHERE slug = 'phoneverification' AND description = 'Vérification de numéros et envoi de codes OTP par SMS.';
UPDATE api_products SET description = 'Ordering, delivery tracking, and menu — the demo API.', tags = '{pizza,demo}' WHERE slug = 'pizzashackapi' AND description = 'Commande, suivi de livraison et menu — l''API de démonstration.';
