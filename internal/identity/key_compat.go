package identity

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
)

var oidSecp256k1 = asn1.ObjectIdentifier{1, 3, 132, 0, 10}

const (
	anpSecp256k1PrivateKeyLabel = "ANP SECP256K1 PRIVATE KEY"
	anpSecp256r1PrivateKeyLabel = "ANP SECP256R1 PRIVATE KEY"
	anpEd25519PrivateKeyLabel   = "ANP ED25519 PRIVATE KEY"
	anpX25519PrivateKeyLabel    = "ANP X25519 PRIVATE KEY"
)

type sec1ECPrivateKey struct {
	Version    int
	PrivateKey []byte
	Parameters asn1.ObjectIdentifier `asn1:"optional,explicit,tag:0"`
	PublicKey  asn1.BitString        `asn1:"optional,explicit,tag:1"`
}

// EnsureKey1PrivatePEMCompatible rewrites legacy ANP/private-key PEM encodings
// into the standard PKCS#8 PEM accepted by ANP SDK 0.8.5+.
func EnsureKey1PrivatePEMCompatible(path string) error {
	return ensurePrivateKeyPEMCompatible(path, "key-1 private key")
}

func ensureIdentityPrivateKeysCompatible(paths Paths) error {
	for _, item := range []struct {
		path string
		name string
	}{
		{paths.Key1PrivatePath, "key-1 private key"},
		{paths.E2EESigningPrivatePath, "e2ee signing private key"},
		{paths.E2EEAgreementPrivatePath, "e2ee agreement private key"},
	} {
		if err := ensurePrivateKeyPEMCompatible(item.path, item.name); err != nil {
			return err
		}
	}
	return nil
}

func ensurePrivateKeyPEMCompatible(path string, name string) error {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	normalized, changed, err := normalizePrivateKeyPEMToPKCS8(raw, name)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if err := writeSecureText(path, string(normalized)); err != nil {
		return fmt.Errorf("rewrite %s as standard PKCS#8 PEM: %w", name, err)
	}
	return nil
}

func normalizePrivateKeyPEMToPKCS8(raw []byte, name string) ([]byte, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false, fmt.Errorf("%w: %s is empty", ErrAuthRequired, name)
	}
	block, rest := pem.Decode(trimmed)
	if block == nil || len(bytes.TrimSpace(rest)) > 0 {
		return nil, false, fmt.Errorf("%w: invalid %s PEM structure", ErrAuthRequired, name)
	}

	switch block.Type {
	case "PRIVATE KEY":
		privateKey, err := anpsdk.PrivateKeyFromPEM(string(append(trimmed, '\n')))
		if err != nil {
			return nil, false, fmt.Errorf("%w: unsupported %s format: %v", ErrAuthRequired, name, err)
		}
		return canonicalPrivateKeyPEM(privateKey, raw)
	case "EC PRIVATE KEY":
		privateKey, err := privateKeyMaterialFromSEC1(block.Bytes)
		if err != nil {
			return nil, false, fmt.Errorf("%w: unsupported %s format (%s): %v", ErrAuthRequired, name, block.Type, err)
		}
		return canonicalPrivateKeyPEM(privateKey, raw)
	case anpEd25519PrivateKeyLabel, anpX25519PrivateKeyLabel, anpSecp256r1PrivateKeyLabel, anpSecp256k1PrivateKeyLabel:
		privateKey, err := privateKeyMaterialFromLegacyANP(block.Type, block.Bytes)
		if err != nil {
			return nil, false, fmt.Errorf("%w: unsupported %s format (%s): %v", ErrAuthRequired, name, block.Type, err)
		}
		return canonicalPrivateKeyPEM(privateKey, raw)
	default:
		return nil, false, fmt.Errorf("%w: unsupported %s PEM label %q", ErrAuthRequired, name, block.Type)
	}
}

func canonicalPrivateKeyPEM(privateKey anpsdk.PrivateKeyMaterial, original []byte) ([]byte, bool, error) {
	encoded := privateKey.ToPEM()
	if strings.TrimSpace(encoded) == "" {
		return nil, false, fmt.Errorf("private key cannot be encoded as standard PKCS#8 PEM")
	}
	normalized := []byte(encoded)
	changed := !bytes.Equal(bytes.TrimRight(original, "\n"), bytes.TrimRight(normalized, "\n"))
	return normalized, changed, nil
}

