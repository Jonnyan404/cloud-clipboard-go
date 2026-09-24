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

Common codes are listed in the [error table](#10-error-codes) below.

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
| GET/POST | `/tasks` | Scheduled automations: list / create or update (**Go only**) | Room |
| POST | `/tasks/preview` | Render an unsaved task without persisting (**Go only**) | Room |
| DELETE | `/tasks/:id` | Delete an automation (**Go only**) | Room |
| POST | `/tasks/:id/run` | Dry run (default), or send now with `?send=1` (**Go only**) | Room |
| GET | `/tasks/cron` | Validate a cron expression and list upcoming fire times (**Go only**) | Room |
| POST | `/tasks/:id/toggle` | Enable / disable one automation (**Go only**) | Room |
| GET | `/automation` | Automation management page (HTML) (**Go only**) | No |
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

## 8. Scheduled automations (`/tasks`)

> **Go implementation only.** The Worker does not implement this family yet.

An automation makes the server post a rendered piece of text into a room **at a fixed time, with
nobody watching**. That makes it a **proxy for write access** — whoever can create a task gains the
ability to speak in that room unattended. So the policy is scoped per **room** and lives in
`roomAuth` (full reference: [configuration](../cloud-clip/config.md)).

### 8.1 Tiers

| `roomAuth[x].automation` | Client credential | Max tasks | Scope |
|---|---|---|---|
| omitted (default) | Follows room auth: room password → `room`; public room → `none` | — | — |
| `none` | anything, including the global password | 0 | config file only |
| `single` | none (a task token instead) | 1 | only the task you created |
| `room` | room password / room session token | 20 | this room only |
| any policy **+ global password** | global password | unlimited | any room, via `?room=` |

`GET /server?room=R` reports what the current credential may do:

```json
{
  "automation": {
    "enabled": true, "room": "home", "tier": "room", "allowed": true,
    "admin": false, "max": 20, "defaultTZ": "Asia/Shanghai",
    "vars": ["date", "weekday", "time", "datetime", "timestamp", "uuid", "task", "room"],
    "actions": [
      {"id": "text.trimLines", "group": "text", "groupKey": "actionGroupText", "key": "actionTrimLines"}
    ]
  }
}
```

`tier` is `admin` / `room` / `single` / `none`; `max` is `0` for unlimited.

`key` and `groupKey` are **i18n keys from the front end's locale files** (`actionTrimLines`,
`actionGroupText`). Clients look up their own translation table with them — showing a raw id like
`text.trimLines` to a non-technical user is meaningless.

> ⚠️ The server ships **keys, never translations**. The single source of translations is the front
> end's `locales/*.json`; keeping a second copy on the server guarantees that the same action ends up
> named differently in two places.

### 8.1.1 Credentials: a session token, not a password

The `/automation` page **never stores the password**: it exchanges it immediately via
`POST /auth/token` for a 1-hour session token, sends `Authorization: Bearer <token>` from then on, and
silently renews it through `POST /auth/token/refresh` before it expires.

The token lives in `sessionStorage['roomAuthCache']` — **the same key and shape the SPA uses**:

```
{ "<room>": { token: "...", expiresAt: 1790225666 } }
```

- Room key: `__default__` for the `default` room, otherwise the room name itself;
- A token obtained with the **global password** has `scope=global` and lives under `__global__`,
  valid for every room;
- `expiresAt` is Unix **seconds** (not milliseconds).

Consequences:

- Moving from the SPA to `/automation` (or back) **in the same tab needs no second login**;
- Closing the tab invalidates the token — nothing long-lived is left on disk;
- Both surfaces share one renewal rule, so they cannot disagree about expiry.

> **Entry point**: the automation button on the SPA toolbar navigates to
> `/automation?room=<room>` with the current room, and the "Back to the app" link on the admin page
> returns to the same room. That hop happens **in the same tab**, never in a new window — a new tab
> has an empty `sessionStorage` and would ask for the password again. So this link **must not carry
> `target="_blank"`**, and a test guards that (`TestSpaToolbarEntryStaysInSameTab`).

