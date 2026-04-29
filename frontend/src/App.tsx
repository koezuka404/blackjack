import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { FormEvent, ReactElement } from 'react'
import axios, { AxiosError } from 'axios'
import './App.css'

type ApiSuccess<T> = { success: true; data: T }
type ApiFailure = { success: false; error: { code: string; message: string } }
type ApiResponse<T> = ApiSuccess<T> | ApiFailure

type LoginResponse = {
  access_token: string
  token_type: string
  expires_in: number
  user: {
    id: string
    username: string
  }
}

type RoomSummary = {
  id: string
  host_user_id: string
  status: string
}

type RoomStateSync = {
  room: { id: string; status: string }
  session: {
    id: string | null
    status: string | null
    version: number | null
    round_no: number | null
    turn_seat: number | null
    turn_deadline_at: string | null
    rematch_deadline_at: string | null
  }
  dealer: {
    visible_cards: string[]
    hidden: boolean
    card_count: number
  }
  players: Array<{
    user_id: string
    seat_no: number
    status: string
    is_me: boolean
    hand?: string[]
    card_count: number
    outcome?: string | null
    final_score?: number | null
  }>
  my_actions: {
    can_hit: boolean
    can_stand: boolean
    can_rematch_vote: boolean
  }
}

type RoomSyncEvent = {
  type: 'ROOM_STATE_SYNC'
  data: RoomStateSync
}

type WsErrorEvent = {
  type: 'ERROR'
  error: {
    code: string
    message: string
    retry_after_ms?: number
  }
}

export function isRoomSyncEvent(payload: unknown): payload is RoomSyncEvent {
  if (typeof payload !== 'object' || payload === null) return false
  const candidate = payload as { type?: string; data?: unknown }
  return candidate.type === 'ROOM_STATE_SYNC' && typeof candidate.data === 'object'
}

export function isWsErrorEvent(payload: unknown): payload is WsErrorEvent {
  if (typeof payload !== 'object' || payload === null) return false
  const candidate = payload as { type?: string; error?: { code?: unknown; message?: unknown } }
  return candidate.type === 'ERROR' && typeof candidate.error?.code === 'string' && typeof candidate.error?.message === 'string'
}

export function resolveApiBaseURL(): string {
  const raw = import.meta.env.VITE_API_BASE_URL

  if (typeof raw === 'string' && raw.trim() !== '') {
    const base = raw.trim().replace(/\/+$/, '')
    return base.endsWith('/api') ? base : `${base}/api`
  }
  if (typeof window !== 'undefined' && window.location?.origin) {
    return `${window.location.origin}/api`
  }

  return 'http://localhost:8080/api'

}

