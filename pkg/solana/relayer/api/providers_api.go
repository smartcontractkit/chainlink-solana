package api

import (
	"context"
	"errors"

	solanago "github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-common/pkg/types/core"
)

func NewDummyKeystoreService(privateKeys []solanago.PrivateKey) core.Keystore {
	return DummyKeystoreService{
		privateKeys: privateKeys,
	}
}

type DummyKeystoreService struct {
	privateKeys []solanago.PrivateKey
}

func (dks DummyKeystoreService) Accounts(ctx context.Context) (accounts []string, err error) {
	return []string{dks.privateKeys[0].PublicKey().String()}, nil
}

func (dks DummyKeystoreService) Sign(ctx context.Context, account string, data []byte) (signed []byte, err error) {
	if account != dks.privateKeys[0].PublicKey().String() {
		return nil, errors.New("Public key " + dks.privateKeys[0].PublicKey().String() + " not stored in Keystore")
	}
	sig, err := dks.privateKeys[0].Sign(data)
	return sig[:], err
}
