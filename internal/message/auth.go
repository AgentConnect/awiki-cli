package message

import (
	"fmt"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	"github.com/agentconnect/awiki-cli/internal/authsdk"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

type authContext struct {
	record     *identity.StoredIdentity
	session    *authsdk.Session
	privateKey anpsdk.PrivateKeyMaterial
}

func newAuthContext(record *identity.StoredIdentity, manager *identity.Manager) (*authContext, error) {
	if record == nil {
		return nil, fmt.Errorf("identity record is required")
	}
	privateKey, err := loadPrivateKeyMaterial(record.Key1PrivatePEM)
	if err != nil {
		return nil, err
	}
	var session *authsdk.Session
	if manager != nil {
		paths, err := manager.PathsForIdentity(record.IdentityName)
		if err != nil {
			return nil, err
		}
		session = authsdk.NewSession(paths.DIDDocumentPath, paths.Key1PrivatePath, record.IdentityName, record.DID, record.JWTToken, func(token string) error { return manager.UpdateJWT(record.IdentityName, token) })
	}
	return &authContext{
		record:     record,
		session:    session,
		privateKey: privateKey,
	}, nil
}

func loadPrivateKeyMaterial(pemText string) (anpsdk.PrivateKeyMaterial, error) {
	_, scalar, err := authsdk.NormalizeSecp256k1PrivatePEM(pemText)
	if err != nil {
		return anpsdk.PrivateKeyMaterial{}, err
	}
	return anpsdk.PrivateKeyMaterial{
		Type:  anpsdk.KeyTypeSecp256k1,
		Bytes: scalar,
	}, nil
}

func verificationMethodID(didDocument map[string]any) string {
	if didDocument == nil {
		return ""
	}
	if authenticationMethods, ok := didDocument["authentication"].([]any); ok && len(authenticationMethods) > 0 {
		if methodID, ok := authenticationMethods[0].(string); ok {
			return methodID
		}
	}
	if verificationMethods, ok := didDocument["verificationMethod"].([]any); ok && len(verificationMethods) > 0 {
		if method, ok := verificationMethods[0].(map[string]any); ok {
			if methodID, ok := method["id"].(string); ok {
				return methodID
			}
		}
	}
	return ""
}
