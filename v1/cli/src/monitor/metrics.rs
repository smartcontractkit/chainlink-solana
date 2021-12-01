//! Initalize and register prometheus metrics
use anyhow::{self, anyhow as anyhow_err};
use bigdecimal::{BigDecimal, FromPrimitive, ToPrimitive};
use once_cell::sync::Lazy;
use prometheus::{self, register_gauge_vec};
use solana_sdk::pubkey::Pubkey;
use std::collections::HashMap;

use super::data;

//
// Aggregator / Feed Metrics
//

static FEED_METADATA: Lazy<prometheus::GaugeVec> = Lazy::new(|| {
    register_gauge_vec!(
        "feed_contract_metadata",
        "Exposes metadata for individual feeds.",
        &[
            "contract_address",
            "contract_type",
            "contract_status",
            "feed_name",
            "feed_path",
            "network_name",
        ]
    )
    .unwrap()
});

static NODE_METADATA: Lazy<prometheus::GaugeVec> = Lazy::new(|| {
    register_gauge_vec!(
        "node_metadata",
        "Exposes metadata for node operators.",
        &["network_name", "oracle_name", "sender"]
    )
    .unwrap()
});

static ANSWERS: Lazy<prometheus::GaugeVec> = Lazy::new(|| {
    register_gauge_vec!(
        "flux_monitor_answers",
        "Reports the current on-chain price for a feed",
        &[
            "contract_address",
            "contract_status",
            "contract_type",
            "feed_name",
            "feed_path",
            "network_name"
        ]
    )
    .unwrap()
});

static SUBMISSION_VALUES: Lazy<prometheus::GaugeVec> = Lazy::new(|| {
    register_gauge_vec!(
        "flux_monitor_submission_received_values",
        "Reports the current submission value for an oracle on a feed.",
        &[
            "contract_address",
            "contract_status",
            "contract_type",
            "feed_name",
            "feed_path",
            "network_name",
            "oracle_name",
            "sender"
        ]
    )
    .unwrap()
});

static SUBMISSION_TIMESTAMP: Lazy<prometheus::GaugeVec> = Lazy::new(|| {
    register_gauge_vec!(
        "flux_monitor_submission_received_timestamp",
        "Reports the current submission timestamp for an oracle on a feed.",
        &[
            "contract_address",
            "contract_status",
            "contract_type",
            "feed_name",
            "feed_path",
            "network_name",
            "oracle_name",
            "sender"
        ]
    )
    .unwrap()
});

//
// Application / System Metrics
//

static CONFIGURATIONS_TOTAL: Lazy<prometheus::GaugeVec> = Lazy::new(|| {
    register_gauge_vec!(
        "configurations_total",
        "Tracks successful configuration loads from weiwatchers.",
        &["network_name", "product", "type"]
    )
    .unwrap()
});

static RPC_SUCCESS: Lazy<prometheus::GaugeVec> = Lazy::new(|| {
    register_gauge_vec!(
        "rpc_success",
        "Similar to prometheus 'up' metric, but for RPC calls.",
        &["network_name"]
    )
    .unwrap()
});

/// Simple structure to record aggregator metrics, given a
/// configurable network name, so it works across deployments.
#[derive(Clone)]
pub struct Recorder {
    network: String,
    oracles_by_key: HashMap<Pubkey, String>,
}

impl Recorder {
    pub fn new<S>(network: S) -> Self
    where
        S: Into<String>,
    {
        Recorder {
            network: network.into(),
            oracles_by_key: HashMap::new(),
        }
    }

    /// Initialize metadata gauges for all feeds
    pub fn init_feed_metadata(&self, feeds: &[data::Feed]) {
        for feed in feeds {
            FEED_METADATA
                .with_label_values(&[
                    &feed.contract_address.to_string(),
                    &feed.contract_type,
                    "live",
                    &feed.name,
                    &feed.path,
                    &self.network,
                ])
                .set(1.0);
        }
    }

