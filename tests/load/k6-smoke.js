import http from 'k6/http'
import { check, sleep } from 'k6'
import ws from 'k6/ws'
import { Rate, Trend } from 'k6/metrics'

const wsSyncSuccess = new Rate('ws_sync_success')
const wsSyncLatencyMs = new Trend('ws_sync_latency_ms')

export const options = {
  vus: Number(__ENV.K6_VUS || 100),
  duration: __ENV.K6_DURATION || '5m',
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<500'],
    checks: ['rate>0.95'],
    ws_sync_success: ['rate>0.95'],
    ws_sync_latency_ms: ['p(95)<100'],
  },
}

const apiBase = (__ENV.API_BASE_URL || '').replace(/\/+$/, '')
const wsBase = (__ENV.WS_BASE_URL || '').replace(/\/+$/, '')
const wsRatio = Number(__ENV.K6_WS_RATIO || 0.3)
const wsReconnectRatio = Number(__ENV.K6_WS_RECONNECT_RATIO || 0.5)

if (!apiBase) {
  throw new Error('API_BASE_URL is required')
}

export default function () {
  const ts = Date.now()
  const username = `k6_user_${__VU}_${__ITER}_${ts}`
  const password = 'password12'

  const signup = http.post(
    `${apiBase}/auth/signup`,
    JSON.stringify({ username, password }),
    { headers: { 'Content-Type': 'application/json' } },
  )
  check(signup, {
    'signup status 200': (r) => r.status === 200,
    'signup success true': (r) => {
      try {
        return JSON.parse(r.body).success === true
      } catch (_) {
        return false
      }
    },
  })

  let token = ''
  try {
    token = JSON.parse(signup.body).data.access_token
  } catch (_) {
    token = ''
  }
  if (!token) {
    return
  }

  const authHeaders = {
    Authorization: `Bearer ${token}`,
    'Content-Type': 'application/json',
  }

  const me = http.get(`${apiBase}/me`, { headers: authHeaders })
  check(me, {
    'me status 200': (r) => r.status === 200,
    'me success true': (r) => {
      try {
        return JSON.parse(r.body).success === true
      } catch (_) {
        return false
      }
    },
  })

  const createRoom = http.post(`${apiBase}/rooms`, null, { headers: authHeaders })
  check(createRoom, {
    'room status 200': (r) => r.status === 200,
    'room success true': (r) => {
      try {
        return JSON.parse(r.body).success === true
      } catch (_) {
        return false
      }
    },
  })

  let roomID = ''
  try {
    roomID = JSON.parse(createRoom.body).data.room.id
  } catch (_) {
    roomID = ''
  }

  if (roomID) {
    const joinRoom = http.post(`${apiBase}/rooms/${roomID}/join`, null, { headers: authHeaders })
    check(joinRoom, {
      'join status 200': (r) => r.status === 200,
      'join success true': (r) => {
        try {
          return JSON.parse(r.body).success === true
        } catch (_) {
          return false
        }
      },
    })
  }

  if (wsBase && roomID && Math.random() < wsRatio) {
    const wsURL = `${wsBase}/rooms/${roomID}`
    let syncReceived = false
    let errorReceived = false
    const openedAt = Date.now()
    const response = ws.connect(wsURL, {}, function (socket) {
      socket.on('open', () => {
        socket.send(JSON.stringify({ type: 'AUTH', request_id: `k6-auth-${Date.now()}`, access_token: token }))
      })
      socket.on('message', (raw) => {
        try {
          const msg = JSON.parse(String(raw))
          if (msg.type === 'ROOM_STATE_SYNC') {
            syncReceived = true
            wsSyncLatencyMs.add(Date.now() - openedAt)
            socket.close()
          } else if (msg.type === 'ERROR') {
            errorReceived = true
            socket.close()
          }
        } catch (_) {}
      })
      socket.setTimeout(() => {
        socket.close()
      }, 3000)
    })

    check(response, {
      'ws handshake status 101': (r) => r && r.status === 101,
      'ws room sync received': () => syncReceived && !errorReceived,
    })
    wsSyncSuccess.add(syncReceived && !errorReceived)

    if (Math.random() < wsReconnectRatio) {
      let secondSync = false
      const reconnectOpenedAt = Date.now()
      const reconnectResp = ws.connect(wsURL, {}, function (socket) {
        socket.on('open', () => {
          socket.send(JSON.stringify({ type: 'AUTH', request_id: `k6-auth-re-${Date.now()}`, access_token: token }))
        })
        socket.on('message', (raw) => {
          try {
            const msg = JSON.parse(String(raw))
            if (msg.type === 'ROOM_STATE_SYNC') {
              secondSync = true
              wsSyncLatencyMs.add(Date.now() - reconnectOpenedAt)
              socket.close()
            }
          } catch (_) {}
        })
        socket.setTimeout(() => {
          socket.close()
        }, 3000)
      })
      check(reconnectResp, {
        'ws reconnect handshake 101': (r) => r && r.status === 101,
        'ws reconnect room sync': () => secondSync,
      })
      wsSyncSuccess.add(secondSync)
    }
  }

  sleep(0.1)
}
