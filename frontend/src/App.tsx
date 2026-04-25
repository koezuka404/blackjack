import { useMemo, useRef, useState } from 'react'
import type { FormEvent } from 'react'
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

function isRoomSyncEvent(payload: unknown): payload is RoomSyncEvent {
  if (typeof payload !== 'object' || payload === null) return false
  const candidate = payload as { type?: string; data?: unknown }
  return candidate.type === 'ROOM_STATE_SYNC' && typeof candidate.data === 'object'
}

function isWsErrorEvent(payload: unknown): payload is WsErrorEvent {
  if (typeof payload !== 'object' || payload === null) return false
  const candidate = payload as { type?: string; error?: { code?: unknown; message?: unknown } }
  return candidate.type === 'ERROR' && typeof candidate.error?.code === 'string' && typeof candidate.error?.message === 'string'
}

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080/api'
const WS_BASE_URL = import.meta.env.VITE_WS_BASE_URL ?? 'ws://localhost:8080/ws'
const TOKEN_STORAGE_KEY = 'blackjack.access_token'

function randomID(prefix: string): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `${prefix}-${crypto.randomUUID()}`
  }
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 100000)}`
}

function parseAPIError(error: unknown): string {
  if (axios.isAxiosError(error)) {
    const axiosError = error as AxiosError<ApiFailure>
    const payload = axiosError.response?.data
    if (payload?.error?.message) {
      return `${payload.error.code}: ${payload.error.message}`
    }
    if (axiosError.message) {
      return axiosError.message
    }
  }
  if (error instanceof Error) {
    return error.message
  }
  return 'unknown error'
}

function unwrapData<T>(payload: ApiResponse<T>): T {
  if (!payload.success) {
    throw new Error(`${payload.error.code}: ${payload.error.message}`)
  }
  return payload.data
}

function App() {
  const [username, setUsername] = useState('pm_user_01')
  const [password, setPassword] = useState('password12')
  const [token, setToken] = useState(localStorage.getItem(TOKEN_STORAGE_KEY) ?? '')
  const [roomID, setRoomID] = useState('')
  const [statusMessage, setStatusMessage] = useState('Ready')
  const [rooms, setRooms] = useState<RoomSummary[]>([])
  const [roomState, setRoomState] = useState<RoomStateSync | null>(null)
  const [historyJSON, setHistoryJSON] = useState('')
  const [wsLog, setWsLog] = useState<string[]>([])
  const [wsConnectionState, setWsConnectionState] = useState<'disconnected' | 'connecting' | 'connected'>('disconnected')
  const wsRef = useRef<WebSocket | null>(null)

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

  const appendWSLog = (message: string) => {
    setWsLog((prev) => [`${new Date().toISOString()} ${message}`, ...prev].slice(0, 60))
  }

  const saveToken = (nextToken: string) => {
    setToken(nextToken)
    localStorage.setItem(TOKEN_STORAGE_KEY, nextToken)
  }

  const withStatus = async (label: string, task: () => Promise<void>) => {
    try {
      setStatusMessage(`${label}...`)
      await task()
      setStatusMessage(`${label} done`)
    } catch (error) {
      setStatusMessage(`${label} failed: ${parseAPIError(error)}`)
    }
  }

  const signup = async () => {
    await withStatus('Signup', async () => {
      const response = await authClient.post<ApiResponse<LoginResponse>>('/auth/signup', {
        username,
        password,
      })
      saveToken(unwrapData(response.data).access_token)
    })
  }

  const login = async () => {
    await withStatus('Login', async () => {
      const response = await authClient.post<ApiResponse<LoginResponse>>('/auth/login', {
        username,
        password,
      })
      saveToken(unwrapData(response.data).access_token)
    })
  }

  const logout = async () => {
    await withStatus('Logout', async () => {
      await authClient.post('/auth/logout')
      setToken('')
      localStorage.removeItem(TOKEN_STORAGE_KEY)
      setRoomState(null)
      setWsLog([])
      if (wsRef.current) {
        wsRef.current.close()
      }
    })
  }

  const listRooms = async () => {
    await withStatus('List rooms', async () => {
      const response = await authClient.get<ApiResponse<{ rooms: RoomSummary[] }>>('/rooms')
      setRooms(unwrapData(response.data).rooms)
    })
  }

  const createRoom = async () => {
    await withStatus('Create room', async () => {
      const response = await authClient.post<ApiResponse<{ room: RoomSummary }>>('/rooms', {})
      const data = unwrapData(response.data)
      const nextRoomID = data.room.id
      setRoomID(nextRoomID)
      setRooms((prev) => [data.room, ...prev.filter((room) => room.id !== nextRoomID)])
    })
  }

  const joinRoom = async () => {
    await withStatus('Join room', async () => {
      const response = await authClient.post<ApiResponse<{ room: RoomSummary }>>(`/rooms/${roomID}/join`, {})
      unwrapData(response.data)
    })
  }

  const startRoom = async () => {
    await withStatus('Start room', async () => {
      const response = await authClient.post<ApiResponse<{ session: { version: number } }>>(`/rooms/${roomID}/start`, {})
      unwrapData(response.data)
      await fetchRoom()
    })
  }

  const fetchRoom = async () => {
    await withStatus('Get room', async () => {
      const response = await authClient.get<ApiResponse<Record<string, unknown>>>(`/rooms/${roomID}`)
      setHistoryJSON(JSON.stringify(response.data, null, 2))
    })
  }

  const fetchHistory = async () => {
    await withStatus('Get room history', async () => {
      const response = await authClient.get<ApiResponse<Record<string, unknown>>>(`/rooms/${roomID}/history`)
      setHistoryJSON(JSON.stringify(response.data, null, 2))
    })
  }

  const fetchHint = async () => {
    await withStatus('Get play hint', async () => {
      const response = await authClient.get<ApiResponse<Record<string, unknown>>>(`/rooms/${roomID}/play_hint`)
      setHistoryJSON(JSON.stringify(response.data, null, 2))
    })
  }

  const expectedVersion = roomState?.session.version ?? 0

  const hitByHTTP = async () => {
    await withStatus('HIT', async () => {
      const response = await authClient.post(`/rooms/${roomID}/hit`, {
        action_id: randomID('http-hit'),
        expected_version: expectedVersion,
      })
      setHistoryJSON(JSON.stringify(response.data, null, 2))
    })
  }

  const standByHTTP = async () => {
    await withStatus('STAND', async () => {
      const response = await authClient.post(`/rooms/${roomID}/stand`, {
        action_id: randomID('http-stand'),
        expected_version: expectedVersion,
      })
      setHistoryJSON(JSON.stringify(response.data, null, 2))
    })
  }

  const connectWebSocket = () => {
    if (!roomID) {
      setStatusMessage('Connect WS failed: room_id is empty')
      return
    }
    if (!token) {
      setStatusMessage('Connect WS failed: login first')
      return
    }
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }
    const url = `${WS_BASE_URL}/rooms/${roomID}`
    const socket = new WebSocket(url)
    wsRef.current = socket
    setWsConnectionState('connecting')
    appendWSLog(`WS open request -> ${url}`)

    socket.onopen = () => {
      setWsConnectionState('connected')
      appendWSLog('WS connected')
      socket.send(
        JSON.stringify({
          type: 'AUTH',
          request_id: randomID('auth'),
          access_token: token,
        }),
      )
      appendWSLog('AUTH sent')
    }

    socket.onmessage = (event) => {
      const text = String(event.data)
      appendWSLog(`<= ${text}`)
      try {
        const parsed = JSON.parse(text) as unknown
        if (isRoomSyncEvent(parsed)) {
          setRoomState(parsed.data)
        } else if (isWsErrorEvent(parsed)) {
          setStatusMessage(`WS error: ${parsed.error.code} ${parsed.error.message}`)
        }
      } catch {
        // no-op
      }
    }

    socket.onerror = () => {
      setStatusMessage('WS error')
      appendWSLog('WS error')
    }

    socket.onclose = () => {
      setWsConnectionState('disconnected')
      appendWSLog('WS closed')
    }
  }

  const sendWSMessage = (payload: Record<string, unknown>) => {
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      setStatusMessage('WS is not connected')
      return
    }
    const body = JSON.stringify(payload)
    wsRef.current.send(body)
    appendWSLog(`=> ${body}`)
  }

  const canPlay = roomState?.my_actions.can_hit || roomState?.my_actions.can_stand

  return (
    <main className="container">
      <h1>BlackJack Frontend</h1>
      <p className="subtle">{statusMessage}</p>

      <section className="panel">
        <h2>Connection Settings</h2>
        <p className="subtle">API: {API_BASE_URL}</p>
        <p className="subtle">WS: {WS_BASE_URL}</p>
      </section>

      <section className="panel">
        <h2>Auth</h2>
        <form
          className="row"
          onSubmit={(event: FormEvent<HTMLFormElement>) => {
            event.preventDefault()
            void signup()
          }}
        >
          <input value={username} onChange={(event) => setUsername(event.target.value)} placeholder="username" />
          <input value={password} onChange={(event) => setPassword(event.target.value)} placeholder="password" type="password" />
          <button type="submit">Signup</button>
          <button
            type="button"
            onClick={() => {
              void login()
            }}
          >
            Login
          </button>
          <button type="button" onClick={logout}>
            Logout
          </button>
        </form>
        <p className="subtle">{token ? `Token stored (${token.slice(0, 14)}...)` : 'No token'}</p>
      </section>

      <section className="panel">
        <h2>Room Controls</h2>
        <div className="row">
          <input value={roomID} onChange={(event) => setRoomID(event.target.value)} placeholder="room_id" />
          <button onClick={createRoom}>Create</button>
          <button onClick={listRooms}>List</button>
          <button onClick={joinRoom} disabled={!roomID}>
            Join
          </button>
          <button onClick={startRoom} disabled={!roomID}>
            Start
          </button>
          <button onClick={fetchRoom} disabled={!roomID}>
            Get Room
          </button>
          <button onClick={fetchHistory} disabled={!roomID}>
            History
          </button>
          <button onClick={fetchHint} disabled={!roomID}>
            Hint
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
                use
              </button>{' '}
              {room.id} ({room.status})
            </li>
          ))}
        </ul>
      </section>

      <section className="panel">
        <h2>WebSocket</h2>
        <p className="subtle">status: {wsConnectionState}</p>
        <div className="row">
          <button onClick={connectWebSocket} disabled={!roomID || !token}>
            Connect
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
            Sync Request
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
            WS HIT
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
            WS STAND
          </button>
          <button
            onClick={() =>
              sendWSMessage({
                type: 'REMATCH_VOTE',
                request_id: randomID('vote'),
                action_id: randomID('vote'),
                expected_version: expectedVersion,
                agree: true,
              })
            }
            disabled={wsConnectionState !== 'connected'}
          >
            Vote Rematch
          </button>
        </div>
      </section>

      <section className="panel">
        <h2>HTTP Turn Actions</h2>
        <div className="row">
          <button onClick={hitByHTTP} disabled={!roomID || expectedVersion <= 0}>
            HIT
          </button>
          <button onClick={standByHTTP} disabled={!roomID || expectedVersion <= 0}>
            STAND
          </button>
        </div>
      </section>

      <section className="panel">
        <h2>State Snapshot</h2>
        <pre>{roomState ? JSON.stringify(roomState, null, 2) : 'no ROOM_STATE_SYNC received yet'}</pre>
      </section>

      <section className="panel">
        <h2>HTTP Output</h2>
        <pre>{historyJSON || 'empty'}</pre>
      </section>

      <section className="panel">
        <h2>WebSocket Log</h2>
        <pre>{wsLog.length > 0 ? wsLog.join('\n') : 'empty'}</pre>
      </section>
    </main>
  )
}

export default App
