package solclient

import (
	"context"
	"github.com/gagliardetto/solana-go"
	"math/big"
)

type LinkToken struct {
	Client *Client
	State  *solana.Wallet
}

func (l *LinkToken) Address() string {
	return l.State.PublicKey().String()
}

func (l *LinkToken) Approve(to string, amount *big.Int) error {
	panic("implement me")
}

func (l *LinkToken) Transfer(to string, amount *big.Int) error {
	panic("implement me")
}

func (l *LinkToken) BalanceOf(ctx context.Context, addr string) (*big.Int, error) {
	panic("implement me")
}

func (l *LinkToken) TransferAndCall(to string, amount *big.Int, data []byte) error {
	panic("implement me")
}

func (l *LinkToken) Name(ctx context.Context) (string, error) {
	panic("implement me")
}
