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
			// These fields (validator, flagging_threshold, decimals, description) have been removed from the program's
			// state but they have been kept here to preserve backwards compatibility.
			Field("validator", Opts{Doc: "[32]byte"}, Bytes),
			Field("flagging_threshold", Opts{Doc: "uint32"}, Long),
			Field("decimals", Opts{Doc: "uint8"}, Int),
			Field("description", Opts{Doc: "[32]byte"}, Bytes),
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
	ConfigSetAvroSchema           string
	ConfigSetSimplifiedAvroSchema string
	TransmissionAvroSchema        string

	// These codecs are used in tests
	configSetCodec           *goavro.Codec
	configSetSimplifiedCodec *goavro.Codec
	transmissionCodec        *goavro.Codec
)

func init() {
	var err error
	buf, err := json.Marshal(configSetAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to generate Avro schema for config set: %w", err))
	}
	ConfigSetAvroSchema = string(buf)

	buf, err = json.Marshal(configSetSimplifiedAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to generate Avro schema for configSimplified: %w", err))
	}
	ConfigSetSimplifiedAvroSchema = string(buf)

	buf, err = json.Marshal(transmissionAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to generate Avro schema for transmission: %w", err))
	}
	TransmissionAvroSchema = string(buf)

	configSetCodec, err = goavro.NewCodec(ConfigSetAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to parse Avro schema for the config set: %w", err))
	}

	configSetSimplifiedCodec, err = goavro.NewCodec(ConfigSetSimplifiedAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to parse Avro schema for the latest configSetSimplified: %w", err))
	}

	transmissionCodec, err = goavro.NewCodec(TransmissionAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to parse Avro schema for the latest transmission: %w", err))
	}

	// These codecs are used in tests but not in main, so the linter complains.
	_ = configSetCodec
	_ = configSetSimplifiedCodec
	_ = transmissionCodec
}
