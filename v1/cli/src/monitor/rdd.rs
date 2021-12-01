use anyhow::{self, anyhow as anyhow_err};
use hyper::{body::to_bytes, Client};
use hyper_tls::HttpsConnector;
use solana_sdk::pubkey::Pubkey;
use std::str::FromStr;
use std::time;
use tokio::sync::mpsc::Sender;
use tracing::debug;

use super::data::{Feed, FeedRaw, Node, NodeRaw};

pub async fn poll(
    tx: Sender<anyhow::Result<(Vec<Node>, Vec<Feed>)>>,
    interval: time::Duration,
) -> anyhow::Result<()> {
    let mut interval = tokio::time::interval(interval);
    loop {
        interval.tick().await;
        debug!("polling rdd");

        match tokio::try_join!(nodes(), feeds()) {
            Ok((nodes, feeds)) => tx.send(Ok((nodes, feeds))).await?,
            Err(err) => {
                tx.send(Err(anyhow_err!("could not get config: {:?}", err)))
                    .await?
            }
        }
    }
}

pub async fn nodes() -> anyhow::Result<Vec<Node>> {
    let client = Client::builder().build::<_, hyper::Body>(HttpsConnector::new());
    let uri = "https://weiwatchers.com/nodes-solana-devnet.json".parse()?;

    let response = client.get(uri).await?;
    let nodes: Vec<NodeRaw> = serde_json::from_slice(&to_bytes(response.into_body()).await?)?;
    let nodes: Vec<Node> = nodes
        .iter()
        .map(|n| {
            // parse the contract address from rdd format to native value
            // TODO: handle errors here? set a metric used in an alert?
            let node_address: Vec<Pubkey> = n
                .node_address
                .iter()
                .map(|addr| Pubkey::from_str(&addr).unwrap())
                .collect();

            Node {
                id: n.id.clone(),
                name: n.name.clone(),
                status: n.status.clone(),
                node_address,
            }
        })
        .collect();

    Ok(nodes)
}

pub async fn feeds() -> anyhow::Result<Vec<Feed>> {
    let client = Client::builder().build::<_, hyper::Body>(HttpsConnector::new());
    let uri = "https://weiwatchers.com/feeds-solana-devnet.json".parse()?;

    let response = client.get(uri).await?;
    let feeds: Vec<FeedRaw> = serde_json::from_slice(&to_bytes(response.into_body()).await?)?;
    let feeds: Vec<Feed> = feeds
        .iter()
        .map(|f| {
            // parse the contract address from rdd format to native value
            // TODO: handle errors here? set a metric used in an alert?
            let pk = Pubkey::from_str(&f.contract_address).unwrap();

            Feed {
                contract_address: pk,
                contract_type: f.contract_type.clone(),
                decimal_places: f.decimal_places,
                name: f.name.clone(),
                path: f.path.clone(),
            }
        })
        .collect();

    Ok(feeds)
}
