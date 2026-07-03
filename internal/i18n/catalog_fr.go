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
}
