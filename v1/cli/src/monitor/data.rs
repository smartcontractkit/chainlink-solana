use serde::Deserialize;
use solana_sdk::pubkey::Pubkey;

#[derive(Deserialize, Debug)]
#[serde(rename_all = "camelCase")]
pub struct FeedRaw {
    pub contract_address: String,
    pub contract_type: String,
    pub decimal_places: u8,
    pub name: String,
    pub path: String,
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
#[serde(rename_all = "camelCase")]
pub struct NodeRaw {
    pub id: String,
    pub name: String,
    pub status: String,
    pub node_address: Vec<String>,
}

#[derive(Debug)]
pub struct Node {
    pub id: String,
    pub name: String,
    pub status: String,
    pub node_address: Vec<Pubkey>,
}