> The locale is shared the same way: `localStorage['locale']`, one of `zh` / `zh-TW` / `en` / `ja`.
> Priority: `?lang=` > `localStorage['locale']` > `navigator.language` (same rules as the SPA).

> The one thing that stays in `localStorage` is the **task token** for `single`-tier rooms
> (key `ccgAutomationTaskToken:<room>`): it must survive a tab close, otherwise the user can never
> edit the automation they created. It only governs one task and has nothing to do with the password.

> Do **not** decide whether to show an automation UI by checking "does this room have a password".
> That drifts from the server policy (`{"open": true}` rooms have no password but may still be
> `none`). `/server` is the single source of truth. Conversely, hiding the UI is **not** a security
> boundary — the API still rejects with `automation_forbidden`.

### 8.2 The room comes from the credential, not the body

`POST /tasks` has **no `room` field**; sending one is ignored. The room is derived from the
authenticated context (`?room=` must pass room auth). Same reasoning as `/file/:uuid/:name`:
any spoofable input will eventually be used (`?room=default`). An existing task **cannot** change
its room.

### 8.3 Variables

The body is a template; `{{ }}` is evaluated **at the moment of firing**. The offset grammar is the
same one the action library uses for `date.add` (`[+-]N[dwmy]`):

| Variable | Meaning | Example |
|---|---|---|
| `{{date}}` | Date of the base instant | `2026-09-24` |
| `{{date:+1d}}` | Plus one day (`w` / `m` / `y` work too; unit defaults to days) | `2026-09-25` |
| `{{weekday}}` | Weekday of the base instant | `周四` |
| `{{weekday:+1d}}` | Weekday of **tomorrow** | `周五` |
| `{{weekday:en\|+1d}}` | Styles: `zh` / `zh-short` / `en` / `en-short`; order is free | `Friday` |
| `{{time}}` | `HH:MM` at firing time | `09:30` |
| `{{datetime}}` | `YYYY-MM-DD HH:MM` | `2026-09-24 09:30` |
| `{{timestamp}}` | Unix seconds | `1790219280` |
| `{{uuid}}` | A fresh UUID per evaluation | — |
| `{{task}}` / `{{room}}` | Task name / room name | — |
| `{{latest}}` | Newest **human-posted** text in the task's own room | — |
| `{{latest:room}}` | Newest human-posted text in another room | — |

⚠️ **The offset belongs to each variable**, not to a shared "today". To get
"tomorrow is 9-25 (Friday)" you must write `{{date:+1d}}` **and** `{{weekday:+1d}}`; using
`{{weekday}}` yields the self-contradictory "tomorrow is 9-25 (Thursday)".

Unknown variables and malformed offsets fail at **save time** with `invalid_task` — never silently.
Rendering is all-or-nothing; a half-substituted body is never returned.

#### `{{latest}}`: using a room's newest message as an input source

This is the only variable that reads **external state** (all the others depend solely on their
arguments and the clock), so it comes with three rules:

1. **Human text only.** File messages are skipped, and so is anything with
   `source == "automation"` — that is the **loop guard**: a task reading room A and posting back
   into A would otherwise feed its own output into the next run, stacking prefixes forever.
2. **Room access is judged for an unattended actor, strictly**: only rooms readable **without a
   password** (public rooms), plus the task's own room. Password-protected rooms are out of reach,
   because an unattended task has no credential to present, and granting access based on "the
   creator could read it at the time" would be a privilege-escalation path (create the task while
   you have access, keep reading after the password changes).
   **Exception: admins** (holding the global password, or a session token obtained with it) **may
   reference any room** — they are the one for whom `canAccessRoom` is always true, so blocking
   them would cost a step and block nothing. Save and preview both enforce this and return
   `source_room_forbidden`.
