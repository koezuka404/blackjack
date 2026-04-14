package model

// RoundEngine は外部ブラックジャックライブラリ経由の手札更新・勝敗解決のポート（仕様 §5.3 / §5.1）。
// ディーラー方針の最終決定は model.NextDealerAction（§4.3）を用い、ライブラリの Dealer 行動決定 API は正本にしない。
type RoundEngine interface {
	// ApplyPlayerHit は draw を手札に追加した結果を返す（ライブラリ Hand の Draw と同等の評価ルール）。
	ApplyPlayerHit(hand []StoredCard, draw StoredCard) ([]StoredCard, error)
	// ResolveOutcome は §6.2 の優先順位で勝敗を決める（HandEvaluator による評価に基づく）。
	ResolveOutcome(ev HandEvaluator, playerHand, dealerHand []StoredCard) (Outcome, error)
}
