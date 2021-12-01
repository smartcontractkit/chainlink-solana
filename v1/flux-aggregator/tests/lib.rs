use program::{
    decimal,
    error::Error,
    instruction,
    processor::{self, process_instruction},
    state::{Aggregator, Config, Submission},
};
use solana_program_test::*;
use solana_sdk::{
    account::Account,
    borsh::try_from_slice_unchecked,
    clock::{Clock, UnixTimestamp},
    instruction::InstructionError,
    pubkey::Pubkey,
    signature::{Keypair, Signer},
    sysvar,
    transaction::{Transaction, TransactionError},
};

#[allow(dead_code)]
pub async fn get_bincode_account<T: serde::de::DeserializeOwned>(
    client: &mut BanksClient,
    address: &Pubkey,
) -> T {
    use std::borrow::Borrow;
    client
        .get_account(*address)
        .await
        .unwrap()
        .map(|a| bincode::deserialize::<T>(&a.data.borrow()).unwrap())
        .expect(format!("GET-TEST-ACCOUNT-ERROR: Account {}", address).as_str())
}

#[allow(dead_code)]
pub async fn get_clock(client: &mut BanksClient) -> Clock {
    get_bincode_account::<Clock>(client, &sysvar::clock::id()).await
}

#[allow(dead_code)]
pub async fn advance_clock_past_timestamp(
    context: &mut ProgramTestContext,
    client: &mut BanksClient,
    unix_timestamp: UnixTimestamp,
) {
    let mut clock = get_clock(client).await;
    let mut n = 1;

    while clock.unix_timestamp <= unix_timestamp {
        // Since the exact time is not deterministic keep wrapping by arbitrary 400 slots until we pass the requested timestamp
        context.warp_to_slot(clock.slot + n * 400).unwrap();

        n = n + 1;
        clock = get_clock(client).await;
    }
}

#[allow(dead_code)]
pub async fn advance_clock_by_min_timespan(
    context: &mut ProgramTestContext,
    client: &mut BanksClient,
    time_span: u64,
) {
    let clock = get_clock(client).await;
    advance_clock_past_timestamp(context, client, clock.unix_timestamp + (time_span as i64)).await;
}

#[allow(dead_code)]
pub async fn advance_clock(context: &mut ProgramTestContext, client: &mut BanksClient) {
    let clock = get_clock(client).await;
    context.warp_to_slot(clock.slot + 2).unwrap();
}

