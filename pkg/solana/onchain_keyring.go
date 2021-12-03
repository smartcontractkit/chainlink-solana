package solana

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

var (
	curve = secp256k1.S256()
)

type OnchainKeyring struct {
	privKey []byte
	pubKey  []byte
}

func NewOnchainKeyring(privKey []byte) (OnchainKeyring, error) {
	if l := len(privKey); l != 32 {
		return OnchainKeyring{}, fmt.Errorf("invalid raw key length: %d", l)
	}

	// derive public key from private key
	ecdsaD := big.NewInt(0).SetBytes(privKey)
	publicKey := ecdsa.PublicKey{Curve: curve}
	publicKey.X, publicKey.Y = curve.ScalarBaseMult(ecdsaD.Bytes())

	return OnchainKeyring{
		privKey: privKey,
		pubKey:  crypto.FromECDSAPub(&publicKey), // 65 byte length pub key
	}, nil
}

func (kr OnchainKeyring) PublicKey() types.OnchainPublicKey {
	return kr.pubKey[1:] // compressed = false, skip first byte (0x04)
}

func (kr OnchainKeyring) Sign(reportCtx types.ReportContext, report types.Report) (signature []byte, err error) {
	sigDataHash, err := HashReport(reportCtx, report)
	if err != nil {
		return []byte{}, err
	}
	return secp256k1.Sign(sigDataHash, kr.privKey)
}

func (kr OnchainKeyring) Verify(key types.OnchainPublicKey, reportCtx types.ReportContext, report types.Report, signature []byte) bool {
	sigDataHash, err := HashReport(reportCtx, report)
	if err != nil {
		log.Printf("error generating hash data: %s", err)
		return false
	}

	k, err := secp256k1.RecoverPubkey(sigDataHash, signature)
	if err != nil {
		log.Printf("error recovering public key: %s", err)
		return false
	}

	return bytes.Equal(k[1:], key[:]) // compressed = false, skip first byte (0x04)
}

func (kr OnchainKeyring) MaxSignatureLength() int {
	return 64 + 1 // 64 byte signature + 1 byte recovery id
}
