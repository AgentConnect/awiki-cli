package anpsdk

import (
	anp "github.com/agent-network-protocol/anp/golang"
	anpauth "github.com/agent-network-protocol/anp/golang/authentication"
	directe2ee "github.com/agent-network-protocol/anp/golang/direct_e2ee"
	anpproof "github.com/agent-network-protocol/anp/golang/proof"
)

const (
	ModulePath    = "github.com/agent-network-protocol/anp/golang"
	ModuleVersion = "v0.7.2"
)

type (
	KeyType                  = anp.KeyType
	PrivateKeyMaterial       = anp.PrivateKeyMaterial
	PublicKeyMaterial        = anp.PublicKeyMaterial
	GeneratedKeyPairPEM      = anp.GeneratedKeyPairPEM
	DidDocumentBundle        = anpauth.DidDocumentBundle
	DidDocumentOptions       = anpauth.DidDocumentOptions
	DIDWbaAuthHeader         = anpauth.DIDWbaAuthHeader
	DidWbaVerifierConfig     = anpauth.DidWbaVerifierConfig
	AuthMode                 = anpauth.AuthMode
	HttpSignatureOptions     = anpauth.HttpSignatureOptions
	MessageServiceE2EEClient = directe2ee.MessageServiceDirectE2eeClient
	PrekeyBundle             = directe2ee.PrekeyBundle
	DirectSessionState       = directe2ee.DirectSessionState
	IMProof                  = anpproof.IMProof
	IMGenerationOptions      = anpproof.IMGenerationOptions
	ParsedIMSignatureInput   = anpproof.ParsedIMSignatureInput
)

var (
	KeyTypeSecp256k1                   = anp.KeyTypeSecp256k1
	KeyTypeSecp256r1                   = anp.KeyTypeSecp256r1
	KeyTypeEd25519                     = anp.KeyTypeEd25519
	KeyTypeX25519                      = anp.KeyTypeX25519
	AuthModeHTTPSignatures             = anpauth.AuthModeHTTPSignatures
	AuthModeLegacyDidWba               = anpauth.AuthModeLegacyDidWba
	AuthModeAuto                       = anpauth.AuthModeAuto
	GenerateKeyPairPEM                 = anp.GenerateKeyPairPEM
	PrivateKeyFromPEM                  = anp.PrivateKeyFromPEM
	PublicKeyFromPEM                   = anp.PublicKeyFromPEM
	CreateDidWBADocument               = anpauth.CreateDidWBADocument
	CreateDidWBADocumentWithKeyBinding = anpauth.CreateDidWBADocumentWithKeyBinding
	ResolveDidDocument                 = anpauth.ResolveDidDocument
	ResolveDidDocumentWithOptions      = anpauth.ResolveDidDocumentWithOptions
	GenerateAuthHeader                 = anpauth.GenerateAuthHeader
	GenerateHTTPSignatureHeaders       = anpauth.GenerateHTTPSignatureHeaders
	NewDIDWbaAuthHeader                = anpauth.NewDIDWbaAuthHeader
	NewDidWbaVerifier                  = anpauth.NewDidWbaVerifier
	NewFileSessionStore                = directe2ee.NewFileSessionStore
	NewFileSignedPrekeyStore           = directe2ee.NewFileSignedPrekeyStore
	NewFilePendingOutboundStore        = directe2ee.NewFilePendingOutboundStore
	NewMessageServiceDirectE2eeClient  = directe2ee.NewMessageServiceDirectE2eeClient
	BuildIMContentDigest               = anpproof.BuildIMContentDigest
	BuildIMSignatureInput              = anpproof.BuildIMSignatureInput
	ParseIMSignatureInput              = anpproof.ParseIMSignatureInput
	EncodeIMSignature                  = anpproof.EncodeIMSignature
	GenerateIMProof                    = anpproof.GenerateIMProof
	VerifyIMProofWithDocument          = anpproof.VerifyIMProofWithDocument
)
