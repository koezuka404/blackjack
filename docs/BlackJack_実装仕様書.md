# BlackJack 実装仕様書

## 0. 文書情報

| 項目 | 内容 |
|------|------|
| 文書名 | BlackJack 実装仕様書 |
| 対象 | 認証済みユーザー向け **1対1ブラックジャック** |
| 最大参加人数 | **利用者1名／卓**（※コンピューターディーラーは人数に含めない） |
| 対戦形式 | **利用者（プレイヤー） vs コンピューターディーラー** |
| ディーラー | **コンピューター（ヒューリスティックAI基本戦略）** |
| 通信 | HTTP + WebSocket |
| 正本 | PostgreSQL |
| 補助 | Redis |
| アーキテクチャ | Clean Architecture |
| ブラックジャックのルール計算 | 外部ライブラリ（Adapter経由）。手評価に加え、**1ラウンド内の進行アルゴリズム・勝敗判定の算出を委譲** |
| ゲーム進行（システム） | **永続化・通知・再接続・冪等・整合性制御・利用者ターン締切は自作**。ルールエンジンはライブラリ |
| 初版スコープ | Hit / Stand / ルーム / 再戦 / 切断復帰 / CI/CD |
| 非対象 | Split / Double / Insurance / Bet / チップ管理 / 観戦モード |

---

## 1. 目的

本システムは、認証済みユーザーが同一バックエンドに接続し、**1卓あたり利用者1名**で、**コンピューターディーラー**を相手にリアルタイムでブラックジャックをプレイできるWebアプリケーションを提供する。

本システムの設計原則は以下とする。

- ブラックジャックの**進行・勝敗算出を含むルール計算**は外部ライブラリに委譲する
- **ディーラーAIとライブラリ呼び出し順のオーケストレーションはUseCaseに集約する**
- ルーム管理、再接続復元、状態遷移、同期配信、永続化、冪等制御は自作する
- ゲーム状態の真実値は常にPostgreSQLとする
- WebSocketは通知・追従同期のために用い、正本にしない
- 更新系処理は必ず**冪等性、状態チェック、競合制御**を通す
- CI/CDパイプライン上で**品質ゲートを通過した成果物のみ**をデプロイする

---

## 2. 技術スタック

### 2.1 アプリケーション

| 区分 | 採用 |
|------|------|
| フロントエンド | React + TypeScript + Vite |
| バックエンド | **Go 1.25.x** |
| HTTP | Echo v4 |
| WebSocket | gorilla/websocket |
| ORM | GORM |
| DB | **PostgreSQL 16系** |
| Redis | go-redis v9 |
| 認証 | **JWT Bearer トークン** |
| CSRF | Bearer認証リクエストには適用しない（§16.2） |
| FEテスト | Vitest / Testing Library |
| BEテスト | Go testing / testify |

### 2.2 ブラックジャックライブラリ

依存固定：

- `github.com/ethanefung/blackjack` を go.mod で最新以外のコミットハッシュまたはタグに pin する
- 間接依存 `github.com/ethanefung/cards` も合わせて pin する

| 用途 | 採用 |
|------|------|
| ルールエンジン（手評価・進行・勝敗算出） | `github.com/ethanefung/blackjack` |
| 間接依存 | `github.com/ethanefung/cards` |
| Adapter役割 | `HandEvaluator` + `RoundEngine`（名称は実装に合わせてよい） |

### 2.3 CI/CD / 運用

| 区分 | 採用 |
|------|------|
| CI/CD | GitHub Actions |
| デプロイ先 | **Vercel（GitHub Actionsから直接デプロイ。staging段なし・production直行構成）** |
| コンテナ | docker |
| レジストリ | GHCR |
| シークレット管理 | **GitHub Secrets + Vercel Environment Variables** |
| 監視 | Vercel Observability + アプリケーションログ |
| 負荷試験 | k6 |

---

## 3. システム概要

### 3.1 プレイモデル

- 利用者はコンピューターディーラーと対戦する
- **1ルーム（卓）あたり人間プレイヤー最大1名**
- **プレイヤー同士の相互対戦は存在しない**
- 1ラウンドは配札開始から結果確定まで
- ラウンド終了後は再戦投票フェーズへ遷移する

### 3.2 状態正本

- 真実値はPostgreSQL
- Redisはレート制限、Pub/Sub、接続補助のみ
- リアルタイム通信はGoバックエンドのWebSocketサーバーにより処理される
- RedisはPub/Subを補助的に利用する場合があるが、リアルタイム通信の主体はWebSocketとする
- Vercel Functionsはステートレスであり、長時間接続は持たない
- 再接続時はDB正本から状態を再構築して送信する

---

## 4. 設計原則

### 4.1 Clean Architecture

| 層 | 責務 |
|----|------|
| Controller | HTTP/WS入出力、認証、DTO変換、レスポンス生成 |
| UseCase | トランザクション、ルーム・セッション状態遷移、整合性保証、**ディーラーAIの適用、ライブラリ呼び出し順の統制（唯一のオーケストレータ）** |
| Domain | 副作用なしの整合性・遷移可否。**数値計算は直接ライブラリを呼ばずポートに委譲し**、ルールの正・不変条件を担う |
| Infrastructure | PostgreSQL、Redis、Session、WS接続管理、**Blackjack Adapter（HandEvaluator + RoundEngine）** |

### 4.2 原則

- Domainは副作用を持たない
- DB更新はUseCaseのみ
- Controllerは業務ロジックを持たない
- WS受信から直接DBを更新しない
- WS通知はコミット後のみ
- クライアントは常にサーバー正本を正とする

### 4.3 ディーラーAI

目的：コンピューターディーラーのHit / Standを、ヒューリスティック基本戦略に基づいて決定する。

入力例：
- ディーラーの現在手札
- 公開済みカード情報
- 現在のsession状態
- 仕様で固定されたルール（6.1）

出力例：
- `HIT`
- `STAND`

ルール：
- ディーラーAIは **6.1の必須ルールを破ってはならない**
- ディーラーAIの判断結果は **UseCaseが最終ガードする**
- 適用タイミング・Tx境界はUseCaseが管理する
- AIはあくまで行動選択を補助し、DB更新・version更新・WS通知は直接行わない

#### 4.3.1 ヒューリスティックAIの定義

本仕様における**ヒューリスティックAI**とは、機械学習モデルや外部推論APIに依存せず、**手札の評価結果（スコア・ソフト可否・バスト可否）に対する決定規則の優先順位付き集合**により、コンピューターディーラーのHit / Standを決定する方式をいう。

