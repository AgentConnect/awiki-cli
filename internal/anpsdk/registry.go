package anpsdk

import (
	anp "github.com/agent-network-protocol/anp/golang"
	anpauth "github.com/agent-network-protocol/anp/golang/authentication"
	directe2ee "github.com/agent-network-protocol/anp/golang/direct_e2ee"
)

const (
	ModulePath = "github.com/agent-network-protocol/anp/golang"
	SourcePath = "/Users/cs/work/anp/AgentConnect/golang"
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
	MessageServiceE2EEClient = directe2ee.MessageServiceDirectE2eeClient
	PrekeyBundle             = directe2ee.PrekeyBundle
	DirectSessionState       = directe2ee.DirectSessionState
)

var (
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
)
