use anchor_lang::prelude::*;

#[event]
pub struct TestEvent {
    pub str_val: String,
    pub u64_value: u64,
}