- 同一の内部状態に対して**決定は一意に定まる**（再現性・テスト容易性のため）
- 6.1の必須ルールを満たさない行動を返してはならない
- ディーラー標準ルールに従い、**利用者の手札情報を行動決定の入力に用いない**（将来ルールを変える場合は別途仕様化する）

#### 4.3.2 レイヤ別責務（Clean Architecture）

| 層 | 責務 |
|----|------|
| **UseCase** | ディーラーターンにおける**いつヒューリスティックを呼ぶか**、ライブラリ呼び出し順、トランザクション境界、永続化・version・通知のオーケストレーション。**ヒューリスティックの結果に対する最終ガード（6.1適合確認）**を必ず行う |
| **Domain** | 行動の列挙（例：Hit / Stand）や6.1に基づく**不変条件の宣言**に留める。外部ライブラリ具象に依存しない |
| **Infrastructure** | `HandEvaluator`および`RoundEngine`等のAdapter実装。副作用のあるDB・WSは本層またはUseCase経由で行う |

ヒューリスティック本体は副作用を持たない**純粋関数**または `DealerPolicy`（または同等名称）**インタフェースの実装**とし、UseCaseから依存注入する。

#### 4.3.3 基本戦略表（ディーラー・決定版）

前提：6.1「17未満 Hit、17以上 Stand」「Soft17はStand」。

| 条件（評価後） | 行動 | 備考 |
|--------------|------|------|
| `isBust == true` | （ドロー不要） | ラウンドは結果フェーズへ。「次のディーラー行動」自体が存在しない |
| `handValue >= 21`（非Bustなら21のみ） | `STAND` | 実質21で確定 |
| `handValue >= 17` | `STAND` | Hard 17 / Soft 17 いずれも Stand |
| `handValue == 17 && isSoft` | `STAND` | Soft 17 = Stand（6.1明示。上行と重複だが監査・可読性のため残す） |
| `handValue < 17` | `HIT` | 16以下（hard / soft 問わず） |

**論理の簡約形（実装ヒューリスティック）**

1. `IsBust(hand)` → 以降のHit/Stand判断を行わない（結果処理へ）
2. `Value(hand) > 21` はBustと同義
3. `Value(hand) >= 17` → `STAND`
4. それ以外 → `HIT`

#### 4.3.4 決定アルゴリズム（擬似コード）

UseCase内の**最終ガード**として使う想定。ライブラリ呼び出し前後どちらでも使用可だが、**永続化前に必ず一度通す**。

```go
// NextDealerAction: 6.1 準拠の次アクション
func NextDealerAction(ev HandEvaluator, dealer []StoredCard) (action HitOrStand, terminal bool) {
    if ev.IsBust(dealer) {
        return Stand, true // 以降ディーラーはカードを引かない（結果処理）
    }
    v := ev.Value(dealer)
    if v == 17 && ev.IsSoft(dealer) {
        return Stand, false // Soft 17 明示ガード
    }
    if v >= 17 {
        return Stand, false
    }
    return Hit, false
}
```

- `terminal == true` のときは「ディーラーターン内でこれ以上ドローしない」を意味するが、状態機械のRESULTへの遷移は別Tx（§10）
- `IsSoft`の明示分岐は `v >= 17` で既にStandになる場合と重複するが、監査・可読性のため残してよい

#### 4.3.5 導入手順（実装の正規手順）

1. **手札評価ポートの確定**：`HandEvaluator`（`Value` / `IsSoft` / `IsBust`等）をAdapter経由で利用可能にする（§5.3）
2. **決定関数の実装**：入力：ディーラー現在手（正本から復元した表現）。出力：`HIT`または`STAND`（実装で表記を固定する）。論理は4.3.3〜4.3.4に従い、6.1と一致させる
3. **UseCaseへの組み込み**：ディーラーターンにおいて各ドロー前（またはライブラリのディーラー行動決定API呼び出し直後）に上記関数を実行し、永続化前に最終確認する
4. **ライブラリとの整合**：ライブラリが `DecideDealerAction` 等を返す場合、返却値をヒューリスティックまたは6.1に基づき正規化し、矛盾時は**本仕様（6.1・5.5）を優先する**
5. **テスト**：代表状態（Hard 16、Hard 17、Soft 17、Bust等）に対する**表駆動テスト**を行い、決定性を検証する（4.3.6・§21）

#### 4.3.6 ライブラリ連携パターン（ディーラーHit/Stand）

ディーラーのHit / Standの最終可否は、以下方式で決定する。

**ただし、正本は常に6.1および4.3系ルールとする。**

- ライブラリの返却値はそのまま正本として扱わない
- 次アクションは4.3の決定ロジック（または同等）のみで決定する
- ライブラリは以下の用途に限定する
  - 手札評価
  - Hit後の手札更新
  - スコア計算
- `RoundEngine.DecideDealerAction` は**使用しない**

**共通ルール**
- DB正本の行動履歴・outcomeは **6.2の列挙に従う**
- Controller / WSハンドラでルール判定を行わない
- **UseCaseが唯一のオーケストレーター**

#### 4.3.7 禁止事項

- **Controller**または**WebSocket受信ハンドラ**内で、ディーラーのHit/Standを決定しない
- ヒューリスティックの結果として**ランダムにHit/Standを選択しない**（基本戦略の再現性と監査に反する）
- ヒューリスティック関数内で**DB・Redis・WebSocketを直接呼び出さない**（純粋性を維持する）

---

### 4.4 プレイヤー向け行動補助（任意・読み取り専用）

#### 4.4.1 目的

§4.3 で定義する**ディーラー向けヒューリスティック**とは別に、**人間プレイヤー**が HIT / STAND を選択する際の**参考情報**を返す。機械学習・外部推論 API に依存せず、手札評価（スコア・ソフト可否等）と**ディーラーアップカード 1 枚**に基づく**決定規則の集合**により HIT または STAND を推奨する（再現性・テスト容易性のため同一局面では一意）。

#### 4.4.2 スコープと非スコープ

- **対象**：初版スコープに含まれる **HIT / STAND のみ**（§0「非対象」の Split / Double / Insurance / Bet 等は**推奨対象外**。推奨ロジック内でもこれらは扱わない）
- **正本ではない**：推奨は**ゲーム結果やサーバー正本の状態を変更しない**。クライアント表示用の補助情報に過ぎない
- **ディーラー AI との関係**：ディーラーの Hit / Stand は **§4.3・§6.1** に従い **UseCase が最終ガード**する。本節の推奨は**プレイヤー側の UI 補助**であり、ディーラー方針の入出力とは独立する

#### 4.4.3 レイヤ責務（Clean Architecture）

