//! Initalize and register prometheus metrics
use anyhow::{self, anyhow as anyhow_err};
use bigdecimal::{BigDecimal, FromPrimitive, ToPrimitive};
use once_cell::sync::Lazy;
use prometheus::{self, register_gauge_vec};

use super::rdd;

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
        &["contract_address"]
    )
    .unwrap()
});

static SUBMISSION_VALUES: Lazy<prometheus::GaugeVec> = Lazy::new(|| {
    register_gauge_vec!(
        "flux_monitor_submission_received_values",
        "Reports the current submission value for an oracle on a feed.",
        &["contract_address", "sender"]
    )
    .unwrap()
});

/// Initialize metadata gauges for all feeds
pub fn init_feed_metadata(feeds: &[rdd::Feed]) {
    for feed in feeds {
        FEED_METADATA
            .with_label_values(&[
                &feed.contract_address.to_string(),
                &feed.contract_type,
                &feed.name,
                &feed.path,
                &"solana/devnet",
            ])
            .set(1.0);
    }
}

/// Initiaize metadata gauges for all node operators.
pub fn init_node_metadata(nodes: &[rdd::Node]) {
    for node in nodes {
        NODE_METADATA
            .with_label_values(&[
                &"solana/devnet",
                &node.id,
                &node.node_address[0].to_string(),
            ])
            .set(1.0);
    }
}

/// Set all metrics for the given feed (by public key) and its account state.
pub fn set(feed: &rdd::Feed, state: program::state::Aggregator) -> anyhow::Result<()> {
    // ensure we have a value to work with
    let answer = state
        .answer
        .ok_or_else(|| anyhow_err!("no answer present"))?;
    // stringify feed / contract address
    let contract = feed.contract_address.to_string();

    // set the current answer
    ANSWERS
        .with_label_values(&[&contract])
        .set(value_as_float(answer, state.config.decimals)?);

    // set the current answer, by oracle
    for (i, oracle) in state.config.oracles.iter().enumerate() {
        SUBMISSION_VALUES
            .with_label_values(&[&contract, &oracle.to_string()])
            .set(value_as_float(
                state.submissions[i].1,
                state.config.decimals,
            )?);
    }

    Ok(())
}

/// Reset all metrics, regardless of their label values.
/// This is a simple way to add/remove node/feed metadata "atomically" without having to diff
/// against previously stored values.
pub fn reset() {
    FEED_METADATA.reset();
    NODE_METADATA.reset();
    ANSWERS.reset();
    SUBMISSION_VALUES.reset();
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
