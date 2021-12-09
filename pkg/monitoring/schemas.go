package monitoring

import (
	"fmt"

	"github.com/linkedin/goavro"
)

// See https://avro.apache.org/docs/current/spec.html#schemas

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
				{"name": "flagging_threshold", "type": "int"}
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

// These codecs are used in tests
var (
	configSetCodec    *goavro.Codec
	transmissionCodec *goavro.Codec
)

func init() {
	var err error
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
