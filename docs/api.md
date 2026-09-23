# Cloud Clipboard REST API

Interface reference for third-party clients (web, mobile, scripts, embedded devices).

There are two server implementations and they **share this one contract**:

| Implementation | Location | Notes |
|---|---|---|
| Go | `cloud-clip/` | Self-hosting first choice: single binary, static assets embedded |
| Cloudflare Worker | `cloudflare/workers/` | Serverless, backed by D1 + R2 |

Differences are called out inline; anything unmarked behaves the same on both.

> The contract is guarded by tests, so changing it turns them red:
> `lib/shortcut_contract_test.go` (Go) and `test/shortcut-contract.test.mjs` (Worker).

---

## 1. Conventions

### 1.1 Base URL and prefix

The server listens on `9501` by default. If it is deployed under a sub-path
(`server.prefix` / `PREFIX`), every endpoint carries that prefix:

```
http://host:9501/text                    # no prefix
https://host/cloud-clipboard/text        # PREFIX=/cloud-clipboard
```

### 1.2 Authentication

Three ways, pick one:

| Method | How | Notes |
|---|---|---|
| Bearer header | `Authorization: Bearer <credential>` | **Preferred.** The credential is the global password, a room password, or a session token from `/auth/token` |
| Query string | `?auth=<credential>` | Kept for compatibility (shortcuts use it). **Never put a password in a shareable URL** |
| Session token | Either of the above | Issued by `/auth/token`, valid for 1 hour by default |

**Rooms and credentials:**

- `?room=` empty means the `default` room; **omitting `room` entirely** means "any room"
- Whether a room needs a password comes from the global `auth` plus that room's `roomAuth` entry:
  not configured → follows the global `auth`; an empty string → also follows the global `auth`
  (**not** "open"); `{"open": true}` → **open**, even with a global `auth` set; a non-empty password →
  requires it, and the global password still works too (an extra key, not a replacement lock)
- The server authorizes against **the room recorded on the content itself**, not the
  `?room=` the client claims — `/file/:uuid/:name` especially, to prevent forgery

**Symptom cheat sheet** (handy when debugging):

| Response | Meaning |
|---|---|
| `404 content_not_found` | Room mismatch — the content is not in that room |
| `401 unauthorized_invalid_token` | Room matched, but the credential is empty or wrong |
| `404 file_expired` | Content exists but has expired |

### 1.3 Content format (`/content/*` only)

`/content/latest` and `/content/:id` choose their output with `?format=`:

| Priority | Signal | Effect |
|---|---|---|
| 1 | `?format=raw\|json` | Explicit, overrides everything else. **New code should use this** |
| 2 | `.json` path suffix | ⚠️ **Legacy, being retired** — shipped shortcuts still use it, kept for now |
| 3 | `?json=1` / `?json=true` | ⚠️ **Legacy, being retired**, same as above |
| 4 | `Accept: application/json` | **Text responses only** |
| 5 | Default | `raw` |

```bash
curl "http://localhost:9501/content/7?format=json"
# {"id":"7","type":"text","content":"foobar","timestamp":1758000000}

curl "http://localhost:9501/content/7?format=raw"
# foobar
```

Two things to note:

- **`Accept` does not apply to files.** The download path sees every Accept header the wild
  produces (browsers, downloaders, scripts), so the file branch keys off explicit signals
  only — otherwise clicking a file link in a browser would return a blob of JSON.
- **An unknown `format` returns 400** (`code: unsupported_format`) rather than falling back.

### 1.4 Error responses

**Every** error path returns the same shape, `Content-Type: application/json; charset=utf-8`:

```json
{"code": "text_too_long", "error": "Text too long", "message": "文本内容超出限制 (最大 4096 字符)"}
```

| Field | Purpose |
|---|---|
| `code` | Machine code (snake_case) for programmatic handling. **Do not change once shipped** |
| `error` | Plain English, for logs and English-speaking users |
| `message` | Plain Chinese, for humans |