#[tokio::test]
async fn test_it_works() {
    let program_id = Pubkey::new_unique();
    let aggregator_pubkey = Pubkey::new_unique();
    let owner = Keypair::new();
    let oracle0 = Keypair::new();
    let oracle1 = Keypair::new();

    let mut program_test = ProgramTest::new(
        "program", // Run the BPF version with `cargo test-bpf`
        program_id,
        processor!(process_instruction), // Run the native version with `cargo test`
    );
    program_test.add_account(
        aggregator_pubkey,
        Account {
            lamports: u32::MAX as u64,
            data: vec![0_u8; 1024 * 8],
            owner: program_id,
            ..Account::default()
        },
    );
    let mut context = program_test.start_with_context().await;

    // Initialize a new feed
    let oracles = vec![oracle0.pubkey(), oracle1.pubkey()];

    let mut config = Config {
        oracles,
        min_answer_threshold: 2,
        staleness_threshold: 10,
        decimals: 9,
    };

    let mut transaction = Transaction::new_with_payer(
        &[instruction::initialize(
            program_id,
            &aggregator_pubkey,
            &owner.pubkey(),
            config.clone(),
        )
        .unwrap()],
        Some(&context.payer.pubkey()),
    );
    transaction.sign(&[&context.payer, &owner], context.last_blockhash);
    context
        .banks_client
        .process_transaction(transaction)
        .await
        .unwrap();

    // submit a value from oracle0
    let clock = get_clock(&mut context.banks_client).await;
    let mut transaction = Transaction::new_with_payer(
        &[instruction::submit(
            program_id,
            &aggregator_pubkey,
            &oracle0.pubkey(),
            clock.unix_timestamp,
            decimal(5, config.decimals),
        )
        .unwrap()],
        Some(&context.payer.pubkey()),
    );
    transaction.sign(&[&context.payer, &oracle0], context.last_blockhash);
    context
        .banks_client
        .process_transaction(transaction)
        .await
        .unwrap();

    // reject an impostor submission
    let clock = get_clock(&mut context.banks_client).await;
    let impostor = Keypair::new();
    let mut transaction = Transaction::new_with_payer(
        &[instruction::submit(
            program_id,
            &aggregator_pubkey,
            &impostor.pubkey(),
            clock.unix_timestamp,
            decimal(5, config.decimals),
        )
        .unwrap()],
        Some(&context.payer.pubkey()),
    );
    transaction.sign(&[&context.payer, &impostor], context.last_blockhash);
    let result = context
        .banks_client
        .process_transaction(transaction)
        .await
        .unwrap_err()
        .unwrap();

    assert_eq!(
        result,
        TransactionError::InstructionError(
            0,
            InstructionError::Custom(Error::InvalidOracle as u32)
        )
    );

    // submit a value from oracle1
    let clock = get_clock(&mut context.banks_client).await;
    let mut transaction = Transaction::new_with_payer(
        &[instruction::submit(
            program_id,
            &aggregator_pubkey,
            &oracle1.pubkey(),
            clock.unix_timestamp,
            decimal(4, config.decimals),
        )
        .unwrap()],
        Some(&context.payer.pubkey()),
    );
    transaction.sign(&[&context.payer, &oracle1], context.last_blockhash);
    context
        .banks_client
        .process_transaction(transaction)
        .await
        .unwrap();

    // Fetch the current price
    let account = context
        .banks_client
        .get_account(aggregator_pubkey)
        .await
        .unwrap()
        .unwrap();

    // can't use get_account_data_with_borsh because it uses the checked version
    let state = try_from_slice_unchecked::<Aggregator>(&account.data).unwrap();

    // 4.5 (9 decimal places)
    assert_eq!(state.answer, Some(4_500_000_000));

    // reconfigure the aggregator, swapping oracle1 for oracle2
    let oracle2 = Keypair::new();
    config.oracles = vec![oracle0.pubkey(), oracle2.pubkey()];
    let mut transaction = Transaction::new_with_payer(
        &[instruction::reconfigure(
            program_id,
            &aggregator_pubkey,
            &owner.pubkey(),
            config.clone(),
        )
        .unwrap()],
        Some(&context.payer.pubkey()),
    );
    transaction.sign(&[&context.payer, &owner], context.last_blockhash);
    context
        .banks_client
        .process_transaction(transaction)
        .await
        .unwrap();

    // increment time

    // submit from oracle2 succeeds
    let clock = get_clock(&mut context.banks_client).await;
    let mut transaction = Transaction::new_with_payer(
        &[instruction::submit(
            program_id,
            &aggregator_pubkey,
            &oracle2.pubkey(),
            clock.unix_timestamp,
            decimal(4, config.decimals),
        )
        .unwrap()],
        Some(&context.payer.pubkey()),
    );
    transaction.sign(&[&context.payer, &oracle2], context.last_blockhash);
    context
        .banks_client
        .process_transaction(transaction)
        .await
        .unwrap();

    // submit from oracle0 succeeds
    let clock = get_clock(&mut context.banks_client).await;
    let mut transaction = Transaction::new_with_payer(
        &[instruction::submit(
            program_id,
            &aggregator_pubkey,
            &oracle0.pubkey(),
            clock.unix_timestamp,
            decimal(8, config.decimals),
        )
        .unwrap()],
        Some(&context.payer.pubkey()),
    );
    transaction.sign(&[&context.payer, &oracle0], context.last_blockhash);
    context
        .banks_client
        .process_transaction(transaction)
        .await
        .unwrap();

    // submit from oracle1 now fails
    let clock = get_clock(&mut context.banks_client).await;
    let mut transaction = Transaction::new_with_payer(
        &[instruction::submit(
            program_id,
            &aggregator_pubkey,
            &oracle1.pubkey(),
            clock.unix_timestamp,
            decimal(5, config.decimals), // TODO: if this sends 4 again it generates an identical transaction to earlier an no-ops without failing..
        )
        .unwrap()],
        Some(&context.payer.pubkey()),
    );
    transaction.sign(&[&context.payer, &oracle1], context.last_blockhash);
    let result = context
        .banks_client
        .process_transaction(transaction)
        .await
        .unwrap_err()
        .unwrap();
    assert_eq!(
        result,
        TransactionError::InstructionError(
            0,
            InstructionError::Custom(Error::InvalidOracle as u32)
        )
    );

    // Fetch the current price
    let mut account = context
        .banks_client
        .get_account(aggregator_pubkey)
        .await
        .unwrap()
        .unwrap();

    // Assert past rounds
    let (pos, rounds) = processor::get_rounds_from_data(&mut account.data);
    assert_eq!(*pos, 2);
    assert_eq!(rounds[0], Submission(clock.unix_timestamp, 4_500_000_000));
    assert_eq!(rounds[1], Submission(clock.unix_timestamp, 6_000_000_000));
}
