package codec

import (
	"fmt"
	"reflect"

	"github.com/smartcontractkit/chainlink-common/pkg/codec"
	commonencodings "github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
)

type Entry interface {
	Encode(value any, into []byte) ([]byte, error)
	Decode(encoded []byte) (any, []byte, error)
	GetCodecType() commonencodings.TypeCodec
	GetType() reflect.Type

	Modifier() codec.Modifier
}

//// TODO this can also be an event entry, but anchor-go defines events differently, maybe just have a separate struct and method that satisfy entry interface for events.
//func NewEntry(idlAccount IdlTypeDef, idlTypes IdlTypeDefSlice, includeDiscriminator bool, mod codec.Modifier, builder commonencodings.Builder) (Entry, error) {
//	refs := &codecRefs{
//		builder:      builder,
//		codecs:       make(map[string]commonencodings.TypeCodec),
//		typeDefs:     idlTypes,
//		dependencies: make(map[string][]string),
//	}
//
//	if mod == nil {
//		mod = codec.MultiModifier{}
//	}
//
//	_, accCodec, err := createCodecType(idlAccount, refs, false)
//	if err != nil {
//		return nil, err
//	}
//
//	entry := &codecEntry{name: idlAccount.Name, includeDiscriminator: includeDiscriminator, codecType: accCodec, typ: accCodec.GetType(), mod: mod}
//	if entry.includeDiscriminator {
//		entry.Discriminator = commonencodings.NamedTypeCodec{Name: "Discriminator" + idlAccount.Name, Codec: NewDiscriminator(idlAccount.Name)}
//	}
//
//	return entry, nil
//}

type codecEntry struct {
	name                 string
	IDLTypeName          string
	includeDiscriminator bool
	Discriminator        commonencodings.NamedTypeCodec
	typ                  reflect.Type
	codecType            commonencodings.TypeCodec
	mod                  codec.Modifier
}

func (entry *codecEntry) GetType() reflect.Type {
	return entry.typ
}

func (entry *codecEntry) GetCodecType() commonencodings.TypeCodec {
	return entry.codecType
}

func (entry *codecEntry) Encode(value any, into []byte) ([]byte, error) {
	if value == nil && entry.typ.Kind() == reflect.Pointer && entry.typ.Elem().Kind() == reflect.Struct && entry.typ.Elem().NumField() == 0 {
		return []byte{}, nil
	} else if value == nil {
		return nil, fmt.Errorf("%w: cannot encode nil value for %s", commontypes.ErrInvalidType, entry.name)
	}

	encodedVal, err := entry.codecType.Encode(value, into)
	if err != nil {
		return nil, err
	}

	if entry.includeDiscriminator {
		var byt []byte
		disc := NewDiscriminator(entry.IDLTypeName)
		encodedDisc, err := disc.Encode(&disc.hashPrefix, byt)
		if err != nil {
			return nil, err
		}
		return append(encodedDisc, encodedVal...), nil
	}

	return encodedVal, nil
}

func (entry *codecEntry) Decode(encoded []byte) (any, []byte, error) {
	if entry.includeDiscriminator {
		encoded = encoded[discriminatorLength:]
	}
	return entry.codecType.Decode(encoded)
}

func (entry *codecEntry) Modifier() codec.Modifier {
	return entry.mod
}
