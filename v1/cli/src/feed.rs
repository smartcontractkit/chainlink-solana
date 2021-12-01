use program::instruction;
use solana_client::rpc_client::RpcClient;
use solana_sdk::{
    commitment_config::CommitmentConfig,
    signature::Signer,
    signer::keypair::{read_keypair_file, Keypair},
    system_instruction,
    transaction::Transaction,
};

pub fn get() -> anyhow::Result<()> {
    let config_file = solana_cli_config::CONFIG_FILE.as_ref().unwrap();
    let cli_config = solana_cli_config::Config::load(&config_file).unwrap_or_default();

    let rpc_client = RpcClient::new_with_commitment(
        cli_config.json_rpc_url.clone(),
        CommitmentConfig::confirmed(),
    );

    let result = rpc_client.get_program_accounts(&program::id())?;

    for (pubkey, account) in result {
        println!("{}", pubkey);
        let state: program::state::Aggregator =
            program::processor::get_state_from_data(&account.data).unwrap();
        println!("{:#?}", state);
        println!("---");
    }

    Ok(())
}

pub fn initialize(config: crate::Initialize) -> anyhow::Result<()> {
    let config_file = solana_cli_config::CONFIG_FILE.as_ref().unwrap();
    let cli_config = solana_cli_config::Config::load(&config_file).unwrap_or_default();

    let rpc_client = RpcClient::new_with_commitment(
        cli_config.json_rpc_url.clone(),
        CommitmentConfig::confirmed(),
    );

    let len: u64 = 1024 * 100; // 100kb
    let minimum_balance_for_rent_exemption =
        rpc_client.get_minimum_balance_for_rent_exemption(len as usize)?;

    let aggregator = Keypair::new();
    let fee_payer = read_keypair_file(&config.fee_payer).unwrap();
    let owner = read_keypair_file(&config.owner).unwrap();

    let mut transaction = Transaction::new_with_payer(
        &[
            system_instruction::create_account(
                &fee_payer.pubkey(),
                &aggregator.pubkey(),
                minimum_balance_for_rent_exemption,
                len,
                &program::id(),
            ),
            instruction::initialize(
                program::id(),
                &aggregator.pubkey(),
                &owner.pubkey(),
                program::state::Config {
                    decimals: 9,
                    min_answer_threshold: 2,
                    staleness_threshold: 60,
                    oracles: config.oracles,
                },
            )?,
        ],
        Some(&fee_payer.pubkey()),
    );
    let blockhash = rpc_client.get_recent_blockhash()?.0;
    transaction.try_sign(&[&fee_payer, &aggregator, &owner], blockhash)?;
    rpc_client.send_and_confirm_transaction_with_spinner(&transaction)?;
    Ok(())
}

pub fn configure(config: crate::Configure) -> anyhow::Result<()> {
    let config_file = solana_cli_config::CONFIG_FILE.as_ref().unwrap();
    let cli_config = solana_cli_config::Config::load(&config_file).unwrap_or_default();

    let rpc_client = RpcClient::new_with_commitment(
        cli_config.json_rpc_url.clone(),
        CommitmentConfig::confirmed(),
    );

    let aggregator = config.aggregator;
    let fee_payer = read_keypair_file(&config.fee_payer).unwrap();
    let owner = read_keypair_file(&config.owner).unwrap();

    let mut transaction = Transaction::new_with_payer(
        &[instruction::reconfigure(
            program::id(),
            &aggregator,
            &owner.pubkey(),
            program::state::Config {
                decimals: 9,
                min_answer_threshold: 2,
                staleness_threshold: 60,
                oracles: config.oracles,
            },
        )?],
        Some(&fee_payer.pubkey()),
    );
    let blockhash = rpc_client.get_recent_blockhash()?.0;
    transaction.try_sign(&[&fee_payer, &owner], blockhash)?;
    rpc_client.send_and_confirm_transaction_with_spinner(&transaction)?;
    Ok(())
}

pub fn submit(config: crate::Submit) -> anyhow::Result<()> {
    let config_file = solana_cli_config::CONFIG_FILE.as_ref().unwrap();
    let cli_config = solana_cli_config::Config::load(&config_file).unwrap_or_default();

    let rpc_client = RpcClient::new_with_commitment(
        cli_config.json_rpc_url.clone(),
        CommitmentConfig::confirmed(),
    );

    // TODO

    let fee_payer = read_keypair_file(&config.fee_payer).unwrap();
    let oracle = read_keypair_file(&config.oracle).unwrap();

    let timestamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)?
        .as_secs() as i64; // TODO: try_into

    let mut transaction = Transaction::new_with_payer(
        &[instruction::submit(
            program::id(),
            &config.aggregator,
            &oracle.pubkey(),
            timestamp,
            config.value, // TODO: parse string and scale the value
        )?],
        Some(&fee_payer.pubkey()),
    );
    let blockhash = rpc_client.get_recent_blockhash()?.0;
    transaction.try_sign(&[&fee_payer, &oracle], blockhash)?;
    rpc_client.send_and_confirm_transaction_with_spinner(&transaction)?;
    Ok(())
}
