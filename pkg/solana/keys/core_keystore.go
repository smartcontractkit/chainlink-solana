package keys

import (
	"context"
	"errors"
	"sync"

	"github.com/smartcontractkit/chainlink-common/keystore"
	"github.com/smartcontractkit/chainlink-common/pkg/types/core"
)

var _ core.Keystore = &TxKeyCoreKeystore{}

type TxKeyCoreKeystore struct {
	ks interface {
		keystore.Reader
		keystore.Signer
	}
	cacheMu         sync.RWMutex
	cache           map[string]string
	allowedKeyNames []string
}

type Option func(*TxKeyCoreKeystore)

// Filter key names for example if using KMS and only certain key names are accessible.
// (may not have ListKeys permission)
func WithAllowedKeyNames(names []string) Option {
	return func(s *TxKeyCoreKeystore) {
		s.allowedKeyNames = names
	}
}

// NewTxKeyCoreKeystore creates a new CoreKeystore for transaction keys.
// This wrapper is required for using TxKeys with the txm
// which requires address based lookups.
func NewTxKeyCoreKeystore(ks interface {
	keystore.Reader
	keystore.Signer
}, options ...Option) *TxKeyCoreKeystore {
	txKeyCoreKeystore := &TxKeyCoreKeystore{
		ks:              ks,
		cache:           make(map[string]string),
		allowedKeyNames: []string{},
	}
	for _, opt := range options {
		opt(txKeyCoreKeystore)
	}
	return txKeyCoreKeystore
}

func (s *TxKeyCoreKeystore) getKeys(ctx context.Context) ([]*TxKey, error) {
	if len(s.allowedKeyNames) != 0 {
		return GetTxKeys(ctx, s.ks, s.allowedKeyNames, WithNoPrefix())
	}
	return GetTxKeys(ctx, s.ks, []string{})
}

func (s *TxKeyCoreKeystore) Accounts(ctx context.Context) ([]string, error) {
	keys, err := s.getKeys(ctx)
	if err != nil {
		return nil, err
	}
	accounts := make([]string, 0, len(keys))
	for _, key := range keys {
		accounts = append(accounts, key.Address().String())
	}
	return accounts, nil
}

func (s *TxKeyCoreKeystore) Sign(ctx context.Context, account string, data []byte) ([]byte, error) {
	s.cacheMu.RLock()
	keyPath, ok := s.cache[account]
	s.cacheMu.RUnlock()
	if ok {
		return s.getSignature(ctx, keyPath, data)
	}
	// Otherwise do the first time lookup to find the key by address.
	keys, err := s.getKeys(ctx)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, errors.New("no keys found")
	}
	for _, key := range keys {
		if key.Address().String() == account {
			s.cacheMu.Lock()
			s.cache[account] = key.KeyPath().String()
			s.cacheMu.Unlock()
			return s.getSignature(ctx, key.KeyPath().String(), data)
		}
	}
	return nil, errors.New("key not found")
}

func (s *TxKeyCoreKeystore) getSignature(ctx context.Context, keyName string, data []byte) ([]byte, error) {
	resp, err := s.ks.Sign(ctx, keystore.SignRequest{
		KeyName: keyName,
		Data:    data,
	})
	if err != nil {
		return nil, err
	}
	return resp.Signature, nil
}

func (s *TxKeyCoreKeystore) Decrypt(ctx context.Context, account string, data []byte) ([]byte, error) {
	return nil, errors.New("not implemented")
}
