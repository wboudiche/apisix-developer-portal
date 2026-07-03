package i18n

// fr holds French message strings keyed by dotted area keys. Grown per package
// sweep; kept in key-set parity with en (enforced by TestParity).
var fr = map[string]string{
	// auth
	"auth.register.credentialsRequired": "email et mot de passe (8 caractères min.) requis",
	"auth.password.tooLong":             "le mot de passe doit faire au plus 72 octets",
	"auth.password.hashFailed":          "impossible de hacher le mot de passe",
	"auth.register.emailTaken":          "cet email est déjà enregistré",
	"auth.register.createFailed":        "impossible de créer le compte",
	"auth.token.issueFailed":            "impossible de délivrer le jeton",
	"auth.login.tooManyAttempts":        "trop de tentatives",
	"auth.login.invalidCredentials":     "identifiants invalides",
	"auth.middleware.missingToken":      "jeton bearer manquant",
	"auth.middleware.invalidToken":      "jeton invalide",
	"auth.middleware.adminOnly":         "réservé aux administrateurs",
	"auth.middleware.roleCheckFailed":   "impossible de vérifier le rôle",

	// catalog
	"catalog.list.failed":      "impossible de lister les produits",
	"catalog.productNotFound":  "produit introuvable",
	"catalog.get.failed":       "impossible de charger le produit",
	"catalog.specNotFound":     "spécification introuvable",
	"catalog.spec.failed":      "impossible de charger la spécification",
	"catalog.changelog.failed": "impossible de charger le journal des modifications",

	// app
	"app.create.nameRequired":          "le nom est requis",
	"app.create.noPersonalTeam":        "aucune équipe personnelle",
	"app.create.membershipCheckFailed": "échec de la vérification d'appartenance",
	"app.create.notMember":             "vous n'êtes pas membre de cette équipe",
	"app.create.failed":                "impossible de créer l'application",
	"app.list.failed":                  "impossible de lister les applications",

	// common
	"common.invalidBody": "corps de requête invalide",

	// subscribe
	"subscribe.badAppID":                "identifiant d'application invalide",
	"subscribe.ownershipCheckFailed":    "échec de la vérification de propriété",
	"subscribe.notYourApplication":      "cette application ne vous appartient pas",
	"subscribe.productPlanRequired":     "productId et planId sont requis",
	"subscribe.productPlanNotFound":     "produit ou offre introuvable",
	"subscribe.alreadySubscribed":       "déjà abonné à ce produit",
	"subscribe.productDeprecated":       "Cette API n'accepte plus de nouveaux abonnements.",
	"subscribe.subscribeFailed":         "échec de l'abonnement",
	"subscribe.badProductID":            "identifiant de produit invalide",
	"subscribe.unsubscribeFailed":       "échec du désabonnement",
	"subscribe.credentialLoadFailed":    "impossible de charger les identifiants",
	"subscribe.subscriptionsLoadFailed": "impossible de charger les abonnements",
	"subscribe.noKeyToRotate":           "aucune clé à faire tourner — abonnez-vous et attendez d'abord l'approbation",
	"subscribe.rotationFailed":          "échec de la rotation",
	"subscribe.planLoadFailed":          "impossible de charger l'offre",
	"subscribe.sandboxUnavailable":      "bac à sable indisponible — abonnez-vous d'abord à une API compatible bac à sable",
	"subscribe.enableSandboxFailed":     "échec de l'activation du bac à sable",
	"subscribe.noSandboxKeyToRotate":    "aucune clé de bac à sable à faire tourner — activez d'abord le bac à sable",
	"subscribe.badBody":                 "corps de requête incorrect",
	"subscribe.invalidClientID":         "identifiant client invalide",
	"subscribe.oidcSetFailed":           "échec",
	"subscribe.unsupportedRange":        "plage non prise en charge",
	"subscribe.metricsUnavailable":      "métriques indisponibles",

	"subscribe.admin.listFailed":        "impossible de lister les abonnements",
	"subscribe.admin.badSubscriptionID": "identifiant d'abonnement invalide",
	"subscribe.admin.notFound":          "abonnement introuvable",
	"subscribe.admin.invalidTransition": "l'abonnement ne peut pas changer depuis son état actuel",
	"subscribe.admin.actionFailed":      "échec de l'action %s",
}
