package monitoring

import (
	"context"

	"github.com/gagliardetto/solana-go"
)

type solanaSource struct {
	account solana.PublicKey
	reader  AccountReader
}

func NewSolanaSource(account solana.PublicKey, reader AccountReader) Source {
	return &solanaSource{account, reader}
}

func (s *solanaSource) Name() string {
	return "solana"
}

func (s *solanaSource) Fetch(ctx context.Context) (interface{}, error) {
	return s.reader.Read(ctx, s.account)
}