    /// Initiaize metadata gauges for all node operators.
    pub fn init_node_metadata(&mut self, nodes: &[data::Node]) {
        self.oracles_by_key = HashMap::new();

        for node in nodes {
            self.oracles_by_key
                .insert(node.node_address[0].clone(), node.name.clone());

            NODE_METADATA
                .with_label_values(&[&self.network, &node.id, &node.node_address[0].to_string()])
                .set(1.0);
        }
    }

    /// Set all metrics for the given feed (by public key) and its account state.
    pub fn set(&self, feed: &data::Feed, state: program::state::Aggregator) -> anyhow::Result<()> {
        // ensure we have a value to work with
        let answer = state
            .answer
            .ok_or_else(|| anyhow_err!("no answer present"))?;
        // stringify feed / contract address
        let contract = feed.contract_address.to_string();

        // set the current answer
        ANSWERS
            .with_label_values(&[
                &contract,
                "live",
                &feed.contract_type,
                &feed.name,
                &feed.path,
                &self.network,
            ])
            .set(value_as_float(answer, state.config.decimals)?);

        // set the current answer and timestamp, by oracle
        // `state.submissions` is an array of tuples (timestamp, submission value)
        for (i, oracle) in state.config.oracles.iter().enumerate() {
            SUBMISSION_VALUES
                .with_label_values(&[
                    &contract,
                    "live",
                    &feed.contract_type,
                    &feed.name,
                    &feed.path,
                    &self.network,
                    &self.oracles_by_key.get(oracle).unwrap(),
                    &oracle.to_string(),
                ])
                .set(value_as_float(
                    state.submissions[i].1,
                    state.config.decimals,
                )?);

            SUBMISSION_TIMESTAMP
                .with_label_values(&[
                    &contract,
                    "live",
                    &feed.contract_type,
                    &feed.name,
                    &feed.path,
                    &self.network,
                    &self.oracles_by_key.get(oracle).unwrap(),
                    &oracle.to_string(),
                ])
                .set(state.submissions[i].0 as f64);
        }

        Ok(())
    }

    /// Set the configuration metric (1 for success, 0 for failure)
    pub fn config_load(&self, success: bool) {
        CONFIGURATIONS_TOTAL
            .with_label_values(&[&self.network, "solana", "feeds"])
            .set(if success { 1.0 } else { 0.0 });
    }

    /// Set the RPC metric (1 for success, 0 for failure)
    pub fn rpc_success(&self, success: bool) {
        RPC_SUCCESS
            .with_label_values(&[&self.network])
            .set(if success { 1.0 } else { 0.0 });
    }
}

/// Reset all metrics, regardless of their label values.
/// This is a simple way to add/remove node/feed metadata "atomically" without having to diff
/// against previously stored values.
pub fn reset() {
    FEED_METADATA.reset();
    NODE_METADATA.reset();
    ANSWERS.reset();
    SUBMISSION_VALUES.reset();
    SUBMISSION_TIMESTAMP.reset();
}

// Represent the program state `Value` (u128) as a 64-bit float, using BigDecimal for calcuations.
fn value_as_float(v: program::state::Value, decimals: u8) -> anyhow::Result<f64> {
    let big = BigDecimal::from_u128(v)
        .ok_or_else(|| anyhow_err!("unable to convert answer to big decimal"))?;

    let div = big / BigDecimal::from_u128(10u128.pow(decimals as u32)).unwrap();

    Ok(div
        .to_f64()
        .ok_or_else(|| anyhow_err!("unable to represent value as f64"))?)
}

#[test]
fn test_value_as_float() {
    assert_eq!(value_as_float(1u128, 5).unwrap(), 0.00001);
    assert_eq!(value_as_float(10u128, 5).unwrap(), 0.0001);
    assert_eq!(value_as_float(1234567u128, 5).unwrap(), 12.34567);
}