3. **An empty source records `skipped`, not `error`.** An empty room is normal (nobody spoke, or
   the messages rolled off); recording an error leaves "last failed" pinned to the task and makes
   users think it is broken. The manual `POST /tasks/:id/run` is interactive, so there it returns
   the reason directly.

### 8.4 Action chains

`chain` is a list of **steps** applied in order to the rendered text. **A failing step stops the
chain** (same as the front end's `runChain`: later steps consume earlier output, and pressing on
only produces a plausible-looking wrong message).

A step has two accepted shapes — **both are read**:

```json
"chain": ["text.trimLines", {"id": "text.replace", "params": {"find": "internal", "with": "public"}}]
```

- **string** — a step without parameters (the vast majority);
- **`{id, params}`** — a step with parameters. `params` belongs to *that step*, so the same action
  may appear twice in one chain with different parameters ("replace A→B" then "replace C→D" is a
  legitimate intent; chains have always allowed repeats).

> ⚠️ Steps without parameters are **written back as strings**. Older `tasks.json` files hold
> strings only, so reading and re-writing one never reshapes it — upgrading touches no existing data.

The server implements only the tier that can run unattended (pure functions, input + clock only,
no network):

| Group | Actions |
|---|---|
| Format | `format.json.pretty` `format.json.min` |
| Text | `text.trimLines` `text.dropBlank` `text.dedupe` `text.sort` `text.upper` `text.lower` `text.replace` `text.reverse` `text.extractUrl` `text.extractEmail` `text.extractPhone` `text.extractIp` `text.extractNumber` |
| Encoding | `encode.base64` `encode.base64.decode` `encode.url` `encode.url.decode` `encode.hex` `encode.hex.decode` `encode.html` `encode.html.decode` `encode.unicode` `encode.unicode.decode` |
| Chinese | `zh.fullwidth` `zh.halfwidth` `zh.punctuation` `zh.number` |
| Date | `date.add` `date.diff` |
| Inspect | `inspect.sha256` `inspect.timestamp` `inspect.dateToTimestamp` |

**Four families are deliberately outside this tier** (not removed — client-only):

- ones producing HTML for *viewing*: `format.markdown` `format.code`;
- ones needing browser-loaded dictionaries: `zh.pinyin*` `zh.simplified` `zh.traditional`;
- ones whose output needs translating, or whose detection leans on front-end heuristics:
  `inspect.stats` `inspect.detect`;
- **the "generate" family** (`generate.uuid` / `generate.time` / `generate.datetime`): in a chain
  they **throw away the text computed so far**. Use the **template variables** instead
  (`{{uuid}}` / `{{time}}` / `{{datetime}}`) — those are inline (`Order: {{uuid}}`) and overwrite
  nothing.

> `text.replace` is a **literal** replacement and **does not support regex**: both ends implement it
> and JS's RegExp differs from Go's RE2 in too many ways (lookaround, backreferences, Unicode
> property escapes…), so promising identical behaviour would be a promise we cannot keep. An empty
> `params.find` is an error (never "insert between every character").
>
> An id outside the list returns 400 **listing what is available**; it is never skipped silently.

### 8.5 Create / update

```bash
curl -X POST "http://localhost:9501/tasks?room=home" \
  -H "Authorization: Bearer <room password>" -H "Content-Type: application/json" \
  -d '{
        "name": "On-call reminder",
        "freq": "daily",
        "time": "09:30",
        "tz": "Asia/Shanghai",
        "template": "Today is {{date}}, tomorrow is {{date:+1d}} ({{weekday:+1d}})",
        "chain": ["text.trimLines"],
        "keepHistory": false
      }'
```

