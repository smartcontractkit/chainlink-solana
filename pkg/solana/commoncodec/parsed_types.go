package commoncodec

import (
	"fmt"
	"reflect"

	commoncodec "github.com/smartcontractkit/chainlink-common/pkg/codec"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type ParsedTypes struct {
	EncoderDefs map[string]Entry
	DecoderDefs map[string]Entry
	Modifiers   commoncodec.Modifier
}

func (parsed *ParsedTypes) ToCodec() (commontypes.RemoteCodec, error) {
	directionalMods := map[string]map[string]map[string]commoncodec.Modifier{}

	modByTypeName := map[string]map[string]commoncodec.Modifier{}
	if err := AddEntries(parsed.EncoderDefs, modByTypeName); err != nil {
		return nil, err
	}

	directionalMods["input"] = modByTypeName

	modByTypeName = map[string]map[string]commoncodec.Modifier{}
	if err := AddEntries(parsed.DecoderDefs, modByTypeName); err != nil {
		return nil, err
	}

	directionalMods["output"] = modByTypeName

	collapsed := map[string]commoncodec.Modifier{}
	for direction, dMods := range directionalMods {
		dCollapsed := map[string]commoncodec.Modifier{}

		for namespace, mods := range dMods {
			mod, err := commoncodec.NewNestableByItemTypeModifier(mods)
			if err != nil {
				return nil, err
			}

			dCollapsed[namespace] = mod
		}

		mod, err := commoncodec.NewNestableByItemTypeModifier(dCollapsed)
		if err != nil {
			return nil, err
		}

		collapsed[direction] = mod
	}

	mod, err := commoncodec.NewNestableByItemTypeModifier(collapsed)
	if err != nil {
		return nil, err
	}

	parsed.Modifiers = mod
	underlying := &solanaCodec{
		Encoder:     newEncoder(parsed.EncoderDefs),
		Decoder:     newDecoder(parsed.DecoderDefs),
		ParsedTypes: parsed,
	}

	return commoncodec.NewModifierCodec(underlying, mod, DecoderHooks...)
}

// AddEntries extracts the mods from entry and adds them to modByTypeName use with codec.NewByItemTypeModifier
// Since each input/output can have its own modifications, we need to keep track of them by type name
func AddEntries(defs map[string]Entry, modByTypeName map[string]map[string]commoncodec.Modifier) error {
	for itemType, def := range defs {
		_, tail := commoncodec.ItemTyper(itemType).Next()
		head, tail := commoncodec.ItemTyper(tail).Next()

		if _, ok := modByTypeName[head]; !ok {
			modByTypeName[head] = make(map[string]commoncodec.Modifier)
		}

		modByTypeName[head][tail] = def.Modifier()

		if _, err := def.Modifier().RetypeToOffChain(reflect.PointerTo(def.GetType()), ""); err != nil {
			return fmt.Errorf("%w: cannot retype %v: %w", commontypes.ErrInvalidConfig, itemType, err)
		}
	}

	return nil
}
