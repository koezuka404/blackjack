# Blackjack — ドキュメント

**本ファイル（`docs/README.md`）が開発ガイドおよびドキュメント索引の入口**です。

JWT 認証でルームを作成・参加し、ブラックジャックを進行する Web API（Go/Echo + PostgreSQL + Redis）と React（Vite）フロントエンドです。ルーム状態の同期に WebSocket、HTTP API のレート制限に Redis を使います。

## 資料一覧

| 資料 | 内容 |
|------|------|
| **このファイル（`docs/README.md`）** | リポジトリ構成・起動・環境変数・API 概要 |
| [BlackJack_要件定義書.md](./BlackJack_要件定義書.md) | 要件定義 |
| [BlackJack_実装仕様書.md](./BlackJack_実装仕様書.md) | 実装仕様（API・認証・フロー等） |
| [アーキテクチャ.drawio](./アーキテクチャ.drawio) | システム構成図（draw.io / diagrams.net） |
| [ER.png](./ER.png) | ER 図 |

閲覧のヒント: draw.io は [diagrams.net](https://app.diagrams.net/) に `アーキテクチャ.drawio` を読み込むか、デスクトップ版で開いてください。

---

## 構成（リポジトリ）

- `backend/`: Go + Echo（REST / WebSocket / ゲームロジック / DB 永続 / Redis）
- `frontend/`: React + Vite（UI）
- [`docker-compose.yml`](../docker-compose.yml): PostgreSQL、Redis（ルーム用・レート制限用の 2 系統）、migrate、backend、nginx 付きフロントをまとめて起動
- `docs/`: 本フォルダ（設計資料・本ガイド）

## 起動（Docker）

リポジトリ直下で:

```bash
docker compose up --build
```

起動後のアクセス先（既定ポート）:

| 用途 | URL |
|------|-----|
| フロント（nginx） | `http://localhost`（`WEB_PORT` で変更可。既定 **80**） |
| API（直接） | `http://localhost:8080` |
| 健康確認 | `GET http://localhost:8080/health` |

初回は `migrate` サービスで PostgreSQL にスキーマが適用されます。以降は `docker-compose.yml` の volume に DB / Redis データが残ります。既存 DB でマイグレーションが `users.email` の NULL 等で失敗する場合は、`docker compose logs migrate` を確認するか、開発用途ではボリューム削除後にやり直してください。

## ローカル開発（環境変数）

バックエンドは環境変数を参照します（[`docker-compose.yml`](../docker-compose.yml) に開発用の例あり）。

最低限必要なもの:

| 変数 | 説明 |
|------|------|
| `DATABASE_URL` | PostgreSQL 接続 URL |
| `JWT_SECRET` | JWT 署名用（**16 文字以上**） |
| `REDIS_ROOM_ADDR` | ルーム Pub/Sub 用 Redis `host:port`（未設定時は `REDIS_ADDR` または `localhost:6379`） |
| `REDIS_RATE_LIMIT_ADDR` | レート制限用 Redis（同上） |

任意の例: `PORT`（既定 `8080`）、`WS_ALLOWED_ORIGINS`（カンマ区切り）、`WS_CONNECTION_EPOCH_TTL`、`SERVER_ID`。

フロントは [`frontend/.env.example`](../frontend/.env.example) を `frontend/.env` にコピーして編集:

- `VITE_API_BASE_URL`（例: `http://localhost:8080/api`）
- `VITE_WS_BASE_URL`（例: `ws://localhost:8080/ws`）

## 認証・CSRF

### 認証

- `POST /api/auth/signup` / `POST /api/auth/login` で `access_token`（JWT）が返ります。
- それ以外の保護された API は **`Authorization: Bearer <access_token>`** が必要です。
- WebSocket `GET /ws/rooms/:id` は HTTP ヘッダの JWT ではなく、**接続直後の最初のテキストフレーム**で `type: "AUTH"` と `access_token`（JWT）を送る方式です（`backend/dto/ws_events.go` 参照）。

### CSRF（Double Submit Cookie）

`POST` / `PUT` / `PATCH` / `DELETE` は CSRF 対象ですが、次の場合は CSRF を要求しません。

- `Authorization: Bearer` が付いている場合（実装上スキップ）
- パスが `/api/auth/login` / `/api/auth/signup` の場合

それ以外では **`csrf_token` Cookie** とヘッダ **`X-CSRF-Token`**（値が一致）が必要です。

## API（エンドポイント概要）

### 共通

レスポンスは成功・失敗ともに次の形です。

- 成功: `{ "success": true, "data": ... }`
- 失敗: `{ "success": false, "error": { "code": "...", "message": "..." } }`

### 健康確認・監視

- `GET /health`
- `GET /metrics`（Prometheus）

### 認証（`/api`）

- `POST /api/auth/signup` — ボディ: `username`, `email`, `password`
- `POST /api/auth/login` — ボディ: `email`, `password`
- `POST /api/auth/logout`（要認証）
- `GET /api/me`（要認証）

### ルーム・ゲーム（`/api`、要認証）

- `POST /api/rooms` / `GET /api/rooms`
- `POST /api/rooms/:id/join` / `POST /api/rooms/:id/leave`
- `GET /api/rooms/:id` / `GET /api/rooms/:id/history` / `GET /api/rooms/:id/play_hint`
- `POST /api/rooms/:id/start` / `hit` / `stand`
- `POST /api/rooms/:id/reset`（デバッグ用途）

### WebSocket

- `GET /ws/rooms/:id` — ハンドシェイク後、最初のメッセージで `AUTH` と JWT を送って認証。以降はゲームアクション・同期イベントを JSON で送受信

## Rate Limit

- 対象: `/api` でユーザー ID が取れるリクエスト（匿名は実質スキップ）
- 超過時: `429 Too Many Requests`
- 追加ヘッダ: `X-RateLimit-Retry-After-Ms`（再試行までのミリ秒）

## フロント開発

```bash
cd frontend
npm ci
npm run dev
```

Docker Compose 利用時は `web` サービスでビルド済み静的ファイルが nginx 経由で配信されます。

## ゲーム仕様・API 詳細

ブラックジャックの詳細ルール・状態遷移・エンドポイント細目は **[BlackJack_実装仕様書.md](./BlackJack_実装仕様書.md)** および **[BlackJack_要件定義書.md](./BlackJack_要件定義書.md)** を参照してください。
