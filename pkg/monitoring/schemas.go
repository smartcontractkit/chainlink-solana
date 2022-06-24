package monitoring

import (
	"encoding/json"
	"fmt"

	"github.com/linkedin/goavro"
	"github.com/smartcontractkit/chainlink-relay/pkg/monitoring/avro"
)

// Taken from https://github.com/smartcontractkit/chainlink-solana/blob/c2f59be377d85feb451f62b5d687807fb90fd0dd/pkg/monitoring/avro_schemas.go
// Note, make sure you check your schemas with this online tool https://json-schema-validator.herokuapp.com/avro.jsp

var stateAvroSchema = avro.Record("state_account", avro.Opts{Namespace: "link.chain.ocr2"}, avro.Fields{
	avro.Field("account_public_key", avro.Opts{Doc: "[32]byte solana.PublicKey"}, avro.Bytes),

	avro.Field("slot", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
	avro.Field("lamports", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
	avro.Field("owner", avro.Opts{Doc: "[32]byte solana.PublicKey"}, avro.Bytes),
	avro.Field("executable", avro.Opts{}, avro.Boolean),
	avro.Field("rent_epoch", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),

	avro.Field("state", avro.Opts{}, avro.Record("state", avro.Opts{}, avro.Fields{
		avro.Field("account_discriminator", avro.Opts{Doc: "[8]byte"}, avro.Bytes),
		avro.Field("version", avro.Opts{Doc: "uint8"}, avro.Int),
		avro.Field("nonce", avro.Opts{Doc: "uint8"}, avro.Int),
		avro.Field("transmissions", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
		avro.Field("config", avro.Opts{}, avro.Record("config", avro.Opts{}, avro.Fields{
			avro.Field("owner", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
			avro.Field("proposed_owner", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
			avro.Field("token_mint", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
			avro.Field("token_vault", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
			avro.Field("requester_access_controller", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
			avro.Field("billing_access_controller", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
			avro.Field("min_answer", avro.Opts{Doc: "big.Int"}, avro.Bytes),
			avro.Field("max_answer", avro.Opts{Doc: "big.Int"}, avro.Bytes),
			avro.Field("f", avro.Opts{Doc: "uint8"}, avro.Int),
			avro.Field("round", avro.Opts{Doc: "uint8"}, avro.Int),
			avro.Field("epoch", avro.Opts{Doc: "uint32"}, avro.Long),
			avro.Field("latest_aggregator_round_id", avro.Opts{Doc: "uint32"}, avro.Long),
			avro.Field("latest_transmitter", avro.Opts{Doc: "[32]bytes"}, avro.Bytes),
			avro.Field("config_count", avro.Opts{Doc: "uint32"}, avro.Long),
			avro.Field("latest_config_digest", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
			avro.Field("latest_config_block_number", avro.Opts{Doc: "uint64"}, avro.Bytes),
			avro.Field("billing", avro.Opts{}, avro.Record("billing", avro.Opts{}, avro.Fields{
				avro.Field("observation_payment", avro.Opts{Doc: "uint32"}, avro.Long),
				avro.Field("transmission_payment", avro.Opts{Doc: "uint32"}, avro.Long),
			})),
		})),

		avro.Field("offchain_config_version", avro.Opts{Doc: "uint64"}, avro.Bytes),
		avro.Field("offchain_config", avro.Opts{}, avro.Union{
			avro.Record("ocr2_offchain_config", avro.Opts{}, avro.Fields{
				avro.Field("delta_progress_nanoseconds", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
				avro.Field("delta_resend_nanoseconds", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
				avro.Field("delta_round_nanoseconds", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
				avro.Field("delta_grace_nanoseconds", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
				avro.Field("delta_stage_nanoseconds", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
				avro.Field("r_max", avro.Opts{Doc: "uint32"}, avro.Long),
				avro.Field("s", avro.Opts{Doc: "[]uint32"}, avro.Array(avro.Long)),
				avro.Field("offchain_public_keys", avro.Opts{}, avro.Array(avro.Bytes)),
				avro.Field("peer_ids", avro.Opts{}, avro.Array(avro.String)),
				avro.Field("reporting_plugin_config", avro.Opts{}, avro.Union{
					avro.Record("ocr2_numerical_median_offchain_config", avro.Opts{}, avro.Fields{
						avro.Field("alpha_report_infinite", avro.Opts{}, avro.Boolean),
						avro.Field("alpha_report_ppb", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
						avro.Field("alpha_accept_infinite", avro.Opts{}, avro.Boolean),
						avro.Field("alpha_accept_ppb", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
						avro.Field("delta_c_nanoseconds", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
					}),
				}),
				avro.Field("max_duration_query_nanoseconds", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
				avro.Field("max_duration_observation_nanoseconds", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
				avro.Field("max_duration_report_nanoseconds", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
				avro.Field("max_duration_should_accept_finalized_report_nanoseconds", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
				avro.Field("max_duration_should_transmit_accepted_report_nanoseconds", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
				avro.Field("shared_secret_encryptions", avro.Opts{}, avro.Record("shared_secret_encryptions", avro.Opts{}, avro.Fields{
					avro.Field("diffie_hellman_point", avro.Opts{}, avro.Bytes),
					avro.Field("shared_secret_hash", avro.Opts{}, avro.Bytes),
					avro.Field("encryptions", avro.Opts{}, avro.Array(avro.Bytes)),
				})),
			}),
		}),

		avro.Field("oracles", avro.Opts{}, avro.Array(avro.Record("oracle", avro.Opts{}, avro.Fields{
			avro.Field("transmitter", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
			avro.Field("signer", avro.Opts{}, avro.Record("signer", avro.Opts{}, avro.Fields{
				avro.Field("key", avro.Opts{Doc: "[20]byte"}, avro.Bytes),
			})),
			avro.Field("payee", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
			avro.Field("proposed_payee", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
			avro.Field("from_round_id", avro.Opts{Doc: "uint32"}, avro.Long),
			avro.Field("payment", avro.Opts{Doc: "uint64"}, avro.Bytes),
		}))),
	})),
})

var transmissionAvroSchema = avro.Record("transmissions_account", avro.Opts{Namespace: "link.chain.ocr2"}, avro.Fields{
	avro.Field("account_public_key", avro.Opts{Doc: "[32]byte solana.PublicKey"}, avro.Bytes),

	avro.Field("slot", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
	avro.Field("lamports", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
	avro.Field("owner", avro.Opts{Doc: "[32]byte solana.PublicKey"}, avro.Bytes),
	avro.Field("executable", avro.Opts{}, avro.Boolean),
	avro.Field("rent_epoch", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),

	avro.Field("header", avro.Opts{}, avro.Record("header", avro.Opts{}, avro.Fields{
		avro.Field("version", avro.Opts{Doc: "uint8"}, avro.Int),
		avro.Field("state", avro.Opts{Doc: "uint8"}, avro.Int),
		avro.Field("owner", avro.Opts{Doc: "[32]byte solana.PublicKey"}, avro.Bytes),
		avro.Field("proposed_owner", avro.Opts{Doc: "[32]byte solana.PublicKey"}, avro.Bytes),
		avro.Field("writer", avro.Opts{Doc: "[32]byte solana.PublicKey"}, avro.Bytes),
		avro.Field("description", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
		avro.Field("decimals", avro.Opts{Doc: "uint8"}, avro.Int),
		avro.Field("flagging_threshold", avro.Opts{Doc: "uint32"}, avro.Long),
		avro.Field("latest_round_id", avro.Opts{Doc: "uint32"}, avro.Long),
		avro.Field("granularity", avro.Opts{Doc: "uint8"}, avro.Int),
		avro.Field("live_length", avro.Opts{Doc: "uint32"}, avro.Long),
		avro.Field("live_cursor", avro.Opts{Doc: "uint32"}, avro.Long),
		avro.Field("historical_cursor", avro.Opts{Doc: "uint32"}, avro.Long),
	})),

	avro.Field("transmission", avro.Opts{}, avro.Record("transmission", avro.Opts{}, avro.Fields{
		avro.Field("slot", avro.Opts{Doc: "uint64"}, avro.Bytes),
		avro.Field("timestamp", avro.Opts{Doc: "int64"}, avro.Long),
		avro.Field("answer", avro.Opts{Doc: "bin.Int128"}, avro.Bytes),
	})),
})

var eventsAvroSchema = avro.Record("events", avro.Opts{Namespace: "link.chain.ocr2"}, avro.Fields{
	avro.Field("program_public_key", avro.Opts{Doc: "[32]byte solana.PublicKey"}, avro.Bytes),

	avro.Field("slot", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
	avro.Field("signature", avro.Opts{Doc: "[64]byte solana.Signature"}, avro.Bytes),
	avro.Field("err", avro.Opts{}, avro.String),

	avro.Field("events", avro.Opts{}, avro.Array(avro.Union{
		avro.Record("ocr2_event_set_config", avro.Opts{}, avro.Fields{
			avro.Field("config_digest", avro.Opts{Doc: "[32]uint8"}, avro.Bytes),
			avro.Field("f", avro.Opts{Doc: "uint8"}, avro.Int),
			avro.Field("signers", avro.Opts{Doc: "[][20]uint8"}, avro.Array(avro.Bytes)),
		}),
		avro.Record("ocr2_event_set_billing", avro.Opts{}, avro.Fields{
			avro.Field("observation_payment_gjuels", avro.Opts{Doc: "uint32"}, avro.Long),
			avro.Field("transmission_payment_gjuels", avro.Opts{Doc: "uint32"}, avro.Long),
		}),
		avro.Record("ocr2_event_round_requested", avro.Opts{}, avro.Fields{
			avro.Field("config_digest", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
			avro.Field("requester", avro.Opts{Doc: "[32]byte solana.PublicKey"}, avro.Bytes),
			avro.Field("epoch", avro.Opts{Doc: "uint32"}, avro.Long),
			avro.Field("round", avro.Opts{Doc: "uint8"}, avro.Int),
		}),
		avro.Record("ocr2_event_new_transmission", avro.Opts{}, avro.Fields{
			avro.Field("round_id", avro.Opts{Doc: "uint32"}, avro.Long),
			avro.Field("config_digest", avro.Opts{Doc: "[32]uint8"}, avro.Bytes),
			avro.Field("answer", avro.Opts{Doc: "bin.Int128"}, avro.Bytes),
			avro.Field("transmitter", avro.Opts{Doc: "uint8"}, avro.Int),
			avro.Field("observations_timestamp", avro.Opts{Doc: "uint32"}, avro.Long),
			avro.Field("observer_count", avro.Opts{Doc: "uint8"}, avro.Int),
			avro.Field("observers", avro.Opts{Doc: "[19]uint8"}, avro.Array(avro.Int)),
			avro.Field("juels_per_lamport", avro.Opts{Doc: "uint64}"}, avro.Bytes),
			avro.Field("reimbursement_gjuels", avro.Opts{Doc: "uint64}"}, avro.Bytes),
		}),
	})),
})

var blockAvroSchema = avro.Record("block", avro.Opts{Namespace: "link.chain.ocr2"}, avro.Fields{
	avro.Field("program_public_key", avro.Opts{Doc: "[32]byte solana.PublicKey"}, avro.Bytes),

	avro.Field("slot", avro.Opts{Doc: "uint64 big endian"}, avro.Bytes),
	avro.Field("err", avro.Opts{}, avro.String),
	avro.Field("blockhash", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
	avro.Field("previous_blockhash", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
	avro.Field("parent_slot", avro.Opts{Doc: "uint64"}, avro.Bytes),
	avro.Field("block_time_sec", avro.Opts{Doc: "uint64"}, avro.Bytes),
	avro.Field("block_height", avro.Opts{Doc: "uint64"}, avro.Bytes),

	avro.Field("transactions", avro.Opts{}, avro.Array(avro.Record("transaction", avro.Opts{}, avro.Fields{
		avro.Field("data", avro.Opts{}, avro.Bytes),
		avro.Field("meta", avro.Opts{}, avro.Record("meta", avro.Opts{}, avro.Fields{
			avro.Field("err", avro.Opts{}, avro.String),
			avro.Field("fee", avro.Opts{Doc: "uint64"}, avro.Bytes),
			avro.Field("pre_balances", avro.Opts{Doc: "uint64"}, avro.Array(avro.Bytes)),
			avro.Field("post_balances", avro.Opts{Doc: "uint64"}, avro.Array(avro.Bytes)),
			avro.Field("pre_token_balances", avro.Opts{}, avro.Array(avro.Record("pre_token_balance", avro.Opts{}, avro.Fields{
				avro.Field("account_index", avro.Opts{Doc: "int32"}, avro.Int),
				avro.Field("lamports", avro.Opts{Doc: "uint64"}, avro.Bytes),
				avro.Field("post_balance", avro.Opts{Doc: "uint64"}, avro.Bytes),
				avro.Field("reward_type", avro.Opts{}, avro.String),
				avro.Field("commission", avro.Opts{Doc: "byte"}, avro.Int),
			}))),
			avro.Field("post_token_balances", avro.Opts{}, avro.Array(avro.Record("post_token_balance", avro.Opts{}, avro.Fields{
				avro.Field("account_index", avro.Opts{Doc: "int32"}, avro.Int),
				avro.Field("lamports", avro.Opts{Doc: "uint64"}, avro.Bytes),
				avro.Field("post_balance", avro.Opts{Doc: "uint64"}, avro.Bytes),
				avro.Field("reward_type", avro.Opts{}, avro.String),
				avro.Field("commission", avro.Opts{Doc: "byte"}, avro.Int),
			}))),
			avro.Field("log_messages", avro.Opts{}, avro.Array(avro.String)),
			avro.Field("rewards", avro.Opts{}, avro.Array(avro.Record("tx_rewards", avro.Opts{}, avro.Fields{
				avro.Field("public_key", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
				avro.Field("lamports", avro.Opts{Doc: "uint64"}, avro.Bytes),
				avro.Field("post_balance", avro.Opts{Doc: "uint64"}, avro.Bytes),
				avro.Field("reward_type", avro.Opts{}, avro.String),
				avro.Field("commission", avro.Opts{Doc: "uint8"}, avro.Int),
			}))),
		})),
	}))),
	avro.Field("rewards", avro.Opts{}, avro.Array(avro.Record("block_rewards", avro.Opts{}, avro.Fields{
		avro.Field("public_key", avro.Opts{Doc: "[32]byte"}, avro.Bytes),
		avro.Field("lamports", avro.Opts{Doc: "uint64"}, avro.Bytes),
		avro.Field("post_balance", avro.Opts{Doc: "uint64"}, avro.Bytes),
		avro.Field("reward_type", avro.Opts{}, avro.String),
		avro.Field("commission", avro.Opts{Doc: "uint8"}, avro.Int),
	}))),
})

var (
	// Avro schemas to sync with the registry
	StateAvroSchema        string
	TransmissionAvroSchema string
	EventsAvroSchema       string
	BlockAvroSchema        string

	// These codecs are used in tests
	stateCodec        *goavro.Codec
	transmissionCodec *goavro.Codec
	eventsCodec       *goavro.Codec
	blockCodec        *goavro.Codec
)

func init() {
	var err error
	StateAvroSchema, stateCodec, err = parseSchema(stateAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to generate Avro schema for State objects: %w", err))
	}
	TransmissionAvroSchema, transmissionCodec, err = parseSchema(transmissionAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to generate Avro schema for Transmisison objects: %w", err))
	}
	EventsAvroSchema, eventsCodec, err = parseSchema(eventsAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to generate Avro schema for Events objects: %w", err))
	}
	BlockAvroSchema, blockCodec, err = parseSchema(blockAvroSchema)
	if err != nil {
		panic(fmt.Errorf("failed to generate Avro schema for Block objects: %w", err))
	}

	// These are only needed in tests!
	_ = stateCodec
	_ = transmissionCodec
	_ = eventsCodec
	_ = blockCodec
}

func parseSchema(schema avro.Schema) (jsonEncoded string, codec *goavro.Codec, err error) {
	buf, err := json.Marshal(schema)
	if err != nil {
		return "", nil, fmt.Errorf("failed to encode Avro schema to JSON: %w", err)
	}
	jsonEncoded = string(buf)
	codec, err = goavro.NewCodec(jsonEncoded)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse JSON-encoded Avro schema into a codec: %w", err)
	}
	return jsonEncoded, codec, nil
}
