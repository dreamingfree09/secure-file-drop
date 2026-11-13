package server

import (
	"errors"
	"os"
	"strings"
)

// LoadRegistrationInvitationsConfig requires the independently gated schema15
// and a canonical HTTPS origin. Older deployment modes remain off by default.
func LoadRegistrationInvitationsConfig(managedLinks bool) (bool, error) {
	switch os.Getenv("SFD_REGISTRATION_INVITATIONS_ENABLED") {
	case "", "false", "0":
		return false, nil
	case "true":
		origin, err := publicOrigin()
		if !managedLinks || err != nil || !strings.HasPrefix(origin, "https://") {
			return false, errors.New("registration invitations require managed storage and HTTPS")
		}
		return true, nil
	default:
		return false, errors.New("invalid registration invitation configuration")
	}
}

// The new alternative is consulted only in explicitly schema16-enabled auth.
// A role elevation or missing redeemed ledger record invalidates this method.
const invitationIdentityPredicate = `(email_verified=TRUE OR
	(is_admin=FALSE AND registration_method='administrator_invitation' AND
	 EXISTS(SELECT 1 FROM registration_invitations ri WHERE ri.id=users.registration_invitation_id
	 AND ri.redeemed_by=users.id AND ri.redeemed_at IS NOT NULL AND ri.revoked_at IS NULL)))`