export function resolveWsBaseURL(): string {
  const raw = import.meta.env.VITE_WS_BASE_URL

  if (typeof raw === 'string' && raw.trim() !== '') {
    const base = raw.trim().replace(/\/+$/, '')
    return base.endsWith('/ws') ? base : `${base}/ws`
  }
  if (typeof window !== 'undefined' && window.location) {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${window.location.host}/ws`
  }

  return 'ws://localhost:8080/ws'

}

const API_BASE_URL = resolveApiBaseURL()
const WS_BASE_URL = resolveWsBaseURL()
const TOKEN_STORAGE_KEY = 'blackjack.access_token'

export function randomID(prefix: string): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `${prefix}-${crypto.randomUUID()}`
  }
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 100000)}`
}

export function parseAPIError(error: unknown): string {
  if (axios.isAxiosError(error)) {
    const axiosError = error as AxiosError<ApiFailure>
    const payload = axiosError.response?.data
    if (payload?.error?.code) {
      switch (payload.error.code) {
        case 'username_taken':
          return 'このユーザー名は既に使われています'
        case 'invalid_input':
          return '入力内容が正しくありません'
        case 'unauthorized':
          return ''
        case 'forbidden':
          return 'この操作は許可されていません'
        case 'invalid_game_state':
          return '現在の状態ではこの操作はできません'
        case 'room_full':
          return 'ルームが満員です'
        case 'not_found':
          return '対象データが見つかりません'
        case 'rate_limited':
          return 'アクセスが多すぎます。少し待ってください'
        case 'internal_error':
          return 'サーバーエラーが発生しました'
        default:
          return payload.error.message || 'エラーが発生しました'
      }
    }
    if (axiosError.message) {
      return '通信エラーが発生しました'
    }
  }
  if (error instanceof Error) {
    return error.message || 'エラーが発生しました'
  }
  return '不明なエラーが発生しました'
}

export function unwrapData<T>(payload: ApiResponse<T>): T {
  if (!payload.success) {
    throw new Error(`${payload.error.code}: ${payload.error.message}`)
  }
  return payload.data
}

type CardFace = {
  rank: string
  suit: 'S' | 'H' | 'D' | 'C'
}

const SUIT_SYMBOL: Record<CardFace['suit'], string> = {
  S: '♠',
  H: '♥',
  D: '♦',
  C: '♣',
}

export function parseCardFace(raw: string): CardFace | null {
  const text = raw.trim().toUpperCase()
  if (text === '') return null

  const normalized = text
    .replace(/SPADES?|SPADE/g, 'S')
    .replace(/HEARTS?|HEART/g, 'H')
    .replace(/DIAMONDS?|DIAMOND/g, 'D')
    .replace(/CLUBS?|CLUB/g, 'C')
    .replace(/[♠]/g, 'S')
    .replace(/[♥]/g, 'H')
    .replace(/[♦]/g, 'D')
    .replace(/[♣]/g, 'C')
    .replace(/[^0-9JQKASHDC]/g, '')

  const m = normalized.match(/(10|[2-9JQKA])([SHDC])$/)
  if (!m) return null
  const rank = m[1]
  const suit = m[2] as CardFace['suit']
  return { rank, suit }
}

export function renderPlayingCards(cards?: string[], hiddenCount = 0): ReactElement {
  const safeCards = cards ?? []
  if (safeCards.length === 0 && hiddenCount <= 0) {
    return <div className="hand-cards-empty">--</div>
  }
  return (
    <div className="playing-cards">
      {safeCards.map((raw, idx) => {
        const parsed = parseCardFace(raw)
        if (!parsed) {
          return (
            <div key={`raw-${idx}-${raw}`} className="playing-card card-back" aria-label="unknown-card">
              <span className="card-center">?</span>
            </div>
          )
        }
        const suitSymbol = SUIT_SYMBOL[parsed.suit]
        const red = parsed.suit === 'H' || parsed.suit === 'D'
        return (
          <div key={`face-${idx}-${raw}`} className={`playing-card ${red ? 'card-red' : 'card-black'}`} aria-label={raw}>
            <span className="card-corner card-corner-top">
              {parsed.rank}
              {suitSymbol}
            </span>
            <span className="card-center">{suitSymbol}</span>
            <span className="card-corner card-corner-bottom">
              {parsed.rank}
              {suitSymbol}
            </span>
          </div>
        )
      })}
      {Array.from({ length: hiddenCount }).map((_, idx) => (
        <div key={`hidden-${idx}`} className="playing-card card-back" aria-label="hidden-card">
          <span className="card-center">🂠</span>
        </div>
      ))}
    </div>
  )
}

export function rankToPoint(rank: string): number {
  if (rank === 'A') return 11
  if (rank === 'K' || rank === 'Q' || rank === 'J' || rank === '10') return 10
  const n = Number.parseInt(rank, 10)
  return Number.isNaN(n) ? 0 : n
}

export function handScore(cards?: string[]): number | null {
  if (!cards || cards.length === 0) return null
  const faces = cards.map(parseCardFace).filter((v): v is CardFace => v !== null)
  if (faces.length === 0) return null

  let total = 0
  let aces = 0
  for (const c of faces) {
    total += rankToPoint(c.rank)
    if (c.rank === 'A') aces++
  }
  while (total > 21 && aces > 0) {
    total -= 10
    aces--
  }
  return total
}

export function outcomeToLabel(outcome?: string | null): { text: string; tone: 'win' | 'lose' | 'draw' | 'idle' } {
  const key = (outcome ?? '').trim().toUpperCase()
  switch (key) {
    case 'WIN':
    case 'BLACKJACK':
      return { text: 'あなたの勝ち', tone: 'win' }
    case 'LOSE':
    case 'BUST':
      return { text: 'あなたの負け', tone: 'lose' }
    case 'PUSH':
    case 'DRAW':
      return { text: '引き分け', tone: 'draw' }
    default:
      return { text: '', tone: 'idle' }
  }
}


export function formatPlayerTurnCountdown(remainingMs: number): { text: string; urgent: boolean; isOver: boolean } {
  if (remainingMs <= 0) {
    return { text: '時間切れ', urgent: true, isOver: true }
  }
  const sec = Math.max(1, Math.ceil(remainingMs / 1000))
  const urgent = sec <= 5
  const mm = Math.floor(sec / 60)
  const ss = sec % 60
  if (mm > 0) {
    return { text: `${mm}:${String(ss).padStart(2, '0')}`, urgent, isOver: false }
  }
  return { text: `${sec}秒`, urgent, isOver: false }
}

function App() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [token, setToken] = useState(localStorage.getItem(TOKEN_STORAGE_KEY) ?? '')
  const [roomID, setRoomID] = useState('')
  const [statusMessage, setStatusMessage] = useState('')
  const [authMode, setAuthMode] = useState<'login' | 'signup'>('login')
  const [rooms, setRooms] = useState<RoomSummary[]>([])
  const [roomState, setRoomState] = useState<RoomStateSync | null>(null)
  const [historyJSON, setHistoryJSON] = useState('')
  const [wsLog, setWsLog] = useState<string[]>([])
  const [wsConnectionState, setWsConnectionState] = useState<'disconnected' | 'connecting' | 'connected'>('disconnected')
  const [isJoiningRoom, setIsJoiningRoom] = useState(false)
  const [isStartingGame, setIsStartingGame] = useState(false)
  const [isInRoom, setIsInRoom] = useState(false)
  const [hasStartedCurrentRoom, setHasStartedCurrentRoom] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)

  const maintainWsConnectionRef = useRef(false)

  const silentWsReplaceCloseRef = useRef(false)
  const wsReconnectAttemptRef = useRef(0)
  const wsReconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const wsReconnectDeadlineRef = useRef<number | null>(null)
  const wsReconnectDisplayIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const [wsReconnectRemainingSec, setWsReconnectRemainingSec] = useState(0)

  const [wsReconnectBannerVisible, setWsReconnectBannerVisible] = useState(false)
  const [turnClockNowMs, setTurnClockNowMs] = useState(() => Date.now())
  const tokenRef = useRef(token)
  const roomIDRef = useRef(roomID)
  const isInRoomRef = useRef(isInRoom)
  useEffect(() => {
    tokenRef.current = token
    roomIDRef.current = roomID
    isInRoomRef.current = isInRoom
  }, [token, roomID, isInRoom])

  const clearReconnectCountdown = useCallback(() => {

    wsReconnectDeadlineRef.current = null
    if (wsReconnectDisplayIntervalRef.current) {
      clearInterval(wsReconnectDisplayIntervalRef.current)
      wsReconnectDisplayIntervalRef.current = null
    }
    setWsReconnectRemainingSec(0)
    setWsReconnectBannerVisible(false)

  }, [])

  const clearAuthState = useCallback(() => {
    setToken('')
    localStorage.removeItem(TOKEN_STORAGE_KEY)
    setUsername('')
    setPassword('')
    setRoomID('')
    setRooms([])
    setRoomState(null)
    maintainWsConnectionRef.current = false
    isInRoomRef.current = false
    setIsInRoom(false)
    setHasStartedCurrentRoom(false)
    setHistoryJSON('')
    setWsLog([])
    wsReconnectAttemptRef.current = 0
    if (wsReconnectTimerRef.current) {
      clearTimeout(wsReconnectTimerRef.current)
      wsReconnectTimerRef.current = null
    }
    clearReconnectCountdown()
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }
    setWsConnectionState('disconnected')
  }, [clearReconnectCountdown])

  const authClient = useMemo(() => {
    const client = axios.create({
      baseURL: API_BASE_URL,
      headers: {
        'Content-Type': 'application/json',
      },
    })
    client.interceptors.request.use((config) => {

      if (token) {
        config.headers.Authorization = `Bearer ${token}`
      }
      return config
    })
    return client
  }, [token])

  useEffect(() => {
    const responseInterceptors = authClient.interceptors?.response
    if (!responseInterceptors || typeof responseInterceptors.use !== 'function') {
      return
    }
    const interceptorID = responseInterceptors.use(
      (response) => response,
      (error) => {
        if (axios.isAxiosError(error) && error.response?.status === 401) {
          clearAuthState()
          setStatusMessage('セッションが切れました。再ログインしてください')
        }
        return Promise.reject(error)
      },
    )
    return () => {
      if (typeof responseInterceptors.eject === 'function') {
        responseInterceptors.eject(interceptorID)
      }
    }
  }, [authClient, clearAuthState])

  const appendWSLog = (message: string) => {
    setWsLog((prev) => [`${new Date().toISOString()} ${message}`, ...prev].slice(0, 60))
  }

  const startReconnectCountdown = (delayMs: number) => {

    if (wsReconnectDisplayIntervalRef.current) {
      clearInterval(wsReconnectDisplayIntervalRef.current)
      wsReconnectDisplayIntervalRef.current = null
    }
    wsReconnectDeadlineRef.current = Date.now() + delayMs
    setWsReconnectBannerVisible(true)
    const tick = () => {
      const d = wsReconnectDeadlineRef.current
      if (d === null) {
        return
      }
      const msLeft = d - Date.now()
      const left = Math.max(0, Math.ceil(msLeft / 1000))
      setWsReconnectRemainingSec(left > 0 ? left : 0)
      if (msLeft <= 0) {
        if (wsReconnectDisplayIntervalRef.current) {
          clearInterval(wsReconnectDisplayIntervalRef.current)
          wsReconnectDisplayIntervalRef.current = null
        }
        return
      }
    }
    tick()
    wsReconnectDisplayIntervalRef.current = window.setInterval(tick, 250)

  }

  const saveToken = (nextToken: string) => {
    setToken(nextToken)
    localStorage.setItem(TOKEN_STORAGE_KEY, nextToken)
  }

  const withStatus = async (label: string, task: () => Promise<void>) => {
    try {
      setStatusMessage(`${label}中...`)
      await task()
      setStatusMessage('')
    } catch (error) {
      const message = parseAPIError(error).trim()
      setStatusMessage(message)
    }
  }

  const signup = async () => {
    await withStatus('新規登録', async () => {
      const response = await authClient.post<ApiResponse<LoginResponse>>('/auth/signup', {
        username,
        password,
      })
      saveToken(unwrapData(response.data).access_token)
    })
  }

  const login = async () => {
    await withStatus('ログイン', async () => {
      const response = await authClient.post<ApiResponse<LoginResponse>>('/auth/login', {
        username,
        password,
      })

      saveToken(unwrapData(response.data).access_token)
    })
  }

  const logout = async () => {
    await withStatus('ログアウト', async () => {
      await authClient.post('/auth/logout')
      clearAuthState()
    })
  }

  const listRooms = async () => {
    await withStatus('ルーム一覧取得', async () => {
      const response = await authClient.get<ApiResponse<{ rooms: RoomSummary[] }>>('/rooms')
      setRooms(unwrapData(response.data).rooms)
    })
  }

  const createRoom = async () => {
    await withStatus('ルーム作成', async () => {
      const response = await authClient.post<ApiResponse<{ room: RoomSummary }>>('/rooms', {})
      const data = unwrapData(response.data)
      const nextRoomID = data.room.id
      setRoomID(nextRoomID)
      setRooms((prev) => [data.room, ...prev.filter((room) => room.id !== nextRoomID)])
    })
  }

  const joinRoom = async () => {

    if (!token) {

      setStatusMessage('先にログインしてください')
      return
    }

    const rid = roomID.trim()
    if (!rid) {
      setStatusMessage('ルームIDを入力してください')
      return
    }
    await withStatus('ルーム参加', async () => {
      const response = await authClient.post<ApiResponse<{ room: RoomSummary }>>(`/rooms/${rid}/join`, {})
      unwrapData(response.data)
      roomIDRef.current = rid
      setRoomID(rid)
      isInRoomRef.current = true
      setIsInRoom(true)
      setHasStartedCurrentRoom(false)
      maintainWsConnectionRef.current = true
      wsReconnectAttemptRef.current = 0
      connectWebSocket()
    })
  }

  const startRoom = async () => {

    if (!token) {

      setStatusMessage('先にログインしてください')
      return
    }

    const rid = roomID.trim()
    if (!rid) {
      setStatusMessage('ルームIDを入力してください')
      return
    }
    await withStatus('ゲーム開始', async () => {
      const response = await authClient.post<ApiResponse<{ session: { version: number } }>>(`/rooms/${rid}/start`, {})
      unwrapData(response.data)
      setHasStartedCurrentRoom(true)
      connectWebSocket()
      await fetchRoom()
    })
  }

  const fetchRoom = async () => {
    await withStatus('ルーム情報取得', async () => {
      const response = await authClient.get<ApiResponse<Record<string, unknown>>>(`/rooms/${roomID}`)
      setHistoryJSON(JSON.stringify(response.data, null, 2))
    })
  }

  const fetchHistory = async () => {
    await withStatus('履歴取得', async () => {
      const response = await authClient.get<ApiResponse<Record<string, unknown>>>(`/rooms/${roomID}/history`)
      setHistoryJSON(JSON.stringify(response.data, null, 2))
    })
  }

  const fetchHint = async () => {
    await withStatus('ヒント取得', async () => {
      const response = await authClient.get<ApiResponse<Record<string, unknown>>>(`/rooms/${roomID}/play_hint`)
      setHistoryJSON(JSON.stringify(response.data, null, 2))
    })
  }

  const expectedVersion = roomState?.session.version ?? 0
  const me = roomState?.players.find((p) => p.is_me)

  const hitByHTTP = async () => {
    await withStatus('ヒット', async () => {
      const response = await authClient.post(`/rooms/${roomID}/hit`, {
        action_id: randomID('http-hit'),
        expected_version: expectedVersion,
      })
      setHistoryJSON(JSON.stringify(response.data, null, 2))
    })
  }

  const standByHTTP = async () => {
    await withStatus('スタンド', async () => {
      const response = await authClient.post(`/rooms/${roomID}/stand`, {
        action_id: randomID('http-stand'),
        expected_version: expectedVersion,
      })
      setHistoryJSON(JSON.stringify(response.data, null, 2))
    })
  }

  const connectWebSocket = (authToken?: string) => {
    const resolvedRoomID = roomIDRef.current.trim()
    const nextToken = authToken ?? tokenRef.current
    if (!resolvedRoomID) {
      setStatusMessage('接続失敗: ルームIDが空です')
      return
    }

    if (!nextToken) {
      setStatusMessage('接続失敗: 先にログインしてください')
      return
    }
    clearReconnectCountdown()
    if (wsRef.current) {
      silentWsReplaceCloseRef.current = true
      wsRef.current.close()
      wsRef.current = null
    }


    if (wsReconnectTimerRef.current) {
      clearTimeout(wsReconnectTimerRef.current)
      wsReconnectTimerRef.current = null
    }

    const url = `${WS_BASE_URL}/rooms/${resolvedRoomID}`
    const socket = new WebSocket(url)
    wsRef.current = socket
    setWsConnectionState('connecting')
    appendWSLog(`WS open request -> ${url}`)

    socket.onopen = () => {
      wsReconnectAttemptRef.current = 0
      setWsConnectionState('connected')
      appendWSLog('WS接続完了')
      socket.send(
        JSON.stringify({
          type: 'AUTH',
          request_id: randomID('auth'),
          access_token: nextToken,
        }),
      )
      appendWSLog('認証メッセージ送信')

      setTimeout(() => {
        if (socket.readyState === WebSocket.OPEN) {
          socket.send(
            JSON.stringify({
              type: 'ROOM_SYNC_REQUEST',
              request_id: randomID('auto-sync'),
            }),
          )
          appendWSLog('初期同期リクエスト送信')
        }
      }, 120)
    }

    socket.onmessage = (event) => {
      const text = String(event.data)
      appendWSLog(`<= ${text}`)
      try {
        const parsed = JSON.parse(text) as unknown
        if (isRoomSyncEvent(parsed)) {
          setRoomState(parsed.data)
        } else if (isWsErrorEvent(parsed)) {
          appendWSLog(`WSエラー応答: ${parsed.error.code}`)
        }
      } catch {
        appendWSLog('WSメッセージのJSON解析に失敗')
      }
    }

    socket.onerror = () => {
      appendWSLog('WSエラー')
    }

    socket.onclose = () => {

      if (silentWsReplaceCloseRef.current) {
        silentWsReplaceCloseRef.current = false
        return
      }
      setWsConnectionState('disconnected')
      appendWSLog('WS切断')

      if (!maintainWsConnectionRef.current || !isInRoomRef.current) {
        return
      }
      const nextRoom = roomIDRef.current.trim()
      if (!nextRoom || !tokenRef.current) {
        return
      }
      const attempt = wsReconnectAttemptRef.current
      wsReconnectAttemptRef.current = attempt + 1
      const delayMs = Math.min(30_000, 800 * 2 ** Math.min(attempt, 6))
      appendWSLog(`WS自動再接続を ${delayMs}ms 後に試行 (試行 ${attempt + 1})`)
      startReconnectCountdown(delayMs)
      wsReconnectTimerRef.current = setTimeout(() => {
        wsReconnectTimerRef.current = null
        if (!maintainWsConnectionRef.current || !isInRoomRef.current) {
          return
        }
        if (!roomIDRef.current.trim() || !tokenRef.current) {
          return
        }
        connectWebSocket(tokenRef.current)
      }, delayMs)

    }
  }

  useEffect(() => {
    return () => {

      if (wsReconnectTimerRef.current) {
        clearTimeout(wsReconnectTimerRef.current)
        wsReconnectTimerRef.current = null
      }

      clearReconnectCountdown()
    }
  }, [clearReconnectCountdown])

  useEffect(() => {
    if (wsConnectionState !== 'connected') {
      return
    }

    const intervalId = window.setInterval(() => {
      if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
        return
      }
      const body = JSON.stringify({
        type: 'PING',
        request_id: randomID('ka-ping'),
      })
      wsRef.current.send(body)
    }, 25_000)
    return () => {
      window.clearInterval(intervalId)
    }

  }, [wsConnectionState])

  const turnDeadlineRaw = roomState?.session.turn_deadline_at
  const turnDeadlineMs = turnDeadlineRaw ? Date.parse(turnDeadlineRaw) : Number.NaN
  const showTurnTimer =
    Boolean(roomState?.session.id) &&
    roomState?.session.status === 'PLAYER_TURN' &&
    Number.isFinite(turnDeadlineMs) &&
    Boolean(roomState?.my_actions.can_hit || roomState?.my_actions.can_stand)
  const rematchDeadlineRaw = roomState?.session.rematch_deadline_at
  const rematchDeadlineMs = rematchDeadlineRaw ? Date.parse(rematchDeadlineRaw) : Number.NaN
  const showRematchAutoEndTimer =
    Boolean(roomState?.session.id) && roomState?.session.status === 'RESETTING' && Number.isFinite(rematchDeadlineMs)

  useEffect(() => {
    if (!showTurnTimer && !showRematchAutoEndTimer) {
      return
    }

    const intervalId = window.setInterval(() => {
      setTurnClockNowMs(Date.now())
    }, 250)
    return () => {
      window.clearInterval(intervalId)
    }

  }, [showTurnTimer, showRematchAutoEndTimer, turnDeadlineRaw, rematchDeadlineRaw])

  const turnCountdownView = showTurnTimer
    ? formatPlayerTurnCountdown(turnDeadlineMs - turnClockNowMs)
    : { text: '', urgent: false, isOver: false }
  const rematchAutoEndSec = Math.max(0, Math.ceil((rematchDeadlineMs - turnClockNowMs) / 1000))

  const sendWSMessage = (payload: Record<string, unknown>) => {

    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      return
    }

    const body = JSON.stringify(payload)
    wsRef.current.send(body)
    appendWSLog(`=> ${body}`)
  }

  const joinRoomFlow = async () => {

    if (!token) {

      setStatusMessage('先にログインしてください')
      return
    }


    setHasStartedCurrentRoom(false)
    setIsJoiningRoom(true)
    await withStatus('ルーム参加', async () => {
      let targetRoomID = roomID.trim()
      if (!targetRoomID) {
        const createRes = await authClient.post<ApiResponse<{ room: RoomSummary }>>('/rooms', {})
        const createdRoom = unwrapData(createRes.data).room
        targetRoomID = createdRoom.id
        setRoomID(targetRoomID)
        setRooms((prev) => [
          createdRoom,

          ...prev.filter((room) => room.id !== targetRoomID),
        ])
      }
      await authClient.post<ApiResponse<{ room: RoomSummary }>>(`/rooms/${targetRoomID}/join`, {})
      roomIDRef.current = targetRoomID
      setRoomID(targetRoomID)
      isInRoomRef.current = true
      setIsInRoom(true)
      setHasStartedCurrentRoom(false)
      maintainWsConnectionRef.current = true
      wsReconnectAttemptRef.current = 0
      connectWebSocket()
    })
    setIsJoiningRoom(false)
  }

  const leaveRoomFlow = async () => {

    if (!token) {

      setStatusMessage('先にログインしてください')
      return
    }
    if (!roomID.trim()) {

      setStatusMessage('先にルームに入ってください')
      return
    }
    if (roomState?.session.id) {

      setStatusMessage('対局中はルーム退出できません')
      return
    }

    await withStatus('ルーム退出', async () => {
      await authClient.post<ApiResponse<{ room: RoomSummary }>>(`/rooms/${roomID}/leave`, {})
      maintainWsConnectionRef.current = false
      isInRoomRef.current = false
      setIsInRoom(false)
      setHasStartedCurrentRoom(false)
      setRoomState(null)
      wsReconnectAttemptRef.current = 0

      if (wsReconnectTimerRef.current) {
        clearTimeout(wsReconnectTimerRef.current)
        wsReconnectTimerRef.current = null
      }
      clearReconnectCountdown()
      if (wsRef.current) {

        wsRef.current.close()
        wsRef.current = null
      }

      setWsConnectionState('disconnected')
    })
  }

  const startGameFlow = async () => {

    if (!token) {

      setStatusMessage('先にログインしてください')
      return
    }
    if (!roomID.trim()) {

      setStatusMessage('先にルームIDを指定してルームに入ってください')
      return
    }

    setIsStartingGame(true)
    await withStatus('ゲーム開始', async () => {
      await authClient.post<ApiResponse<{ session: { version: number } }>>(`/rooms/${roomID}/start`, {})
      setHasStartedCurrentRoom(true)
      connectWebSocket()
    })
    setIsStartingGame(false)
  }

  const canPlay = roomState?.my_actions.can_hit || roomState?.my_actions.can_stand
  const isLoggedIn = token.trim() !== ''
  const dealerVisibleCount = roomState?.dealer.visible_cards.length ?? 0
  const dealerHiddenCount = Math.max(0, (roomState?.dealer.card_count ?? 0) - dealerVisibleCount)
  const outcomeView = outcomeToLabel(me?.outcome)
  const dealerScore = handScore(roomState?.dealer.visible_cards)
  const myScore = handScore(me?.hand)

  if (!isLoggedIn) {
    return (
      <main className="container auth-only">
        <h1>ブラックジャック</h1>
        {statusMessage && <p className="subtle">{statusMessage}</p>}

        <section className="panel auth-card">
          <h2>{authMode === 'login' ? 'ログイン' : '新規登録'}</h2>
          <form
            className="row auth-form"
            onSubmit={(event: FormEvent<HTMLFormElement>) => {
              event.preventDefault()
              void (authMode === 'login' ? login() : signup())
            }}
          >
            <input value={username} onChange={(event) => setUsername(event.target.value)} placeholder="ユーザー名" />
            <input value={password} onChange={(event) => setPassword(event.target.value)} placeholder="パスワード" type="password" />
            <button type="submit">{authMode === 'login' ? 'ログインする' : '新規登録する'}</button>
          </form>
          <div className="auth-switch">
            <button
              type="button"
              className="btn-auth-switch"
              onClick={() => setAuthMode((prev) => (prev === 'login' ? 'signup' : 'login'))}
            >
              {authMode === 'login' ? '新規登録' : 'ログイン'}
            </button>
          </div>
        </section>

      </main>
    )
  }

  return (
    <main className="container">
      <div className="header-row">
        <h1>ブラックジャック</h1>
        <div className="top-actions">
          <button type="button" onClick={logout} className="btn-logout-standalone">
            ログアウト
          </button>
        </div>
      </div>
      {statusMessage && <p className="subtle">{statusMessage}</p>}
      {wsReconnectBannerVisible && (
        <p className="ws-reconnect-countdown" role="status" aria-live="polite">
          WebSocketが切断されました。自動再接続まで
          {` あと ${wsReconnectRemainingSec} 秒`}
        </p>
      )}

      <section className="layout-main">
        <div className="left-pane">
          <section className="panel panel-table">
        <div className="game-notice-top" aria-live="polite">
          {showRematchAutoEndTimer && (
          <div className={`rematch-auto-end-timer ${rematchAutoEndSec <= 5 ? 'rematch-auto-end-timer-urgent' : ''}`}>
            あと {rematchAutoEndSec} 秒で自動的にこのゲームを終了します
            </div>
          )}
          {outcomeView.tone !== 'idle' && <div className={`result-banner result-${outcomeView.tone}`}>{outcomeView.text}</div>}
        </div>
        <div className="row start-row">
          <button onClick={joinRoomFlow} className="btn-join-room" disabled={isJoiningRoom || isInRoom}>
            {isJoiningRoom ? '参加中…' : 'AIと対戦する'}
          </button>
          <button
            onClick={startGameFlow}
            className="btn-start-game"
            disabled={isStartingGame || hasStartedCurrentRoom || !roomID.trim()}
          >
            {isStartingGame ? '開始中…' : hasStartedCurrentRoom ? '開始済み' : 'ゲーム開始'}
          </button>
          <button onClick={leaveRoomFlow} className="btn-leave-room" disabled={!isInRoom || Boolean(roomState?.session.id)}>
            ルーム退出
          </button>
          <button onClick={() => connectWebSocket()} disabled={!roomID || !token}>
            再接続
          </button>
        </div>
        <div className="game-notice-turn" aria-live="polite">
          {showTurnTimer && (
            <div className={`turn-timer ${turnCountdownView.isOver ? 'turn-timer-over' : turnCountdownView.urgent ? 'turn-timer-urgent' : ''}`}>
              <strong>行動制限時間:</strong> {turnCountdownView.text}
              {turnCountdownView.isOver && <span className="turn-timer-note">（時間切れであなたの負け）</span>}
            </div>
          )}
        </div>
        <div className="table-felt">
          <div className="table-top-row">
            <div className="seat dealer-seat">
              <strong>ディーラー</strong>
              {renderPlayingCards(roomState?.dealer.visible_cards, dealerHiddenCount)}
              <div className="subtle">枚数: {roomState?.dealer.card_count ?? 0}</div>
              <div className="subtle">点数: {dealerScore ?? '-'}</div>
            </div>
          </div>
          <div className="table-bottom-row">
            <div className="seat">
              <strong>あなた</strong>
              {renderPlayingCards(me?.hand)}
              <div className="subtle">点数: {myScore ?? '-'}</div>
            </div>
          </div>
        </div>
        <div className="row action-row">
          <button
            onClick={() =>
              sendWSMessage({
                type: 'HIT',
                request_id: randomID('hit'),
                action_id: randomID('ws-hit'),
                expected_version: expectedVersion,
              })
            }
            disabled={wsConnectionState !== 'connected' || !roomState?.my_actions.can_hit}
            className="btn-hit"
          >
            ヒット
          </button>
          <button
            onClick={() =>
              sendWSMessage({
                type: 'STAND',
                request_id: randomID('stand'),
                action_id: randomID('ws-stand'),
                expected_version: expectedVersion,
              })
            }
            disabled={wsConnectionState !== 'connected' || !roomState?.my_actions.can_stand}
            className="btn-stand"
          >
            スタンド
          </button>
          <button
            onClick={() =>
              sendWSMessage({
                type: 'REMATCH_VOTE',
                request_id: randomID('vote-yes'),
                action_id: randomID('vote-yes'),
                expected_version: expectedVersion,
                agree: true,
              })
            }
            disabled={wsConnectionState !== 'connected' || !roomState?.my_actions.can_rematch_vote}
            className="btn-rematch"
          >
            次のゲーム
          </button>
          <button
            onClick={() =>
              sendWSMessage({
                type: 'REMATCH_VOTE',
                request_id: randomID('vote-no'),
                action_id: randomID('vote-no'),
                expected_version: expectedVersion,
                agree: false,
              })
            }
            disabled={wsConnectionState !== 'connected' || !roomState?.my_actions.can_rematch_vote}
            className="btn-rematch"
          >
            終了する
          </button>
        </div>
          </section>

          <section className="panel tech-panel">
            <h2>WebSocket</h2>
            <p className="subtle">状態: {wsConnectionState}</p>
            <div className="row">
              <button onClick={() => connectWebSocket()} disabled={!roomID || !token}>
                接続
              </button>
              <button
                onClick={() =>
                  sendWSMessage({
                    type: 'ROOM_SYNC_REQUEST',
                    request_id: randomID('sync'),
                  })
                }
                disabled={wsConnectionState !== 'connected'}
              >
                同期取得
              </button>
              <button
                onClick={() =>
                  sendWSMessage({
                    type: 'PING',
                    request_id: randomID('ping'),
                  })
                }
                disabled={wsConnectionState !== 'connected'}
              >
                Ping
              </button>
              <button
                onClick={() =>
                  sendWSMessage({
                    type: 'HIT',
                    request_id: randomID('hit'),
                    action_id: randomID('ws-hit'),
                    expected_version: expectedVersion,
                  })
                }
                disabled={wsConnectionState !== 'connected' || !canPlay}
              >
                WSヒット
              </button>
              <button
                onClick={() =>
                  sendWSMessage({
                    type: 'STAND',
                    request_id: randomID('stand'),
                    action_id: randomID('ws-stand'),
                    expected_version: expectedVersion,
                  })
                }
                disabled={wsConnectionState !== 'connected' || !canPlay}
              >
                WSスタンド
              </button>
            </div>
          </section>

          <section className="panel tech-panel">
            <h2>HTTP操作（予備）</h2>
            <div className="row">
              <button onClick={hitByHTTP} disabled={!roomID || expectedVersion <= 0}>
                ヒット
              </button>
              <button onClick={standByHTTP} disabled={!roomID || expectedVersion <= 0}>
                スタンド
              </button>
            </div>
          </section>
        </div>

        <div className="right-pane">
          <section className="panel tech-panel">
            <h2>ルーム操作</h2>
            <div className="row">
              <button onClick={joinRoomFlow}>AIと対戦する</button>
              <input value={roomID} onChange={(event) => setRoomID(event.target.value)} placeholder="ルームID" />
              <button onClick={createRoom}>作成</button>
              <button onClick={listRooms}>一覧</button>
              <button onClick={joinRoom} disabled={!roomID}>
                参加
              </button>
              <button onClick={startRoom} disabled={!roomID}>
                開始
              </button>
              <button onClick={fetchRoom} disabled={!roomID}>
                取得
              </button>
              <button onClick={fetchHistory} disabled={!roomID}>
                履歴
              </button>
              <button onClick={fetchHint} disabled={!roomID}>
                ヒント
              </button>
            </div>
            <ul className="list">
              {rooms.map((room) => (
                <li key={room.id}>
                  <button
                    onClick={() => {
                      setRoomID(room.id)
                    }}
                  >
                    使用
                  </button>{' '}
                  {room.id} ({room.status})
                </li>
              ))}
            </ul>
          </section>

        </div>
      </section>

      <section className="panel debug-panel tech-panel">
        <h2>状態スナップショット</h2>
        <pre>{roomState ? JSON.stringify(roomState, null, 2) : 'ROOM_STATE_SYNC をまだ受信していません'}</pre>
      </section>

      <section className="panel debug-panel tech-panel">
        <h2>HTTP出力</h2>
        <pre>{historyJSON || '空です'}</pre>
      </section>

      <section className="panel debug-panel tech-panel">
        <h2>WebSocketログ</h2>
        <pre>{wsLog.length > 0 ? wsLog.join('\n') : '空です'}</pre>
      </section>
    </main>
  )
}

export default App
