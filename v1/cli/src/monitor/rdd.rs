use hyper::{body::to_bytes, Client};
use hyper_tls::HttpsConnector;
use serde::Deserialize;
use solana_sdk::pubkey::Pubkey;
use std::str::FromStr;
use std::time;
use tokio::sync::mpsc::Sender;
use tracing::debug;

#[derive(Deserialize, Debug)]
struct FeedRaw {
    #[serde(rename(deserialize = "contractAddress"))]
    contract_address: String,
    #[serde(rename(deserialize = "contractType"))]
    contract_type: String,
    #[serde(rename(deserialize = "decimalPlaces"))]
    decimal_places: u8,
    name: String,
    path: String,
}

#[derive(Debug)]
pub struct Feed {
    pub contract_address: Pubkey,
    pub contract_type: String,
    pub decimal_places: u8,
    pub name: String,
    pub path: String,
}

#[derive(Deserialize, Debug)]
struct NodeRaw {
    id: String,
    name: String,
    status: String,
    #[serde(rename(deserialize = "nodeAddress"))]
    node_address: Vec<String>,
}

#[derive(Debug)]
pub struct Node {
    pub id: String,
    pub name: String,
    pub status: String,
    pub node_address: Vec<Pubkey>,
}

pub async fn poll(
    tx: Sender<(Vec<Node>, Vec<Feed>)>,
    interval: time::Duration,
) -> anyhow::Result<()> {
    let mut interval = tokio::time::interval(interval);
    loop {
        interval.tick().await;
        debug!("polling rdd");

        let (nodes, feeds) = tokio::try_join!(nodes(), feeds())?;
        tx.send((nodes, feeds)).await?
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
                .map(|addr| {
                    let s = &addr.strip_prefix("sol-").unwrap();
                    Pubkey::from_str(&s).unwrap()
                })
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
            let s = &f.contract_address.as_str().strip_prefix("sol-").unwrap();
            let pk = Pubkey::from_str(&s).unwrap();

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