- **UseCase**：局面取得（正本は PostgreSQL）のうえ、純粋な推奨関数を呼び、レスポンス用に整形する。DB 更新・version 更新・WS 通知は**行わない**（本 API は読み取り専用）
- **Domain / Model**：推奨は**副作用のない純粋関数**（または同等）として実装する
- **Controller**：HTTP の入出力と DTO 変換のみ。ルール判定の正本化は行わない

#### 4.4.4 プレイヤーターン締切時のオプション挙動（環境変数）

§9.3.6・§11.1 のタイムアウト時の挙動は、運用環境変数 **`BLACKJACK_PLAYER_TIMEOUT_POLICY`** により次のいずれかとする。

| 値 | 挙動 |
|----|------|
| **未設定または空文字** | **TIME_FORFEIT**（敗北確定）。`actor_type=SYSTEM` で `action_logs` 記録 |
| **`heuristic`** | 同一の中級向け簡略ルールで **HIT が推奨される局面**では、SYSTEM が**通常の HIT ユースケース**（冪等キー・version 整合・action_logs を含む）を **1 回**試行する。推奨が STAND に相当する場合は自動 STAND と同じ経路とする |

**禁止事項**：
- ヒューリスティック関数内で DB・Redis・WebSocket を**直接**呼び出さない（§4.3.7 と同趣旨）。タイムアウト時の更新は**既存の HIT / 自動 STAND のユースケース経路**に限定する
- ランダムな HIT / STAND 選択を行わない

---

## 5. 外部ライブラリ利用方針

> **注（厳密整合）**：「Dealerの行動決定」は、ライブラリが次の行動を返却する機能を指す。ただしゲーム上の正しい行動は6.1・4.3に従う。

### 5.1 利用範囲

外部ライブラリは以下に限定する。

| 項目 | 内容 |
|------|------|
| 手札スコア計算 | ライブラリ委譲 |
| Ace解釈（1/11） | ライブラリ委譲 |
| Blackjack判定 | ライブラリ委譲 |
| Bust判定 | ライブラリ委譲 |
| Soft判定 | ライブラリ委譲 |
| プレイヤー手更新（Hit/Stand） | ライブラリ委譲 |
| ディーラー進行 | ライブラリ委譲（手札更新・進行ステップのルール計算）。ただし**Hit/Standの最終判断は4.3.6に従い、6.1・4.3を正とする** |
| 勝敗判定 | ライブラリ委譲（Adapterで正規化） |

### 5.2 利用禁止

以下は**ブラックジャックのゲームルールそのものとしては扱わず**、システム側で責務を持つ。

- ルーム管理
- ラウンド開始トリガ
- セッション作成・終了
- 山札の永続化
- 配札結果・進行結果の永続化
- WebSocket同期
- 再接続復元
- 冪等制御
- version / 排他制御
- タイムアウト監視
- 認証・認可
- 再戦受付・再戦確定
- 監査ログ保存

**補足**

- **ブラックジャックのゲーム進行そのものはライブラリに委譲する**（手札評価、Hit後の手札更新、Stand後の進行判定、Dealerの行動決定、Dealerの進行、Bust/Blackjack判定、勝敗算出）
- システム側は、ライブラリが返した結果をDB正本へ保存し、必要に応じて6.2のoutcomeに正規化して扱う
- **UseCaseはルール計算を再実装しない**。役割は、ライブラリ呼び出し、Tx制御、整合性保証、永続化、通知のオーケストレーションに限定する

#### 5.2.1

- ディーラーHit / Standの正は **6.1および4.3**
- `DecideDealerAction` は必須ではない
- 使用する場合は**必ず正規化する**
- UseCaseは**完全なルールエンジンを再実装しない**
- 4.3は「ルール」ではなく**ポリシー／ガード層**

> ※ ディーラーのHit / Standの最終可否については、§4.3.6および§5.2.1を優先して解釈すること。ライブラリは進行・手札更新等の計算に用いるが、**正本の行動決定は§4.3の決定ロジック（または同等）および§6.1とする**。

### 5.3 Adapter契約

```go
type HandEvaluator interface {
    Value(hand []StoredCard) int
    IsBlackjack(hand []StoredCard) bool
    IsBust(hand []StoredCard) bool
    IsSoft(hand []StoredCard) bool
}

type RoundEngine interface {
    ApplyPlayerHit(hand []StoredCard, draw StoredCard) (nextHand []StoredCard, err error)
    DecideDealerAction(dealerHand []StoredCard) (action string, err error)
    ResolveOutcome(playerHand []StoredCard, dealerHand []StoredCard) (outcome string, err error)
}
```

**実装メモ（Adapter）**：

- `DecideDealerAction` はインタフェース上は存在し得るが、**ディーラーの次手の最終決定には用いない**（§4.3.6）。本仕様では§4.3の決定ロジック（または同等）のみで次手を確定する
- ライブラリ実装が当該メソッドを提供する場合でも、UseCase側では**返却値を正本として保存・通知に用いず**、必要なら§5.5に従い§6.1・§4.3に正規化した結果のみを永続化・配信する
- 新規Adapter実装時は、`DecideDealerAction` を呼ばない、または呼んでも結果を捨てて§4.3で再決定する、のいずれかとすること

### 5.4 判定ルール

- `IsBlackjack` : `len(hand) == 2 && Value(hand) == 21`
- `IsBust` : `Value(hand) > 21`
- `IsSoft` : Aceを11として評価している状態

### 5.5 ライブラリ挙動差分の優先順位

外部ライブラリの挙動が本仕様と矛盾する場合、本仕様を優先する。

特にディーラー進行は本仕様の `Soft17 = Stand` を優先し、必要ならAdapter内で補正する。

**outcomeは常に6.2の列挙へ正規化し、ライブラリの勝敗型をそのまま正本にしない。**

最終的な保存値・通知値・監査値は、本仕様に従ってUseCaseが決定する。

---

## 6. ゲームルール

### 6.1 基本ルール

- 初期配布はプレイヤー2枚、ディーラー2枚
- 21超えでBust
- 初期2枚で21のみBlackjack
- 3枚以上で21は通常の21
- 数札は額面、J/Q/Kは10、Aは1または11
- ディーラーは17未満でHit、17以上でStand
- Soft17はStand
- コンピューターディーラーは4.3のヒューリスティック戦略を適用しつつ、本条の必須ルールを満たす

### 6.2 勝敗優先順位

1. プレイヤーBust → LOSE
2. ディーラーBustかつプレイヤー非Bust → WIN
3. プレイヤーBJかつディーラー非BJ → WIN
4. ディーラーBJかつプレイヤー非BJ → LOSE
5. 両者BJまたは同点 → PUSH
6. その他は点数比較

---

## 7. 永続化モデル

### 7.1 テーブル一覧

