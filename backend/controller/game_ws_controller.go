package controller

import (
	"context"
	"time"

	"blackjack/backend/dto"
	"blackjack/backend/model"
	"blackjack/backend/observability"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/gorilla/websocket"
)

func (r *RoomController) handleGameWSAction(ws *WsAuditLogContext, req dto.WSActionRequest, roomID, userID string, conn *websocket.Conn, meta wsConnMeta, msgStart time.Time) {
	switch req.Type {
	case dto.WSEventHit:

		if req.ActionID == "" || req.ExpectedVersion <= 0 {
			sendWSError(conn, meta, dto.WSErrorInvalidInput, "action_id and expected_version are required")
			return
		}
		_, sess, err := r.room.Hit(context.Background(), roomID, userID, req.ExpectedVersion, req.ActionID)
		ev := req.ExpectedVersion
		if err != nil {
			code, message := mapWSError(err)
			logWSEvent(ws, req, roomID, userID, nil, &ev, &ev, msgStart, "failure", code, nil)
			sendWSError(conn, meta, code, message)
			return
		}
		sv := sess.Version
		gid := sess.ID
		logWSEvent(ws, req, roomID, userID, &gid, &ev, &sv, msgStart, "success", "", nil)
		r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
	case dto.WSEventStand:

		if req.ActionID == "" || req.ExpectedVersion <= 0 {
			sendWSError(conn, meta, dto.WSErrorInvalidInput, "action_id and expected_version are required")
			return
		}
		_, sess, err := r.room.Stand(context.Background(), roomID, userID, req.ExpectedVersion, req.ActionID)
		ev := req.ExpectedVersion
		if err != nil {
			code, message := mapWSError(err)
			logWSEvent(ws, req, roomID, userID, nil, &ev, &ev, msgStart, "failure", code, nil)
			sendWSError(conn, meta, code, message)
			return
		}
		sv := sess.Version
		gid := sess.ID
		logWSEvent(ws, req, roomID, userID, &gid, &ev, &sv, msgStart, "success", "", nil)
		r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
	case dto.WSEventRematchVote:

		if req.ActionID == "" || req.ExpectedVersion <= 0 || req.Agree == nil {
			sendWSError(conn, meta, dto.WSErrorInvalidInput, "agree, action_id and expected_version are required")
			return
		}
		_, sess, err := r.room.VoteRematch(context.Background(), roomID, userID, *req.Agree, req.ExpectedVersion, req.ActionID)
		ev := req.ExpectedVersion
		if err != nil {
			code, message := mapWSError(err)
			logWSEvent(ws, req, roomID, userID, nil, &ev, &ev, msgStart, "failure", code, nil)
			sendWSError(conn, meta, code, message)
			return
		}
		sv := sess.Version
		gid := sess.ID
		logWSEvent(ws, req, roomID, userID, &gid, &ev, &sv, msgStart, "success", "", nil)
		r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
	case dto.WSEventRoomSyncReq:

		logWSEvent(ws, req, roomID, userID, nil, nil, nil, msgStart, "success", "", nil)
		r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
	case dto.WSEventPing:

		logWSEvent(ws, req, roomID, userID, nil, nil, nil, msgStart, "success", "", nil)
		sendWSPong(conn, meta)
	default:
		logWSEvent(ws, req, roomID, userID, nil, nil, nil, msgStart, "failure", dto.WSErrorInvalidInput, nil)
		sendWSError(conn, meta, dto.WSErrorInvalidInput, "unsupported ws event type")
	}
}


func mapWSError(err error) (string, string) {
	switch err {
	case usecase.ErrUnauthorizedUser:
		return dto.WSErrorUnauthorized, "login required"
	case usecase.ErrForbiddenAction:
		return dto.WSErrorForbidden, "room access denied"
	case usecase.ErrInvalidInput:
		return dto.WSErrorInvalidInput, "action_id and expected_version are required"
	case usecase.ErrInvalidGameState, model.ErrNotPlayerTurn, model.ErrNotYourTurn, model.ErrInvalidPlayerStatus:
		return dto.WSErrorInvalidGame, err.Error()
	case model.ErrRoomFull:
		return dto.WSErrorRoomFull, "room is full"
	case model.ErrVersionConflict:
		observability.IncVersionConflict()
		return dto.WSErrorVersionConflict, "session version conflict"
	case model.ErrDuplicateAction:
		observability.IncDuplicateAction()
		return dto.WSErrorDuplicateAction, "action id already used with different payload"
	case repository.ErrNotFound:
		return dto.WSErrorNotFound, "room or session not found"
	default:
		return dto.WSErrorInternal, err.Error()
	}
}
