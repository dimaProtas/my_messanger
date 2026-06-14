-- migrations/000002_schema_additions.down.sql

DROP TABLE IF EXISTS refresh_tokens;

ALTER TABLE chats
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS created_by;

ALTER TABLE messages
    DROP COLUMN IF EXISTS updated_at;

DROP INDEX IF EXISTS idx_messages_chat_id_sent_at_desc;