- `rooms`
- `room_players`
- `game_sessions`
- `player_states`
- `dealer_states`
- `action_logs`
- `round_logs`
- `rematch_votes`

### 7.2 rooms

役割：ルーム永続単位、粗粒度状態（ロビー表示・参加可否・開始可否）、現在の進行セッション参照

主カラム：`id` / `host_user_id` / `status` / `current_session_id` / `created_at` / `updated_at`

補足：
- roomsは楽観ロック用のversion列を持たない
- ルーム作成・参加・退出など「セッション未作成」の更新では、競合制御用のversionは存在しない
- ルームに関する競合制御・更新整合性は `game_sessions.version` を正とする（セッションが存在する操作のみ）
- `rooms.status` はロビー表示・参加可否・開始可否判断用の粗粒度状態とする
- ラウンド内の詳細進行は `game_sessions.status` を正とする

### 7.3 room_players

主カラム：`room_id` / `user_id` / `seat_no` / `status` / `joined_at` / `left_at`

ルール：
- **同一Room内の人間プレイヤーは最大1名**
- 2人目の `join` は `room_full`
- `seat_no` は1固定でよい
- 同一Room内で `ACTIVE` / `DISCONNECTED` の人間プレイヤー重複は禁止
- `LEFT` は進行対象外

### 7.4 game_sessions

主カラム：`id` / `room_id` / `round_no` / `status` / `version`（bigint、楽観ロック。新規セッションは1から開始） / `deck` / `draw_index` / `turn_seat` / `turn_deadline_at` / `result_snapshot` / `rematch_deadline_at` / `created_at` / `updated_at`

補足：本システムの楽観ロックは `game_sessions.version` のみ（§8.1参照）。

### 7.5 player_states

役割：参加者ごとの手札と進行状態（単一status）

主カラム：`session_id` / `user_id` / `seat_no` / `hand` / `status`（ACTIVE | STAND | BUST | BLACKJACK | DISCONNECTED | LEFT） / `outcome`（RESULT確定後のみ） / `final_score`（RESULT確定後のみ。任意）

制約：`UNIQUE(session_id, user_id)` / `UNIQUE(session_id, seat_no)`

禁止：`is_stand` / `is_bust` のような重複表現（statusと二重管理しない）

### 7.6 dealer_states

主カラム：`session_id` / `hand` / `hole_hidden` / `final_score`

### 7.7 action_logs

役割：冪等性、監査、再送応答保存

主カラム：`session_id` / `actor_type` / `actor_user_id` / `target_user_id` / `action_id` / `request_type` / `request_payload_hash` / `response_snapshot`

一意制約：`UNIQUE(session_id, actor_user_id, action_id)`

成功応答のみ `response_snapshot` に保存する。

### 7.8 rematch_votes

主カラム：`id` / `session_id` / `user_id` / `agree` / `created_at` / `updated_at`

制約：`UNIQUE(session_id, user_id)`

- 同一ユーザーの再投票はupsertで上書き
- 最終票のみ有効

### 7.9 round_logs

役割：ラウンド単位の監査用・不変ログ（正本は `game_sessions` 等）。

列例：`id` / `session_id` / `round_no` / `result_payload`（JSONB）/ `created_at`。`UNIQUE(session_id, round_no)`。§9.3.8のTxコミット成功後にappendのみ。

---

## 8. version / action_id / request_id

### 8.1 version

本システムの楽観ロック用バージョンは `game_sessions.version` のみとする。

`rooms.version` は保持しない（または保持してもAPI/WSの競合制御には使用しない）。

更新系処理（HIT / STAND / 自動STAND / Dealer Draw / Dealer終了 / REMATCH確定）成功時にのみ `game_sessions.version = version + 1`。

read操作、再接続、同一action_idの冪等再送では増加させない。

### 8.2 action_id（業務冪等キー）

更新系イベントは `payload.action_id` 必須。

同一 `(session_id, actor_user_id, action_id)` が既に成功している場合：
- `request_payload_hash` が同一なら前回成功レスポンスを返す（冪等成功）
- 異なるなら `duplicate_action` を返す

### 8.3 request_id（通信相関キー）

`request_id` は通信上の対応付けキーであり、冪等性判定には使わない。

再送時に新しい `request_id` を用いてよい。

---

## 9. 状態遷移仕様

### 9.1 Room状態

- `WAITING`
- `READY`
- `PLAYING`

判定ルール：Room状態の再計算は、進行中sessionの有無と `room_players` 内の有効な人間プレイヤー数のみを根拠とする。

- 人間プレイヤー0名 → `WAITING`
- 人間プレイヤー1名かつ進行中sessionなし → `READY`
- `current_session_id` が存在し、`session.status` が `DEALING` / `PLAYER_TURN` / `DEALER_TURN` / `RESULT` / `RESETTING` のいずれか → `PLAYING`

最優先：進行中sessionあり → `PLAYING`

次：1名・sessionなし → `READY` / 0名 → `WAITING`

禁止事項：
- CreateRoom時にhostを暗黙的に参加扱いしてはならない
- `rooms.status` を直接更新してはならない（必ず再計算で決定する）

### 9.2 Session状態

- `DEALING`
- `PLAYER_TURN`
- `DEALER_TURN`
- `RESULT`
- `RESETTING`

### 9.3 状態遷移表

#### 9.3.1 ルーム作成

| 項目 | 内容 |
|------|------|
| 前提 | 認証済み |
| 実行主体 | USER |
| 更新 | `rooms` を生成する。`room_players` は生成しない |
| 結果状態 | `WAITING` |
| version | roomsにversion列は持たない |
| 失敗 | `unauthorized` / `internal_error` |

#### 9.3.2 ルーム参加

| 項目 | 内容 |
|------|------|
| 前提 | `room.status = WAITING or READY` |
| 実行主体 | USER |
| 更新 | 最初の1人を `room_players` に追加する（`seat_no = 1`） |
| 結果状態 | 成功時 `READY` |
| version | ルーム再計算のみ。sessionは未作成のためversion増分なし |
| 失敗 | `room_not_found` / `room_full` / `invalid_game_state` / `forbidden` |

追記：JoinRoomは**host本人のみ許可**とする。既に `ACTIVE` または `DISCONNECTED` の人間プレイヤーが存在する場合は `room_full`。

#### 9.3.3 開始

前提：
- `room.status = READY`
- 進行中sessionなし（`current_session_id` がnull）
- クライアントはversionを送らない（セッション未作成のため競合制御対象が存在しない）
- 実行者がhost

更新：
- `game_sessions` を新規作成（`version = 1`）
- deck保存、配札、`dealer_states` 作成、`player_states` 作成
- `rooms.current_session_id` を新session idに更新

