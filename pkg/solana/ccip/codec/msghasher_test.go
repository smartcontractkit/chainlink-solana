package codec

import (
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/ccip"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/types/ccipocr3"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/ccip/codec/mocks"
)

func TestMessageHasher_EVM2SVM(t *testing.T) {
	any2AnyMsg, any2SolanaMsg, msgAccounts := createEVM2SolanaMessages(t)
	mockExtraDataCodec := mocks.NewSourceChainExtraDataCodec(t)
	for _, ta := range any2SolanaMsg.TokenAmounts {
		mockExtraDataCodec.On("DecodeDestExecDataToMap", mock.Anything).Return(map[string]any{
			"destGasAmount": ta.DestGasAmount, // All dest gas amounts are the same so use one
		}, nil).Once()
	}
	accountBytes := make([][32]byte, 0, len(msgAccounts))
	// Skip first account since it's the message receiver that's prepended in the hasher
	for i := 1; i < len(msgAccounts); i++ {
		accountBytes = append(accountBytes, [32]byte(msgAccounts[i].Bytes()))
	}
	mockExtraDataCodec.On("DecodeExtraArgsToMap", mock.Anything).Return(map[string]any{
		"ComputeUnits":            any2SolanaMsg.ExtraArgs.ComputeUnits,
		"AccountIsWritableBitmap": any2SolanaMsg.ExtraArgs.IsWritableBitmap,
		"TokenReceiver":           [32]byte(any2SolanaMsg.TokenReceiver.Bytes()),
		"Accounts":                accountBytes,
	}, nil).Once()

	registeredExtraDataCodecMap := map[string]ccipocr3.SourceChainExtraDataCodec{
		chainsel.FamilySolana: ExtraDataDecoder{},
		chainsel.FamilyEVM:    mockExtraDataCodec,
	}
	var extraDataCodec = ccipocr3.ExtraDataCodecMap(registeredExtraDataCodecMap)
	msgHasher := NewMessageHasherV1(logger.Test(t), extraDataCodec)
	actualHash, err := msgHasher.Hash(t.Context(), any2AnyMsg)
	require.NoError(t, err)
	expectedHash, err := ccip.HashAnyToSVMMessage(any2SolanaMsg, any2AnyMsg.Header.OnRamp, msgAccounts)
	require.NoError(t, err)
	require.Equal(t, expectedHash, actualHash[:32])
}

func TestMessageHasher_InvalidReceiver(t *testing.T) {
	any2AnyMsg, _, _ := createEVM2SolanaMessages(t)

	// Set receiver to a []byte of 2 length
	any2AnyMsg.Receiver = []byte{0, 0}
	mockExtraDataCodec := mocks.NewSourceChainExtraDataCodec(t)
	mockExtraDataCodec.On("DecodeDestExecDataToMap", mock.Anything).Return(map[string]any{
		"destGasAmount": uint32(10),
	}, nil).Maybe()
	mockExtraDataCodec.On("DecodeExtraArgsToMap", mock.Anything).Return(map[string]any{
		"ComputeUnits":            uint32(1000),
		"AccountIsWritableBitmap": uint64(10),
		"Accounts": [][32]byte{
			[32]byte(config.CcipLogicReceiver.Bytes()),
			[32]byte(config.ReceiverTargetAccountPDA.Bytes()),
			[32]byte(solana.SystemProgramID.Bytes()),
		},
	}, nil).Maybe()

	registeredMockExtraDataCodecMap := map[string]ccipocr3.SourceChainExtraDataCodec{
		chainsel.FamilyEVM:    mockExtraDataCodec,
		chainsel.FamilySolana: mockExtraDataCodec,
	}

	edc := ccipocr3.ExtraDataCodecMap(registeredMockExtraDataCodecMap)
	msgHasher := NewMessageHasherV1(logger.Test(t), edc)
	_, err := msgHasher.Hash(t.Context(), any2AnyMsg)
	require.Error(t, err)
}

