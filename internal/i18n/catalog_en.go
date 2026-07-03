package i18n

// en holds English message strings (the pre-i18n verbatim wording).
var en = map[string]string{
	// auth
	"auth.register.credentialsRequired": "email and password (min 8 chars) required",
	"auth.password.tooLong":             "password must be at most 72 bytes",
	"auth.password.hashFailed":          "could not hash password",
	"auth.register.emailTaken":          "email already registered",
	"auth.register.createFailed":        "could not create account",
	"auth.token.issueFailed":            "could not issue token",
	"auth.login.tooManyAttempts":        "too many attempts",
	"auth.login.invalidCredentials":     "invalid credentials",
	"auth.middleware.missingToken":      "missing bearer token",
	"auth.middleware.invalidToken":      "invalid token",
	"auth.middleware.adminOnly":         "admin only",
	"auth.middleware.roleCheckFailed":   "could not verify role",

	// catalog
	"catalog.list.failed":      "failed to list products",
	"catalog.productNotFound":  "product not found",
	"catalog.get.failed":       "failed to load product",
	"catalog.specNotFound":     "spec not found",
	"catalog.spec.failed":      "failed to load spec",
	"catalog.changelog.failed": "could not load changelog",

	// app
	"app.create.nameRequired":          "name is required",
	"app.create.noPersonalTeam":        "no personal team",
	"app.create.membershipCheckFailed": "membership check failed",
	"app.create.notMember":             "not a member of that team",
	"app.create.failed":                "failed to create application",
	"app.list.failed":                  "failed to list applications",

	// common
	"common.invalidBody": "invalid body",
}
