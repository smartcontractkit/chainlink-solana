package codec_test

import (
	"bytes"
	_ "embed"
	"slices"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	ocr2types "github.com/smartcontractkit/libocr/offchainreporting2plus/types"
	"github.com/stretchr/testify/require"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	clcommontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	. "github.com/smartcontractkit/chainlink-common/pkg/types/interfacetests" //nolint common practice to import test mods with .
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/testutils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/testutils/test_item_slice_type"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/codec/testutils/test_item_type"
)

const anyExtraValue = 3

func TestCodec(t *testing.T) {
	tester := &codecInterfaceTester{}
	RunCodecInterfaceTests(t, tester)
	//RunCodecInterfaceTests(t, looptestutils.WrapCodecTesterForLoop(tester))
}

type codecInterfaceTester struct {
	TestSelectionSupport
}

func (it *codecInterfaceTester) Setup(_ *testing.T) {}

func (it *codecInterfaceTester) GetAccountBytes(i int) []byte {
	pk, _ := solana.NewRandomPrivateKey()
	return pk.PublicKey().Bytes()
}

func (it *codecInterfaceTester) GetAccountString(i int) string {
	return solana.PublicKeyFromBytes(it.GetAccountBytes(i)).String()
}

func (it *codecInterfaceTester) EncodeFields(t *testing.T, request *EncodeRequest) []byte {
	if request.TestOn == testutils.TestItemType {
		return encodeFieldsOnItem(t, request)
	}

	return encodeFieldsOnSliceOrArray(t, request)
}

func encodeFieldsOnItem(t *testing.T, request *EncodeRequest) ocr2types.Report {
	buf := new(bytes.Buffer)
	if err := testutils.EncodeRequestToTestItem(request).MarshalWithEncoder(bin.NewBorshEncoder(buf)); err != nil {
		require.NoError(t, err)
	}
	return buf.Bytes()
}

func encodeFieldsOnSliceOrArray(t *testing.T, request *EncodeRequest) []byte {
	args := make([]any, 1)
	switch request.TestOn {
	case testutils.TestItemSliceType:
		testItemSlice := []test_item_type.TestItem{testutils.ToInternalType(request.TestStructs[0])}
		buf := new(bytes.Buffer)
		if err := test_item_slice_type.NewTestItemSliceTypeInstructionBuilder().SetTestItemSliceType(testItemSlice).Build().MarshalWithEncoder(bin.NewBorshEncoder(buf)); err != nil {
			require.NoError(t, err)
			return nil
		}
		return buf.Bytes()
	case testutils.TestItemArray1Type:
		args[0] = [1]test_item_type.TestItem{testutils.ToInternalType(request.TestStructs[0])}
	case testutils.TestItemArray2Type:
		args[0] = [2]test_item_type.TestItem{testutils.ToInternalType(request.TestStructs[0]), testutils.ToInternalType(request.TestStructs[1])}
	default:
		tmp := make([]test_item_type.TestItem, len(request.TestStructs))
		for i, ts := range request.TestStructs {
			tmp[i] = testutils.ToInternalType(ts)
		}
		args[0] = tmp
	}

	return []byte{}
}

func (it *codecInterfaceTester) GetCodec(t *testing.T) clcommontypes.Codec {
	codecConfig := codec.Config{Configs: map[string]codec.ChainConfig{}}
	TestItem := CreateTestStruct[*testing.T](0, it)
	for k, v := range testutils.CodecDefs {
		entry := codecConfig.Configs[k]
		entry.IDL = v.IDL
		entry.Type = v.ItemType

		if k != testutils.SizeItemType && k != testutils.NilType {
			entry.ModifierConfigs = commoncodec.ModifiersConfig{
				&commoncodec.RenameModifierConfig{Fields: map[string]string{"NestedDynamicStruct.Inner.IntVal": "I"}},
				&commoncodec.RenameModifierConfig{Fields: map[string]string{"NestedStaticStruct.Inner.IntVal": "I"}},
			}
		}

		if slices.Contains([]string{testutils.TestItemType, testutils.TestItemSliceType, testutils.TestItemWithConfigExtra}, k) {
			addressByteModifier := &commoncodec.AddressBytesToStringModifierConfig{
				Fields:   []string{"AccountStruct.AccountStr"},
				Modifier: codec.SolanaAddressModifier{},
			}
			entry.ModifierConfigs = append(entry.ModifierConfigs, addressByteModifier)
		}

		if k == testutils.TestItemWithConfigExtra {
			hardCode := &commoncodec.HardCodeModifierConfig{
				OnChainValues: map[string]any{
					"BigField":              TestItem.BigField.String(),
					"AccountStruct.Account": solana.PublicKeyFromBytes(TestItem.AccountStruct.Account),
				},
				OffChainValues: map[string]any{"ExtraField": anyExtraValue},
			}
			entry.ModifierConfigs = append(entry.ModifierConfigs, hardCode)
		}
		codecConfig.Configs[k] = entry
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
