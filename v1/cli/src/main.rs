use std::net::SocketAddr;

use clap::{ArgEnum, Parser};
use solana_sdk::{
    pubkey::Pubkey,
    signature::{Keypair, Signer},
};
use url::Url;

mod feed;
mod monitor;
mod server;

/// This doc string acts as a help message when the user runs '--help'
/// as do all doc strings on fields
#[derive(Parser)]
#[clap(version = "1.0")]
struct Opts {
    #[clap(subcommand)]
    subcmd: Command,
}

#[derive(Parser)]
enum Command {
    Serve(Serve),

    Feed(FeedOpts),

    Encode58(Encode),
    Encode64(Encode),

    Monitor(Monitor),
}

#[derive(Parser)]
pub struct Encode {
    #[clap(long)]
    keypair: bool,

    input: String,
}

/// A subcommand for controlling testing
#[derive(Parser, Clone)]
pub struct Serve {
    /// The address the external adapter should listen on
    #[clap(long, default_value = "0.0.0.0:9090")]
    listen_address: SocketAddr,

    json_rpc_url: Url,

    program_id: Pubkey,

    oracle_keypair: String,

    fee_payer_keypair: String,
}

#[derive(Parser, Clone)]
struct FeedOpts {
    #[clap(subcommand)]
    subcmd: Feed,
}

#[derive(Parser, Clone)]
pub enum Feed {
    Init(Initialize),
    Configure(Configure),
    Submit(Submit),
    Get,
}

#[derive(Parser, Clone)]
pub struct Initialize {
    #[clap(short, long)]
    fee_payer: std::path::PathBuf,
    #[clap(short, long)]
    owner: std::path::PathBuf,

    oracles: Vec<Pubkey>,
}

#[derive(Parser, Clone)]
pub struct Configure {
    #[clap(short, long)]
    fee_payer: std::path::PathBuf,
    #[clap(short, long)]
    owner: std::path::PathBuf,
    #[clap(short, long)]
    aggregator: Pubkey,

    oracles: Vec<Pubkey>,
}

#[derive(Parser, Clone)]
pub struct Submit {
    #[clap(short, long)]
    fee_payer: std::path::PathBuf,
    #[clap(short, long)]
    oracle: std::path::PathBuf,
    #[clap(short, long)]
    aggregator: Pubkey,

    value: u128,
}

#[derive(Parser)]
#[clap(version = "1.0")]
pub struct Monitor {
    #[clap(
        long,
        env = "LISTEN_ADDR",
        about = "http address:port to listen on",
        default_value = "0.0.0.0:9090"
    )]
    listen_addr: SocketAddr,

    #[clap(
        long,
        env = "RPC_ENDPOINT",
        about = "rpc address:port for client connection",
        default_value = "127.0.0.1:8899"
    )]
    rpc_endpoint: String, // supports using DNS for the RPC address, e.g. api.devnet.solana.com

    #[clap(
        arg_enum,
        long,
        env = "RPC_SCHEME",
        about = "http scheme to use (http or https)",
        default_value = "https"
    )]
    rpc_scheme: Scheme,

    #[clap(
        long,
        env = "NETWORK",
        about = "human-readable network name for metric labels",
        default_value = "solana_devnet"
    )]
    network: String,

    #[clap(
        long,
        env = "QUERY_INTERVAL",
        about = "query interval",
        default_value = "1s"
    )]
    query_interval: humantime::Duration,

    #[clap(
        long,
        env = "RDD_INTERVAL",
        about = "rdd polling interval",
        default_value = "1m"
    )]
    rdd_interval: humantime::Duration,
}

#[derive(ArgEnum, Clone, Copy, Debug)]
enum Scheme {
    Http,
    Https,
}

fn main() -> anyhow::Result<()> {
    pretty_env_logger::init();

    let opts: Opts = Opts::parse();

    match opts.subcmd {
        Command::Serve(config) => tokio::runtime::Builder::new_multi_thread()
            .enable_all()
            .build()
            .unwrap()
            .block_on(server::serve(config)),
        Command::Monitor(config) => tokio::runtime::Builder::new_multi_thread()
            .enable_all()
            .build()
            .unwrap()
            .block_on(monitor::monitor(config)),
        Command::Feed(FeedOpts { subcmd: feed }) => match feed {
            Feed::Init(config) => feed::initialize(config)?,
            Feed::Configure(config) => feed::configure(config)?,
            Feed::Get => feed::get()?,
            Feed::Submit(config) => feed::submit(config)?,
        },
        Command::Encode58(Encode { input, keypair }) => {
            let bytes: Vec<u8> = serde_json::from_str(&input).unwrap();
            if keypair {
                let key = Keypair::from_bytes(&bytes).unwrap();
                println!(
                    "Base 58 encoded keypair material: {:?}",
                    key.to_base58_string()
                );
                println!("Base 58 encoded public key: {:?}", key.pubkey());
            } else {
                println!(
                    "Base 58 encoded material: {}",
                    bs58::encode(bytes).into_string()
                )
            }
        }
        Command::Encode64(Encode { input, keypair }) => {
            let bytes: Vec<u8> = serde_json::from_str(&input).unwrap();
            if keypair {
                let key = Keypair::from_bytes(&bytes).unwrap();
                println!(
                    "Base 64 encoded keypair material: {}",
                    base64::encode(key.to_bytes())
                );
                println!(
                    "Base 64 encoded public key: {:?}",
                    base64::encode(key.pubkey().to_bytes())
                );
            } else {
                println!("Base 64 encoded material: {}", base64::encode(bytes))
            }
        }
    }

    Ok(())
}
