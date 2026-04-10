package anpsdk

import (
	anp "github.com/agent-network-protocol/anp/golang"
	anpauth "github.com/agent-network-protocol/anp/golang/authentication"
	directe2ee "github.com/agent-network-protocol/anp/golang/direct_e2ee"
	anpproof "github.com/agent-network-protocol/anp/golang/proof"
)

const (
	ModulePath    = "github.com/agent-network-protocol/anp/golang"
	ModuleVersion = "v0.8.2"
)

type (
	KeyType                               = anp.KeyType
	DidProfile                            = anpauth.DidProfile
	PrivateKeyMaterial                    = anp.PrivateKeyMaterial
	PublicKeyMaterial                     = anp.PublicKeyMaterial
	GeneratedKeyPairPEM                   = anp.GeneratedKeyPairPEM
	DidDocumentBundle                     = anpauth.DidDocumentBundle
	DidDocumentOptions                    = anpauth.DidDocumentOptions
	AnpMessageServiceOptions              = anpauth.AnpMessageServiceOptions
	DIDWbaAuthHeader                      = anpauth.DIDWbaAuthHeader
	DidWbaVerifierConfig                  = anpauth.DidWbaVerifierConfig
	AuthMode                              = anpauth.AuthMode
	HttpSignatureOptions                  = anpauth.HttpSignatureOptions
	MessageServiceE2EEClient              = directe2ee.MessageServiceDirectE2eeClient
	PrekeyBundle                          = directe2ee.PrekeyBundle
	DirectSessionState                    = directe2ee.DirectSessionState
	IMProof                               = anpproof.IMProof
	IMGenerationOptions                   = anpproof.IMGenerationOptions
	ParsedIMSignatureInput                = anpproof.ParsedIMSignatureInput
	TargetKind                            = anpproof.TargetKind
	SignedRequestObject                   = anpproof.SignedRequestObject
	RFC9421OriginProof                    = anpproof.RFC9421OriginProof
	RFC9421OriginProofGenerationOptions   = anpproof.RFC9421OriginProofGenerationOptions
	RFC9421OriginProofVerificationOptions = anpproof.RFC9421OriginProofVerificationOptions
)

var (
	KeyTypeSecp256r1                  = anp.KeyTypeSecp256r1
	KeyTypeEd25519                    = anp.KeyTypeEd25519
	KeyTypeX25519                     = anp.KeyTypeX25519
	DidProfileE1                      = anpauth.DidProfileE1
	AuthModeHTTPSignatures            = anpauth.AuthModeHTTPSignatures
	AuthModeAuto                      = anpauth.AuthModeAuto
	GenerateKeyPairPEM                = anp.GenerateKeyPairPEM
	PrivateKeyFromPEM                 = anp.PrivateKeyFromPEM
	PublicKeyFromPEM                  = anp.PublicKeyFromPEM
	BuildANPMessageService            = anpauth.BuildANPMessageService
	CreateDidWBADocument              = anpauth.CreateDidWBADocument
	ResolveDidDocument                = anpauth.ResolveDidDocument
	ResolveDidDocumentWithOptions     = anpauth.ResolveDidDocumentWithOptions
	GenerateAuthHeader                = anpauth.GenerateAuthHeader
	GenerateHTTPSignatureHeaders      = anpauth.GenerateHTTPSignatureHeaders
	NewDIDWbaAuthHeader               = anpauth.NewDIDWbaAuthHeader
	NewDidWbaVerifier                 = anpauth.NewDidWbaVerifier
	NewFileSessionStore               = directe2ee.NewFileSessionStore
	NewFileSignedPrekeyStore          = directe2ee.NewFileSignedPrekeyStore
	NewFilePendingOutboundStore       = directe2ee.NewFilePendingOutboundStore
	NewMessageServiceDirectE2eeClient = directe2ee.NewMessageServiceDirectE2eeClient
	BuildIMContentDigest              = anpproof.BuildIMContentDigest
	BuildIMSignatureInput             = anpproof.BuildIMSignatureInput
	ParseIMSignatureInput             = anpproof.ParseIMSignatureInput
	EncodeIMSignature                 = anpproof.EncodeIMSignature
	GenerateIMProof                   = anpproof.GenerateIMProof
	VerifyIMProofWithDocument         = anpproof.VerifyIMProofWithDocument
	GenerateGroupReceiptProof         = anpproof.GenerateGroupReceiptProof
	VerifyGroupReceiptProof           = anpproof.VerifyGroupReceiptProof
	BuildSignedRequestObject          = anpproof.BuildSignedRequestObject
	CanonicalizeSignedRequestObject   = anpproof.CanonicalizeSignedRequestObject
	BuildLogicalTargetURI             = anpproof.BuildLogicalTargetURI
	BuildRFC9421OriginSignatureBase   = anpproof.BuildRFC9421OriginSignatureBase
	GenerateRFC9421OriginProof        = anpproof.GenerateRFC9421OriginProof
	VerifyRFC9421OriginProof          = anpproof.VerifyRFC9421OriginProof
	TargetKindAgent                   = anpproof.TargetKindAgent
	TargetKindGroup                   = anpproof.TargetKindGroup
	TargetKindService                 = anpproof.TargetKindService
)
