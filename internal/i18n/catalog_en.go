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

	// subscribe
	"subscribe.badAppID":                "bad application id",
	"subscribe.ownershipCheckFailed":    "ownership check failed",
	"subscribe.notYourApplication":      "not your application",
	"subscribe.productPlanRequired":     "productId and planId are required",
	"subscribe.productPlanNotFound":     "product or plan not found",
	"subscribe.alreadySubscribed":       "already subscribed to this product",
	"subscribe.productDeprecated":       "This API no longer accepts new subscriptions.",
	"subscribe.subscribeFailed":         "subscription failed",
	"subscribe.badProductID":            "bad product id",
	"subscribe.unsubscribeFailed":       "unsubscribe failed",
	"subscribe.credentialLoadFailed":    "failed to load credential",
	"subscribe.subscriptionsLoadFailed": "failed to load subscriptions",
	"subscribe.noKeyToRotate":           "no key to rotate — subscribe and get approved first",
	"subscribe.rotationFailed":          "rotation failed",
	"subscribe.planLoadFailed":          "failed to load plan",
	"subscribe.sandboxUnavailable":      "sandbox unavailable — subscribe to a sandbox-enabled API first",
	"subscribe.enableSandboxFailed":     "enable sandbox failed",
	"subscribe.noSandboxKeyToRotate":    "no sandbox key to rotate — enable sandbox first",
	"subscribe.badBody":                 "bad body",
	"subscribe.invalidClientID":         "invalid client id",
	"subscribe.oidcSetFailed":           "failed",
	"subscribe.unsupportedRange":        "unsupported range",
	"subscribe.metricsUnavailable":      "metrics unavailable",

	"subscribe.admin.listFailed":        "failed to list subscriptions",
	"subscribe.admin.badSubscriptionID": "bad subscription id",
	"subscribe.admin.notFound":          "subscription not found",
	"subscribe.admin.invalidTransition": "subscription cannot change from its current state",
	"subscribe.admin.actionFailed":      "%s failed",
}
