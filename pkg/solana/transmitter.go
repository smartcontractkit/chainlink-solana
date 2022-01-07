package solana

import (
	"bytes"
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

var _ types.ContractTransmitter = (*ContractTracker)(nil)

// Transmit sends the report to the on-chain OCR2Aggregator smart contract's Transmit method
func (c *ContractTracker) Transmit(
	ctx context.Context,
	reportCtx types.ReportContext,
	report types.Report,
	sigs []types.AttributedOnchainSignature,
) error {
	recent, err := c.client.rpc.GetRecentBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.GetRecentBlock")
	}

	// Determine store authority
	seeds := [][]byte{[]byte("store"), c.StateID.Bytes()}
	storeAuthority, storeNonce, err := solana.FindProgramAddress(seeds, c.ProgramID)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.FindProgramAddress")
	}

	accounts := []*solana.AccountMeta{
		// state, transmitter, transmissions, store_program, store, store_authority
		{PublicKey: c.StateID, IsWritable: true, IsSigner: false},
		{PublicKey: c.Transmitter.PublicKey(), IsWritable: false, IsSigner: true},
		{PublicKey: c.TransmissionsID, IsWritable: true, IsSigner: false},
		{PublicKey: c.StoreProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: c.store, IsWritable: true, IsSigner: false},
		{PublicKey: storeAuthority, IsWritable: false, IsSigner: false},
	}

	reportContext := RawReportContext(reportCtx)

	// Construct the instruction payload
	data := new(bytes.Buffer) // store_nonce || report_context || raw_report || raw_signatures
	data.WriteByte(storeNonce)
	data.Write(reportContext[0][:])
	data.Write(reportContext[1][:])
	data.Write(reportContext[2][:])
	data.Write([]byte(report))
	for _, sig := range sigs {
		// Signature = 64 bytes + 1 byte recovery id
		data.Write(sig.Signature)
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			solana.NewInstruction(c.ProgramID, accounts, data.Bytes()),
		},
		recent.Value.Blockhash,
		solana.TransactionPayer(c.Transmitter.PublicKey()),
	)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.NewTransaction")
	}

	msgToSign, err := tx.Message.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "error on Transmit.Message.MarshalBinary")
	}
	finalSigBytes, err := c.Transmitter.Sign(msgToSign)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.Sign")
	}
	var finalSig [64]byte
	copy(finalSig[:], finalSigBytes)
	tx.Signatures = append(tx.Signatures, finalSig)

	// Send transaction, and wait for confirmation:
	go func() {
		txSig, err := c.client.rpc.SendTransactionWithOpts(
			context.Background(), // does not use libocr transmit context
			tx,
			false, // use preflight as first check
			rpc.CommitmentConfirmed,
		)

		if err != nil {
			c.lggr.Errorf("error on Transmit.SendAndConfirmTransaction: %s", err.Error())
		}
		// TODO: poll rpc for tx confirmation (WS connection unreliable)
		// TODO: see if SendTransactionWithOpts returns quickly enough to be used outside of goroutine
		c.lggr.Debugf("tx signature from Transmit.SendAndConfirmTransaction: %s", txSig.String())
	}()
	return nil
}

func (c *ContractTracker) LatestConfigDigestAndEpoch(
	ctx context.Context,
) (
	configDigest types.ConfigDigest,
	epoch uint32,
	err error,
) {
	err = c.fetchState(ctx)
	return c.state.Config.LatestConfigDigest, c.state.Config.Epoch, err
}

func (c ContractTracker) FromAccount() types.Account {
	return types.Account(c.Transmitter.PublicKey().String())
}