Common codes are listed in the [error table](#9-error-codes) below.

> **Do not split responses by `Accept`**: Apple Shortcuts' "Get Contents of URL" sends no
> `Accept` header and does not expose the status code to the shortcut, so clients can only
> read the body. Two body shapes for one status code forces two parsers per client.

### 1.5 Request body types

- Text endpoints take **plain text** (`Content-Type: text/plain`), **not JSON**
- File endpoints take `multipart/form-data`
- Auth endpoints (`/auth/token`, `/share`) take JSON

---

## 2. Endpoint overview

| Method | Path | Purpose | Auth |
|---|---|---|---|
| GET | `/server` | Service info and limits | No |
| GET | `/myip` | Client's egress IP | No |
| GET | `/health` | Health check (**Worker only**) | No |
| POST | `/auth/token` | Exchange a password for a session token | Password |
| POST | `/auth/token/refresh` | Renew a session token | Token |
| POST | `/text` | Send text | Yes |
| POST | `/upload` | Upload a file | Yes |
| POST | `/upload/chunk/:uuid` | Chunked upload (**Go only**) | Yes |
| POST | `/upload/finish/:uuid` | Finish a chunked upload (**Go only**) | Yes |
| POST | `/upload/multipart/*` | R2 multipart upload (**Worker only**) | Yes |
| GET | `/content/latest` | Fetch the newest entry | Yes |
| GET | `/content/:id` | Fetch one entry by ID | Yes |
| POST | `/content/:id/column` | Move an entry to a board column | Password |
| GET | `/file/:uuid/:name` | Download a file | Yes |
| GET | `/rooms` | Room list | Yes |
| POST | `/share` | Create a share token | Yes |
| GET | `/share?t=` | Share-page metadata (no use consumed) | No |
| GET | `/share/list` | Recent shares of a room, with open counts | Room |
| POST | `/share/visit` | Report that a human opened a share | No |
| GET | `/s/:token` | Share page: SPA shell with injected Open Graph tags (HTML) | No |
| DELETE | `/revoke/:id` | Delete one entry | Yes |
| DELETE | `/revoke/all` | Clear the room | Yes |
| WS | `/push` | Real-time push | Yes |

---

## 3. Service info

### GET /server

No auth. Call this on startup to learn the limits — never hard-code them.

```json
{
  "version": "5.0.8",
  "server": { "prefix": "", "history": 100, "roomList": false },
  "text": { "limit": 4096 },
  "file": { "limit": 268435456, "expire": 3600, "chunk": 1048576 }
}
```

Fields such as `authNeeded` / `authorized` reflect the current auth state, which is how the
web UI decides whether to show a password prompt.

### GET /myip

```json
{ "ip": "203.0.113.7" }
```

### GET /health (Worker only)

Returns the plain text `OK`. The Go build has no such endpoint — it uses `/server` for liveness.

---

## 4. Authentication

### POST /auth/token

```http
POST /auth/token?room=default
Content-Type: application/json

{"password": "your-password"}
```

Response:

```json
{"token": "eyJ...", "expiresAt": 1758003600, "scope": "global"}
```

- `scope` is `global` when the global password was used (token valid for every room);
  it is `""` for a room password.
- Tokens live for 1 hour by default.

### POST /auth/token/refresh

Renew silently with the old token — no password needed:

```http
POST /auth/token/refresh?room=default
Authorization: Bearer <old token>
```

Same response as `/auth/token`. Renewal preserves the original `scope`, so a global session
never degrades into a room-only one.

---

## 5. Sending

### POST /text

```http
POST /text?room=default&name=iPhone&client=<client-id>
Content-Type: text/plain
Authorization: Bearer <credential>

the text to send
```

| Parameter | Where | Notes |
|---|---|---|
| `room` | query | Room name; empty means `default` |
| `name` | query | **Device display name**, stored as `senderDevice.name`. Max 32 chars, control characters stripped; if empty the server infers it from the User-Agent |
| `client` | query | **Unique client ID**, used to tell "is this mine?" (chat bubble ownership). Not interchangeable with `name` |
| `id` | query | If present, **overwrites** that existing message instead of creating one |

Response:

```json
{"id": "7", "type": "text", "url": "http://localhost:9501/content/7"}
```

Over the limit returns `413` with `code: text_too_long` (limit from `/server` → `text.limit`).

**The body comes in three shapes**, picked by `Content-Type`:

| `Content-Type` | Body |
|---|---|
| `text/plain`, absent, or anything else | **the whole request body is the text** |
| `application/json` | `{"content": "the text to send"}` |
| `multipart/form-data` | the form field `content` |

The last two exist for **Shortcuts**: when it sends a string variable as the request body the bytes come
out UTF-16, while a structured body is serialized as UTF-8.

⚠️ `application/x-www-form-urlencoded` is **deliberately not recognised** and keeps taking the
"whole body is the text" path — it is what `curl --data-binary` and friends send by default, and
treating it as a form would make those requests **silently store an empty entry**.

Declaring `application/json` with a body that is not valid JSON returns `400` with `code: invalid_body`.

**The plain-text path also recognises UTF-16** (a BOM, or a byte pattern that gives it away) and decodes
it — that is what a Shortcuts shortcut sends. Anything else is stored as UTF-8, byte for byte, with
**nothing escaped or rewritten**.

### POST /upload

```http
POST /upload?room=default&name=iPhone
Authorization: Bearer <credential>
Content-Type: multipart/form-data

file=@photo.png
```

The form field name is always **`file`**. The response carries `uuid` and `url`:

```json
{
  "id": "8", "type": "file", "name": "photo.png", "size": 20480,
  "uuid": "11111111-2222-3333-4444-555555555555",
  "url": "http://localhost:9501/file/11111111-.../photo.png",
  "expire": 1758003600
}
```

> **Files take two requests and both need credentials.** The `url` returned by `/content/*`
> is just an address — it does not embed credentials. Text has no second step (the content is
> inline in the JSON), which is why a missing credential looks like "text works, files 401".

**Large files:**

- **Go**: `POST /upload/chunk/:uuid` to append chunks → `POST /upload/finish/:uuid` to seal it
- **Worker**: R2 multipart — `create` → `PUT /upload/multipart/:partNumber` → `complete`
  (`DELETE /upload/multipart` aborts)

---

## 6. Receiving

### GET /content/latest

Fetches the **newest entry** in the room (text or file record).

```bash
curl "http://localhost:9501/content/latest?room=default&format=json" -H "Authorization: Bearer xxx"
```

```json
{
  "id": "7", "type": "text", "content": "foobar",
  "timestamp": 1758000000,
  "senderDevice": { "name": "iPhone", "type": "mobile", "os": "iOS 18" },
  "senderIP": "203.0.113.7"
}
```

For file entries you get `uuid` / `name` / `size` / `url` / `expire` instead of content bytes.

> **What "newest" means**: `timestamp` is in **seconds**, so when several entries share a
> second the **last inserted** one wins. On the Worker that is
> `ORDER BY timestamp DESC, id DESC`.

### GET /content/:id

Same as above, by exact ID. ⚠️ The `.json` path suffix is a **legacy signal being retired**; new code should use `?format=json`.

### GET /file/:uuid/:name

Downloads the file bytes.

| Parameter | Notes |
|---|---|
| `?auth=` | Credential (**authorization uses the room recorded on the file**; a client-supplied `room` is ignored) |
| `?download=true` | Adds `Content-Disposition: attachment` so browsers download instead of rendering inline |

---

## 7. Rooms and management

### GET /rooms

Returns the room list (requires `roomList` to be enabled):

```json
{
  "rooms": [
    { "name": "default", "messageCount": 12, "isProtected": false, "isActive": true }
  ]
}
```

### POST /content/:id/column

Moves an entry to a board column. The board is a **view over the same entries**, not a second
store — this sets one field on the entry and nothing else:

```http
POST /content/7/column?room=default
Content-Type: application/json
Authorization: Bearer <credential>

{"column": "doing"}
```

| Value | Meaning |
|---|---|
| `todo` | To do — also the default: a missing or empty `column` normalises to this |
| `doing` | In progress |
| `done` | Done |

Response:

```json
{"id": "7", "type": "text", "column": "doing"}
```

- The three columns are **fixed** — no per-room column configuration, and no ordering inside a
  column. Moving a card only changes *which* column it is in.
- ⚠️ **`timestamp` is not touched.** `POST /text?id=` does bump it when it rewrites the body, but
  moving a card must not send it to the top of the timeline — that would reshuffle the whole list
  every time you drag one card.
- Works for text and file entries alike.
- Auth is the room password. A share token will **not** work: that credential is read-only.
- Broadcasts an `update` event on the room's WebSocket, so other clients move the card too.
- Errors: `invalid_column` (400), `invalid_body` (400), `invalid_content_id` (400),
  `content_not_found` (404), `method_not_allowed` (405).

### POST /share

Creates a **short-lived share token** so someone with the link can read one entry:

```http
POST /share
Content-Type: application/json
Authorization: Bearer <credential>

{"type": "content", "id": "7", "ttl": 900, "maxUses": 0, "password": ""}
```

- `type` is `content` or `file`
- File shares use `uuid` instead of `id`
- `ttl` is in seconds, default 900 (15 min), range 60 – 86400
- `maxUses` of `0` means unlimited
- `password` is optional; when set, the recipient must supply it (see below)

A token is issued **always** — an open room gets one too, because the TTL, the usage limit and the
password all live in it. The response carries the addresses:

```json
{
  "url": "https://host/s/<token>",
  "pageUrl": "https://host/s/<token>",
  "rawUrl": "https://host/content/7?t=<token>",
  "token": "<token>",
  "jti": "9f2c…",
  "expiresAt": 1750000000,
  "maxUses": 0,
  "visits": 0,
  "scans": 0
}
```

- `url` **is** the share page: one address serves the crawler and the human alike. The server answers
  `/s/<token>` with the SPA shell and the Open Graph tags already injected into its `<head>`, so a
  chat app unfurling this URL gets a real preview card, while a human opening the very same URL lands
  in the share page itself — no redirect, no second address
- `pageUrl` is kept for compatibility and currently holds **the same value as `url`** (clients that
  only learned about `pageUrl` keep working)
- `rawUrl` reaches the content / file endpoint directly with the same token (used for downloads)
- `jti` identifies this share in the logs below; `visits` / `scans` start at 0

### GET /share/list?room=&limit=

What this room shared recently, and how often each link was opened. Same authorisation as
`POST /share` for that room (`room` defaults to `default`, `limit` defaults to 50, max 200).

```json
{
  "room": "default",
  "total": 3,
  "limit": 50,
  "records": [
    {
      "jti": "9f2c…", "type": "content", "kind": "text", "id": "7", "room": "default",
      "name": "first line of the text", "size": 0,
      "createdAt": 1749999000, "expiresAt": 1750000000,
      "maxUses": 0, "used": 0, "visits": 2, "scans": 1,
      "password": false, "expired": false
    }
  ]
}
```

> **The list never contains the tokens themselves.** They are bearer credentials; a list that hands
> them out would let anyone who can read a room's history reuse somebody else's share.
>
> **Who can read it**: whoever can create a share in that room. In an open room that is everyone
> who can reach the server — the list records what was shared from that room, and that room's
> contents are already public. Give the room a password if you need the history protected.

### POST /share/visit

Reports that a **human** opened the share page. The share page calls this once; the server
validates the token itself (invalid or expired tokens are rejected with 401 and never counted).

```http
POST /share/visit
Content-Type: application/json

{"token": "<token>", "qr": true}
```

```json
{ "ok": true, "tracked": true, "visits": 3, "scans": 1 }
```

- `tracked` is `false` when the same visitor reports again within ten minutes — repeated reports
  must not inflate the number
- `qr: true` (or `?q=1`) marks one scan in addition to the open; QR codes should encode
  `/s/<token>?q=1`, and the share page reads that flag straight from the query string of the address
  it was opened with — nothing has to be forwarded, because the crawler and the human share one
  address
- **No authentication**: holding the link is what lets you report, and the response only describes
  that one share
- Counting happens **only** through this endpoint (the share page calls it once on mount). Serving
  `/s/<token>` never counts by itself, so a chat app crawling that address repeatedly cannot inflate
  anything: crawlers do not execute the page, hence they never report

### GET /share?t=&lt;token&gt;

What the share page asks before fetching anything: type, file name and size, remaining validity,
and whether a password is needed. **It does not consume a use** — opening the page should not burn one.

```json
{"type": "content", "kind": "text", "id": "7", "room": "default",
 "expiresAt": 1750000000, "maxUses": 0, "used": 0, "needsPassword": false}
```

Failures are distinguishable so the page can react:

| `code` | Status | Meaning |
|---|---|---|
| `share_token_invalid` | 401 | Bad signature, or expired |
| `share_password_required` | 401 | Password missing or wrong |
| `content_not_found` / `file_not_found` | 404 | Gone |
| `file_expired` | 404 | Expired |

**The password travels in the `X-Share-Password` header, never in the URL** — query strings end up in
browser history and server access logs. The token only stores `HMAC(server key, password)`.

### DELETE /revoke/:id

Deletes one entry. Both backends accept `DELETE`.

### DELETE /revoke/all

Clears every message in the room and broadcasts `clearAll` over WebSocket.

---

## 8. Real-time push

### WS /push

```
ws://localhost:9501/push?room=default&token=<token>
```

Once connected, new messages in that room are pushed to every listener:

```json
{"event": "newMessage", "data": { ...same shape as /content/:id JSON... }}
```

Reconnection is the client's job (the web UI retries with exponential backoff).

---

## 9. Error codes

| `code` | Typical status | Meaning |
|---|---|---|
| `unauthorized` | 401 | Missing credential |
| `unauthorized_invalid_token` | 401 | Invalid credential |
| `room_forbidden` | 401 | No access to this room |
| `method_not_allowed` | 405 | Wrong HTTP method |
| `invalid_request_body` | 400 | Body is not valid JSON |
| `content_not_found` | 404 | No such content |
| `no_content` | 404 | The room has no content yet |
| `invalid_content_id` | 400 | Content ID is not a number |
| `file_not_found` | 404 | No such file |
| `file_expired` | 404 | File has expired |
| `file_too_large` | 413 | File exceeds the limit |
| `text_too_long` | 413 | Text exceeds the limit |
| `unsupported_format` | 400 | Unknown `?format=` value |
| `form_parse_failed` | 400 | multipart parsing failed |
| `invalid_uuid` | 400 | Malformed UUID |
| `missing_id` / `missing_type` / `missing_uuid` | 400 | Share request is missing a parameter |
| `unsupported_type` | 400 | Unsupported share type |
| `password_required` | 401 | Empty password |
| `wrong_password` | 401 | Wrong password |
| `share_token_invalid` | 401 | Share token bad or expired |
| `share_password_required` | 401 | Share password missing or wrong |
| `internal_error` | 500 | Server-side failure |

---

## 10. Implementation notes for clients

1. **Call `/server` first** for the limits; never hard-code them.
2. **Limits are dynamic**: the numbers inside limit errors come from server config
   (`text.limit` / `file.limit`). Show the server's `message` verbatim instead of composing
   your own sentence.
3. **Parse errors one way**: read `message` (Chinese) or `error` (English). Do not branch on
   `Accept`.
4. **Downloads need credentials**, and **ignore your own `room`**.
5. **Do not conflate `name` and `client`**: one is for humans, the other tells the program
   which messages are its own.
6. **Do not rely on "no room means default"**: omitting `room` means "any room" on some
   endpoints. If you mean the default room, send `?room=default` explicitly.
