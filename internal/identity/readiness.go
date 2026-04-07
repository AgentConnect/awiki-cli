package identity

import (
	"fmt"
	"strings"
)

func EvaluateUserState(userID string, handle string) UserState {
	missing := make([]string, 0, 2)
	if strings.TrimSpace(userID) == "" {
		missing = append(missing, "registration")
	}
	if strings.TrimSpace(handle) == "" {
		missing = append(missing, "handle")
	}

	state := UserState{
		RegistrationState: "registered_user",
		ReadyForMessaging: len(missing) == 0,
	}
	if len(missing) > 0 {
		state.Missing = missing
		switch len(missing) {
		case 2:
			state.RegistrationState = "local_identity"
		default:
			state.RegistrationState = "partial_user"
		}
	}
	return state
}

func EvaluateStoredIdentityUserState(record *StoredIdentity) UserState {
	if record == nil {
		return UserState{
			RegistrationState: "missing_identity",
			ReadyForMessaging: false,
			Missing:           []string{"identity"},
		}
	}
	return EvaluateUserState(record.UserID, record.Handle)
}

func EvaluateIdentitySummaryUserState(summary *IdentitySummary) UserState {
	if summary == nil {
		return UserState{
			RegistrationState: "missing_identity",
			ReadyForMessaging: false,
			Missing:           []string{"identity"},
		}
	}
	return EvaluateUserState(summary.UserID, summary.Handle)
}

func UserRegistrationError(identityName string, state UserState) error {
	if state.ReadyForMessaging {
		return nil
	}
	name := strings.TrimSpace(identityName)
	if name == "" {
		name = "active identity"
	}
	missing := "user registration metadata"
	if len(state.Missing) > 0 {
		missing = strings.Join(state.Missing, ", ")
	}
	return fmt.Errorf(
		"%w: identity %s is %s and missing %s; complete user setup with `awiki-cli id register --handle <handle> ...` or recover an existing handle first",
		ErrUserRegistrationRequired,
		name,
		state.RegistrationState,
		missing,
	)
}
