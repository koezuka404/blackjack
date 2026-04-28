import { render, screen } from '@testing-library/react'
import {
  formatPlayerTurnCountdown,
  handScore,
  isRoomSyncEvent,
  isWsErrorEvent,
  outcomeToLabel,
  parseAPIError,
  parseCardFace,
  rankToPoint,
  randomID,
  renderPlayingCards,
  resolveApiBaseURL,
  resolveWsBaseURL,
  unwrapData,
} from './App'

describe('App helper functions', () => {
  it('validates room sync and ws error event payloads', () => {
    expect(isRoomSyncEvent({ type: 'ROOM_STATE_SYNC', data: {} })).toBe(true)
    expect(isRoomSyncEvent({ type: 'ROOM_STATE_SYNC' })).toBe(false)
    expect(isRoomSyncEvent(null)).toBe(false)

    expect(isWsErrorEvent({ type: 'ERROR', error: { code: 'x', message: 'y' } })).toBe(true)
    expect(isWsErrorEvent({ type: 'ERROR', error: { code: 1, message: 'y' } })).toBe(false)
    expect(isWsErrorEvent('error')).toBe(false)
  })

  it('maps parseAPIError for axios and generic errors', () => {
    const axiosError = {
      isAxiosError: true,
      response: {
        data: {
          success: false,
          error: { code: 'rate_limited', message: 'slow down' },
        },
      },
      message: 'Request failed',
    }
    expect(parseAPIError(axiosError)).toBe('アクセスが多すぎます。少し待ってください')
    expect(
      parseAPIError({
        isAxiosError: true,
        response: { data: { success: false, error: { code: 'invalid_input', message: 'bad' } } },
        message: 'Request failed',
      }),
    ).toBe('入力内容が正しくありません')
    expect(
      parseAPIError({
        isAxiosError: true,
        response: { data: { success: false, error: { code: 'forbidden', message: 'forbidden' } } },
        message: 'Request failed',
      }),
    ).toBe('この操作は許可されていません')
    expect(
      parseAPIError({
        isAxiosError: true,
        response: { data: { success: false, error: { code: 'invalid_game_state', message: 'invalid' } } },
        message: 'Request failed',
      }),
    ).toBe('現在の状態ではこの操作はできません')
    expect(
      parseAPIError({
        isAxiosError: true,
        response: { data: { success: false, error: { code: 'room_full', message: 'full' } } },
        message: 'Request failed',
      }),
    ).toBe('ルームが満員です')
    expect(
      parseAPIError({
        isAxiosError: true,
        response: { data: { success: false, error: { code: 'not_found', message: 'not found' } } },
        message: 'Request failed',
      }),
    ).toBe('対象データが見つかりません')
    expect(
      parseAPIError({
        isAxiosError: true,
        response: { data: { success: false, error: { code: 'internal_error', message: 'internal' } } },
        message: 'Request failed',
      }),
    ).toBe('サーバーエラーが発生しました')
    expect(
      parseAPIError({
        isAxiosError: true,
        response: { data: { success: false, error: { code: 'unknown_code', message: 'raw message' } } },
        message: 'Request failed',
      }),
    ).toBe('raw message')
    expect(
      parseAPIError({
        isAxiosError: true,
        response: { data: { success: false, error: { code: 'unknown_code', message: '' } } },
        message: 'Request failed',
      }),
    ).toBe('エラーが発生しました')
    expect(
      parseAPIError({
        isAxiosError: true,
        response: { data: { success: false, error: { code: 'unauthorized', message: 'unauthorized' } } },
        message: 'Request failed',
      }),
    ).toBe('')
    expect(parseAPIError({ isAxiosError: true, message: 'network', response: undefined })).toBe('通信エラーが発生しました')
    expect(parseAPIError({ isAxiosError: true, message: '', response: undefined })).toBe('不明なエラーが発生しました')
    expect(parseAPIError(new Error('bad'))).toBe('bad')
    expect(parseAPIError(new Error(''))).toBe('エラーが発生しました')
    expect(parseAPIError('oops')).toBe('不明なエラーが発生しました')
  })

  it('resolves api/ws base URL and random ID fallback', () => {
    expect(resolveApiBaseURL()).toContain('/api')
    expect(resolveWsBaseURL()).toContain('/ws')

    const originalCrypto = globalThis.crypto
    Object.defineProperty(globalThis, 'crypto', { value: undefined, configurable: true })
    expect(randomID('test')).toMatch(/^test-/)
    Object.defineProperty(globalThis, 'crypto', { value: originalCrypto, configurable: true })
  })

  it('unwraps API success and throws on failure', () => {
    expect(unwrapData({ success: true, data: { ok: true } })).toEqual({ ok: true })
    expect(() =>
      unwrapData({
        success: false,
        error: { code: 'invalid_input', message: 'broken' },
      }),
    ).toThrow('invalid_input: broken')
  })

  it('parses card formats and computes score', () => {
    expect(parseCardFace('   ')).toBeNull()
    expect(parseCardFace('A♠')).toEqual({ rank: 'A', suit: 'S' })
    expect(parseCardFace('10 of hearts')).toEqual({ rank: '10', suit: 'H' })
    expect(parseCardFace('xyz')).toBeNull()

    expect(rankToPoint('A')).toBe(11)
    expect(rankToPoint('Q')).toBe(10)
    expect(rankToPoint('7')).toBe(7)
    expect(rankToPoint('X')).toBe(0)

    expect(handScore(['A♠', '9H'])).toBe(20)
    expect(handScore(['A♠', '9H', '5D'])).toBe(15)
    expect(handScore(['xx'])).toBeNull()
    expect(handScore()).toBeNull()
  })

  it('renders playing cards and hidden/unknown cards', () => {
    const { rerender } = render(renderPlayingCards(undefined, 0))
    expect(screen.getByText('--')).toBeInTheDocument()

    rerender(renderPlayingCards(['A♠', '??'], 1))
    expect(screen.getByLabelText('A♠')).toBeInTheDocument()
    expect(screen.getByLabelText('unknown-card')).toBeInTheDocument()
    expect(screen.getByLabelText('hidden-card')).toBeInTheDocument()
  })

  it('formats player turn countdown view', () => {
    expect(formatPlayerTurnCountdown(65_000)).toEqual({ text: '1:05', urgent: false, isOver: false })
    expect(formatPlayerTurnCountdown(4_000)).toEqual({ text: '4秒', urgent: true, isOver: false })
    expect(formatPlayerTurnCountdown(0)).toEqual({ text: '時間切れ', urgent: true, isOver: true })
    expect(formatPlayerTurnCountdown(-10)).toEqual({ text: '時間切れ', urgent: true, isOver: true })
  })

  it('maps outcomes to labels', () => {
    expect(outcomeToLabel('WIN')).toEqual({ text: 'あなたの勝ち', tone: 'win' })
    expect(outcomeToLabel('LOSE')).toEqual({ text: 'あなたの負け', tone: 'lose' })
    expect(outcomeToLabel('BUST')).toEqual({ text: 'あなたの負け', tone: 'lose' })
    expect(outcomeToLabel('PUSH')).toEqual({ text: '引き分け', tone: 'draw' })
    expect(outcomeToLabel('DRAW')).toEqual({ text: '引き分け', tone: 'draw' })
    expect(outcomeToLabel('')).toEqual({ text: '', tone: 'idle' })
  })
})