結果状態：`session.status = DEALING` →（配札完了後）`PLAYER_TURN`、`room.status = PLAYING`

失敗：`forbidden` / `invalid_game_state`

#### 9.3.4 HIT

| 項目 | 内容 |
|------|------|
| 前提 | `session.status = PLAYER_TURN`、自ターン、未Stand、未Bust |
| 実行主体 | USER |
| 更新 | 1枚draw、hand更新、Bust判定 |
| 結果状態 | `PLAYER_TURN` または `DEALER_TURN` |
| version | +1 |
| 失敗 | `not_your_turn` / `invalid_game_state` / `version_conflict` / `duplicate_action` |

#### 9.3.5 STAND

| 項目 | 内容 |
|------|------|
| 前提 | `session.status = PLAYER_TURN`、自ターン、未Stand、未Bust |
| 実行主体 | USER |
| 更新 | `player_states.status = STAND` |
| 結果状態 | `DEALER_TURN` |
| version | +1 |
| 失敗 | `not_your_turn` / `invalid_game_state` / `version_conflict` / `duplicate_action` |

#### 9.3.6 TIME_FORFEIT / 自動STAND（タイムアウト）

| 項目 | 内容 |
|------|------|
| 前提 | `turn_deadline_at` 到達、未操作 |
| 実行主体 | SYSTEM |
| 更新（デフォルト） | **TIME_FORFEIT**：プレイヤー敗北確定、`action_logs` 記録（`actor_type=SYSTEM`） |
| 更新（heuristicモード） | `player_states.status = STAND`、`action_logs` 記録（`actor_type=SYSTEM`） |
| 結果状態 | **DEALER_TURN**（1対1のためPLAYER_TURNは存在しない） |
| version | +1 |
| 失敗 | `internal_error` |

#### 9.3.7 Dealer Draw

| 項目 | 内容 |
|------|------|
| 前提 | `session.status = DEALER_TURN`、dealer score < 17 |
| 実行主体 | SYSTEM |
| 更新 | dealer handに1枚追加、`draw_index` 更新 |
| 結果状態 | `DEALER_TURN` 継続 |
| version | +1 |
| 失敗 | `internal_error` |

#### 9.3.8 Dealer Turn終了

| 項目 | 内容 |
|------|------|
| 前提 | dealer score >= 17 |
| 実行主体 | SYSTEM |
| 更新 | `hole_hidden = false`、勝敗確定、`result_snapshot` 保存、`round_logs` 保存 |
| 結果状態 | `RESULT` |
| version | +1 |
| 失敗 | `internal_error` |

#### 9.3.9 RESULT → RESETTING

| 項目 | 内容 |
|------|------|
| 前提 | 勝敗確定直後 |
| 実行主体 | SYSTEM |
| 更新 | `rematch_deadline_at = now + 30s` |
| 結果状態 | `RESETTING` |
| version | +1 |
| 失敗 | `internal_error` |

#### 9.3.10 REMATCH成立

| 項目 | 内容 |
|------|------|
| 前提 | `session.status = RESETTING`、対象者全員 `agree = true` |
| 実行主体 | SYSTEMのみ（投票受信処理または締切ジョブ内で最終確定） |
| 更新 | 新しい `game_sessions` を作成（次ラウンド）、`rooms.current_session_id` を新セッションへ更新、配札準備 |
| 結果状態 | `rooms.status = PLAYING`、新 `session.status = DEALING`（直後に`PLAYER_TURN`） |
| version | 新セッションはversion = 1で開始（旧セッションは完了済み） |
| 失敗 | `invalid_game_state` / `internal_error` |

#### 9.3.11 REMATCH不成立

| 項目 | 内容 |
|------|------|
| 前提 | `session.status = RESETTING`、締切到達または否認票あり |
| 実行主体 | SYSTEM |
| 更新 | `rooms.current_session_id = null`、`rooms.status` を有効参加者数で再計算 |
| 結果状態 | `WAITING` または `READY` |
| version | ルーム再計算のみ（セッションversionは増やさない） |
| 失敗 | `internal_error` |

---

## 10. Dealer進行のトランザクション境界

### 10.1 方針

Dealerの1ドローを1トランザクションとする。

理由：
- 1ドロー1Txにすることで、観測可能状態とversion増分が一致する
- 障害時の中途状態が明確になる
- 再送・再開がしやすい

### 10.2 Dealer進行アルゴリズム

1. `session.status = DEALER_TURN` を確認
2. **UseCaseがディーラーAI（4.3）とライブラリを用いて次手を決定する**
3. 次手が `HIT` の場合は1枚draw
4. その1ドロー分だけTxで保存
5. `game_sessions.version` を+1（Dealerの1ドロー = 1回の観測可能更新）
6. commit後に `ROOM_STATE_SYNC` 配信
7. 次ドロー要否を再評価
8. 次手が `STAND`、またはdealer score >= 17なら終了確定Txへ進む

### 10.3 Dealer終了トランザクション

終了時は別Txで以下を行う。

- `hole_hidden = false`
- 各プレイヤーoutcome計算
- `final_score` 確定
- `result_snapshot` 保存
- `round_logs` 保存
- `session.status = RESULT`
- `game_sessions.version = game_sessions.version + 1`

### 10.4 禁止事項

- Dealerターン全体を1つの長大トランザクションにしない
- goroutine内メモリ状態だけでDealerを進めない
- commit前にWSを送らない

---

## 11. ゲーム進行仕様

### 11.1 プレイヤーターン

- **プレイヤーターンを持つのは利用者のみ**
- 自ターン制限は15秒
- **締切までに未操作なら TIME_FORFEIT（敗北確定）**（`BLACKJACK_PLAYER_TIMEOUT_POLICY=heuristic` 時のみ自動STAND）
- `status` が `STAND` / `BUST` / `BLACKJACK` ならプレイヤーターン終了
- seat昇順や複数人スキップの概念は**本版では適用しない**

### 11.2 山札

- 1ラウンドごとに52枚1デッキ
- ラウンド開始時にシャッフル
- シャッフル済み順序を `game_sessions.deck` に保存
- 消費位置は `draw_index` で管理
- 再接続時は `deck + draw_index` で復元可能でなければならない
- ルール適用はライブラリ可。ただし、山札の保存・消費位置管理・再接続復元・Tx境界はシステム側で管理する

---

## 12. 再戦仕様

### 12.1 対象者

RESETTING開始時点でACTIVEまたはDISCONNECTEDの**人間プレイヤー**。ディーラーは対象外。

### 12.2 投票方式

- 更新系WSイベント `REMATCH_VOTE`
- HTTP fallbackは持たない

