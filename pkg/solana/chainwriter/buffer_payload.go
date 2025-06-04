package chainwriter

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/go-viper/mapstructure/v2"
	"github.com/google/uuid"

	"github.com/smartcontractkit/chainlink-common/pkg/types"

	ccipsolana "github.com/smartcontractkit/chainlink-ccip/chains/solana"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
)

func FindCreateBufferInstructionsMethod(id string) (func(context.Context, any, solana.AccountMetaSlice, solana.PublicKey, solana.PublicKey) ([]solana.Instruction, solana.Instruction, solana.AccountMetaSlice, any, error), error) {
	switch id {
	case "CCIPExecutionReportBuffer":
		return CCIPExecutionReportBuffer, nil
	default:
		return nil, fmt.Errorf("transform not found")
	}
}

// CCIPExecutionReportBuffer creates the list of instructions needed to write to an on-chain buffer for large transactions. It creates a close buffer instruction for cleanup in case of any failures.
func CCIPExecutionReportBuffer(ctx context.Context, args any, accounts solana.AccountMetaSlice, programID, feePayer solana.PublicKey) ([]solana.Instruction, solana.Instruction, solana.AccountMetaSlice, any, error) {
	var execCallArgs ccipsolana.SVMExecCallArgs
	err := mapstructure.Decode(args, &execCallArgs)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// Extract raw report and root from args
	rawReport := execCallArgs.Report
	bufferID, err := uuid.New().MarshalBinary()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to marshal uuid into bytes: %w", err)
	}

	bufferPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("execution_report_buffer"), bufferID, feePayer.Bytes()}, programID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to calculate buffer PDA: %w", err)
	}
	offrampConfigPDA, _, err := state.FindOfframpConfigPDA(programID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to calculate offramp condig PDA: %w", err)
	}

	// Create empty buffer instruction to calculate an accurate chunk size
	emptyBufferIx, err := buildBufferExecutionReportIx(bufferID, uint32(len(rawReport)), []byte{}, 0, bufferPDA, offrampConfigPDA, feePayer) //nolint:gosec // length of raw report can never exceed the uint32 max
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to build empty buffer instruction: %w", err)
	}

	chunks, err := extractChunks(rawReport, emptyBufferIx)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to extract chunks: %w", err)
	}
	bufferIxs := make([]solana.Instruction, 0, len(chunks))

	for i, chunkPayload := range chunks {
		ix, ixErr := buildBufferExecutionReportIx(bufferID, uint32(len(rawReport)), chunkPayload, uint8(i), bufferPDA, offrampConfigPDA, feePayer) //nolint:gosec // length of raw report can never exceed the uint32 max
		if ixErr != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to build buffer instruction: %w", ixErr)
		}

		bufferIxs = append(bufferIxs, ix)
	}

	// Append buffer PDA at the end of the accounts list since it is expected to be the last account
	accounts = append(accounts, &solana.AccountMeta{
		PublicKey:  bufferPDA,
		IsWritable: true,
		IsSigner:   false,
	})

	closeBufferIx, err := ccip_offramp.NewCloseExecutionReportBufferInstruction(bufferID, bufferPDA, feePayer, solana.SystemProgramID).ValidateAndBuild()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to build close execution report buffer instruction: %w", err)
	}

	// Transform args to empty out the report since the buffer will be used instead
	execCallArgs.Report = []byte{}

	return bufferIxs, closeBufferIx, accounts, execCallArgs, nil
}

func buildBufferExecutionReportIx(bufferID []byte, reportLen uint32, chunkPayload []byte, index uint8, bufferPDA, offrampConfigPDA, feePayer solana.PublicKey) (solana.Instruction, error) {
	ix, ixErr := ccip_offramp.NewBufferExecutionReportInstruction(
		bufferID,
		reportLen,
		chunkPayload,
		index,
		bufferPDA,
		offrampConfigPDA,
		feePayer,
		solana.SystemProgramID).ValidateAndBuild()
	if ixErr != nil {
		return nil, fmt.Errorf("failed to build buffer instruction: %w", ixErr)
	}
	return ix, nil
}

