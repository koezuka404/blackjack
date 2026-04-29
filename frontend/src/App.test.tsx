import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import App from './App'

const {
  postMock,
  getMock,
  requestUseMock,
  responseUseMock,
  responseEjectMock,
  createMock,
} = vi.hoisted(() => ({
  postMock: vi.fn(),
  getMock: vi.fn(),
  requestUseMock: vi.fn(),
  responseUseMock: vi.fn(),
  responseEjectMock: vi.fn(),
  createMock: vi.fn(),
}))
let latestResponseErrorHandler: ((error: unknown) => Promise<unknown>) | undefined
let latestResponseSuccessHandler: ((value: unknown) => unknown) | undefined

vi.mock('axios', async () => {
  const actual = await vi.importActual<typeof import('axios')>('axios')
  return {
    ...actual,
    default: {
      ...actual.default,
      create: createMock,
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
    vi.useRealTimers()
    vi.clearAllMocks()
    localStorage.clear()
    MockWebSocket.instances = []
    vi.stubGlobal('WebSocket', MockWebSocket as unknown as typeof WebSocket)
    postMock.mockResolvedValue({ data: { success: true, data: {} } })
    getMock.mockResolvedValue({ data: { success: true, data: {} } })
    requestUseMock.mockImplementation((fn: (c: { headers: Record<string, string> }) => { headers: Record<string, string> }) => fn)
    latestResponseErrorHandler = undefined
    latestResponseSuccessHandler = undefined
    responseUseMock.mockImplementation((ok: (v: unknown) => unknown, ng: (e: unknown) => Promise<unknown>) => {
      latestResponseSuccessHandler = ok
      latestResponseErrorHandler = ng
      return 1
    })
    responseEjectMock.mockReset()
    createMock.mockImplementation(() => ({
      post: postMock,
      get: getMock,
      interceptors: {
        request: { use: requestUseMock },
        response: { use: responseUseMock, eject: responseEjectMock },
      },
    }))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows authentication screen by default', () => {
    render(<App />)

    expect(screen.getByRole('heading', { name: 'ブラックジャック' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'ログイン' })).toBeInTheDocument()
    expect(screen.getByPlaceholderText('メールアドレス')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('パスワード')).toBeInTheDocument()
  })

  it('shows validation when login submit with empty fields', async () => {
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'ログインする' }))
    await waitFor(() =>
      expect(screen.getByText('メールアドレスとパスワードを入力してください')).toBeInTheDocument(),
    )
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
    fireEvent.change(screen.getByPlaceholderText('ユーザー名'), { target: { value: 'new-user' } })
    fireEvent.change(screen.getByPlaceholderText('メールアドレス'), { target: { value: 'new-user@example.com' } })
    fireEvent.change(screen.getByPlaceholderText('パスワード'), { target: { value: 'password12' } })
    fireEvent.click(screen.getByRole('button', { name: '新規登録する' }))

    await waitFor(() =>
      expect(postMock).toHaveBeenCalledWith('/auth/signup', {
        username: 'new-user',
        email: 'new-user@example.com',
        password: 'password12',
      }),
    )
    await waitFor(() => expect(screen.getByRole('button', { name: 'ログアウト' })).toBeInTheDocument())
    expect(localStorage.getItem('blackjack.access_token')).toBe('token-signup')
  })

  it('shows Japanese validation messages for signup', async () => {
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: '新規登録' }))
    fireEvent.click(screen.getByRole('button', { name: '新規登録する' }))
    await waitFor(() => expect(screen.getByText('ユーザー名・メールアドレス・パスワードを入力してください')).toBeInTheDocument())

    fireEvent.change(screen.getByPlaceholderText('ユーザー名'), { target: { value: 'validuser' } })
    fireEvent.change(screen.getByPlaceholderText('メールアドレス'), { target: { value: 'not-an-email' } })
    fireEvent.change(screen.getByPlaceholderText('パスワード'), { target: { value: 'password1' } })
    fireEvent.click(screen.getByRole('button', { name: '新規登録する' }))
    await waitFor(() => expect(screen.getByText('メールアドレスの形式が正しくありません')).toBeInTheDocument())

    fireEvent.change(screen.getByPlaceholderText('メールアドレス'), { target: { value: 'a@b.co' } })
    fireEvent.change(screen.getByPlaceholderText('ユーザー名'), { target: { value: 'ab' } })
    fireEvent.click(screen.getByRole('button', { name: '新規登録する' }))
    await waitFor(() => expect(screen.getByText('ユーザー名は3〜100文字で入力してください')).toBeInTheDocument())

    fireEvent.change(screen.getByPlaceholderText('ユーザー名'), { target: { value: 'gooduser' } })
    fireEvent.change(screen.getByPlaceholderText('パスワード'), { target: { value: 'onlyletters' } })
    fireEvent.click(screen.getByRole('button', { name: '新規登録する' }))
    await waitFor(() => expect(screen.getByText('パスワードは8文字以上で英字と数字を含めてください')).toBeInTheDocument())
  })

  it('updates auth inputs and switches back from signup', () => {
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: '新規登録' }))
    fireEvent.change(screen.getByPlaceholderText('ユーザー名'), { target: { value: 'alice' } })
    fireEvent.change(screen.getByPlaceholderText('メールアドレス'), { target: { value: 'alice@example.com' } })
    fireEvent.change(screen.getByPlaceholderText('パスワード'), { target: { value: 'secret12' } })
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
    fireEvent.change(screen.getByPlaceholderText('メールアドレス'), { target: { value: 'user1@example.com' } })
    fireEvent.change(screen.getByPlaceholderText('パスワード'), { target: { value: 'password12' } })
    fireEvent.click(screen.getByRole('button', { name: 'ログインする' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'ログアウト' })).toBeInTheDocument())

    const latestUse = requestUseMock.mock.calls.at(-1)?.[0] as (c: { headers: Record<string, string> }) => { headers: Record<string, string> }
    const cfg = latestUse({ headers: {} })
    expect(cfg.headers.Authorization).toBe('Bearer token-login')
  })

  it('keeps request headers unchanged without token', async () => {
    render(<App />)
    await waitFor(() => expect(requestUseMock).toHaveBeenCalled())
    const requestHandler = requestUseMock.mock.calls[0]?.[0] as (cfg: { headers: Record<string, string> }) => {
      headers: Record<string, string>
    }
    const config = requestHandler({ headers: {} })
    expect(config.headers.Authorization).toBeUndefined()
  })

  it('shows localized API error on login failure', async () => {
    localStorage.clear()
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
    fireEvent.change(screen.getByPlaceholderText('メールアドレス'), { target: { value: 'user1@example.com' } })
    fireEvent.change(screen.getByPlaceholderText('パスワード'), { target: { value: 'password12' } })
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

  it('shows validation messages when room id is blank after trim', async () => {
    localStorage.setItem('blackjack.access_token', 'token-roomid-validation')
    render(<App />)

    fireEvent.change(screen.getByPlaceholderText('ルームID'), { target: { value: '   ' } })
    fireEvent.click(screen.getByRole('button', { name: '参加' }))
    await waitFor(() => expect(screen.getByText('ルームIDを入力してください')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: '開始' }))
    await waitFor(() => expect(screen.getByText('ルームIDを入力してください')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: '再接続' }))
    await waitFor(() => expect(screen.getByText('接続失敗: ルームIDが空です')).toBeInTheDocument())
  })

  it('logs out and clears to auth view', async () => {
    localStorage.setItem('blackjack.access_token', 'token-logout')
    postMock.mockResolvedValue({ data: { success: true, data: {} } })

    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'ログアウト' }))

    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/auth/logout'))
    await waitFor(() => expect(screen.getByRole('heading', { name: 'ログイン' })).toBeInTheDocument())
    expect(localStorage.getItem('blackjack.access_token')).toBeNull()
  })

  it('clears local auth state even when logout API returns 401', async () => {
    localStorage.setItem('blackjack.access_token', 'token-logout-401')
    postMock.mockRejectedValueOnce({
      isAxiosError: true,
      response: { status: 401 },
      message: 'unauthorized',
    })

    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'ログアウト' }))

    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/auth/logout'))
    await waitFor(() => expect(screen.getByRole('heading', { name: 'ログイン' })).toBeInTheDocument())
    expect(localStorage.getItem('blackjack.access_token')).toBeNull()
  })

  it('validates saved token on startup by calling /me', async () => {
    localStorage.setItem('blackjack.access_token', 'token-bootstrap-check')
    render(<App />)
    await waitFor(() => expect(getMock).toHaveBeenCalledWith('/me'))
  })

  it('survives /me bootstrap failure without throwing', async () => {
    localStorage.setItem('blackjack.access_token', 'token-bootstrap')
    getMock.mockRejectedValueOnce(new Error('network'))
    render(<App />)
    await waitFor(() => expect(getMock).toHaveBeenCalledWith('/me'))
  })

  it('handles 401 through response interceptor', async () => {
    localStorage.setItem('blackjack.access_token', 'token-401')
    postMock.mockResolvedValue({ data: { success: true, data: {} } })
    render(<App />)

    await waitFor(() => expect(typeof latestResponseErrorHandler).toBe('function'))
    await act(async () => {
      await latestResponseErrorHandler?.({
        isAxiosError: true,
        response: { status: 401 },
        message: 'unauthorized',
      }).catch(() => undefined)
    })

    await waitFor(() => expect(screen.getByRole('heading', { name: 'ログイン' })).toBeInTheDocument())
  })

  it('ignores non-axios errors in response interceptor', async () => {
    localStorage.setItem('blackjack.access_token', 'token-non-axios-error')
    render(<App />)
    await waitFor(() => expect(typeof latestResponseErrorHandler).toBe('function'))
    await expect(latestResponseErrorHandler?.(new Error('plain-error'))).rejects.toBeInstanceOf(Error)
    expect(screen.queryByText('セッションが切れました。再ログインしてください')).not.toBeInTheDocument()
  })

  it('supports client without response interceptor implementation', async () => {
    createMock.mockImplementationOnce(() => ({
      post: postMock,
      get: getMock,
      interceptors: {
        request: { use: requestUseMock },
      },
    }))
    localStorage.setItem('blackjack.access_token', 'token-no-response-interceptor')
    render(<App />)
    expect(screen.getByRole('button', { name: 'ログアウト' })).toBeInTheDocument()
  })

  it('supports client without response interceptor eject', async () => {
    createMock.mockImplementationOnce(() => ({
      post: postMock,
      get: getMock,
      interceptors: {
        request: { use: requestUseMock },
        response: { use: responseUseMock },
      },
    }))
    localStorage.setItem('blackjack.access_token', 'token-no-eject')
    const { unmount } = render(<App />)
    await waitFor(() => expect(screen.getByRole('button', { name: 'ログアウト' })).toBeInTheDocument())
    unmount()
  })

  it('runs interceptor success callback and cleanup eject', async () => {
    localStorage.setItem('blackjack.access_token', 'token-interceptor-success')
    const { unmount } = render(<App />)
    await waitFor(() => expect(typeof latestResponseSuccessHandler).toBe('function'))
    expect(latestResponseSuccessHandler?.({ ok: true })).toEqual({ ok: true })
    unmount()
    expect(responseEjectMock).toHaveBeenCalled()
  })

  it('clears auth state with active ws and reconnect timer', async () => {
    localStorage.setItem('blackjack.access_token', 'token-clear-auth-state')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-clear', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-clear/join') {
        return { data: { success: true, data: { room: { id: 'room-clear', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-clear/join', {}))
    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    act(() => {
      ws.onopen?.()
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
    })
    await waitFor(() => expect(screen.getByText(/WebSocketが切断されました。自動再接続まで/)).toBeInTheDocument())

    await act(async () => {
      await latestResponseErrorHandler?.({
        isAxiosError: true,
        response: { status: 401 },
        message: 'unauthorized',
      }).catch(() => undefined)
    })
    await waitFor(() => expect(screen.getByRole('heading', { name: 'ログイン' })).toBeInTheDocument())
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
  }, 15000)

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

  it('shows urgent rematch auto-end style when deadline is near', async () => {
    localStorage.setItem('blackjack.access_token', 'token-rematch-urgent')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-rematch-urgent', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-rematch-urgent/join') {
        return { data: { success: true, data: { room: { id: 'room-rematch-urgent', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-rematch-urgent/join', {}))

    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    act(() => {
      ws.onopen?.()
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'ROOM_STATE_SYNC',
          data: {
            room: { id: 'room-rematch-urgent', status: 'PLAYING' },
            session: {
              id: 'session-rematch-urgent',
              status: 'RESETTING',
              version: 11,
              round_no: 1,
              turn_seat: 1,
              turn_deadline_at: null,
              rematch_deadline_at: new Date(Date.now() + 3_000).toISOString(),
            },
            dealer: { visible_cards: ['10S', '7D'], hidden: false, card_count: 2 },
            players: [{ user_id: 'u1', seat_no: 1, status: 'ACTIVE', is_me: true, hand: ['9H', '8C'], card_count: 2, outcome: 'WIN', final_score: 17 }],
            my_actions: { can_hit: false, can_stand: false, can_rematch_vote: true },
          },
        }),
      })
    })

    await waitFor(() => expect(document.querySelector('.rematch-auto-end-timer-urgent')).not.toBeNull())
  })

  it('shows turn timer urgent and over states', async () => {
    localStorage.setItem('blackjack.access_token', 'token-turn-timer')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-turn', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-turn/join') {
        return { data: { success: true, data: { room: { id: 'room-turn', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-turn/join', {}))

    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    act(() => {
      ws.onopen?.()
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'ROOM_STATE_SYNC',
          data: {
            room: { id: 'room-turn', status: 'PLAYING' },
            session: {
              id: 'session-turn',
              status: 'PLAYER_TURN',
              version: 3,
              round_no: 1,
              turn_seat: 1,
              turn_deadline_at: new Date(Date.now() + 4_000).toISOString(),
              rematch_deadline_at: null,
            },
            dealer: { visible_cards: ['10S'], hidden: true, card_count: 2 },
            players: [{ user_id: 'u1', seat_no: 1, status: 'ACTIVE', is_me: true, hand: ['9H'], card_count: 1 }],
            my_actions: { can_hit: true, can_stand: false, can_rematch_vote: false },
          },
        }),
      })
    })
    await waitFor(() => expect(document.querySelector('.turn-timer-urgent')).not.toBeNull())

    act(() => {
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'ROOM_STATE_SYNC',
          data: {
            room: { id: 'room-turn', status: 'PLAYING' },
            session: {
              id: 'session-turn',
              status: 'PLAYER_TURN',
              version: 4,
              round_no: 1,
              turn_seat: 1,
              turn_deadline_at: new Date(Date.now() - 2_000).toISOString(),
              rematch_deadline_at: null,
            },
            dealer: { visible_cards: ['10S'], hidden: true, card_count: 2 },
            players: [{ user_id: 'u1', seat_no: 1, status: 'ACTIVE', is_me: true, hand: ['9H'], card_count: 1 }],
            my_actions: { can_hit: true, can_stand: false, can_rematch_vote: false },
          },
        }),
      })
    })
    await waitFor(() => expect(screen.getByText('時間切れ')).toBeInTheDocument())
    expect(screen.getByText('（時間切れであなたの負け）')).toBeInTheDocument()
  })

  it('does not show turn timer when player cannot act', async () => {
    localStorage.setItem('blackjack.access_token', 'token-turn-no-actions')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-turn-no-actions', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-turn-no-actions/join') {
        return { data: { success: true, data: { room: { id: 'room-turn-no-actions', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-turn-no-actions/join', {}))

    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    act(() => {
      ws.onopen?.()
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'ROOM_STATE_SYNC',
          data: {
            room: { id: 'room-turn-no-actions', status: 'PLAYING' },
            session: {
              id: 'session-turn-no-actions',
              status: 'PLAYER_TURN',
              version: 2,
              round_no: 1,
              turn_seat: 1,
              turn_deadline_at: new Date(Date.now() + 20_000).toISOString(),
              rematch_deadline_at: null,
            },
            dealer: { visible_cards: ['10S'], hidden: true, card_count: 2 },
            players: [{ user_id: 'u1', seat_no: 1, status: 'ACTIVE', is_me: true, hand: ['9H'], card_count: 1 }],
            my_actions: { can_hit: false, can_stand: false, can_rematch_vote: false },
          },
        }),
      })
    })

    await waitFor(() => expect(screen.queryByText(/行動制限時間:/)).not.toBeInTheDocument())
  })

  it('shows websocket reconnect banner after disconnect', async () => {
    localStorage.setItem('blackjack.access_token', 'token-reconnect-banner')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-reconnect-banner', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-reconnect-banner/join') {
        return { data: { success: true, data: { room: { id: 'room-reconnect-banner', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-reconnect-banner/join', {}))

    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    act(() => {
      ws.onopen?.()
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
    })

    await waitFor(() =>
      expect(screen.getByText(/WebSocketが切断されました。自動再接続まで あと \d+ 秒/)).toBeInTheDocument(),
    )
  })

  it('handles ws parse failure and reconnect early-return branches', async () => {
    localStorage.setItem('blackjack.access_token', 'token-ws-branches')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-ws-branches', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-ws-branches/join') {
        return { data: { success: true, data: { room: { id: 'room-ws-branches', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })
    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-ws-branches/join', {}))
    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    act(() => {
      ws.onopen?.()
      ws.onmessage?.({ data: '{invalid-json' })
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
    })
    await waitFor(() => expect(screen.getByText(/WebSocketが切断されました。自動再接続まで/)).toBeInTheDocument())
    fireEvent.change(screen.getByPlaceholderText('ルームID'), { target: { value: '' } })
    act(() => {
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
    })
  })

  it('covers reconnect countdown and keepalive timer branches', async () => {
    localStorage.setItem('blackjack.access_token', 'token-timer-branches')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-timer-branches', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-timer-branches/join') {
        return { data: { success: true, data: { room: { id: 'room-timer-branches', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })
    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-timer-branches/join', {}))
    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    vi.useFakeTimers()
    act(() => {
      ws.onopen?.()
    })

    ws.readyState = MockWebSocket.CONNECTING
    act(() => {
      vi.advanceTimersByTime(26_000)
    })
    ws.readyState = MockWebSocket.OPEN
    act(() => {
      vi.advanceTimersByTime(26_000)
    })
    expect(ws.send).toHaveBeenCalled()

    act(() => {
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
      vi.advanceTimersByTime(35_000)
    })
    vi.useRealTimers()
  }, 15000)

  it('covers reconnect clearTimeout and countdown completion branches', async () => {
    localStorage.setItem('blackjack.access_token', 'token-reconnect-branches')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-reconnect-branches', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-reconnect-branches/join') {
        return { data: { success: true, data: { room: { id: 'room-reconnect-branches', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })
    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-reconnect-branches/join', {}))
    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    vi.useFakeTimers()
    act(() => {
      ws.onopen?.()
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
      fireEvent.click(screen.getByRole('button', { name: '再接続' }))
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
      vi.advanceTimersByTime(2_000)
    })
    vi.useRealTimers()
  }, 15000)

  it('covers ws close early return without room membership', async () => {
    localStorage.setItem('blackjack.access_token', 'token-close-early')
    render(<App />)
    fireEvent.change(screen.getByPlaceholderText('ルームID'), { target: { value: 'room-alone' } })
    fireEvent.click(screen.getByRole('button', { name: '接続' }))
    await waitFor(() => expect(MockWebSocket.instances.length).toBeGreaterThan(0))
    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    act(() => {
      ws.onopen?.()
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
    })
  })

  it('covers reconnect callback return branch after leaving room', async () => {
    localStorage.setItem('blackjack.access_token', 'token-reconnect-return')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-reconnect-return', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-reconnect-return/join') {
        return { data: { success: true, data: { room: { id: 'room-reconnect-return', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-reconnect-return/leave') {
        return { data: { success: true, data: { room: { id: 'room-reconnect-return', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })
    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-reconnect-return/join', {}))
    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    act(() => {
      ws.onopen?.()
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
    })
    fireEvent.click(screen.getByRole('button', { name: 'ルーム退出' }))
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-reconnect-return/leave', {}))
    vi.useFakeTimers()
    act(() => {
      vi.advanceTimersByTime(2_000)
    })
    vi.useRealTimers()
  }, 15000)

  it('covers reconnect callback return when room id is cleared', async () => {
    localStorage.setItem('blackjack.access_token', 'token-reconnect-room-empty')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-reconnect-empty', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-reconnect-empty/join') {
        return { data: { success: true, data: { room: { id: 'room-reconnect-empty', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })
    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-reconnect-empty/join', {}))
    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    vi.useFakeTimers()
    act(() => {
      ws.onopen?.()
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
    })
    fireEvent.change(screen.getByPlaceholderText('ルームID'), { target: { value: '' } })
    act(() => {
      vi.advanceTimersByTime(2_000)
    })
    vi.useRealTimers()
  })

  it('ticks turn clock interval when timer is visible', async () => {
    localStorage.setItem('blackjack.access_token', 'token-turn-tick')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-turn-tick', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-turn-tick/join') {
        return { data: { success: true, data: { room: { id: 'room-turn-tick', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })
    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-turn-tick/join', {}))
    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    act(() => {
      ws.onopen?.()
      ws.onmessage?.({
        data: JSON.stringify({
          type: 'ROOM_STATE_SYNC',
          data: {
            room: { id: 'room-turn-tick', status: 'PLAYING' },
            session: {
              id: 'session-turn-tick',
              status: 'PLAYER_TURN',
              version: 1,
              round_no: 1,
              turn_seat: 1,
              turn_deadline_at: new Date(Date.now() + 10_000).toISOString(),
              rematch_deadline_at: null,
            },
            dealer: { visible_cards: ['8S'], hidden: false, card_count: 1 },
            players: [{ user_id: 'u1', seat_no: 1, status: 'ACTIVE', is_me: true, hand: ['9H'], card_count: 1 }],
            my_actions: { can_hit: true, can_stand: true, can_rematch_vote: false },
          },
        }),
      })
    })
    await waitFor(() => expect(screen.getByText(/行動制限時間:/)).toBeInTheDocument())
    await new Promise((resolve) => setTimeout(resolve, 300))
  })

  it('clears reconnect timer when leaving room', async () => {
    localStorage.setItem('blackjack.access_token', 'token-leave-clear-timer')
    postMock.mockImplementation(async (url: string) => {
      if (url === '/rooms') {
        return { data: { success: true, data: { room: { id: 'room-leave-timer', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-leave-timer/join') {
        return { data: { success: true, data: { room: { id: 'room-leave-timer', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      if (url === '/rooms/room-leave-timer/leave') {
        return { data: { success: true, data: { room: { id: 'room-leave-timer', host_user_id: 'u1', status: 'WAITING' } } } }
      }
      return { data: { success: true, data: {} } }
    })

    render(<App />)
    fireEvent.click(screen.getAllByRole('button', { name: 'AIと対戦する' })[0])
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-leave-timer/join', {}))

    const ws = MockWebSocket.instances.at(-1) as MockWebSocket
    act(() => {
      ws.onopen?.()
      ws.onclose?.({ code: 1006, reason: 'network', wasClean: false } as CloseEvent)
    })
    await waitFor(() => expect(screen.getByText(/WebSocketが切断されました。自動再接続まで/)).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: 'ルーム退出' }))
    await waitFor(() => expect(postMock).toHaveBeenCalledWith('/rooms/room-leave-timer/leave', {}))
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