### 12.3 投票内容

- `agree = true | false`
- 1人1票
- 再投票は上書き

### 12.4 締切

- RESULT → RESETTING遷移時点から30秒
- `rematch_deadline_at` に保存

### 12.5 成立条件

- **対象者（人間プレイヤー）の全員が `agree = true` で即成立**
- **1卓1人のため、その1名の `agree = true` で成立**
- 締切時に賛成でなければ不成立
- 未投票はfalseとみなす

### 12.6 再戦確定処理

再戦確定はSYSTEMが一度だけ実行できる。排他はsession_id単位で `SELECT ... FOR UPDATE`（またはadvisory lock）。

確定アルゴリズム：
1. `session.status == RESETTING` を確認
2. 対象者一覧確定
3. 票集計（未投票はfalse）
4. 全員trueなら「成立」、それ以外「不成立」
5. 成立/不成立を単一TXで確定

一度確定後の `REMATCH_VOTE` は `invalid_game_state`。

### 12.7 競合ケースの扱い

**ケース1：締切直前投票と締切ジョブ競合**
- lockを取った側のみ確定可能
- 後続処理は `session.status` 確認でno-op

**ケース2：再戦成立直後にleave**
- 再戦成立後、次ラウンド開始前ならleave可
- leaveにより有効参加者が1名以下になった場合、`room.status` をWAITINGに再計算

**ケース3：hostがDISCONNECTED**
- host権限は保持
- ただし再戦後の開始操作はhost接続有無とは別に、認証済みhostの操作でのみ許可
- hostがLEFTなら最小seatの有効参加者へ委譲

**ケース4：RESETTING中のjoin**
- 禁止
- `invalid_game_state`

---

## 13. 切断・再接続仕様

### 13.1 切断時

- WS切断検知で `room_players.status = DISCONNECTED`
- 席は維持
- 自ターン中は締切まで待つ
- 締切まで未操作なら `TIME_FORFEIT`（または heuristicモード時は自動STAND）
- `action_logs` に `actor_type = SYSTEM`、`target_user_id = 対象ユーザー` を保存

### 13.2 再接続時

- 参加者であれば接続可
- 新接続確立時、旧接続が存在すれば旧接続をclose
- 新接続直後に `ROOM_STATE_SYNC` を1回送信
- 内容はDB正本から再構築したstate
- 再接続ではversionを増やさない

### 13.3 多重接続の扱い

- 同一 `room_id + user_id` に対して最新接続のみ有効
- 接続管理レジストリは各インスタンスのメモリに持つ
- **接続エポックキー**：
  - `ws:room:{roomId}:user:{userId}:epoch_counter`
  - `ws:room:{roomId}:user:{userId}:latest_epoch`
- TTLは `WS_CONNECTION_EPOCH_TTL`（既定2分）で制御する
- 古い接続から届いた更新系イベントは `forbidden` またはclose
- `connection_epoch` は補助。**最終判断はDB + version + action_id**

### 13.4 Rolling Update中の扱い

- WebSocketサーバーのrolling update中、既存接続の切断は許容する
- クライアントは接続closeを検知した場合、自動的に再接続を行う
- 再接続後は以下のいずれかにより状態を復元する：
  - `GET /api/rooms/{id}`
  - WebSocket初回接続時の `ROOM_STATE_SYNC`
- フロントエンドのVercelデプロイはWebSocket接続維持責務を持たない
- 接続断は異常ではなく、DB正本から復元可能であることを前提とする

---

## 14. WebSocket仕様

### 14.1 接続

```
GET /ws/rooms/{roomId}
```

条件：
- **JWT Bearer トークン認証**（Cookie Sessionではなく、接続後の `AUTH` イベントで認証を完了する）
- 参加者のみ接続可能
- 多重接続時は最新のみ有効

**AUTH イベント（接続後の初回必須イベント）**：

クライアントは接続確立後、最初に `AUTH` イベントを送信しなければならない。

```json
{
  "type": "AUTH",
  "payload": {
    "access_token": "<JWT>"
  }
}
```

未送信または不正トークン時はサーバーが接続を終了する。

本システムのWebSocketはGoバックエンドにより提供される常駐サーバーで処理される。Vercelは接続を終端しない。

**本番構成**：
- フロントエンド（React等）はVercelにデプロイする
- HTTP APIおよびWebSocket（長時間接続）を処理するGoバックエンドは、Vercel Functions上では常駐できないため、別の常駐実行環境（コンテナ常駐ホスト、対応するPaaS、自前VM等）にデプロイする
- クライアントのWebSocket接続先URLは、上記GoバックエンドのオリジンへGETリクエストを送る（VercelのオリジンだけではWSを完結させない）

### 14.2 Client → Server

- `AUTH`（接続後初回必須。認証完了後以下が受理される）
- `HIT`
- `STAND`
- `ROOM_SYNC_REQUEST`
- `PING`
- `REMATCH_VOTE`

### 14.3 Server → Client

- `ROOM_STATE_SYNC`
- `ERROR`
- `PONG`

### 14.4 共通ルール

原則：`game_sessions.version` の扱いは**読み取り系（同期）**と**更新系**で返却形が異なる。

#### 14.4.1 読み取り系・同期系

| トリガ | 典型 | クライアントが送るversion | versionがサーバー正本と不一致のとき |
|--------|------|--------------------------|--------------------------------------|
| WS | `ROOM_SYNC_REQUEST`、再接続直後の初回同期 | 任意（不明でも可） | `ERROR` にしない。`ROOM_STATE_SYNC` のみで最新 `state.session.version` を返す |
| HTTP | `GET /api/rooms/{id}` | クエリで送らない | レスポンスに現在の `session.version` を含める |

#### 14.4.2 更新系

- `HIT` / `STAND` / `REMATCH_VOTE`
- 既存sessionに対する更新 → `session.version` 必須
- 不一致は `version_conflict`

**クライアント挙動**：
1. version破棄
2. 正本取得
3. 再送

### 14.5 state schemaルール

- `players` は全員含む
- 自分の `hand` のみ返す
- 他人は `card_count`
- dealerはhidden管理
- 行動可否は `my_actions`
- resultは確定後のみ

#### 14.5.1 ROOM_STATE_SYNC.state