func TestMessageHasher_InvalidDestinationTokenAddress(t *testing.T) {
	any2AnyMsg, _, _ := createEVM2SolanaMessages(t)

	// Set DestTokenAddress to a []byte of 2 length
	any2AnyMsg.TokenAmounts[0].DestTokenAddress = []byte{0, 0}
	mockExtraDataCodec := mocks.NewSourceChainExtraDataCodec(t)
	mockExtraDataCodec.On("DecodeDestExecDataToMap", mock.Anything).Return(map[string]any{
		"destGasAmount": uint32(10),
	}, nil).Maybe()
	mockExtraDataCodec.On("DecodeExtraArgsToMap", mock.Anything).Return(map[string]any{
		"ComputeUnits":            uint32(1000),
		"AccountIsWritableBitmap": uint64(10),
		"Accounts": [][32]byte{
			[32]byte(config.CcipLogicReceiver.Bytes()),
			[32]byte(config.ReceiverTargetAccountPDA.Bytes()),
			[32]byte(solana.SystemProgramID.Bytes()),
		},
	}, nil).Maybe()

	registeredMockExtraDataCodecMap := map[string]ccipocr3.SourceChainExtraDataCodec{
		chainsel.FamilyEVM:    mockExtraDataCodec,
		chainsel.FamilySolana: mockExtraDataCodec,
	}
	edc := ccipocr3.ExtraDataCodecMap(registeredMockExtraDataCodecMap)
	msgHasher := NewMessageHasherV1(logger.Test(t), edc)
	_, err := msgHasher.Hash(t.Context(), any2AnyMsg)
	require.Error(t, err)
}

func createEVM2SolanaMessages(t *testing.T) (ccipocr3.Message, ccip_offramp.Any2SVMRampMessage, []solana.PublicKey) {
	messageID := RandomBytes32()
	sourceChain := uint64(5009297550715157269) // evm mainnet
	seqNum := rand.Uint64()
	nonce := rand.Uint64()
	destChain := rand.Uint64()

	messageData := make([]byte, rand.Intn(2048))
	_, err := cryptorand.Read(messageData)
	require.NoError(t, err)

	sender := abiEncodedAddress(t)
	receiver := getRandomPubKey(t)
	tokenReceiver := getRandomPubKey(t)
	sourcePoolAddr := getRandomPubKey(t)
	msgAccount := getRandomPubKey(t)
	extraArgs := ccip_offramp.Any2SVMRampExtraArgs{
		ComputeUnits:     uint32(10000),
		IsWritableBitmap: uint64(4),
	}
	abiEncodedExtraArgs := []byte{31, 59, 58, 186, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 39, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 44, 230, 105, 156, 244, 184, 196, 235, 30, 58, 209, 82, 8, 202, 25, 73, 167, 169, 34, 150, 141, 129, 169, 150, 219, 160, 186, 44, 72, 156, 50, 170, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 160, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 44, 230, 105, 156, 244, 184, 196, 235, 30, 58, 209, 82, 8, 202, 25, 73, 167, 169, 34, 150, 141, 129, 169, 150, 219, 160, 186, 44, 72, 156, 50, 170}
	tokenAmount := ccipocr3.NewBigInt(big.NewInt(rand.Int63()))
	destGasAmount, err := abiEncodeUint32(10)
	require.NoError(t, err)

	ccipTokenAmounts := make([]ccipocr3.RampTokenAmount, 5)
	for z := range 5 {
		ccipTokenAmounts[z] = ccipocr3.RampTokenAmount{
			SourcePoolAddress: ccipocr3.UnknownAddress(sourcePoolAddr.Bytes()),
			DestTokenAddress:  receiver.Bytes(),
			Amount:            tokenAmount,
			DestExecData:      destGasAmount,
		}
	}

	solTokenAmounts := make([]ccip_offramp.Any2SVMTokenTransfer, 5)
	for z := range 5 {
		solTokenAmounts[z] = ccip_offramp.Any2SVMTokenTransfer{
			SourcePoolAddress: ccipocr3.UnknownAddress(sourcePoolAddr.Bytes()),
			DestTokenAddress:  receiver,
			Amount:            ccip_offramp.CrossChainAmount{LeBytes: [32]uint8(encodeBigIntToFixedLengthLE(tokenAmount.Int, 32))},
			DestGasAmount:     uint32(10),
		}
	}

	any2SolanaMsg := ccip_offramp.Any2SVMRampMessage{
		Header: ccip_offramp.RampMessageHeader{
			MessageId:           messageID,
			SourceChainSelector: sourceChain,
			DestChainSelector:   destChain,
			SequenceNumber:      seqNum,
			Nonce:               nonce,
		},
		Sender:        sender,
		TokenReceiver: tokenReceiver,
		Data:          messageData,
		TokenAmounts:  solTokenAmounts,
		ExtraArgs:     extraArgs,
	}
	any2AnyMsg := ccipocr3.Message{
		Header: ccipocr3.RampMessageHeader{
			MessageID:           messageID,
			SourceChainSelector: ccipocr3.ChainSelector(sourceChain),
			DestChainSelector:   ccipocr3.ChainSelector(destChain),
			SequenceNumber:      ccipocr3.SeqNum(seqNum),
			Nonce:               nonce,
			OnRamp:              abiEncodedAddress(t),
		},
		Sender:         sender,
		Receiver:       receiver.Bytes(),
		Data:           messageData,
		TokenAmounts:   ccipTokenAmounts,
		FeeToken:       []byte{},
		FeeTokenAmount: ccipocr3.NewBigIntFromInt64(0),
		ExtraArgs:      abiEncodedExtraArgs,
	}

	msgAccounts := []solana.PublicKey{receiver, msgAccount}
	return any2AnyMsg, any2SolanaMsg, msgAccounts
}

