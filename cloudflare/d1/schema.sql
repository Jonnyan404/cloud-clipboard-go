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
  senderClientID TEXT, -- 前端每客户端持久ID,用于气泡收发归属
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

-- 当前在线的 WebSocket 会话
CREATE TABLE IF NOT EXISTS room_presence (
  sessionId TEXT PRIMARY KEY,
  room TEXT NOT NULL DEFAULT 'default',
  connectedAt INTEGER NOT NULL,
  userAgent TEXT,
  updatedAt INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_room_presence_room ON room_presence(room);
CREATE INDEX IF NOT EXISTS idx_room_presence_updated_at ON room_presence(updatedAt);

-- 分享链接使用次数（maxUses）
CREATE TABLE IF NOT EXISTS share_token_usage (
  jti TEXT PRIMARY KEY,
  used INTEGER NOT NULL DEFAULT 0,
  maxUses INTEGER NOT NULL,
  exp INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_share_token_usage_exp ON share_token_usage(exp);

-- 分享记录：谁在哪个房间分享了什么、被打开了几次（对应 Go 侧的 share-log.json）。
-- 与 share_token_usage 分开：那张表只在限次分享时才有行，回答不了「我最近分享过什么」。
--
-- 注意：GET /share/list 用房间凭据鉴权，而房间没配密码时鉴权恒为通过 ——
-- 所以开放房间的记录列表是公开可读的（列表不回 token，拿不到正文）。
CREATE TABLE IF NOT EXISTS share_log (
  jti TEXT PRIMARY KEY,
  type TEXT NOT NULL,          -- content | file：这条分享怎么发出来的
  id TEXT NOT NULL,            -- content id 或 file uuid
  room TEXT NOT NULL DEFAULT 'default',
  kind TEXT,                   -- 指向的内容类型：text | file
  name TEXT,                   -- 文件名，或文本首行摘要
  size INTEGER,
  createdAt INTEGER NOT NULL,
  exp INTEGER NOT NULL,
  maxUses INTEGER NOT NULL DEFAULT 0,
  password INTEGER NOT NULL DEFAULT 0,  -- 是否带密码（只存布尔，摘要本身不入库）
  visits INTEGER NOT NULL DEFAULT 0,
  scans INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_share_log_room ON share_log(room, createdAt);
CREATE INDEX IF NOT EXISTS idx_share_log_exp ON share_log(exp);

-- 「同一个人对同一条分享的重复上报」去重窗口，防的是拿着链接刷数字。
CREATE TABLE IF NOT EXISTS share_visit_marks (
  jti TEXT NOT NULL,
  visitor TEXT NOT NULL,
  ts INTEGER NOT NULL,
  PRIMARY KEY (jti, visitor)
);

CREATE INDEX IF NOT EXISTS idx_share_visit_marks_ts ON share_visit_marks(ts);

