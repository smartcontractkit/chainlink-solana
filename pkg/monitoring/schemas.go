package monitoring

import (
	"encoding/json"
	"fmt"

	"github.com/linkedin/goavro"
)

// See https://avro.apache.org/docs/current/spec.html#schemas

var configSetAvroSchema = Record("config_set", Opts{Namespace: "link.chain.ocr2"}, Fields{
	Field("block_number", Opts{Doc: "uin64 big endian"}, Bytes),
	Field("contract_config", Opts{}, Record("contract_config", Opts{}, Fields{
		Field("config_digest", Opts{Doc: "[32]byte"}, Bytes),
		Field("config_count", Opts{Doc: "uint32"}, Long),
		Field("signers", Opts{}, Array(Bytes)),
		Field("transmitters", Opts{}, Array(Bytes)),
		Field("f", Opts{Doc: "uint8"}, Int),
		Field("onchain_config", Opts{}, Union{
			Record("ocr2_numerical_median_onchain_config", Opts{}, Fields{
				Field("min", Opts{Doc: "*big.Int"}, Bytes),
				Field("max", Opts{Doc: "*big.Int"}, Bytes),
			}),
		}),
		Field("offchain_config_version", Opts{Doc: "uint64 big endian"}, Bytes),
		Field("offchain_config", Opts{}, Union{
			Record("ocr2_offchain_config", Opts{}, Fields{
				Field("delta_progress_nanoseconds", Opts{Doc: "uint64 big endian"}, Bytes),
				Field("delta_resend_nanoseconds", Opts{Doc: "uint64 big endian"}, Bytes),
				Field("delta_round_nanoseconds", Opts{Doc: "uint64 big endian"}, Bytes),
				Field("delta_grace_nanoseconds", Opts{Doc: "uint64 big endian"}, Bytes),
				Field("delta_stage_nanoseconds", Opts{Doc: "uint64 big endian"}, Bytes),
				Field("r_max", Opts{Doc: "uint32"}, Long),
				Field("s", Opts{Doc: "[]uint32"}, Array(Long)),
				Field("offchain_public_keys", Opts{}, Array(Bytes)),
				Field("peer_ids", Opts{}, Array(String)),
				Field("reporting_plugin_config", Opts{}, Union{
					Record("ocr2_numerical_median_offchain_config", Opts{}, Fields{
						Field("alpha_report_infinite", Opts{}, Boolean),
						Field("alpha_report_ppb", Opts{Doc: "uint64 big endian"}, Bytes),
						Field("alpha_accept_infinite", Opts{}, Boolean),
						Field("alpha_accept_ppb", Opts{Doc: "uint64 big endian"}, Bytes),
						Field("delta_c_nanoseconds", Opts{Doc: "uint64 big endian"}, Bytes),
					}),
				}),
				Field("max_duration_query_nanoseconds", Opts{Doc: "uint64 big endian"}, Bytes),
				Field("max_duration_observation_nanoseconds", Opts{Doc: "uint64 big endian"}, Bytes),
				Field("max_duration_report_nanoseconds", Opts{Doc: "uint64 big endian"}, Bytes),
				Field("max_duration_should_accept_finalized_report_nanoseconds", Opts{Doc: "uint64 big endian"}, Bytes),
				Field("max_duration_should_transmit_accepted_report_nanoseconds", Opts{Doc: "uint64 big endian"}, Bytes),
				Field("shared_secret_encryptions", Opts{}, Record("shared_secret_encryptions", Opts{}, Fields{
					Field("diffie_hellman_point", Opts{}, Bytes),
					Field("shared_secret_hash", Opts{}, Bytes),
					Field("encryptions", Opts{}, Array(Bytes)),
				})),
			}),
		}),
	})),
	Field("solana_program_state", Opts{}, Record("solana_program_state", Opts{}, Fields{
		Field("account_discriminator", Opts{Doc: "[8]byte"}, Bytes),
		Field("version", Opts{Doc: "uint8"}, Int),
		Field("nonce", Opts{Doc: "uint8"}, Int),
		Field("config", Opts{}, Record("config", Opts{}, Fields{
			Field("owner", Opts{Doc: "[32]byte"}, Bytes),
			Field("token_mint", Opts{Doc: "[32]byte"}, Bytes),
			Field("token_vault", Opts{Doc: "[32]byte"}, Bytes),
			Field("requester_access_controller", Opts{Doc: "[32]byte"}, Bytes),
			Field("billing_access_controller", Opts{Doc: "[32]byte"}, Bytes),
			Field("min_answer", Opts{Doc: "big.Int"}, Bytes),
			Field("max_answer", Opts{Doc: "big.Int"}, Bytes),
			Field("description", Opts{Doc: "[32]byte"}, Bytes),
			Field("decimals", Opts{Doc: "uint8"}, Int),
			Field("f", Opts{Doc: "uint8"}, Int),
			Field("round", Opts{Doc: "uint8"}, Int),
			Field("epoch", Opts{Doc: "uint32"}, Long),
			Field("latest_aggregator_round_id", Opts{Doc: "uint32"}, Long),
			Field("latest_transmitter", Opts{Doc: "[32]bytes"}, Bytes),
			Field("config_count", Opts{Doc: "uint32"}, Long),
			Field("latest_config_digest", Opts{Doc: "[32]byte"}, Bytes),
			Field("latest_config_block_number", Opts{Doc: "uint64"}, Bytes),
			Field("billing", Opts{}, Record("billing", Opts{}, Fields{
				Field("observation_payment", Opts{Doc: "uint32"}, Long),
			})),
			Field("validator", Opts{Doc: "[32]byte"}, Bytes),
			Field("flagging_threshold", Opts{Doc: "uint32"}, Long),
		})),
		Field("oracles", Opts{}, Array(Record("oracle", Opts{}, Fields{
			Field("transmitter", Opts{Doc: "[32]byte"}, Bytes),
			Field("signer", Opts{}, Record("signer", Opts{}, Fields{
				Field("key", Opts{Doc: "[20]byte"}, Bytes),
			})),
			Field("payee", Opts{Doc: "[32]byte"}, Bytes),
			Field("from_round_id", Opts{Doc: "uint32"}, Long),
			Field("payment", Opts{Doc: "uint64"}, Bytes),
		}))),
		Field("leftover_payment", Opts{}, Array(Record("leftover_payment", Opts{}, Fields{
			Field("payee", Opts{Doc: "[32]byte"}, Bytes),
			Field("amount", Opts{Doc: "uint64"}, Bytes),
		}))),
		Field("transmissions", Opts{Doc: "[32]byte"}, Bytes),
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

var transmissionAvroSchema = Record("transmission", Opts{Namespace: "link.chain.ocr2"}, Fields{
	Field("block_number", Opts{Doc: "uint64 big endian"}, Bytes),
	Field("answer", Opts{}, Record("answer", Opts{}, Fields{
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

var (
	// Avro schemas to sync with the registry
	ConfigSetAvroSchema    string
	TransmissionAvroSchema string

	// These codecs are used in tests
	configSetCodec    *goavro.Codec
	transmissionCodec *goavro.Codec
)

func init() {
	var err error
	buf, err := json.Marshal(configSetAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to generate Avro schema for config set: %w", err))
	}
	ConfigSetAvroSchema = string(buf)

	buf, err = json.Marshal(transmissionAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to generate Avro schema for transmission: %w", err))
	}
	TransmissionAvroSchema = string(buf)

	configSetCodec, err = goavro.NewCodec(ConfigSetAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to parse Avro schema for the config set: %w", err))
	}

	transmissionCodec, err = goavro.NewCodec(TransmissionAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to parse Avro schema for the latest transmission: %w", err))
	}

	// These codecs are used in tests but not in main, so the linter complains.
	_ = configSetCodec
	_ = transmissionCodec
}

/* Keeping the original schemas here for reference.

const ConfigSetAvroSchema = `
{
	"namespace": "link.chain.ocr2",
	"type": "record",
	"name": "config_set",
	"fields": [
		{"name": "block_number", "type": "bytes", "doc": "uint64 big endian"},
		{"name": "contract_config", "type": {"type": "record", "name": "contract_config", "fields": [
			{"name": "config_digest", "type": "bytes", "doc": "[32]byte"},
			{"name": "config_count", "type": "long", "doc": "uint32"},
			{"name": "signers", "type": {"type": "array", "items": "bytes"}},
			{"name": "transmitters", "type": {"type": "array", "items": "bytes"}},
			{"name": "f", "type": "int", "doc": "uint8"},
			{"name": "onchain_config", "type": [
				{"name": "ocr2_numerical_median_onchain_config", "type": "record", "fields": [
					{"name": "min", "type": "bytes", "doc": "*big.Int"},
					{"name": "max", "type": "bytes", "doc": "*big.Int"}
				]}
			]},
			{"name": "offchain_config_version", "type": "bytes", "doc": "uint64 big endian"},
			{"name": "offchain_config", "type": [
				{"name": "ocr2_offchain_config", "type": "record", "fields": [
					{"name": "delta_progress_nanoseconds", "type": "bytes", "doc": "uint64 big endian"},
					{"name": "delta_resend_nanoseconds", "type": "bytes", "doc": "uint64 big endian"},
					{"name": "delta_round_nanoseconds", "type": "bytes", "doc": "uint64 big endian"},
					{"name": "delta_grace_nanoseconds", "type": "bytes", "doc": "uint64 big endian"},
					{"name": "delta_stage_nanoseconds", "type": "bytes", "doc": "uint64 big endian"},
					{"name": "r_max", "type": "long", "doc": "uint32"},
					{"name": "s", "type": {"type": "array", "items": "long"}, "doc": "[]uint32"},
					{"name": "offchain_public_keys", "type": {"type": "array", "items": "bytes"}},
					{"name": "peer_ids", "type": {"type": "array", "items": "string"}},
					{"name": "reporting_plugin_config", "type": [
						{"name": "ocr2_numerical_median_offchain_config", "type": "record", "fields": [
							{"name": "alpha_report_infinite", "type": "boolean"},
							{"name": "alpha_report_ppb", "type": "bytes", "doc": "uint64 big endian"},
							{"name": "alpha_accept_infinite", "type": "boolean"},
							{"name": "alpha_accept_ppb", "type": "bytes", "doc": "uint64 big endian"},
							{"name": "delta_c_nanoseconds", "type": "bytes", "doc": "uint64 big endian"}
						]}
					]},
					{"name": "max_duration_query_nanoseconds", "type": "bytes", "doc": "uint64 big endian"},
					{"name": "max_duration_observation_nanoseconds", "type": "bytes", "doc": "uint64 big endian"},
					{"name": "max_duration_report_nanoseconds", "type": "bytes", "doc": "uint64 big endian"},
					{"name": "max_duration_should_accept_finalized_report_nanoseconds", "type": "bytes", "doc": "uint64 big endian"},
					{"name": "max_duration_should_transmit_accepted_report_nanoseconds", "type": "bytes", "doc": "uint64 big endian"},
					{"name": "shared_secret_encryptions", "type": {"type": "record", "name": "shared_secret_encryptions", "fields": [
						{"name": "diffie_hellman_point", "type": "bytes"},
						{"name": "shared_secret_hash", "type": "bytes"},
						{"name": "encryptions", "type": {"type": "array", "items": "bytes"}}
					]}}
				]}
			]}
		]}},
		{"name": "solana_program_state", "type": {"type": "record", "name": "solana_program_state", "fields": [
			{"name": "account_discriminator", "type": "bytes", "doc": "[8]byte" },
			{"name": "nonce", "type": "int" },
			{"name": "config", "type": {"type": "record", "name": "config", "fields": [
				{"name": "version", "type": "int" },
				{"name": "owner", "type": "bytes", "doc": "[32]byte" },
				{"name": "token_mint", "type": "bytes", "doc": "[32]byte" },
				{"name": "token_vault", "type": "bytes", "doc": "[32]byte" },
				{"name": "requester_access_controller", "type": "bytes", "doc": "[32]byte" },
				{"name": "billing_access_controller", "type": "bytes", "doc": "[32]byte" },
				{"name": "min_answer", "type": "bytes", "doc": "big.Int" },
				{"name": "max_answer", "type": "bytes", "doc": "big.Int" },
				{"name": "decimals", "type": "int" },
				{"name": "description", "type": "bytes", "doc": "[32]byte" },
				{"name": "f", "type": "int" },
				{"name": "config_count", "type": "int" },
				{"name": "latest_config_digest", "type": "bytes", "doc": "[32]byte" },
				{"name": "latest_config_block_number", "type": "long" },
				{"name": "latest_aggregator_round_id", "type": "int" },
				{"name": "epoch", "type": "int" },
				{"name": "round", "type": "int" },
				{"name": "billing", "type": {"type": "record", "name": "billing", "fields": [
					{"name": "observation_payment", "type": "int"}
				]}},
				{"name": "validator", "type": "bytes", "doc": "[32]byte"},
				{"name": "flagging_threshold", "type": "long"}
			]}},
			{"name": "oracles", "type": {"type": "array", "items":
				{"name": "oracle", "type": "record", "fields": [
					{"name": "transmitter", "type": "bytes", "doc": "[32]byte" },
					{"name": "signer", "type": {"type": "record", "name": "signer", "fields": [
						{"name": "key", "type": "bytes", "doc": "[20]byte" }
					]}},
					{"name": "payee", "type": "bytes", "doc": "[32]byte" },
					{"name": "proposed_payee", "type": "bytes", "doc": "[32]byte" },
					{"name": "payment", "type": "long" },
					{"name": "from_round_id", "type": "int" }
				]}
			}},
			{"name": "leftover_payment", "type": {"type": "array", "items":
				{"name": "leftover_payment", "type": "record", "fields": [
					{"name": "payee", "type": "bytes", "doc": "[32]byte" },
					{"name": "amount", "type": "long" }
				]}
			}},
			{"name": "leftover_payment_len", "type": "int" },
			{"name": "transmissions", "type": "bytes", "doc": "[32]byte" }
		]}},
		{"name": "solana_chain_config", "type": {"type": "record", "name": "solana_chain_config", "fields": [
			{"name": "network_name", "type": "string"},
			{"name": "network_id", "type": "string"},
			{"name": "chain_id", "type": "string"}
		]}},
		{"name": "feed_config", "type": {"type": "record", "name": "feed_config", "fields": [
			{"name": "feed_name", "type": "string"},
			{"name": "feed_path", "type": "string"},
			{"name": "symbol", "type": "string"},
			{"name": "heartbeat_sec", "type": "long"},
			{"name": "contract_type", "type": "string"},
			{"name": "contract_status", "type": "string"},
			{"name": "contract_address", "type": "bytes", "doc": "[32]byte"},
			{"name": "transmissions_account", "type": "bytes", "doc": "[32]byte"},
			{"name": "state_account", "type": "bytes", "doc": "[32]byte"}
		]}}
	]
}
`

const TransmissionAvroSchema = `
{
  "namespace": "link.chain.ocr2",
  "type": "record",
  "name": "transmission",
  "fields": [
		{"name": "block_number", "type": "bytes", "doc": "uint64 big endian"},
		{"name": "answer", "type": {"type": "record", "name": "answer", "fields": [
			{"name": "data", "type": "bytes", "doc": "*big.Int"},
			{"name": "timestamp", "type": "long", "doc": "uint32"}
		]}},
		{"name": "solana_chain_config", "type": {"type": "record", "name": "solana_chain_config", "fields": [
			{"name": "network_name", "type": "string"},
			{"name": "network_id", "type": "string"},
			{"name": "chain_id", "type": "string"}
		]}},
		{"name": "feed_config", "type": {"type": "record", "name": "feed_config", "fields": [
			{"name": "feed_name", "type": "string"},
			{"name": "feed_path", "type": "string"},
			{"name": "symbol", "type": "string"},
			{"name": "heartbeat_sec", "type": "long"},
			{"name": "contract_type", "type": "string"},
			{"name": "contract_status", "type": "string"},
			{"name": "contract_address", "type": "bytes", "doc": "[32]byte"},
			{"name": "transmissions_account", "type": "bytes", "doc": "[32]byte"},
			{"name": "state_account", "type": "bytes", "doc": "[32]byte"}
		]}}
  ]
}
`
*/
