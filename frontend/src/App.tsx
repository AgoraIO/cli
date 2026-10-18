import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import AgoraRTC from 'agora-rtc-react'
import {
  AgoraRTCProvider,
  RemoteUser,
  useClientEvent,
  useJoin,
  useLocalMicrophoneTrack,
  usePublish,
  useRemoteUsers,
  useRTCClient,
} from 'agora-rtc-react'
import {
  AgoraVoiceAI,
  AgoraVoiceAIEvents,
  TranscriptHelperMode,
  type AgentState,
  type AgentTranscription,
  type AgentMetric,
  type TranscriptHelperItem,
  type UserTranscription,
} from 'agora-agent-client-toolkit'
import { AgentVisualizer } from 'agora-agent-uikit'
import { MicButtonWithVisualizer } from 'agora-agent-uikit/rtc'
import AgoraRTM from 'agora-rtm'
import type { RTMClient } from 'agora-rtm'
import {
  getMessageList,
  getCurrentInProgressMessage,
  mapAgentVisualizerState,
  normalizeTranscript,
} from './lib/conversation'
import { getConfig, type GetConfigResponse } from './services/api'

// ─── Transcript panel ───────────────────────────────────────────────────────

interface TranscriptPanelProps {
  transcript: TranscriptHelperItem<Partial<UserTranscription | AgentTranscription>>[]
  localUid: string
  agentUid: string
}

function TranscriptPanel({ transcript, localUid, agentUid }: TranscriptPanelProps) {
  const normalized = useMemo(
    () => normalizeTranscript(transcript, localUid),
    [transcript, localUid],
  )
  const messageList = useMemo(() => getMessageList(normalized), [normalized])
  const inProgress = useMemo(() => getCurrentInProgressMessage(normalized), [normalized])

  const all = inProgress ? [...messageList, inProgress] : messageList

  return (
    <div className="transcript">
      {all.length === 0 && (
        <div style={{ color: '#555', alignSelf: 'center', marginTop: '40px' }}>
          No transcript yet…
        </div>
      )}
      {all.map((msg, i) => {
        const isUser = String(msg.uid) === localUid
        return (
          <div
            key={`${msg.turn_id}-${i}`}
            className={`msg ${isUser ? 'msg-user' : 'msg-agent'}`}
          >
            {msg.text}
          </div>
        )
      })}
    </div>
  )
}

// ─── Metrics strip ───────────────────────────────────────────────────────────

interface MetricsStripProps {
  metrics: AgentMetric[]
}

function MetricsStrip({ metrics }: MetricsStripProps) {
  if (metrics.length === 0) return null
  return (
    <div className="metrics">
      {metrics.slice(-4).map((m, i) => (
        <div key={i} className="metric">
          <span className="metric-label">{m.name}:</span>
          <span className="metric-value">{m.value}ms</span>
        </div>
      ))}
    </div>
  )
}

// ─── Status bar ──────────────────────────────────────────────────────────────

interface StatusBarProps {
  connectionState: string
  agentState: AgentState | null
  isAgentConnected: boolean
}

function StatusBar({ connectionState, agentState, isAgentConnected }: StatusBarProps) {
  const isConnected = connectionState === 'CONNECTED'
  const isConnecting = connectionState === 'CONNECTING' || connectionState === 'RECONNECTING'
  const dotClass = `status-dot ${isConnected ? 'connected' : isConnecting ? 'connecting' : ''}`

  return (
    <div className="status">
      <span className={dotClass} />
      <span>RTC: {connectionState}</span>
      {isAgentConnected && agentState && <span>· Agent: {agentState}</span>}
      {!isAgentConnected && isConnected && (
        <span style={{ color: '#ff9800' }}>· Waiting for agent to join…</span>
      )}
    </div>
  )
}

// ─── Inner conversation component (inside AgoraRTCProvider) ──────────────────

interface ConversationProps {
  config: GetConfigResponse
  rtmClient: RTMClient
  onTokenWillExpire: (uid: string) => Promise<{ rtcToken: string; rtmToken: string }>
}

