package keys

import (
	"context"
	"errors"

	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/keystore"
)

const (
	// PrefixSolana is the prefix for Solana-related keys.
	PrefixSolana = "solana"
	// PrefixTxKeystore is the prefix for transaction keys.
	PrefixTxKeystore = "tx"
)

// TxKey represents a Solana transaction signing key.
type TxKey struct {
	ks interface {
		keystore.Reader
		keystore.Signer
	}
	keyPath keystore.KeyPath
	addr    solana.PublicKey
}

// SignTxRequest contains the request to sign a transaction.
type SignTxRequest struct {
	Tx *solana.Transaction
}

// SignTxResponse contains the signed transaction.
type SignTxResponse struct {
	Tx *solana.Transaction
}

// KeyPath returns the key path for this transaction key.
func (k *TxKey) KeyPath() keystore.KeyPath {
	return k.keyPath
}

// Address returns the Solana address for this transaction key.
func (k *TxKey) Address() solana.PublicKey {
	return k.addr
}

// SignTx signs a transaction using this key.
func (k *TxKey) SignTx(ctx context.Context, req SignTxRequest) (SignTxResponse, error) {
	txMsg, err := req.Tx.Message.MarshalBinary()
	if err != nil {
		return SignTxResponse{}, err
	}

	signReq := keystore.SignRequest{
		KeyName: k.keyPath.String(),
		Data:    txMsg[:],
	}
	signResp, err := k.ks.Sign(ctx, signReq)
	if err != nil {
		return SignTxResponse{}, err
	}

	if solana.SignatureLength != len(signResp.Signature) {
		return SignTxResponse{}, errors.New("invalid signature length")
	}
	var sigArray [solana.SignatureLength]byte
	copy(sigArray[:], signResp.Signature)
	req.Tx.Signatures = append(req.Tx.Signatures, sigArray)
	return SignTxResponse(req), nil
}

// CreateTxKey creates a new transaction signing key.
// Note that key names are prefixed with PrefixSolana and PrefixTxKeystore.
// For example, a key named "test-key" will be stored at the path "solana/tx/test-key".
func CreateTxKey(ks keystore.Keystore, name string) (*TxKey, error) {
	path := keystore.NewKeyPath(PrefixSolana, PrefixTxKeystore, name)
	createReq := keystore.CreateKeysRequest{
		Keys: []keystore.CreateKeyRequest{
			{
				KeyName: path.String(),
				KeyType: keystore.Ed25519,
			},
		},
	}
	resp, err := ks.CreateKeys(context.Background(), createReq)
	if err != nil {
		return nil, err
	}
	if len(resp.Keys) == 0 {
		return nil, errors.New("no keys created")
	}
	return &TxKey{
		ks:      ks,
		keyPath: path,
		addr:    solana.PublicKeyFromBytes(resp.Keys[0].KeyInfo.PublicKey),
	}, nil
}

// GetTxKeys retrieves transaction keys by name, prepending the solana/tx prefix.
// For example, a key named "test-key" will be retrieved at the path "solana/tx/test-key".
func GetTxKeys(ctx context.Context, ks interface {
	keystore.Reader
	keystore.Signer
}, names []string) ([]*TxKey, error) {
	fullNames := make([]string, 0, len(names))
	for _, name := range names {
		fullNames = append(fullNames, keystore.NewKeyPath(PrefixSolana, PrefixTxKeystore, name).String())
	}
	resp, err := ks.GetKeys(ctx, keystore.GetKeysRequest{KeyNames: fullNames})
	if err != nil {
		return nil, err
	}

	// Note we rely on deterministic order of keys in the response
	keys := make([]*TxKey, 0, len(resp.Keys))
	for _, key := range resp.Keys {
		path := keystore.NewKeyPathFromString(key.KeyInfo.Name)
		// If no names are provided we must filter only solana keys.
		if len(names) == 0 && !strings.HasPrefix(path.String(), keystore.NewKeyPath(PrefixSolana, PrefixTxKeystore).String()) {
			continue
		}
		keys = append(keys, &TxKey{
			ks:      ks,
			keyPath: path,
			addr:    solana.PublicKeyFromBytes(key.KeyInfo.PublicKey),
		})
	}
	return keys, nil
}

// LoadTxKeys loads transaction keys from a keystore directly by name.
// Used for KMS-backed keystores where keys/key names are managed externally.
// Clients responsibility to ensure
func LoadTxKeys(ctx context.Context, ks interface {
	keystore.Reader
	keystore.Signer
}, names []string) ([]*TxKey, error) {
	resp, err := ks.GetKeys(ctx, keystore.GetKeysRequest{KeyNames: names})
	if err != nil {
		return nil, err
	}
	if len(resp.Keys) != len(names) {
		return nil, errors.New("some keys not found")
	}
	keys := make([]*TxKey, 0, len(resp.Keys))
	for _, key := range resp.Keys {
		// Sanity check the provided key is solana compatible.
		if key.KeyInfo.KeyType != keystore.Ed25519 {
			return nil, errors.New("tried to load a non-Ed25519 key: " + key.KeyInfo.Name)
		}
		keys = append(keys, &TxKey{
			ks:      ks,
			keyPath: keystore.NewKeyPathFromString(key.KeyInfo.Name),
			addr:    solana.PublicKeyFromBytes(key.KeyInfo.PublicKey),
		})
	}
	return keys, nil
}
