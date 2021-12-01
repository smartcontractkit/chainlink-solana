use prometheus::{self, Encoder};
use solana_client::rpc_client::RpcClient;
use solana_client::rpc_response::RpcVersionInfo;
use solana_sdk::commitment_config::CommitmentConfig;
use std::net::SocketAddr;
use std::time;
use tokio::sync::mpsc::{channel, Receiver};
use tracing::{debug, error, info};
use warp::Filter;

use crate::{Monitor, Scheme};

mod metrics;
mod rdd;

/// Entrypoint for monitor subcommand.
pub async fn monitor(config: Monitor) {
    info!("starting monitor");

    let (client, version) =
        new_client(config.rpc, config.scheme).expect("could not create rpc client");
    let epoch = client.get_epoch_info().expect("could not get epoch info");

    info!("rpc version: {:?}", version);
    info!("epoch: {:?}", epoch);

    let (tx, rx) = channel(1);
    let nodes = rdd::nodes().await.unwrap();
    let feeds = rdd::feeds().await.unwrap();

    // hydrate the initial query state
    tx.send((nodes, feeds)).await.unwrap();

    tokio::try_join!(
        rdd::poll(tx, config.rdd_interval.into()),
        query(client, config.query_interval.into(), rx),
        http(config.listen)
    )
    .unwrap();
}

/// Query the state of every account associated with the given program IDs (feeds).
/// Wait for new metdata to come from RDD, and if there is, re-initialize the metrics.
/// Otheriwse, at every interval, query for all feed accounts, deserialze the state, and update metrics.
async fn query(
    client: RpcClient,
    interval: time::Duration,
    mut metadata: Receiver<(Vec<rdd::Node>, Vec<rdd::Feed>)>,
) -> anyhow::Result<()> {
    let mut interval = tokio::time::interval(interval);
    let mut feeds: Vec<rdd::Feed> = vec![];

    loop {
        tokio::select! {
            Some((nodes, new_feeds)) = metadata.recv() => {
                debug!("new rdd metadata");

                metrics::reset();
                metrics::init_node_metadata(&nodes);
                metrics::init_feed_metadata(&new_feeds);

                feeds = new_feeds;
            },
            _ = interval.tick() => {
                debug!("querying feed state");
                // query state for every aggregator
                for feed in &feeds {
                    match client.get_account(&feed.contract_address) {
                        Ok(account) => {
                            let state: program::state::Aggregator =
                                program::processor::get_state_from_data(&account.data).unwrap();

                            if let Err(err) = metrics::set(&feed, state) {
                                error!("failed to set metrics: {:?}", err);
                            }
                        }
                        Err(err) => error!(
                            "error getting account data for feed {:?} ({:?}) {:?}",
                            feed.path, feed.contract_address, err
                        ),
                    }
                }
            }
        }
    }
}

/// Serve HTTP for prometheus metrics.
async fn http(addr: SocketAddr) -> anyhow::Result<()> {
    let metrics = warp::get().and(warp::path("metrics")).map(|| {
        let encoder = prometheus::TextEncoder::new();
        let metrics = prometheus::gather();

        let mut buffer = vec![];
        encoder.encode(&metrics, &mut buffer).unwrap();

        warp::http::Response::builder().body(String::from_utf8(buffer).unwrap())
    });

    warp::serve(metrics).run(addr).await;
    Ok(())
}

/// Create a new RpcClient, ensuring we can query the RPC version.
fn new_client(addr: String, scheme: Scheme) -> anyhow::Result<(RpcClient, RpcVersionInfo)> {
    debug!("connecting to {:?}, scheme = {:?}", addr, scheme);
    let url = match scheme {
        Scheme::Http => format!("http://{}/", addr),
        Scheme::Https => format!("https://{}/", addr),
    };

    // TODO(nickmonad): always use confirmed?
    let client = RpcClient::new_with_commitment(url, CommitmentConfig::confirmed());
    let version = client.get_version()?;

    Ok((client, version))
}