func abiEncodedAddress(t *testing.T) []byte {
	encoded, err := abiEncode(`[{"type": "address"}]`, randomAddress())
	require.NoError(t, err)
	return encoded
}

func abiEncodeUint32(data uint32) ([]byte, error) {
	return abiEncode(`[{ "type": "uint32" }]`, data)
}

func TestToLittleEndian(t *testing.T) {
	mustSetString := func(s string) *big.Int {
		b, ok := big.NewInt(0).SetString(s, 10)
		if !ok {
			t.Fatalf("failed to set string %s", s)
		}
		return b
	}

	var tests = []struct {
		input    *big.Int
		expected []byte
	}{
		{
			input:    mustSetString("93632917990780833250"),
			expected: []uint8{0xe2, 0xd, 0xc6, 0xfb, 0xd2, 0xf2, 0x6a, 0x13, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
		{
			input:    mustSetString("9363291799078083325000000910750912570125"),
			expected: []uint8{0xd, 0x63, 0xbf, 0xfc, 0xcb, 0xfc, 0x27, 0x0, 0x4c, 0x7e, 0xe5, 0x81, 0xc7, 0x67, 0x28, 0x84, 0x1b, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
		},
	}

	for _, test := range tests {
		t.Run(test.input.String(), func(t *testing.T) {
			result := encodeBigIntToFixedLengthLE(test.input, 32)
			assert.Equal(t, test.expected, result, "expected %x, got %x", test.expected, result)
		})
	}
}

func randomAddress() common.Address {
	b := make([]byte, 20)
	_, _ = cryptorand.Read(b) // Assignment for errcheck. Only used in tests so we can ignore.
	return common.BytesToAddress(b)
}

// abiEncode is the equivalent of abi.encode.
// See a full set of examples https://github.com/ethereum/go-ethereum/blob/420b78659bef661a83c5c442121b13f13288c09f/accounts/abi/packing_test.go#L31
func abiEncode(abiStr string, values ...interface{}) ([]byte, error) {
	// Create a dummy method with arguments
	inDef := fmt.Sprintf(`[{ "name" : "method", "type": "function", "inputs": %s}]`, abiStr)
	inAbi, err := abi.JSON(strings.NewReader(inDef))
	if err != nil {
		return nil, err
	}
	res, err := inAbi.Pack("method", values...)
	if err != nil {
		return nil, err
	}
	return res[4:], nil
}

func getRandomPubKey(t *testing.T) solana.PublicKey {
	t.Helper()
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	return privKey.PublicKey()
}