```json
{
  "room": {
    "id": "uuid",
    "status": "WAITING|READY|PLAYING"
  },
  "session": {
    "id": "uuid|null",
    "status": "DEALING|PLAYER_TURN|DEALER_TURN|RESULT|RESETTING|null",
    "version": 12,
    "round_no": 3,
    "turn_seat": 1,
    "turn_deadline_at": "2026-01-01T00:00:00Z|null",
    "rematch_deadline_at": "2026-01-01T00:00:30Z|null"
  },
  "dealer": {
    "visible_cards": ["..."],
    "hidden": true,
    "card_count": 2
  },
  "players": [
    {
      "user_id": "uuid",
      "seat_no": 1,
      "status": "ACTIVE|STAND|BUST|BLACKJACK|DISCONNECTED|LEFT",
      "is_me": true,
      "hand": ["..."],
      "card_count": 2,
      "outcome": "WIN|LOSE|PUSH|BUST|null",
      "final_score": 20
    }
  ],
  "my_actions": {
    "can_hit": true,
    "can_stand": true,
    "can_rematch_vote": false
  }
}
```

注記：`players` の人間エントリは1件が標準。`turn_seat` は1固定の例でよい。

---

## 15. HTTP API仕様

### 15.1 一覧

```
POST   /api/auth/signup
POST   /api/auth/login
POST   /api/auth/logout
GET    /api/me
GET    /api/rooms
POST   /api/rooms
POST   /api/rooms/{id}/join
POST   /api/rooms/{id}/leave
GET    /api/rooms/{id}
POST   /api/rooms/{id}/start
POST   /api/rooms/{id}/hit
POST   /api/rooms/{id}/stand
GET    /api/rooms/{id}/history
GET    /api/rooms/{id}/play_hint
```

> **注記**：`POST /api/rooms/{id}/join` は、既に人間プレイヤーが存在する場合 `room_full` を返す。

**`GET /api/rooms/{id}/play_hint`**：参加者のみ。プレイヤーターンかつ HIT 可能なとき、§4.4 の推奨（HIT/STAND）と `session_version` を返す。更新系ではない。

レスポンス例（成功）：

```json
{
  "success": true,
  "data": {
    "recommendation": "HIT",
    "session_version": 12,
    "rationale": "（実装に依存する説明テキスト）"
  }
}
```

失敗例（代表）：
- プレイヤーターンでない / HIT 不可：`invalid_game_state`
- 非参加者：`forbidden`

### 15.2 共通レスポンス

**成功**：

```json
{
  "success": true,
  "data": {}
}
```

**失敗**：

```json
{
  "success": false,
  "error": {
    "code": "invalid_input",
    "message": "..."
  }
}
```

### 15.3 Debug API

```
POST /api/rooms/{id}/reset
```

- `BLACKJACK_DEBUG_ROOM_RESET=true` の場合のみ有効
- 開発専用・本番では無効

---

## 16. 認証・セッション・セキュリティ

### 16.1 認証方式

本版実装の認証方式は **JWT Bearer トークン**を正とする。

- `POST /api/auth/signup` および `POST /api/auth/login` は `data.access_token` を返却する
- 保護APIは `Authorization: Bearer <token>` を必須とする
- WebSocket接続 `GET /ws/rooms/{roomId}` 後、クライアントは最初に `AUTH` イベント（`access_token` 含む）を送信しなければならない
- 未送信または不正トークン時は接続を終了する
- `POST /api/auth/logout` はトークン無効化ストアを持たない構成とし、**クライアント側トークン破棄を基本**とする

### 16.2 CSRF

CSRF検証はCookie認証運用時に適用する。**Bearer認証リクエストではCSRF検証を適用しない**。

### 16.3 ログイン

- bcryptでハッシュ
- login成功時にJWT発行
- session fixation防止（JWTのため本質的に不要だが、実装上のセキュリティガードを維持する）

### 16.4 ログアウト

- クライアント側トークン破棄を基本とする
- サーバー側無効化ストアを持たない構成とする

### 16.5 認可

- room参加者のみ閲覧可
- startはhostのみ
- joinはWAITING / READYのみ
- leaveは参加者のみ
- historyは参加者のみ

### 16.6 レート制限

- Token Bucket
- capacity = 20
- refill = 5/sec
- Redis使用
- HTTP / WS両方適用

---

## 17. 整合性制御

### 17.1 三層防衛

1. action_id（冪等）
2. 状態チェック
3. version（競合制御）

### 17.2 処理順

1. 認証
2. レート制限
3. 読み取り
4. action_id確認
5. 状態チェック
6. 排他取得
7. 更新Tx
8. action_logs保存
9. commit
10. WS通知

### 17.3 排他ポリシー

- session単位で排他
- `SELECT FOR UPDATE` 推奨
- advisory lock可
- version check必須

### 17.4 フェーズとの関係

三層防衛（冪等・状態チェック・version）は `game_sessions` 存在後の全更新系に適用する。

Phase1で以下を保証する：HIT / STAND / Dealer / Result、冪等、version、排他制御、再接続復元

---

## 18. CI/CD仕様

### 18.1 ブランチ戦略

| ブランチ | 用途 |
|----------|------|
| main | 本番 |
| develop | 開発 |
| feature/ | 機能 |
| hotfix/ | 緊急 |

ルール：main直push禁止 / PR必須 / review必須

### 18.2 環境

| 環境 | 用途 |
|------|------|
| local | 開発 |
| ci | テスト |
| production | 本番 |

stagingデプロイ段は持たず、production直行構成とする。

### 18.3 CI（PR時）

必須：
1. frontend lint
2. frontend test
3. frontend build
4. backend vet
5. backend test
6. backend build
7. integration test
8. container build
9. security scan

### 18.4 Integration Test

- PostgreSQL / Redis起動
- room作成 / join / start / hit / stand / dealer / result / rematch / reconnect / duplicate_action / version_conflict

### 18.5 Security Scan

- gosec
- npm audit
- trivy
- gitleaks

重大NG → deploy禁止

### 18.6 CD

正本ワークフローは `.github/workflows/cicd.yml` とする。

ジョブ構成：
- `frontend-ci`
- `backend-ci`
- `integration-test`
- `container-build`
- `security-scan`
- `publish-images`
- `smoke-test`
- `k6-load`
- `deploy-production`

production条件：CI全ジョブ成功 + security NGなし + manual approval

### 18.7 Smoke Test

- `/health` 200
- `/api/me`
- room作成
- WS接続
- state一致

### 18.8 migration

- 自動実行禁止
- pipelineで実行
- backward compatible

### 18.9 migration失敗

- deploy中断
- 旧バージョン維持

### 18.10 rollback

- image戻す
- DB rollbackなし

### 18.11 WS + rolling update

- 切断許容
- 再接続必須
- DBから復元

### 18.12

integration = 実DB/RedisでAPI/WSテスト、security = gosec/audit/trivy/gitleaksを失敗連動。

---

## 19. GitHub Actions

正本ワークフローは `.github/workflows/cicd.yml` とする。