| Field | Notes |
|---|---|
| `id` | Omitted = create; present = replace |
| `name` | Falls back to the first line of the body |
| `enabled` | Defaults to `true` |
| `freq` | `once` / `daily` / `weekly` / `cron` |
| `time` | `HH:MM`, for `daily` / `weekly` |
| `cron` | For `cron`: a 5-field expression (min hour dom month dow), see 8.6 |
| `byWeekday` | For `weekly`: `0` = Sunday … `6` = Saturday (same as `Date.getDay()`) |
| `runAt` | For `once`: RFC3339, or `2026-10-01T09:30` interpreted in `tz`; normalised to RFC3339 |
| `tz` | Omitted → filled with `automation.defaultTZ` (default `Asia/Shanghai`) and stored on the task |
| `template` | Required |
| `chain` | Optional list of steps — an action id string, or `{id, params}` (see 8.4) |
| `keepHistory` | Defaults to `false`: broadcasts only, **does not consume room history quota** |
| `sender` | Display name, defaults to `定时任务` |

The response is `{"task": {...}}`. When the room tier is `single` it also carries a **`taskToken`**:

```json
{ "task": {"id": "…", "room": "lobby"}, "taskToken": "f4a2…" }
```

> The `taskToken` plaintext appears **exactly once**. Clients in a public room have no other
> credential, so it is the only way to later edit or delete that task (`X-Task-Token` header or
> `?taskToken=`). Lose it and only deleting the file on the server will help. `ownerHash` is never
> exposed.

Read-only fields: `nextRunAt`, `lastRunAt`, `lastStatus` (`ok` / `error` / `skipped`), `lastError`,
`lastOutput`, and `desc` for cron tasks (see 8.6).

> **The admin page's editor only exposes `cron` and `once`.** The server still accepts `daily` /
> `weekly` (existing tasks must keep running, and API callers use them), but the UI expresses
> everything as cron: `09:30 daily` = `30 9 * * *`, `10:00 every Monday` = `0 10 * * 1`.
> Editing a legacy `daily` / `weekly` task converts it to the equivalent cron expression, so `freq`
> becomes `cron` after saving — the fire times are identical, only the representation changes.
>
> The five boxes in that UI (`[box] min [box] hour [box] day [box] month [box] weekday`) are just a
> **disassembly view of one expression**; they default to `*` and are joined into
> `"<min> <hour> <dom> <month> <dow>"` before being sent, so the wire format is unchanged.
> The preset buttons only touch day / month / weekday — they never guess your time of day.

### 8.6 Cron expressions

`freq: "cron"` plus `cron: "<min> <hour> <dom> <month> <dow>"`, for schedules the structured fields
cannot express.

| Syntax | Meaning |
|---|---|
| `*` / `?` | Any (`?` is the Quartz spelling; pasted expressions often carry it) |
| `5` | Single value |
| `1-5` | Range |
| `*/10` | Step |
| `1-30/5` | Range with step |
| `1,15,30` | List (items may themselves be ranges or stepped) |
| `MON-FRI` / `JAN-DEC` | Three-letter English names, case-insensitive |

| Common expression | Meaning |
|---|---|
| `0 9 * * *` | Daily at 09:00 |
| `*/30 9-18 * * 1-5` | Every half hour, 09:00–18:00 on weekdays |
| `0 10 * * 1` | Mondays at 10:00 |
| `0 0 1 * *` | The 1st of every month at 00:00 |
| `0 0 29 2 *` | February 29th in leap years |
| `0 9 * JAN MON` | Every Monday in January, at 09:00 |

Three rules worth remembering:

