package codec

import (
	"github.com/smartcontractkit/chainlink-common/pkg/codec/encodings"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

func NewCodec(anchorIDL IDL) (types.RemoteCodec, error) {
	return encodings.CodecFromTypeCodec{}, nil
}