function ConversationInner({ config, rtmClient, onTokenWillExpire }: ConversationProps) {
  const client = useRTCClient()
  const remoteUsers = useRemoteUsers()

  const [isReady, setIsReady] = useState(false)
  useEffect(() => {
    const id = setTimeout(() => setIsReady(true), 0)
    return () => {
      clearTimeout(id)
      setIsReady(false)
    }
  }, [])

  const { isConnected } = useJoin(
    {
      appid: config.app_id,
      channel: config.channel_name,
      token: config.token,
      uid: Number(config.uid),
    },
    isReady,
  )

  const { localMicrophoneTrack } = useLocalMicrophoneTrack(isReady)
  usePublish([localMicrophoneTrack])

  const [connectionState, setConnectionState] = useState('CONNECTING')
  const [agentState, setAgentState] = useState<AgentState | null>(null)
  const [isAgentConnected, setIsAgentConnected] = useState(false)
  const [transcript, setTranscript] = useState<
    TranscriptHelperItem<Partial<UserTranscription | AgentTranscription>>[]
  >([])
  const [metrics, setMetrics] = useState<AgentMetric[]>([])
  const [isMicEnabled, setIsMicEnabled] = useState(true)

  // Detect agent in remote users
  useEffect(() => {
    const found = remoteUsers.some((u) => String(u.uid) === config.agent_uid)
    setIsAgentConnected(found)
  }, [remoteUsers, config.agent_uid])

  useClientEvent(client, 'user-joined', (user) => {
    if (String(user.uid) === config.agent_uid) setIsAgentConnected(true)
  })
  useClientEvent(client, 'user-left', (user) => {
    if (String(user.uid) === config.agent_uid) setIsAgentConnected(false)
  })
  useClientEvent(client, 'connection-state-change', (curState) => {
    setConnectionState(curState)
  })

  // Token renewal
  const handleTokenWillExpire = useCallback(async () => {
    const uid = client.uid
    if (!uid) return
    try {
      const { rtcToken, rtmToken } = await onTokenWillExpire(String(uid))
      await client.renewToken(rtcToken)
      await rtmClient.renewToken(rtmToken)
    } catch (err) {
      console.error('Token renewal failed:', err)
    }
  }, [client, rtmClient, onTokenWillExpire])
  useClientEvent(client, 'token-privilege-will-expire', handleTokenWillExpire)

  // AgoraVoiceAI init
  useEffect(() => {
    if (!isReady || !isConnected) return

    let cancelled = false
    ;(async () => {
      try {
        const ai = await AgoraVoiceAI.init({
          rtcEngine: client,
          rtmConfig: { rtmEngine: rtmClient },
          renderMode: TranscriptHelperMode.TEXT,
        })

        if (cancelled) {
          try {
            ai.unsubscribe()
            ai.destroy()
          } catch {}
          return
        }

        ai.on(AgoraVoiceAIEvents.TRANSCRIPT_UPDATED, (t) => {
          setTranscript([...t])
        })
        ai.on(AgoraVoiceAIEvents.AGENT_STATE_CHANGED, (_, event) => {
          setAgentState(event.state)
        })
        ai.on(AgoraVoiceAIEvents.AGENT_METRICS, (_, m) => {
          setMetrics((prev) => [...prev, m].slice(-8))
        })

        ai.subscribeMessage(config.channel_name)
      } catch (err) {
        if (!cancelled) {
          console.error('[AgoraVoiceAI] init failed:', err)
        }
      }
    })()

    return () => {
      cancelled = true
      try {
        const ai = AgoraVoiceAI.getInstance()
        if (ai) {
          ai.unsubscribe()
          ai.destroy()
        }
      } catch {}
    }
  }, [isReady, isConnected, client, rtmClient, config.channel_name])

  // Mic toggle
  const handleMicToggle = useCallback(async () => {
    const next = !isMicEnabled
    if (localMicrophoneTrack) {
      try {
        await localMicrophoneTrack.setEnabled(next)
      } catch (err) {
        console.error('Mic toggle failed:', err)
      }
    }
    setIsMicEnabled(next)
  }, [isMicEnabled, localMicrophoneTrack])

  const visualizerState = useMemo(
    () => mapAgentVisualizerState(agentState, isAgentConnected, connectionState),
    [agentState, isAgentConnected, connectionState],
  )

  return (
    <div className="app">
      <StatusBar
        connectionState={connectionState}
        agentState={agentState}
        isAgentConnected={isAgentConnected}
      />
      <div className="app-body">
        {/* Left column: visualizer + mic control */}
        <div className="left">
          <AgentVisualizer state={visualizerState} size="lg" />

          {/* Hidden RemoteUser audio players */}
          {remoteUsers.map((u) => (
            <div key={u.uid} style={{ display: 'none' }}>
              <RemoteUser user={u} playAudio />
            </div>
          ))}

          {/* Mic button */}
          <div style={{ display: 'flex', justifyContent: 'center', marginTop: 12 }}>
            <MicButtonWithVisualizer
              isEnabled={isMicEnabled}
              setIsEnabled={setIsMicEnabled}
              track={localMicrophoneTrack}
              onToggle={handleMicToggle}
            />
          </div>

        </div>

        {/* Right column: transcript */}
        <div className="right">
          <TranscriptPanel
            transcript={transcript}
            localUid={config.uid}
            agentUid={config.agent_uid}
          />
          <MetricsStrip metrics={metrics} />
        </div>
      </div>
    </div>
  )
}

// ─── Top-level App ────────────────────────────────────────────────────────────

export default function App() {
  const [config, setConfig] = useState<GetConfigResponse | null>(null)
  const [rtmClient, setRtmClient] = useState<RTMClient | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const clientRef = useRef<ReturnType<typeof AgoraRTC.createClient> | null>(null)

  // Create RTC client once
  if (!clientRef.current) {
    clientRef.current = AgoraRTC.createClient({ mode: 'rtc', codec: 'vp8' })
  }

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const cfg = await getConfig()

        // Create and login RTM client
        const rtm = new AgoraRTM.RTM(cfg.app_id, cfg.uid)
        await rtm.login({ token: cfg.token })
        await rtm.subscribe(cfg.channel_name)

        if (!cancelled) {
          setConfig(cfg)
          setRtmClient(rtm as RTMClient)
        }
      } catch (err) {
        if (!cancelled) {
          console.error('Startup failed:', err)
          setError(err instanceof Error ? err.message : 'Failed to initialize')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()

    return () => {
      cancelled = true
    }
  }, [])

  const handleTokenWillExpire = useCallback(
    async (uid: string) => {
      const cfg = await getConfig({ uid })
      return { rtcToken: cfg.token, rtmToken: cfg.token }
    },
    [],
  )

  if (loading) {
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100vh',
          color: '#888',
        }}
      >
        Connecting…
      </div>
    )
  }

  if (error || !config || !rtmClient) {
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100vh',
          color: '#f44',
        }}
      >
        {error || 'Configuration error'}
      </div>
    )
  }

  return (
    <AgoraRTCProvider client={clientRef.current!}>
      <ConversationInner
        config={config}
        rtmClient={rtmClient}
        onTokenWillExpire={handleTokenWillExpire}
      />
    </AgoraRTCProvider>
  )
}