1. **Five fields only.** A six-field expression (with seconds) is rejected with an explanation of how
   to fix it — silently dropping the first field would turn `0 */5 * * * *` (meant as "every 5
   minutes") into "minute 0 of every hour", and the user would only notice a day later.
2. **Day-of-month and day-of-week are ORed** when both are restricted (standard cron semantics).
   `0 9 1 * 1` means the 1st of the month **or** every Monday. Under AND semantics it would fire a
   handful of times a year and look broken.
3. **One future instant is computed at save time**, so expressions like `0 0 30 2 *` (February 30th)
   — syntactically valid, but unreachable — are rejected up front instead of being discovered months later.

Validation and preview:

```bash
curl "http://localhost:9501/tasks/cron?room=home&expr=*/30%209-18%20*%20*%201-5&tz=Asia/Shanghai" \
  -H "Authorization: Bearer <room password>"
# → {"valid":true,"tz":"Asia/Shanghai","expr":"*/30 9-18 * * 1-5",
#    "next":[...], "nextFormatted":["2026-09-24 09:00 Thu", ...],
#    "desc":{"mode":"everyNMinutes","n":30,"hours":"9-18","day":"weekly",
#            "weekdays":[1,2,3,4,5]}}

# Invalid expressions still return 200 with valid:false + error
```

> ⚠️ An invalid expression returns **200 + `valid:false`**, not 400: validating is what this endpoint
> is *for*, so "invalid" is one of its normal outputs. Clients call it on every keystroke; treating it
> as an error makes the page look broken.

`desc` is a **structured** summary used to render a "means …" line; every cron task in `GET /tasks`
carries the same field. It describes only the *shape* of the expression and contains **no prose** —
the admin page ships zh / zh-TW / en / ja, and a Chinese sentence assembled on the server would leak
into the English and Japanese UI. Clients compose the sentence from their own message table.

| `mode` | Meaning | Extra fields |
|---|---|---|
| `everyMinute` | Every minute | `hours` when constrained |
| `everyNMinutes` | Every N minutes | `n`; `hours` when constrained |
| `everyNHours` | At minute M of every N hours | `n`, `minutes` |
| `minutesEachHour` | At minute M of every hour | `minutes`; `hours` when constrained |
| `times` | Specific instants | `times` (`HH:MM` list); `more: true` when truncated |
| `unknown` | Cannot be summarised reliably | Clients should fall back to the upcoming-instants list |

The date part is carried by `day` plus `weekdays` / `dom` / `month` / `dayN`:

| `day` | Meaning | Extra fields |
|---|---|---|
| `daily` | Every day | — |
| `weekly` | On certain weekdays | `weekdays` (already expanded to concrete days) |
| `monthly` | On certain days of the month | `dom` |
| `monthlyOrWeekly` | On certain days of the month **or** certain weekdays | `dom` + `weekdays` |
| `everyNDays` | Every N days | `dayN` |
| `everyNDaysOrWeekly` | Every N days **or** certain weekdays | `dayN` + `weekdays` |

`monthlyOrWeekly` / `everyNDaysOrWeekly` encode the OR rule between day-of-month and day-of-week;
without them, "the 1st **or** every Monday" would be reported as "the 1st **and** every Monday",
turning a dozen runs a year into one.

> ⚠️ **`everyNDays` is an approximation** and clients should treat it as one: `*/3` in
> day-of-month means "days 1, 4, 7…31 of each month", so crossing a month boundary the gap shrinks
> to as little as **one day**. Standard 5-field cron cannot express a truly even "every N days".
> The server only uses this mode when the concrete days are **too many to list** (`*/15` expands to
> just 3 days, so it is reported precisely as `monthly` with `dom: "1,16,31"`); the admin page puts
> this caveat in the hover text of the matching preset button.

> ⚠️ **Everything in `desc` is human-readable** (`1-5`, `1,15`, an expansion like `5,15,25,35,45,55`)
> and it **never contains expression syntax** — no `*`, `/` or `?`. Fields written with steps or
> English names (`*/3`, `JAN`) are expanded into concrete values first; when the expansion is too
> long to read (over 6 values — `*/3` on day-of-month yields 11) the server answers `unknown`
> instead of reading the raw token out loud. "On days 1,4,7…31 of every month" is not a summary,
> it is the expression recited. Clients may assert this as an invariant.

> ⚠️ `desc` is a **aid**, not a replacement for `nextFormatted`. Summaries always leave something out
> ("every 30 minutes" drops the 9-18 constraint in `*/30 9-18 * * 1-5`), so the UI shows both side by
> side — what the user actually verifies is the list of instants.

### 8.7 Preview / dry run / send now / toggle

```bash
# Render a task that has NOT been saved: nothing is persisted or sent
curl -X POST "http://localhost:9501/tasks/preview?room=home&at=2026-09-24T09:30:00%2B08:00" \
  -H "Authorization: Bearer <room password>" -H "Content-Type: application/json" \
  -d '{"freq":"daily","time":"09:30","tz":"Asia/Shanghai","template":"Tomorrow is {{date:+1d}} ({{weekday:+1d}})"}'
# → {"preview":true,"output":"Tomorrow is 2026-09-25 (Friday)", ...}

# Dry run a saved task (no `send` means dry run)
curl -X POST "http://localhost:9501/tasks/<id>/run?room=home" -H "Authorization: Bearer <room password>"

# Actually send once
curl -X POST "http://localhost:9501/tasks/<id>/run?room=home&send=1" -H "Authorization: Bearer <room password>"

# Enable / disable. Touches only the switch — never the template or the action chain,
# and never the idempotency key, so toggling off and on does not fire an extra message.
curl -X POST "http://localhost:9501/tasks/<id>/toggle?room=home&enabled=0" -H "Authorization: Bearer <room password>"
# Without `enabled` it flips; a body of {"enabled": true} also works
```

The reference instant defaults to the **next scheduled firing**, not "now": otherwise a task
configured at 3pm as "daily 09:30, body `{{date:+1d}}`" would preview as today+1 while tomorrow
morning it actually sends tomorrow+1 — a misleading preview. `?at=` overrides it (RFC3339, or
`2026-09-25` / `2026-09-25 09:30`).

### 8.8 Scheduling semantics

- Firing precision is **one minute**; `automation.tickSeconds` is only the scan interval. The test
  is "now ≥ some scheduled instant", so restarts and suspend never swallow a firing entirely.
- Every scheduled instant has an **idempotency key** (task id + that minute), so a single pass —
  or several instances / browser tabs — never sends twice.
- Missing the window by more than `automation.graceSeconds` means **skip, no backfill** (recorded as
  `skipped`): a restart within 10 minutes catches up, an overnight outage does not. Backfilling
  yesterday's reminder is pure noise.
- A backfilled message is rendered from the **scheduled** instant, not the actual send time, and is
  flagged `late: true`.
- `once` tasks disable themselves after firing (or after being missed).
- Automation messages carry `source: "automation"`, `scheduledAt` and `late` so clients can badge them.
- Automation messages **do not consume room history quota** by default: they are broadcast live but
  never enter the history queue. Room history is counted per room (`server.history`), so a daily task
  would otherwise replace the whole room history with "here's today's date" within a couple of weeks.
  Set `keepHistory: true` to opt in.

---

## 9. Real-time push

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

## 10. Error codes

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
| `automation_disabled` | 404 | Automation is off (`automation.enabled = false`) |
| `automation_forbidden` | 403 | Automation is not enabled for this room |
| `task_not_found` | 404 | No such automation |
| `task_forbidden` | 403 | The automation belongs to someone else or another room |
| `task_limit_reached` | 400 | Room automation limit reached |
| `invalid_task` | 400 | Invalid task (bad variable, bad clock, unusable action…) |
| `render_failed` | 400 | Rendering failed during a dry run |
| `invalid_reference` | 400 | `?at=` reference time could not be parsed |
| `source_room_forbidden` | 400 | `{{latest:room}}` points at a password-protected room (unattended tasks cannot read it) |
| `invalid_timezone` | 400 | Unrecognised time zone name |
| `internal_error` | 500 | Server-side failure |

---

## 11. Implementation notes for clients

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
