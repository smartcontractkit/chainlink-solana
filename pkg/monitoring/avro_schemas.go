package monitoring

import (
	"encoding/json"
	"fmt"

	"github.com/linkedin/goavro"
)

// See https://avro.apache.org/docs/current/spec.html#schemas

var transmissionAvroSchema = Record("transmission", Opts{Namespace: "link.chain.ocr2"}, Fields{
	Field("block_number", Opts{Doc: "uint64 big endian"}, Bytes),
	Field("answer", Opts{}, Record("answer", Opts{}, Fields{
		Field("config_digest", Opts{Doc: "[32]byte encoded as base64"}, String),
		Field("epoch", Opts{Doc: "uint32"}, Long),
		Field("round", Opts{Doc: "uint8"}, Int),
		Field("data", Opts{Doc: "*big.Int"}, Bytes),
		Field("timestamp", Opts{Doc: "uint32"}, Long),
	})),
	Field("solana_chain_config", Opts{}, Record("solana_chain_config", Opts{}, Fields{
		Field("network_name", Opts{}, String),
		Field("network_id", Opts{}, String),
		Field("chain_id", Opts{}, String),
	})),
	Field("feed_config", Opts{}, Record("feed_config", Opts{}, Fields{
		Field("feed_name", Opts{}, String),
		Field("feed_path", Opts{}, String),
		Field("symbol", Opts{}, String),
		Field("heartbeat_sec", Opts{}, Long),
		Field("contract_type", Opts{}, String),
		Field("contract_status", Opts{}, String),
		Field("contract_address", Opts{Doc: "[32]byte"}, Bytes),
		Field("transmissions_account", Opts{Doc: "[32]byte"}, Bytes),
		Field("state_account", Opts{Doc: "[32]byte"}, Bytes),
	})),
})

var configSetSimplifiedAvroSchema = Record("config_set_simplified", Opts{Namespace: "link.chain.ocr2"}, Fields{
	Field("config_digest", Opts{Doc: "[32]byte encoded as base64"}, String),
	Field("block_number", Opts{Doc: "uint64 big endian"}, Bytes),
	Field("signers", Opts{Doc: "json encoded array of base64-encoded signing keys"}, String),
	Field("transmitters", Opts{Doc: "json encoded array of base64-encoded transmission keys"}, String),
	Field("f", Opts{Doc: "uint8"}, Int),
	Field("delta_progress", Opts{Doc: "uint64 big endian"}, Bytes),
	Field("delta_resend", Opts{Doc: "uint64 big endian"}, Bytes),
	Field("delta_round", Opts{Doc: "uint64 big endian"}, Bytes),
	Field("delta_grace", Opts{Doc: "uint64 big endian"}, Bytes),
	Field("delta_stage", Opts{Doc: "uint64 big endian"}, Bytes),
	Field("r_max", Opts{Doc: "uint32"}, Long),
	Field("s", Opts{Doc: "json encoded []int"}, String),
	Field("oracles", Opts{Doc: "json encoded list of oracles' "}, String),
	Field("feed_state_account", Opts{Doc: "[32]byte"}, String),
})

var (
	// Avro schemas to sync with the registry
	TransmissionAvroSchema        string
	ConfigSetSimplifiedAvroSchema string

	// These codecs are used in tests
	transmissionCodec        *goavro.Codec
	configSetSimplifiedCodec *goavro.Codec
)

func init() {
	var err error
	var buf []byte

	buf, err = json.Marshal(transmissionAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to generate Avro schema for transmission: %w", err))
	}
	TransmissionAvroSchema = string(buf)
	transmissionCodec, err = goavro.NewCodec(TransmissionAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to parse Avro schema for the latest transmission: %w", err))
	}

	buf, err = json.Marshal(configSetSimplifiedAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to generate Avro schema for configSimplified: %w", err))
	}
	ConfigSetSimplifiedAvroSchema = string(buf)
	configSetSimplifiedCodec, err = goavro.NewCodec(ConfigSetSimplifiedAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to parse Avro schema for the latest configSetSimplified: %w", err))
	}

	// These codecs are used in tests but not in main, so the linter complains.
	_ = transmissionCodec
	_ = configSetSimplifiedCodec
}
