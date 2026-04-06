package message

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"sort"
	"strings"

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
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemText)))
	if block == nil {
		return anpsdk.PrivateKeyMaterial{}, fmt.Errorf("invalid key-1 private key pem")
	}
	return anpsdk.PrivateKeyMaterial{
		Type:  anpsdk.KeyTypeSecp256k1,
		Bytes: append([]byte(nil), block.Bytes...),
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

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func canonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	buffer := &strings.Builder{}
	if err := writeCanonicalJSON(buffer, normalized); err != nil {
		return nil, err
	}
	return []byte(buffer.String()), nil
}

func writeCanonicalJSON(buffer *strings.Builder, value any) error {
	switch typed := value.(type) {
	case nil:
		buffer.WriteString("null")
	case string:
		raw, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		buffer.Write(raw)
	case bool:
		if typed {
			buffer.WriteString("true")
		} else {
			buffer.WriteString("false")
		}
	case []any:
		buffer.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			if err := writeCanonicalJSON(buffer, item); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
	case []string:
		buffer.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			if err := writeCanonicalJSON(buffer, item); err != nil {
				return err
			}
		}
		buffer.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buffer.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				buffer.WriteByte(',')
			}
			if err := writeCanonicalJSON(buffer, key); err != nil {
				return err
			}
			buffer.WriteByte(':')
			if err := writeCanonicalJSON(buffer, typed[key]); err != nil {
				return err
			}
		}
		buffer.WriteByte('}')
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		var normalized any
		if err := json.Unmarshal(raw, &normalized); err != nil {
			return err
		}
		return writeCanonicalJSON(buffer, normalized)
	}
	return nil
}
