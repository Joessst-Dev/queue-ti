DROP TRIGGER IF EXISTS trg_mint_deliveries ON messages;

DROP FUNCTION IF EXISTS fn_mint_deliveries;

DROP INDEX IF EXISTS idx_message_deliveries_dequeue;

DROP TABLE IF EXISTS message_deliveries;

DROP TABLE IF EXISTS consumer_groups;