func privateKeyMaterialFromLegacyANP(label string, raw []byte) (anpsdk.PrivateKeyMaterial, error) {
	switch label {
	case anpEd25519PrivateKeyLabel:
		if len(raw) != ed25519.SeedSize {
			return anpsdk.PrivateKeyMaterial{}, fmt.Errorf("invalid Ed25519 private key length")
		}
		return anpsdk.PrivateKeyMaterial{Type: anpsdk.KeyTypeEd25519, Bytes: append([]byte(nil), raw...)}, nil
	case anpX25519PrivateKeyLabel:
		if len(raw) != 32 {
			return anpsdk.PrivateKeyMaterial{}, fmt.Errorf("invalid X25519 private key length")
		}
		return anpsdk.PrivateKeyMaterial{Type: anpsdk.KeyTypeX25519, Bytes: append([]byte(nil), raw...)}, nil
	case anpSecp256r1PrivateKeyLabel:
		if len(raw) == 0 || len(raw) > 32 {
			return anpsdk.PrivateKeyMaterial{}, fmt.Errorf("invalid secp256r1 private key length")
		}
		return anpsdk.PrivateKeyMaterial{Type: anpsdk.KeyTypeSecp256r1, Bytes: padScalarBytes(raw, 32)}, nil
	case anpSecp256k1PrivateKeyLabel:
		if err := validateSecp256k1Scalar(raw); err != nil {
			return anpsdk.PrivateKeyMaterial{}, err
		}
		return anpsdk.PrivateKeyMaterial{Type: anpsdk.KeyTypeSecp256k1, Bytes: padScalarBytes(raw, 32)}, nil
	default:
		return anpsdk.PrivateKeyMaterial{}, fmt.Errorf("unsupported legacy ANP private key label: %s", label)
	}
}

func privateKeyMaterialFromSEC1(der []byte) (anpsdk.PrivateKeyMaterial, error) {
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return privateKeyMaterialFromParsedECKey(key)
	}
	scalar, err := decodeSecp256k1SEC1PrivateScalar(der)
	if err != nil {
		return anpsdk.PrivateKeyMaterial{}, err
	}
	return anpsdk.PrivateKeyMaterial{Type: anpsdk.KeyTypeSecp256k1, Bytes: scalar}, nil
}

func privateKeyMaterialFromParsedECKey(key *ecdsa.PrivateKey) (anpsdk.PrivateKeyMaterial, error) {
	if key == nil || key.D == nil {
		return anpsdk.PrivateKeyMaterial{}, fmt.Errorf("EC private scalar is missing")
	}
	if isP256Curve(key.Curve) {
		return anpsdk.PrivateKeyMaterial{
			Type:  anpsdk.KeyTypeSecp256r1,
			Bytes: padScalarBytes(key.D.Bytes(), 32),
		}, nil
	}
	return anpsdk.PrivateKeyMaterial{}, fmt.Errorf("unsupported EC private key curve")
}

func decodeSecp256k1SEC1PrivateScalar(der []byte) ([]byte, error) {
	var key sec1ECPrivateKey
	rest, err := asn1.Unmarshal(der, &key)
	if err != nil || len(rest) != 0 {
		return nil, fmt.Errorf("parse SEC1 EC private key: %w", err)
	}
	if len(key.PrivateKey) == 0 {
		return nil, fmt.Errorf("SEC1 EC private scalar is missing")
	}
	if len(key.Parameters) > 0 && !key.Parameters.Equal(oidSecp256k1) {
		return nil, fmt.Errorf("SEC1 EC curve is not secp256k1")
	}
	if err := validateSecp256k1Scalar(key.PrivateKey); err != nil {
		return nil, err
	}
	return padScalarBytes(key.PrivateKey, 32), nil
}

func validateSecp256k1Scalar(raw []byte) error {
	if len(raw) == 0 || len(raw) > 32 {
		return fmt.Errorf("invalid secp256k1 private key length")
	}
	value := new(big.Int).SetBytes(raw)
	if value.Sign() <= 0 || value.Cmp(secp256k1Order()) >= 0 {
		return fmt.Errorf("invalid secp256k1 private key scalar")
	}
	return nil
}

func secp256k1Order() *big.Int {
	value, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	return value
}

func isP256Curve(curve elliptic.Curve) bool {
	return curve == elliptic.P256() || (curve != nil && curve.Params().Name == elliptic.P256().Params().Name)
}

func padScalarBytes(raw []byte, size int) []byte {
	result := make([]byte, size)
	copy(result[size-len(raw):], raw)
	return result
}
