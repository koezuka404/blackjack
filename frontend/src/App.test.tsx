import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import App from './App'

const postMock = vi.fn()
const getMock = vi.fn()
const requestUseMock = vi.fn()

vi.mock('axios', async () => {
  const actual = await vi.importActual<typeof import('axios')>('axios')
  return {
    ...actual,
    default: {
      ...actual.default,
      create: vi.fn(() => ({
        post: postMock,
        get: getMock,
        interceptors: {
          request: {
            use: requestUseMock,
          },
        },
      })),
    },
  }
})

class MockWebSocket {
  static OPEN = 1
  static CONNECTING = 0
  static instances: MockWebSocket[] = []
  readyState = MockWebSocket.OPEN
  onopen: (() => void) | null = null
  onmessage: ((event: { data: unknown }) => void) | null = null
  onerror: (() => void) | null = null
  onclose: (() => void) | null = null

  constructor(public readonly url: string) {
    MockWebSocket.instances.push(this)
  }

  send = vi.fn()
  close = vi.fn()
}

describe('App', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    MockWebSocket.instances = []
    vi.stubGlobal('WebSocket', MockWebSocket as unknown as typeof WebSocket)
    postMock.mockResolvedValue({ data: { success: true, data: {} } })
    getMock.mockResolvedValue({ data: { success: true, data: {} } })
    requestUseMock.mockImplementation((fn: (c: { headers: Record<string, string> }) => { headers: Record<string, string> }) => fn)
  })

  it('shows authentication screen by default', () => {
    render(<App />)

    expect(screen.getByRole('heading', { name: 'ブラックジャック' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'ログイン' })).toBeInTheDocument()
    expect(screen.getByPlaceholderText('ユーザー名')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('パスワード')).toBeInTheDocument()
  })

  it('toggles to signup and submits successfully', async () => {
    postMock.mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          access_token: 'token-signup',
          token_type: 'Bearer',
          expires_in: 3600,
          user: { id: 'u1', username: 'new-user' },
        },
      },
    })

    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: '新規登録' }))
    expect(screen.getByRole('heading', { name: '新規登録' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '新規登録する' }))

    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/auth/signup', expect.any(Object)))
    await waitFor(() => expect(screen.getByRole('button', { name: 'ログアウト' })).toBeInTheDocument())
    expect(localStorage.getItem('blackjack.access_token')).toBe('token-signup')
  })

  it('updates auth inputs and switches back from signup', () => {
    render(<App />)
    fireEvent.change(screen.getByPlaceholderText('ユーザー名'), { target: { value: 'alice' } })
    fireEvent.change(screen.getByPlaceholderText('パスワード'), { target: { value: 'secret12' } })
    fireEvent.click(screen.getByRole('button', { name: '新規登録' }))
    expect(screen.getByRole('button', { name: 'ログイン' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'ログイン' }))
    expect(screen.getByRole('heading', { name: 'ログイン' })).toBeInTheDocument()
  })

  it('attaches auth header through interceptor when token exists', async () => {
    postMock.mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          access_token: 'token-login',
          token_type: 'Bearer',
          expires_in: 3600,
          user: { id: 'u1', username: 'user1' },
        },
      },
    })

    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'ログインする' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'ログアウト' })).toBeInTheDocument())

    const latestUse = requestUseMock.mock.calls.at(-1)?.[0] as (c: { headers: Record<string, string> }) => { headers: Record<string, string> }
    const cfg = latestUse({ headers: {} })
    expect(cfg.headers.Authorization).toBe('Bearer token-login')
  })

  it('shows localized API error on login failure', async () => {
    postMock.mockRejectedValueOnce({
      isAxiosError: true,
      response: {
        data: {
          success: false,
          error: { code: 'username_taken', message: 'username already exists' },
        },
      },
      message: 'Request failed',
    })

    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'ログインする' }))

    await waitFor(() => {
      expect(screen.getByText('このユーザー名は既に使われています')).toBeInTheDocument()
    })
  })

  it('runs join room flow and disables join button', async () => {
    localStorage.setItem('blackjack.access_token', 'token-joined')
    getMock.mockResolvedValue({
      data: { success: true, data: { rooms: [{ id: 'room-1', host_user_id: 'u1', status: 'WAITING' }] } },
    })
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-1', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-1/join') {
        return { data: { success: true, data: { room: { id: 'room-1', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: '一覧' }))
    await waitFor(() => expect(getMock).toHaveBeenCalledWith('/rooms'))
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])

    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms', {}))
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-1/join', {}))
    await waitFor(() => expect(screen.getAllByRole('button', { name: 'AIと対戦する' })[0]).toBeDisabled())
  })

  it('handles websocket sync/error and sends gameplay actions', async () => {
    localStorage.setItem('blackjack.access_token', 'token-joined')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-2', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-2/join') {
        return { data: { success: true, data: { room: { id: 'room-2', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])

    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-2/join', {}))
    fireEvent.click(screen.getByRole('button', { name: '再接続' }))
    await waitFor(() => expect(MockWebSocket.instances.length).toBeGreaterThan(0))
    const ws = MockWebSocket.instances[0]
    act(() => {
      ws.onopen?.()
    })

    act(() => {
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'ROOM_STATE_SYNC',
          data: {
            room: { id: 'room-2', status: 'ACTIVE' },
            session: {
              id: 'session-1',
              status: 'IN_PROGRESS',
              version: 3,
              round_no: 1,
              turn_seat: 1,
              turn_deadline_at: null,
              rematch_deadline_at: null,
            },
            dealer: { visible_cards: ['10S'], hidden: false, card_count: 1 },
            players: [
              {
                user_id: 'u1',
                seat_no: 1,
                status: 'ACTIVE',
                is_me: true,
                hand: ['AH', '9C'],
                card_count: 2,
                outcome: 'WIN',
                final_score: 20,
              },
            ],
            my_actions: { can_hit: true, can_stand: true, can_rematch_vote: true },
          },
        }),
      })
    })

    await waitFor(() => expect(screen.getByText('あなたの勝ち')).toBeInTheDocument())
    fireEvent.click(screen.getAllByRole('button', { name: 'ヒット' })[0])
    fireEvent.click(screen.getAllByRole('button', { name: 'スタンド' })[0])
    fireEvent.click(screen.getByRole('button', { name: '次のゲーム' }))
    fireEvent.click(screen.getByRole('button', { name: '終了する' }))
    expect(ws.send).toHaveBeenCalled()

    act(() => {
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'ERROR',
          error: { code: 'x', message: 'y' },
        }),
      })
    })
    act(() => {
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'UNHANDLED',
          data: {},
        }),
      })
    })

    act(() => {
      ws.onerror?.()
      ws.onclose?.()
    })
  })

  it('runs tech-panel http action buttons', async () => {
    localStorage.setItem('blackjack.access_token', 'token-tech')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-tech', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-tech/join') {
        return { data: { success: true, data: { room: { id: 'room-tech', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-tech/start') {
        return { data: { success: true, data: { session: { version: 1 } } } }
      }
      if (url === '/rooms/room-tech/hit' || url === '/rooms/room-tech/stand') {
        return { data: { ok: true } }
      }
      return { data: { success: true, data: {} } }
    })
    getMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { rooms: [{ id: 'room-tech', host_user_id: 'u1', status: 'WAITING' }] } } }
      }
      return { data: { success: true, data: { sample: true } } }
    })

    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: '作成' }))
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms', {}))

    fireEvent.click(screen.getByRole('button', { name: '一覧' }))
    await waitFor(() => expect(getMock).toHaveBeenCalledWith('/rooms'))

    fireEvent.click(screen.getByRole('button', { name: '作成' }))
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms', {}))

    fireEvent.click(screen.getByRole('button', { name: '参加' }))
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-tech/join', {}))

    fireEvent.click(screen.getByRole('button', { name: '開始' }))
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-tech/start', {}))

    fireEvent.click(screen.getByRole('button', { name: '接続' }))
    await waitFor(() => expect(MockWebSocket.instances.length).toBeGreaterThan(0))
    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    act(() => {
      ws.onopen?.()
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'ROOM_STATE_SYNC',
          data: {
            room: { id: 'room-tech', status: 'ACTIVE' },
            session: {
              id: 'session-tech',
              status: 'IN_PROGRESS',
              version: 1,
              round_no: 1,
              turn_seat: 1,
              turn_deadline_at: null,
              rematch_deadline_at: null,
            },
            dealer: { visible_cards: ['8S'], hidden: false, card_count: 1 },
            players: [{ user_id: 'u1', seat_no: 1, status: 'ACTIVE', is_me: true, hand: ['9H'], card_count: 1 }],
            my_actions: { can_hit: true, can_stand: true, can_rematch_vote: false },
          },
        }),
      })
    })

    fireEvent.click(screen.getAllByRole('button', { name: 'ヒット' })[1])
    await waitFor(() =>
      expect(postMock).toHaveBeenCalledWith(
        '/rooms/room-tech/hit',
        expect.objectContaining({ expected_version: expect.any(Number) }),
      ),
    )

    fireEvent.click(screen.getAllByRole('button', { name: 'スタンド' })[1])
    await waitFor(() =>
      expect(postMock).toHaveBeenCalledWith(
        '/rooms/room-tech/stand',
        expect.objectContaining({ expected_version: expect.any(Number) }),
      ),
    )

    fireEvent.click(screen.getByRole('button', { name: '取得' }))
    await waitFor(() => expect(getMock).toHaveBeenCalledWith('/rooms/room-tech'))

    fireEvent.click(screen.getByRole('button', { name: '履歴' }))
    await waitFor(() => expect(getMock).toHaveBeenCalledWith('/rooms/room-tech/history'))

    fireEvent.click(screen.getByRole('button', { name: 'ヒント' }))
    await waitFor(() => expect(getMock).toHaveBeenCalledWith('/rooms/room-tech/play_hint'))
  }, 15000)

  it('logs out and clears to auth view', async () => {
    localStorage.setItem('blackjack.access_token', 'token-logout')
    postMock.mockResolvedValue({ data: { success: true, data: {} } })

    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'ログアウト' }))

    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/auth/logout'))
    await waitFor(() => expect(screen.getByRole('heading', { name: 'ログイン' })).toBeInTheDocument())
    expect(localStorage.getItem('blackjack.access_token')).toBeNull()
  })

  it('leaves room successfully after joining', async () => {
    localStorage.setItem('blackjack.access_token', 'token-leave')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-leave', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-leave/join') {
        return { data: { success: true, data: { room: { id: 'room-leave', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-leave/leave') {
        return { data: { success: true, data: { room: { id: 'room-leave', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-leave/join', {}))
    fireEvent.click(screen.getByRole('button', { name: 'ルーム退出' }))
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-leave/leave', {}))
  })

  it('uses tech websocket controls and room list select button', async () => {
    localStorage.setItem('blackjack.access_token', 'token-ws-tech')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-ws', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-ws/join') {
        return { data: { success: true, data: { room: { id: 'room-ws', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })
    getMock.mockResolvedValue({
      data: {
        success: true,
        data: {
          rooms: [{ id: 'room-ws', host_user_id: 'u1', status: 'WAITING' }],
        },
      },
    })

    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: '一覧' }))
    await waitFor(() => expect(screen.getByRole('button', { name: '使用' })).toBeInTheDocument())
    fireEvent.change(screen.getByPlaceholderText('ルームID'), { target: { value: 'room-ws' } })
    fireEvent.click(screen.getByRole('button', { name: '使用' }))

    fireEvent.click(screen.getByRole('button', { name: '参加' }))
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-ws/join', {}))

    fireEvent.click(screen.getByRole('button', { name: '接続' }))
    await waitFor(() => expect(MockWebSocket.instances.length).toBeGreaterThan(0))
    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    ws.readyState = MockWebSocket.CONNECTING
    act(() => {
      ws.onopen?.()
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'ROOM_STATE_SYNC',
          data: {
            room: { id: 'room-ws', status: 'ACTIVE' },
            session: {
              id: 'sess-ws',
              status: 'IN_PROGRESS',
              version: 5,
              round_no: 1,
              turn_seat: 1,
              turn_deadline_at: null,
              rematch_deadline_at: null,
            },
            dealer: { visible_cards: ['9S'], hidden: false, card_count: 1 },
            players: [{ user_id: 'u1', seat_no: 1, status: 'ACTIVE', is_me: true, hand: ['7H'], card_count: 1 }],
            my_actions: { can_hit: true, can_stand: true, can_rematch_vote: false },
          },
        }),
      })
    })

    fireEvent.click(screen.getByRole('button', { name: '同期取得' }))
    fireEvent.click(screen.getByRole('button', { name: 'Ping' }))
    fireEvent.click(screen.getByRole('button', { name: 'WSヒット' }))
    fireEvent.click(screen.getByRole('button', { name: 'WSスタンド' }))

    expect(ws.send).toHaveBeenCalled()
  })

  it('joins room flow with preset room id (no create path)', async () => {
    localStorage.setItem('blackjack.access_token', 'token-preset-join')
    postMock.mockResolvedValue({
      data: { success: true, data: { room: { id: 'room-fixed', host_user_id: 'u1', status: 'WAITING' } } },
    })

    render(<App />)
    fireEvent.change(screen.getByPlaceholderText('ルームID'), { target: { value: 'room-fixed' } })
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])

    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-fixed/join', {}))
  })


  it('shows visible player turn timer from turn_deadline_at', async () => {
    localStorage.setItem('blackjack.access_token', 'token-timer')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-timer', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-timer/join') {
        return { data: { success: true, data: { room: { id: 'room-timer', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-timer/join', {}))

    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    const deadline = new Date(Date.now() + 12_000).toISOString()
    act(() => {
      ws.onopen?.()
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'ROOM_STATE_SYNC',
          data: {
            room: { id: 'room-timer', status: 'PLAYING' },
            session: {
              id: 'session-timer',
              status: 'PLAYER_TURN',
              version: 7,
              round_no: 1,
              turn_seat: 1,
              turn_deadline_at: deadline,
              rematch_deadline_at: null,
            },
            dealer: { visible_cards: ['8S'], hidden: true, card_count: 2 },
            players: [{ user_id: 'u1', seat_no: 1, status: 'ACTIVE', is_me: true, hand: ['7H'], card_count: 1 }],
            my_actions: { can_hit: true, can_stand: true, can_rematch_vote: false },
          },
        }),
      })
    })

    await waitFor(() => expect(screen.getByText(/行動制限時間:/)).toBeInTheDocument())
  })


  it('shows rematch auto-end countdown in resetting phase', async () => {
    localStorage.setItem('blackjack.access_token', 'token-rematch-timer')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-rematch', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-rematch/join') {
        return { data: { success: true, data: { room: { id: 'room-rematch', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-rematch/join', {}))

    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    const rematchDeadline = new Date(Date.now() + 9_000).toISOString()
    act(() => {
      ws.onopen?.()
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'ROOM_STATE_SYNC',
          data: {
            room: { id: 'room-rematch', status: 'PLAYING' },
            session: {
              id: 'session-rematch',
              status: 'RESETTING',
              version: 10,
              round_no: 1,
              turn_seat: 1,
              turn_deadline_at: null,
              rematch_deadline_at: rematchDeadline,
            },
            dealer: { visible_cards: ['10S', '7D'], hidden: false, card_count: 2 },
            players: [{ user_id: 'u1', seat_no: 1, status: 'ACTIVE', is_me: true, hand: ['9H', '8C'], card_count: 2, outcome: 'WIN', final_score: 17 }],
            my_actions: { can_hit: false, can_stand: false, can_rematch_vote: true },
          },
        }),
      })
    })

    await waitFor(() => expect(screen.getByText(/秒で自動的にこのゲームを終了します/)).toBeInTheDocument())
  })

  it('starts game from primary button and shows started state', async () => {
    localStorage.setItem('blackjack.access_token', 'token-start')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-start', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-start/join') {
        return { data: { success: true, data: { room: { id: 'room-start', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-start/start') {
        return { data: { success: true, data: { session: { version: 1 } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-start/join', {}))

    fireEvent.click(screen.getByRole('button', { name: 'ゲーム開始' }))
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-start/start', {}))
    await waitFor(() => expect(screen.getByRole('button', { name: '開始済み' })).toBeInTheDocument())
  })
})
