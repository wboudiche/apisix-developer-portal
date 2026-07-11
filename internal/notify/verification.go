package notify

import (
	"context"
	"fmt"
)

// verification email — same fr/en table pattern as emailTemplates; body args
// are (greetingName, link).
var verifyTemplates = map[string]emailTemplate{
	"fr": {
		subject: "Vérifiez votre adresse e-mail",
		body:    "Bonjour %s,\n\nConfirmez votre adresse e-mail pour activer votre compte du portail développeur :\n\n%s\n\nCe lien est valable 24 heures. Si vous n'êtes pas à l'origine de cette inscription, ignorez ce message.\n",
	},
	"en": {
		subject: "Verify your email address",
		body:    "Hello %s,\n\nConfirm your email address to activate your developer portal account:\n\n%s\n\nThis link is valid for 24 hours. If you did not sign up, you can ignore this message.\n",
	},
}

// SendVerificationEmail renders the localized verification email and sends it
// via the given Sender. lang falls back like the notifier's other emails;
// an empty name falls back to the email address in the greeting.
func SendVerificationEmail(ctx context.Context, s Sender, lang, email, name, link string) error {
	if name == "" {
		name = email
	}
	tpl := verifyTemplates[normalizeLang(lang)]
	return s.Send(ctx, []string{email}, tpl.subject, fmt.Sprintf(tpl.body, name, link))
}
