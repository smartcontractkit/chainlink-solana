use crate::{context::QueryFeedMetadata, error::DataCacheError, WorkflowMetadata};
use anchor_lang::prelude::*;

pub fn handler(
    ctx: Context<QueryFeedMetadata>,
    _data_id: [u8; 16],
    start_index: u8,
    max_count: u8,
) -> Result<Vec<WorkflowMetadata>> {
    let feed_config = ctx.accounts.feed_config.load()?;

    require!(
        !feed_config.workflow_metadata.is_empty(),
        DataCacheError::FeedNotConfigured
    );

    let len = feed_config.workflow_metadata.len();

    let start_index: usize = start_index.into();
    let max_count: usize = max_count.into();

    if start_index >= len {
        return Ok(Vec::new());
    }

    // max count 0 means take start_index and everything after it

    let mut end_index = start_index + max_count;
    end_index = if end_index > len || max_count == 0 {
        len
    } else {
        end_index
    };

    Ok(feed_config.workflow_metadata[start_index..end_index].to_vec())
}
