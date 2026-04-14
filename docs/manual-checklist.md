# Manual Checklist (Backend UseCase)

テストコードを書かずに、ターミナルから最低限の回帰確認を行うためのコピペ用メモ。

## 0. 前提

```bash
BASE=http://localhost:8080/api
```

- バックエンドが `:8080` で起動済み
- `cookies.txt` にログイン済みセッションがある
- `CSRF` が有効（`signup` / `login` レスポンスの `data.csrf_token`）
- ローカル HTTP 開発時は `.env` で `COOKIE_SECURE=false`（本番は `true` 推奨）

必要なら新規ログイン:

```bash
USER="u$(date +%s)"
PASS="password12"
RESP=$(curl -s -c cookies.txt -b cookies.txt -H "Content-Type: application/json" \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" "$BASE/auth/signup")
CSRF=$(echo "$RESP" | jq -r '.data.csrf_token')
```

---

## 1. ルーム作成〜開始

```bash
RESP=$(curl -s -c cookies.txt -b cookies.txt \
  -H "X-CSRF-Token: $CSRF" \
  -X POST "$BASE/rooms")
echo "$RESP" | jq .
ROOM_ID=$(echo "$RESP" | jq -r '.data.room.id')
echo "ROOM_ID=$ROOM_ID"
```

```bash
curl -s -c cookies.txt -b cookies.txt \
  -H "X-CSRF-Token: $CSRF" \
  -X POST "$BASE/rooms/$ROOM_ID/join" | jq .
```

```bash
curl -s -c cookies.txt -b cookies.txt \
  -H "X-CSRF-Token: $CSRF" \
  -X POST "$BASE/rooms/$ROOM_ID/start" | jq .
```

---

## 2. version を毎回取り直す（重要）

更新系 API を叩く前に必ず実行:

```bash
VER=$(curl -s -c cookies.txt -b cookies.txt "$BASE/rooms/$ROOM_ID" | jq -r '.data.session.version // empty')
echo "VER=$VER"
```

`VER` が空の場合は、その時点でセッションが無い（ターン操作不可）。

---

## 3. action_id は毎回一意化（重要）

```bash
AID="act-$(date +%s)-$RANDOM"
echo "$AID"
```

---

## 4. version_conflict チェック

同じ `expected_version` を2回使って競合確認。

```bash
VER=$(curl -s -c cookies.txt -b cookies.txt "$BASE/rooms/$ROOM_ID" | jq -r '.data.session.version // empty')
AID1="hit-$(date +%s)-$RANDOM"
curl -s -c cookies.txt -b cookies.txt \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d "{\"expected_version\":$VER,\"action_id\":\"$AID1\"}" \
  -X POST "$BASE/rooms/$ROOM_ID/hit" | jq .

AID2="stand-$(date +%s)-$RANDOM"
curl -s -c cookies.txt -b cookies.txt \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d "{\"expected_version\":$VER,\"action_id\":\"$AID2\"}" \
  -X POST "$BASE/rooms/$ROOM_ID/stand" | jq .
```

期待:
- 1回目は成功
- 2回目は `version_conflict`（または状態遷移済みなら `invalid_game_state` 相当）

---

## 5. duplicate_action チェック

同じ `action_id` を再送する。

```bash
VER=$(curl -s -c cookies.txt -b cookies.txt "$BASE/rooms/$ROOM_ID" | jq -r '.data.session.version // empty')
AID="dup-$(date +%s)-$RANDOM"

# 1回目
curl -s -c cookies.txt -b cookies.txt \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d "{\"expected_version\":$VER,\"action_id\":\"$AID\"}" \
  -X POST "$BASE/rooms/$ROOM_ID/hit" | jq .

# 2回目（同一payload再送）
curl -s -c cookies.txt -b cookies.txt \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d "{\"expected_version\":$VER,\"action_id\":\"$AID\"}" \
  -X POST "$BASE/rooms/$ROOM_ID/hit" | jq .
```

期待:
- 同一 payload なら冪等再送として扱われる

差分 payload で `duplicate_action` を見る例:

```bash
VER=$(curl -s -c cookies.txt -b cookies.txt "$BASE/rooms/$ROOM_ID" | jq -r '.data.session.version // empty')
AID="dup-mismatch-$(date +%s)-$RANDOM"

# 1回目: hit
curl -s -c cookies.txt -b cookies.txt \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d "{\"expected_version\":$VER,\"action_id\":\"$AID\"}" \
  -X POST "$BASE/rooms/$ROOM_ID/hit" | jq .

# 2回目: 同じ action_id で stand（payload変更）
curl -s -c cookies.txt -b cookies.txt \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d "{\"expected_version\":$VER,\"action_id\":\"$AID\"}" \
  -X POST "$BASE/rooms/$ROOM_ID/stand" | jq .
```

期待:
- `duplicate_action`

---

## 6. 自動STAND（15秒）チェック

```bash
echo "wait 16s..."
sleep 16
curl -s -c cookies.txt -b cookies.txt "$BASE/rooms/$ROOM_ID" | jq .
```

期待:
- `PLAYER_TURN` から進行している（`DEALER_TURN` / `RESULT` / `RESETTING` など）

---

## 7. 失敗時の最短確認

```bash
curl -s -c cookies.txt -b cookies.txt "$BASE/rooms/$ROOM_ID" | jq '.data.session'
echo "CSRF=$CSRF"
echo "ROOM_ID=$ROOM_ID"
```

- `session == null` なら更新系不可
- `CSRF` 空なら再ログイン
- `ROOM_ID` 空/古いなら再作成

---

## 8. Debug API: 卓リセット（§15.3）

本番では無効。ローカルで次を **シェルに export** してからバックエンドを起動する。

```bash
export BLACKJACK_DEBUG_ROOM_RESET=true
```

ホストのみ実行可。`game_sessions` 系と `room_players` を削除し、卓を `WAITING` に戻す。

```bash
curl -s -c cookies.txt -b cookies.txt \
  -H "X-CSRF-Token: $CSRF" \
  -X POST "$BASE/rooms/$ROOM_ID/reset" | jq .
```

無効時は `403` / `debug_disabled`。
