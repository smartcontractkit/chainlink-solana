package solana

import (
	"bytes"
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

var _ types.ContractTransmitter = (*ContractTransmitter)(nil)

type ContractTransmitter struct {
	client *Client
	cache  *ContractCache
	signer TransmissionSigner
	lggr   Logger
}

// Transmit sends the report to the on-chain OCR2Aggregator smart contract's Transmit method
func (c *ContractTransmitter) Transmit(
	ctx context.Context,
	reportCtx types.ReportContext,
	report types.Report,
	sigs []types.AttributedOnchainSignature,
) error {
	recent, err := c.client.rpc.GetRecentBlockhash(ctx, c.client.commitment)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.GetRecentBlock")
	}

	// Determine store authority
	seeds := [][]byte{[]byte("store"), c.cache.StateID.Bytes()}
	storeAuthority, storeNonce, err := solana.FindProgramAddress(seeds, c.cache.ProgramID)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.FindProgramAddress")
	}

	_, store, err := c.cache.ReadState()
	if err != nil {
		return errors.Wrap(err, "error on Transmit.ReadState")
	}
	accounts := []*solana.AccountMeta{
		// state, transmitter, transmissions, store_program, store, store_authority
		{PublicKey: c.cache.StateID, IsWritable: true, IsSigner: false},
		{PublicKey: c.signer.PublicKey(), IsWritable: false, IsSigner: true},
		{PublicKey: c.cache.TransmissionsID, IsWritable: true, IsSigner: false},
		{PublicKey: c.cache.StoreProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: store, IsWritable: true, IsSigner: false},
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
			solana.NewInstruction(c.cache.ProgramID, accounts, data.Bytes()),
		},
		recent.Value.Blockhash,
		solana.TransactionPayer(c.signer.PublicKey()),
	)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.NewTransaction")
	}

	msgToSign, err := tx.Message.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "error on Transmit.Message.MarshalBinary")
	}
	finalSigBytes, err := c.signer.Sign(msgToSign)
	if err != nil {
		return errors.Wrap(err, "error on Transmit.Sign")
	}
	var finalSig [64]byte
	copy(finalSig[:], finalSigBytes)
	tx.Signatures = append(tx.Signatures, finalSig)

	// Send transaction, and wait for confirmation:
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), c.client.txTimeout)
		defer cancel()
		txSig, err := c.client.rpc.SendTransactionWithOpts(
			ctx, // does not use libocr transmit context
			tx,
			c.client.skipPreflight,
			c.client.commitment,
		)

		if err != nil {
			c.lggr.Errorf("error on Transmit.SendAndConfirmTransaction: %s", err.Error())
			return
		}
		// TODO: poll rpc for tx confirmation (WS connection unreliable)
		c.lggr.Debugf("tx signature from Transmit.SendAndConfirmTransaction: %s", txSig.String())
	}()
	return nil
}

func (c *ContractTransmitter) LatestConfigDigestAndEpoch(
	ctx context.Context,
) (
	configDigest types.ConfigDigest,
	epoch uint32,
	err error,
) {
	state, _, err := c.cache.ReadState()
	return state.Config.LatestConfigDigest, state.Config.Epoch, err
}

func (c *ContractTransmitter) FromAccount() types.Account {
	return types.Account(c.signer.PublicKey().String())
}
