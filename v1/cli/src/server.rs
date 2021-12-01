use std::convert::TryInto;
use std::str::FromStr;
use std::sync::Arc;

use bigdecimal::{BigDecimal, ToPrimitive};
use program::instruction;
use serde::{Deserialize, Serialize};
use serde_json::json;
use solana_client::rpc_client::RpcClient;
use solana_sdk::{
    commitment_config::CommitmentConfig,
    pubkey::Pubkey,
    signature::{Keypair, Signer},
    transaction::Transaction,
};
use tracing::{debug, warn};
use warp::{Filter, Reply};

use crate::Serve;

#[derive(Debug, Serialize)]
struct QueryResponse {
    price: f64,
}

#[derive(Debug, Deserialize)]
struct SubmitRequest {
    value: BigDecimal,
    aggregator: String,
}

pub async fn serve(config: Serve) {
    let client = Arc::new(RpcClient::new_with_commitment(
        config.json_rpc_url.to_string(),
        CommitmentConfig::confirmed(),
    ));

    let root = warp::path::end().map(|| warp::http::StatusCode::NO_CONTENT);

    let query = warp::get()
        .and(warp::path::param::<String>())
        .map(move |_market: String| {
            warp::reply::json(&json!({
                "data": QueryResponse{price: rand::random()},
            }))
        });

    let listen_address = config.listen_address.clone();

    let submit = warp::path!("submit")
        .and(warp::post())
        .and(warp::body::json())
        .map(move |request: SubmitRequest| {
            debug!("Got chainlink request for {:?}", request);

            let aggregator_pubkey = match Pubkey::from_str(&request.aggregator) {
                Ok(v) => v,
                Err(e) => {
                    warn!("{}", e);
                    return warp::http::StatusCode::BAD_REQUEST.into_response()
                }
            };

            let decimal = &request.value * BigDecimal::from(10u64.pow(9));
            if !decimal.is_integer() {
                warn!("Discarding fractional component of value {:#?} because it has more than 9 decimal places", &request.value)
            }

            // Parse the keypairs
            let oracle_keypair = Keypair::from_base58_string(&config.oracle_keypair);
            let fee_payer_keypair = Keypair::from_base58_string(&config.fee_payer_keypair);

            let timestamp : i64 = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH).unwrap()
                .as_secs()
                .try_into()
                .unwrap();

            let instruction = instruction::submit(
                config.program_id,
                &aggregator_pubkey,
                &oracle_keypair.pubkey(),
                timestamp,
                decimal.to_u128().unwrap(),
            )
            .unwrap();

            let mut transaction = Transaction::new_with_payer(
                &[instruction],
                Some(&fee_payer_keypair.pubkey()),
            );
            let keypairs = [&fee_payer_keypair, &oracle_keypair];
            let (blockhash, _) = client.get_recent_blockhash().unwrap();
            transaction.try_sign(&keypairs, blockhash).unwrap();
            debug!("Sending TX to Solana {:#?}", transaction);

            match client.send_and_confirm_transaction(&mut transaction) {
                Ok(signature) => {
                    debug!("{} => {}: TX signature {:?}", &oracle_keypair.pubkey(), aggregator_pubkey, signature);
                    warp::reply().into_response()
                },
                Err(e) => {
                    warn!("RPC call failed {:?}", e);
                    warp::http::StatusCode::INTERNAL_SERVER_ERROR.into_response()
                }
            }
        });

    let routes = root.or(query).or(submit);
    warp::serve(routes).run(listen_address).await;
}
