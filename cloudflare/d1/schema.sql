-- 创建消息表
CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL, -- 'text' 或 'file'
  content TEXT, -- 文本内容 (仅文本消息)
  name TEXT, -- 文件名 (仅文件消息)  
  size INTEGER, -- 文件大小 (仅文件消息)
  room TEXT DEFAULT 'default',
  timestamp INTEGER NOT NULL,
  senderIP TEXT,
  userAgent TEXT,
  uuid TEXT, -- 文件的 UUID (仅文件消息)
  expireTime INTEGER, -- 过期时间 (仅文件消息)
  url TEXT -- 文件的访问 URL (仅文件消息)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_messages_room ON messages(room);
CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp);
CREATE INDEX IF NOT EXISTS idx_messages_uuid ON messages(uuid);
CREATE INDEX IF NOT EXISTS idx_messages_expire ON messages(expireTime);