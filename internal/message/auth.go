package message

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

type authContext struct {
	record          *identity.StoredIdentity
	identityManager *identity.Manager
	privateKey      anpsdk.PrivateKeyMaterial
}

func newAuthContext(record *identity.StoredIdentity, manager *identity.Manager) (*authContext, error) {
	if record == nil {
		return nil, fmt.Errorf("identity record is required")
	}
	privateKey, err := loadPrivateKeyMaterial(record.Key1PrivatePEM)
	if err != nil {
		return nil, err
	}
	return &authContext{
		record:          record,
		identityManager: manager,
		privateKey:      privateKey,
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

func (a *authContext) authHeaders(requestURL string, method string, body []byte, forceNew bool) (map[string]string, error) {
	if !forceNew && strings.TrimSpace(a.record.JWTToken) != "" {
		return map[string]string{"Authorization": "Bearer " + a.record.JWTToken}, nil
	}
	if a.record.DIDDocument == nil {
		return nil, fmt.Errorf("identity %s is missing a DID document", a.record.IdentityName)
	}
	headers := map[string]string{"Content-Type": "application/json"}
	return anpsdk.GenerateHTTPSignatureHeaders(
		a.record.DIDDocument,
		requestURL,
		method,
		a.privateKey,
		headers,
		body,
		anpsdk.HttpSignatureOptions{},
	)
}

func (a *authContext) captureResponseToken(response *http.Response) {
	if response == nil {
		return
	}
	token := bearerFromHeaders(response.Header)
	if token == "" || a.identityManager == nil {
		return
	}
	if token == a.record.JWTToken {
		return
	}
	if err := a.identityManager.UpdateJWT(a.record.IdentityName, token); err == nil {
		a.record.JWTToken = token
	}
}

func bearerFromHeaders(headers http.Header) string {
	if authorization := strings.TrimSpace(headers.Get("Authorization")); strings.HasPrefix(authorization, "Bearer ") {
		return strings.TrimPrefix(authorization, "Bearer ")
	}
	if info := strings.TrimSpace(headers.Get("Authentication-Info")); info != "" {
		for _, part := range strings.Split(info, ",") {
			key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				continue
			}
			if key == "access_token" {
				return strings.Trim(strings.TrimSpace(value), "\"")
			}
		}
	}
	return ""
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
