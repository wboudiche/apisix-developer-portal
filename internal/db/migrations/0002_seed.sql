INSERT INTO plans (name, rate_limit_count, rate_limit_window_s) VALUES
  ('Free',   60,   60),
  ('Silver', 300,  60),
  ('Gold',   1000, 60)
ON CONFLICT (name) DO NOTHING;

INSERT INTO api_products (name, slug, category, version, context_path, description, tags, icon, rating) VALUES
  ('SEOAPI','seoapi','Marketing','1.0.0','/seo','Audit on-page, suivi de positions et analyse de backlinks à la demande.','{seo,marketing,temps-réel}','seo',5.0),
  ('ReviewsAPI','reviewsapi','Marketing','1.0.0','/reviews','Collecte et agrégation d''avis clients depuis plusieurs sources.','{avis,marketing}','reviews',4.5),
  ('StockAnalysisAPI','stockanalysisapi','Finance','1.0.0','/stockAnalysis','Cours boursiers, indicateurs techniques et signaux en temps réel.','{finance,temps-réel}','stock',4.0),
  ('testAPI','testapi','Engineering','1.0','/test','Bac à sable pour valider vos intégrations avant la mise en production.','{sandbox,interne}','test',3.0),
  ('KeyWordResearchAPI','keywordresearchapi','Marketing','1.0.0','/keyword','Volume de recherche, difficulté et suggestions de mots-clés.','{seo,mots-clés}','keyword',4.5),
  ('PeopleAPI','peopleapi','Administration','1.0.0','/people','Annuaire, rôles et provisioning des utilisateurs de l''organisation.','{identité,admin}','people',4.0),
  ('CurrencyConverterAPI','currencyconverterapi','Finance','1.0.0','/currencyconv','Taux de change actualisés et conversion multidevise instantanée.','{finance,devises}','currency',5.0),
  ('PhoneVerification','phoneverification','Administration','1.0','/phoneverify','Vérification de numéros et envoi de codes OTP par SMS.','{otp,identité}','phone',4.0),
  ('PizzaShackAPI','pizzashackapi','Engineering','1.0.0','/pizzashack','Commande, suivi de livraison et menu — l''API de démonstration.','{pizza,démo}','pizza',4.5)
ON CONFLICT (slug) DO NOTHING;
