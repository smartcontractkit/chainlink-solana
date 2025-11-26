package keys

import (
	"context"
	"errors"

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
	ks      keystore.Keystore
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

	// why?
	var finalSig [64]byte
	copy(finalSig[:], signResp.Signature)
	req.Tx.Signatures = append(req.Tx.Signatures, finalSig)
	return SignTxResponse{Tx: req.Tx}, nil
}

// CreateTxKey creates a new transaction signing key.
// Note that key names are prefixed with PrefixSolana and PrefixTxKeystore.
// For example, a key named "test-key" will be stored at the path "solana/test-key".
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

// GetTxKeys retrieves transaction keys by name.
func GetTxKeys(ctx context.Context, ks keystore.Keystore, names []string) ([]*TxKey, error) {
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
		keys = append(keys, &TxKey{
			ks:      ks,
			keyPath: keystore.NewKeyPathFromString(key.KeyInfo.Name),
			addr:    solana.PublicKeyFromBytes(key.KeyInfo.PublicKey),
		})
	}
	return keys, nil
}