掲載YAMLはスケルトン。integration/securityのechoはプレースホルダでRequired不可。checkout・services等は正本workflowで補完する。正本workflowにはVercelデプロイジョブを含める。stagingデプロイ段は持たず、production直行構成とする。

---

## 20. 監視・観測性

フロントエンドの監視はVercel Observabilityを利用する。WebSocketおよびAPIサーバーの監視は、各バックエンド基盤のログおよびメトリクス基盤を利用する。

### 20.1 必須ログ項目

`timestamp` / `level` / `request_id` / `action_id` / `room_id` / `session_id` / `user_id` / `actor_type` / `request_type` / `session_version_before` / `session_version_after` / `latency_ms` / `result` / `error_code`

### 20.2 メトリクス

HTTP request count / HTTP p95 latency / WS send latency p95 / active ws connections / reconnect count / version_conflict count / duplicate_action count / auto_stand count / dealer_draw_count / room_count / session_count / deploy_success_count / deploy_failure_count

### 20.3 アラート条件

- 5xx急増
- WS reconnect急増
- DB接続枯渇
- migration failure
- deploy failure
- health check failure
- version_conflict異常増加

### 20.4 監査ログ

以下は追跡可能にする：start / hit / stand / auto-stand / rematch vote / host移譲 / room leave / reconnect epoch更新

`play_hint` は読み取り専用のため、version の増分は伴わない（HTTP アクセスログまたは任意の監査方針に従う）。

---

## 21. テスト要件

### 21.1 Domain

- 2枚A + 10はBlackjack
- 3枚7 + 7 + 7は通常21
- 22以上はBust
- Dealer A + 6はSoft17でStand
- 勝敗優先順位が仕様どおり
- ライブラリ委譲時は、**Adapter正規化後のoutcomeが6.2の列挙どおり**であることを検証する
- 表駆動テスト必須
- 決定性保証（同一入力→同一出力）

### 21.2 UseCase

- HIT/STANDの正常系・異常系
- version_conflict / duplicate_action
- timeout → TIME_FORFEIT（またはheuristicモード時は自動STAND）
- **1対1のため必ずDEALER_TURNに遷移**

### 21.3 E2E

**Phase1〜2**
- 1利用者接続でプレイ完了
- 再接続復元
- 多重接続制御
- Dealer伏せ札非公開
- rolling update耐性
- deploy後正常

**Phase3**
- Redis Pub/Sub配信
- 複数インスタンス同期

### 21.4 負荷試験

- `k6/scenarios/spec_api.js` を正本とし、100 VU / 5分を基準とする
- 現行シナリオはAPI中心（`/health`、`auth`、`me`）であり、WS再接続混在負荷は将来拡張項目とする

### 21.5 品質ゲート

- PR: unit + build + integration必須
- main: smoke test必須
- production: security NGなし
- 完成版 = `.github/workflows/cicd.yml` が18.3・18.12を実コマンドで満たすこと

### 21.6 受け入れ条件

- versionは `game_sessions.version` のみ
- 再戦はREADYを経由しない
- state schema完全一致
- statusが単一状態
- 三層防衛がPhase1で実装済
- multi-instanceはPhase3

---

## 22. 環境変数契約

| 変数名 | 説明 |
|--------|------|
| `DATABASE_URL` | PostgreSQL接続文字列 |
| `JWT_SECRET` | JWT署名秘密鍵（**16文字以上必須**） |
| `PORT` | バックエンドリスニングポート |
| `REDIS_ROOM_ADDR` | ルーム管理用Redis接続先 |
| `REDIS_RATE_LIMIT_ADDR` | レート制限用Redis接続先 |
| `REDIS_ADDR` | 互換fallback（上記2つ未設定時に使用） |
| `WS_ALLOWED_ORIGINS` | WebSocket許可オリジン（CORS） |
| `WS_CONNECTION_EPOCH_TTL` | 接続エポックTTL（既定: 2分） |
| `WS_AUTH_DEADLINE` | AUTH受信タイムアウト |
| `BLACKJACK_WS_MARK_DISCONNECTED` | WS切断時にDISCONNECTEDマークを行う（`true`/`false`） |
| `BLACKJACK_PLAYER_TIMEOUT_POLICY` | タイムアウト時の動作（未設定=TIME_FORFEIT、`heuristic`=自動STAND） |
| `DB_MAX_IDLE_CONNS` | DBコネクションプール最大アイドル数 |
| `DB_MAX_OPEN_CONNS` | DBコネクションプール最大接続数 |
| `DB_CONN_MAX_LIFETIME` | DB接続最大生存時間 |
| `DB_CONN_MAX_IDLE_TIME` | DB接続最大アイドル時間 |

---

## 23. 非機能要件

| 項目 | 目標 |
|------|------|
| WS遅延 | p95 100ms以内 |
| HTTP応答 | p95 500ms以内 |
| 同時接続 | 100ユーザー |
| ルーム人数 | 卓あたり人間**最大1名** |
| 自動復元 | 即時 |
| deploy | **GitHub Actions → Vercel直接** |
| rollback | 可能 |

staging、100同接前提、HTTP/WS p95のサーバー側定義、未達時は数値変更より先にチューニング。

---

## 24. 実装優先順位

**Phase 1**：認証 / ルーム / WebSocket / start / hit / stand / dealer / result / 三層防衛 / TIME_FORFEIT（タイムアウト） / 再接続 / 1卓1人制約 / ディーラーAI + ライブラリ進行

**Phase 2**：rematch / history / 境界ケース

**Phase 3**：Redis Pub/Sub / multi-instance / CI/CD完全化 / 監視 / 負荷試験

---

## 25. 最終ルール

1. ブラックジャック計算は外部ライブラリ
2. **進行アルゴリズムはライブラリをUseCase経由で適用する**
3. 正本はPostgreSQL
4. Redisは補助
5. WSは同期専用
6. 更新系は三層防衛必須
7. Dealerは1ドロー1Tx
8. 再接続は完全復元
9. CI通過必須
10. **deployはGitHub ActionsからVercelへ直接行う（staging段なし）**
11. migration失敗は中断
12. 未定義挙動は実装しない
13. 親（ディーラー）＝コンピューター、子（プレイヤー）＝利用者 1対1
14. **ディーラーはヒューリスティックAI基本戦略を用いる**
15. **AIとブラックジャックアルゴリズムの適用はUseCaseに集約する**
16. **ディーラーのHit/Standは§4.3・§6.1を正とする**
17. **ライブラリは提案であり最終決定ではない**
18. **認証はJWT Bearerトークンを正とする**
19. **タイムアウト時のデフォルト動作はTIME_FORFEIT（敗北確定）とする**
20. **正本ワークフローは `.github/workflows/cicd.yml` とする**