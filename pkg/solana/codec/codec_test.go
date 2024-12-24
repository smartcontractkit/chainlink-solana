package codec_test

import (
	"bytes"
	_ "embed"
	"slices"
	"sync"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	ocr2types "github.com/smartcontractkit/libocr/offchainreporting2plus/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	looptestutils "github.com/smartcontractkit/chainlink-common/pkg/loop/testutils"
	clcommontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	. "github.com/smartcontractkit/chainlink-common/pkg/types/interfacetests" //nolint common practice to import test mods with .
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/testutils"
)

const anyExtraValue = 3

func TestCodec(t *testing.T) {
	tester := &codecInterfaceTester{}
	RunCodecInterfaceTests(t, tester)
	RunCodecInterfaceTests(t, looptestutils.WrapCodecTesterForLoop(tester))

	t.Run("Events are encode-able and decode-able for a single item", func(t *testing.T) {
		ctx := tests.Context(t)
		item := CreateTestStruct[*testing.T](0, tester)
		req := &EncodeRequest{TestStructs: []TestStruct{item}, TestOn: testutils.TestEventItem}
		resp := tester.EncodeFields(t, req)

		codec := tester.GetCodec(t)
		actualEncoding, err := codec.Encode(ctx, item, testutils.TestEventItem)
		require.NoError(t, err)
		assert.Equal(t, resp, actualEncoding)

		into := TestStruct{}
		require.NoError(t, codec.Decode(ctx, actualEncoding, &into, testutils.TestEventItem))
		assert.Equal(t, item, into)
	})
}

func FuzzCodec(f *testing.F) {
	tester := &codecInterfaceTester{}
	RunCodecInterfaceFuzzTests(f, tester)
}

type codecInterfaceTester struct {
	accountBytesMu sync.Mutex
	accountBytes   []byte
	TestSelectionSupport
}

func (it *codecInterfaceTester) Setup(_ *testing.T) {}

func (it *codecInterfaceTester) GetAccountBytes(_ int) []byte {
	it.accountBytesMu.Lock()
	defer it.accountBytesMu.Unlock()
	if len(it.accountBytes) != 32 {
		pk, _ := solana.NewRandomPrivateKey()
		it.accountBytes = pk.PublicKey().Bytes()
	}

	return it.accountBytes
}

func (it *codecInterfaceTester) GetAccountString(i int) string {
	return solana.PublicKeyFromBytes(it.GetAccountBytes(i)).String()
}

func (it *codecInterfaceTester) EncodeFields(t *testing.T, request *EncodeRequest) []byte {
	if request.TestOn == TestItemType || request.TestOn == testutils.TestEventItem {
		return encodeFieldsOnItem(t, request)
	}

	return encodeFieldsOnSliceOrArray(t, request)
}

func encodeFieldsOnItem(t *testing.T, request *EncodeRequest) ocr2types.Report {
	buf := new(bytes.Buffer)
	// The underlying TestItemAsAccount adds a discriminator by default while being Borsh encoded.
	if err := testutils.EncodeRequestToTestItemAsAccount(request.TestStructs[0]).MarshalWithEncoder(bin.NewBorshEncoder(buf)); err != nil {
		require.NoError(t, err)
	}
	return buf.Bytes()
}

func encodeFieldsOnSliceOrArray(t *testing.T, request *EncodeRequest) []byte {
	var toEncode interface{}
	buf := new(bytes.Buffer)
	switch request.TestOn {
	case TestItemArray1Type:
		toEncode = [1]testutils.TestItemAsArgs{testutils.EncodeRequestToTestItemAsArgs(request.TestStructs[0])}
	case TestItemArray2Type:
		toEncode = [2]testutils.TestItemAsArgs{testutils.EncodeRequestToTestItemAsArgs(request.TestStructs[0]), testutils.EncodeRequestToTestItemAsArgs(request.TestStructs[1])}
	default:
		// encode TestItemSliceType as instruction args (similar to accounts, but no discriminator) because accounts can't be just a vector
		var itemSliceType []testutils.TestItemAsArgs
		for _, req := range request.TestStructs {
			itemSliceType = append(itemSliceType, testutils.EncodeRequestToTestItemAsArgs(req))
		}
		toEncode = itemSliceType
	}

	if err := bin.NewBorshEncoder(buf).Encode(toEncode); err != nil {
		require.NoError(t, err)
	}
	return buf.Bytes()
}

func (it *codecInterfaceTester) GetCodec(t *testing.T) clcommontypes.Codec {
	codecConfig := codec.Config{Configs: map[string]codec.ChainConfig{}}
	TestItem := CreateTestStruct[*testing.T](0, it)
	for offChainName, v := range testutils.CodecDefs {
		codecEntryCfg := codecConfig.Configs[offChainName]
		codecEntryCfg.IDL = v.IDL
		codecEntryCfg.Type = v.ItemType
		codecEntryCfg.ChainSpecificName = v.IDLTypeName

		if offChainName != NilType {
			codecEntryCfg.ModifierConfigs = commoncodec.ModifiersConfig{
				&commoncodec.RenameModifierConfig{Fields: map[string]string{"NestedDynamicStruct.Inner.IntVal": "I"}},
				&commoncodec.RenameModifierConfig{Fields: map[string]string{"NestedStaticStruct.Inner.IntVal": "I"}},
			}
		}

		if slices.Contains([]string{TestItemType, TestItemSliceType, TestItemArray1Type, TestItemArray2Type, testutils.TestItemWithConfigExtraType, testutils.TestEventItem}, offChainName) {
			addressByteModifier := &commoncodec.AddressBytesToStringModifierConfig{
				Fields:   []string{"AccountStruct.AccountStr"},
				Modifier: codec.SolanaAddressModifier{},
			}
			codecEntryCfg.ModifierConfigs = append(codecEntryCfg.ModifierConfigs, addressByteModifier)
		}

		if offChainName == testutils.TestItemWithConfigExtraType {
			hardCode := &commoncodec.HardCodeModifierConfig{
				OnChainValues: map[string]any{
					"BigField":              TestItem.BigField.String(),
					"AccountStruct.Account": solana.PublicKeyFromBytes(TestItem.AccountStruct.Account),
				},
				OffChainValues: map[string]any{"ExtraField": anyExtraValue},
			}
			codecEntryCfg.ModifierConfigs = append(codecEntryCfg.ModifierConfigs, hardCode)
		}
		codecConfig.Configs[offChainName] = codecEntryCfg
	}

	c, err := codec.NewCodec(codecConfig)
	require.NoError(t, err)

	return c
}

func (it *codecInterfaceTester) IncludeArrayEncodingSizeEnforcement() bool {
	return true
}
func (it *codecInterfaceTester) Name() string {
	return "Solana"
}
