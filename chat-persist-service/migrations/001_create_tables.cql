-- Keyspace: wes_chat
-- NetworkTopologyStrategy for production multi-datacenter deployment
-- Replication factor of 3 for high availability

CREATE KEYSPACE IF NOT EXISTS wes_chat
WITH replication = {
  'class': 'NetworkTopologyStrategy',
  'dc1': 1
};

-- Table: messages_by_room_session
-- Query pattern: Get all messages for a specific room + session (live streaming session)
-- Partition Key: (room_id, session_id) - groups all messages from one live session
-- Clustering Key: message_id DESC - Snowflake ID provides time ordering, DESC for newest first
-- TTL: 90 days = 7,776,000 seconds

CREATE TABLE IF NOT EXISTS wes_chat.messages_by_room_session (
    room_id    TEXT,
    session_id TEXT,
    message_id TEXT,
    user_id    TEXT,
    username   TEXT,
    content    TEXT,
    created_at TIMESTAMP,
    PRIMARY KEY ((room_id, session_id), message_id)
) WITH CLUSTERING ORDER BY (message_id DESC)
  AND default_time_to_live = 7776000
  AND gc_grace_seconds = 864000
  AND compaction = {
    'class': 'org.apache.cassandra.db.compaction.TimeWindowCompactionStrategy',
    'compaction_window_unit': 'DAYS',
    'compaction_window_size': 1
  }
  AND compression = {
    'class': 'org.apache.cassandra.io.compress.LZ4Compressor'
  };
