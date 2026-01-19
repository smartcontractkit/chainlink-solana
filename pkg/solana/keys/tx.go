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

// GetTxKeysOption configures GetTxKeys behavior.
type GetTxKeysOption func(*getTxKeysOptions)

type getTxKeysOptions struct {
	noPrefix bool
}

// WithNoPrefix disables adding the solana/tx prefix to key names.
// When set, names are used as-is (useful for keystores with externally managed names
// like KMS-backed keystores).
func WithNoPrefix() GetTxKeysOption {
	return func(opts *getTxKeysOptions) {
		opts.noPrefix = true
	}
}

// GetTxKeys retrieves transaction keys by name.
// By default, prepends the solana/tx prefix to names.
// For example, a key named "test-key" will be retrieved at the path "solana/tx/test-key".
// Use WithNoPrefix() to use names as-is (for KMS-backed keystores).
func GetTxKeys(ctx context.Context, ks interface {
	keystore.Reader
	keystore.Signer
}, names []string, opts ...GetTxKeysOption) ([]*TxKey, error) {
	options := &getTxKeysOptions{}
	for _, opt := range opts {
		opt(options)
	}

	fullNames := make([]string, 0, len(names))
	if options.noPrefix {
		fullNames = names
	} else {
		for _, name := range names {
			fullNames = append(fullNames, keystore.NewKeyPath(PrefixSolana, PrefixTxKeystore, name).String())
		}
	}

	resp, err := ks.GetKeys(ctx, keystore.GetKeysRequest{KeyNames: fullNames})
	if err != nil {
		return nil, err
	}

	// Always require all requested keys to be found
	if len(names) > 0 && len(resp.Keys) != len(names) {
		return nil, errors.New("some keys not found")
	}

	// Note we rely on deterministic order of keys in the response
	keys := make([]*TxKey, 0, len(resp.Keys))
	prefixPath := keystore.NewKeyPath(PrefixSolana, PrefixTxKeystore)
	for _, key := range resp.Keys {
		path := keystore.NewKeyPathFromString(key.KeyInfo.Name)

		// If no prefix, sanity check key type (for KMS-backed keystores)
		if options.noPrefix {
			if key.KeyInfo.KeyType != keystore.Ed25519 {
				return nil, errors.New("tried to load a non-Ed25519 key: " + key.KeyInfo.Name)
			}
		}

		// If no names are provided and we're using prefix, filter only solana keys
		if !options.noPrefix && len(names) == 0 && !path.HasPrefix(prefixPath) {
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
