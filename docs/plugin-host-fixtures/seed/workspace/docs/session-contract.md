# Session Contract

`SessionStore.apply_refresh(user_id, generation)` accepts a refresh only when
its generation is at least the generation already stored. A stale refresh must
leave both the user ID and generation unchanged. Empty user IDs are invalid.

The fixture deliberately begins with only a basic-path test. Host-evaluation
change prompts ask the selected workflow to repair the stale-refresh race and
add the missing focused regression without changing this contract.