// sendBufferInstructions enqueues the transactions to write to the on-chain buffer. It tracks unique IDs for each and returns them to be used as dependency IDs for the main transaction.
func (s *SolanaChainWriterService) sendBufferInstructions(
	ctx context.Context,
	bufferIxs []solana.Instruction,
	closeBufferIx solana.Instruction,
	methodConfig MethodConfig,
	contractName, method, txID, debugID string,
	programID, feePayer solana.PublicKey,
	accounts solana.AccountMetaSlice,
	args any,
	options []txmutils.SetTxConfig,
	lookupTableMap map[solana.PublicKey]solana.PublicKeySlice,
) error {
	blockhash, err := s.client.LatestBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("error fetching latest blockhash: %w", err)
	}

	bufferTxIDs := make([]string, 0, len(bufferIxs))

	closeBufferTx, err := solana.NewTransaction(
		[]solana.Instruction{closeBufferIx},
		blockhash.Value.Blockhash,
		solana.TransactionPayer(feePayer),
	)
	if err != nil {
		return fmt.Errorf("failed to build close buffer transaction: %w", err)
	}

	s.lggr.Debugw("Sending transactions to write to buffer", "contract", contractName, "method", method, "transactionID", txID, "bufferTransactionCount", len(bufferTxIDs))

	for i, ix := range bufferIxs {
		bufferTx, bufferErr := solana.NewTransaction(
			[]solana.Instruction{ix},
			blockhash.Value.Blockhash,
			solana.TransactionPayer(feePayer),
		)
		if bufferErr != nil {
			return fmt.Errorf("failed to build buffer transaction: %w", bufferErr)
		}

		bufferUUID := fmt.Sprintf("Buffer-%d-%s", i, uuid.NewString())

		// Enqueue execution report buffer transaction
		if err = s.txm.Enqueue(ctx, methodConfig.FromAddress, bufferTx, &bufferUUID, blockhash.Value.LastValidBlockHeight, txmutils.SetEstimateComputeUnitLimit(true)); err != nil {
			return fmt.Errorf("error enqueuing buffer transaction: %w", err)
		}
		bufferTxIDs = append(bufferTxIDs, bufferUUID)
	}

	// Mark main transaction as dependent on the buffer transactions
	// Waits till buffer transactions are finalized before proceeding with the main transaction
	bufferTxs := make([]txmutils.DependencyTx, 0, len(bufferTxIDs))
	for _, id := range bufferTxIDs {
		bufferTxs = append(bufferTxs, txmutils.DependencyTx{TxID: id, DesiredStatus: types.Finalized})
	}
	options = append(options, txmutils.AppendDependencyTxs(bufferTxs))

	s.lggr.Debugw("Encoding new transformed payload for transaction using buffer", "contract", contractName, "method", method)
	transformedPayload, err := s.encoder.Encode(ctx, args, codec.WrapItemType(true, contractName, method))
	if err != nil {
		return fmt.Errorf("error encoding transformed payload for transaction using buffer: %w", err)
	}

	// Recreate transaction with transformed payload and new account list which includes the buffer PDA
	mainTx, err := solana.NewTransaction(
		[]solana.Instruction{solana.NewInstruction(programID, accounts, transformedPayload)},
		blockhash.Value.Blockhash,
		solana.TransactionPayer(feePayer),
		solana.TransactionAddressTables(lookupTableMap),
	)
	if err != nil {
		return errorWithDebugID(fmt.Errorf("error reconstructing transaction with empty payload: %w", err), debugID)
	}

	s.lggr.Debugw("Sending main transaction", "contract", contractName, "method", method, "tx", txID, "debugID", debugID)
	if err = s.txm.Enqueue(ctx, methodConfig.FromAddress, mainTx, &txID, blockhash.Value.LastValidBlockHeight, options...); err != nil {
		return fmt.Errorf("error enqueuing maintransaction: %w", err)
	}

	closeBufferUUID := fmt.Sprintf("CloseBuffer-%s", uuid.NewString())
	opts := []txmutils.SetTxConfig{
		txmutils.SetEstimateComputeUnitLimit(true),
		// Mark close buffer transaction as dependent on the main transaction. Only send the close buffer transaction if main transaction marked as failed
		// Main transaction would be marked as failed if any of the buffer transactions failed or if itself failed
		txmutils.AppendDependencyTxs([]txmutils.DependencyTx{{TxID: txID, DesiredStatus: types.Failed}}),
		// Ignore dependency errors because this transaction is expected to be dropped in the happy path
		txmutils.SetDependencyTxMetaIgnoreError(true),
	}
	if err = s.txm.Enqueue(ctx, methodConfig.FromAddress, closeBufferTx, &closeBufferUUID, blockhash.Value.LastValidBlockHeight, opts...); err != nil {
		return fmt.Errorf("error enqueuing close buffer transaction: %w", err)
	}

	return nil
}

// Breaks down the report into smaller chunks
// Calculates the max chunk size using an empty transaction for overhead
func extractChunks(rawReport []byte, emptyBufferIx solana.Instruction) ([][]byte, error) {
	// Build transaction with empty buffer instruction
	emptyBufferTx, err := solana.NewTransaction(
		[]solana.Instruction{emptyBufferIx},
		solana.Hash{},
		solana.TransactionPayer(solana.PublicKey{}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build empty buffer tx: %w", err)
	}
	// Get the empty buffer transaction bytes to calculate the tx overhead
	emptyTxBytes, err := emptyBufferTx.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal empty buffer tx: %w", err)
	}

	// Use the empty buffer tx overhead to calculate the largest chunk size that can be supported
	chunkSize := MaxSolanaTxSize - len(emptyTxBytes)

	chunkCount := len(rawReport) / chunkSize
	if len(rawReport)%chunkSize != 0 {
		chunkCount++
	}

	chunks := make([][]byte, 0, chunkCount)
	for i := range chunkCount {
		start := i * chunkSize
		end := min(((i + 1) * chunkSize), len(rawReport))

		chunk := rawReport[start:end]
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}
