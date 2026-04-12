package usecase

import (
	"context"

	"blackjack/backend/model"
	"blackjack/backend/repository"
)

// EnsureActionIdempotency は同一 action_id の再送を検知し、成功済みなら保存レスポンスを返す。
func EnsureActionIdempotency(ctx context.Context, store repository.Store, actionLog *model.ActionLog) (cachedSnapshot string, replay bool, err error) {
	if err := actionLog.Validate(); err != nil {
		return "", false, err
	}
	prev, err := store.GetActionLogByActionID(ctx, actionLog.SessionID, actionLog.ActorUserID, actionLog.ActionID)
	if err == nil {
		if prev.RequestPayloadHash == actionLog.RequestPayloadHash {
			return prev.ResponseSnapshot, true, nil
		}
		return "", false, model.ErrDuplicateAction
	}
	if err != repository.ErrNotFound {
		return "", false, err
	}
	return "", false, nil
}

// SaveActionSuccessSnapshot は更新成功後の監査・冪等応答用に action_logs を保存する。
func SaveActionSuccessSnapshot(ctx context.Context, store repository.Store, actionLog *model.ActionLog, responseSnapshot string) error {
	actionLog.ResponseSnapshot = responseSnapshot
	return store.CreateActionLog(ctx, actionLog)
}
