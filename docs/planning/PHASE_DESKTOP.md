# Redoubt — Desktop Client Polish Phase

**Status:** Ready for implementation
**Last updated:** 2026-02-25
**Author:** Michael

This document defines the complete scope, technical decisions, and implementation details for the Desktop Client Polish phase of Redoubt. This is a **parallel track** that runs alongside other phases to ensure the Tauri desktop client is production-ready.

---

## Table of Contents

- [1. Phase Scope Summary](#1-phase-scope-summary)
- [2. Architecture Decisions](#2-architecture-decisions)
- [3. Multi-Instance Support](#3-multi-instance-support)
- [4. Audio Device Management](#4-audio-device-management)
- [5. Sound Effects System](#5-sound-effects-system)
- [6. Speaking Indicators](#6-speaking-indicators)
- [7. Voice Controls Polish](#7-voice-controls-polish)
- [8. Global Keyboard Shortcuts](#8-global-keyboard-shortcuts)
- [9. Connection & Reconnection](#9-connection--reconnection)
- [10. Error Handling UI](#10-error-handling-ui)
- [11. Settings Panel](#11-settings-panel)
- [12. Cross-Platform Validation](#12-cross-platform-validation)
- [13. Implementation Tasks](#13-implementation-tasks)
- [14. Feature Validation Checklist](#14-feature-validation-checklist)

---

## 1. Phase Scope Summary

This phase ensures the Tauri desktop client is polished, reliable, and feature-complete:

| Component | Scope |
|-----------|-------|
| Multi-Instance | Support multiple Redoubt server instances with tabbed UI |
| Audio Devices | Full device management with input level meter, output test |
| Sound Effects | Essential sounds for join/leave, mute/unmute, disconnect |
| Speaking Indicator | Small visualizer next to speaker's name |
| Voice Controls | Input level display, connection quality icon, PTT indicator |
| Keyboard Shortcuts | Global PTT, mute, deafen shortcuts |
| Reconnection | Auto-reconnect with UI overlay and exponential backoff |
| Error Handling | Comprehensive user-friendly error states |
| Settings | Centralized settings panel |
| Cross-Platform | Validation on macOS, Windows, Linux |

---

## 2. Architecture Decisions

### Key Design Decisions

| Decision | Choice |
|----------|--------|
| Multi-instance UI | Tabbed interface (like browser tabs) |
| Instance connections | On-demand (connect only when active) |
| Voice across instances | One instance at a time |
| Credentials storage | Per-instance tokens in secure storage |
| E2EE keys | Per-instance (separate identity per server) |
| Settings scope | Global only (apply to all instances) |
| Sound effects | Essential only (join/leave, mute/unmute, disconnect) |
| Sound scope | Self + others (sounds for all actions) |
| Speaking indicator | Small visualizer next to speaker's name |
| Device management | Full (input level meter, output test, noise suppression toggle) |
| Device hot-swap | Manual only (user switches in settings) |
| Noise suppression | Use LiveKit defaults |
| Input level display | Always visible during calls |
| Background voice | Stay connected when app loses focus |
| System tray | None for now |
| Auto-start | No |
| Keyboard shortcuts | Essential only (PTT, mute, deafen - global) |
| Quality indicator | Simple icon (green/yellow/red) |
| Reconnection | Auto-reconnect with UI overlay |
| Notifications | On/off toggle only |
| Error UX | Comprehensive (all error types with friendly messages) |
| Platform support | Cross-platform (macOS, Windows, Linux) |
| Instance limit | No limit |
| Launch behavior | Restore last active instance |

---

## 3. Multi-Instance Support

### 3.1 Overview

Users can connect to multiple Redoubt server instances (e.g., personal server, work server, community servers). Each instance is a completely separate deployment with its own:
- Authentication (separate accounts/tokens)
- Spaces and channels
- E2EE identity keys
- WebSocket connection

### 3.2 Data Model

```typescript
// src/types/instance.ts

interface Instance {
  id: string;              // UUID, generated client-side
  url: string;             // Base URL (e.g., "https://redoubt.example.com")
  name: string;            // Display name (fetched from server or user-set)
  iconUrl: string | null;  // Instance icon
  addedAt: string;         // ISO timestamp
  lastUsedAt: string;      // ISO timestamp
}

interface InstanceCredentials {
  instanceId: string;
  accessToken: string | null;
  refreshToken: string;
  expiresAt: string;
}

interface InstanceState {
  instances: Instance[];
  activeInstanceId: string | null;
  credentials: Map<string, InstanceCredentials>;
}
```

### 3.3 Storage Schema

```typescript
// Secure storage keys (per-instance)
`redoubt:instance:${instanceId}:refresh_token`  // Refresh token
`redoubt:instance:${instanceId}:e2ee_identity`  // E2EE identity key (encrypted)
`redoubt:instance:${instanceId}:e2ee_prekeys`   // E2EE prekeys (encrypted)

// Local storage keys (global)
`redoubt:instances`           // JSON array of Instance objects
`redoubt:active_instance`     // Active instance ID
`redoubt:settings`            // Global settings
```

### 3.4 Instance Store

```typescript
// src/store/instanceStore.ts

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface InstanceStore {
  // State
  instances: Instance[];
  activeInstanceId: string | null;

  // Actions
  addInstance: (url: string) => Promise<Instance>;
  removeInstance: (id: string) => Promise<void>;
  setActiveInstance: (id: string) => void;
  updateInstance: (id: string, updates: Partial<Instance>) => void;

  // Selectors
  getActiveInstance: () => Instance | null;
  getInstanceById: (id: string) => Instance | null;
}

export const useInstanceStore = create<InstanceStore>()(
  persist(
    (set, get) => ({
      instances: [],
      activeInstanceId: null,

      addInstance: async (url: string) => {
        // Validate URL is a valid Redoubt server
        const normalized = normalizeUrl(url);
        const serverInfo = await fetchServerInfo(normalized);

        const instance: Instance = {
          id: crypto.randomUUID(),
          url: normalized,
          name: serverInfo.name,
          iconUrl: serverInfo.iconUrl,
          addedAt: new Date().toISOString(),
          lastUsedAt: new Date().toISOString(),
        };

        set((state) => ({
          instances: [...state.instances, instance],
        }));

        return instance;
      },

      removeInstance: async (id: string) => {
        // Clear all stored data for this instance
        await clearInstanceData(id);

        set((state) => ({
          instances: state.instances.filter((i) => i.id !== id),
          activeInstanceId: state.activeInstanceId === id
            ? state.instances[0]?.id ?? null
            : state.activeInstanceId,
        }));
      },

      setActiveInstance: (id: string) => {
        set((state) => {
          const instance = state.instances.find((i) => i.id === id);
          if (instance) {
            instance.lastUsedAt = new Date().toISOString();
          }
          return { activeInstanceId: id };
        });
      },

      getActiveInstance: () => {
        const { instances, activeInstanceId } = get();
        return instances.find((i) => i.id === activeInstanceId) ?? null;
      },

      getInstanceById: (id: string) => {
        return get().instances.find((i) => i.id === id) ?? null;
      },
    }),
    {
      name: 'redoubt:instances',
    }
  )
);
```

### 3.5 Connection Manager

```typescript
// src/lib/connectionManager.ts

import { useInstanceStore } from '@/store/instanceStore';

interface ConnectionState {
  websocket: WebSocket | null;
  livekitRoom: Room | null;
  status: 'disconnected' | 'connecting' | 'connected' | 'reconnecting';
}

class ConnectionManager {
  private connections: Map<string, ConnectionState> = new Map();
  private activeInstanceId: string | null = null;

  // Connect to an instance (called when switching tabs)
  async connect(instanceId: string): Promise<void> {
    // Disconnect from previous active instance
    if (this.activeInstanceId && this.activeInstanceId !== instanceId) {
      await this.disconnect(this.activeInstanceId);
    }

    this.activeInstanceId = instanceId;

    const instance = useInstanceStore.getState().getInstanceById(instanceId);
    if (!instance) throw new Error('Instance not found');

    const state: ConnectionState = {
      websocket: null,
      livekitRoom: null,
      status: 'connecting',
    };
    this.connections.set(instanceId, state);

    // Establish WebSocket connection
    const credentials = await getCredentials(instanceId);
    state.websocket = await this.connectWebSocket(instance.url, credentials);
    state.status = 'connected';
  }

  // Disconnect from an instance
  async disconnect(instanceId: string): Promise<void> {
    const state = this.connections.get(instanceId);
    if (!state) return;

    // Leave voice if connected
    if (state.livekitRoom) {
      await state.livekitRoom.disconnect();
      state.livekitRoom = null;
    }

    // Close WebSocket
    if (state.websocket) {
      state.websocket.close();
      state.websocket = null;
    }

    state.status = 'disconnected';
  }

  // Get connection state for an instance
  getState(instanceId: string): ConnectionState | null {
    return this.connections.get(instanceId) ?? null;
  }

  // Check if voice is active on any instance
  getVoiceInstance(): string | null {
    for (const [id, state] of this.connections) {
      if (state.livekitRoom?.state === ConnectionState.Connected) {
        return id;
      }
    }
    return null;
  }

  // Join voice, handling cross-instance disconnect
  async joinVoice(instanceId: string, channelId: string): Promise<void> {
    const currentVoiceInstance = this.getVoiceInstance();

    if (currentVoiceInstance && currentVoiceInstance !== instanceId) {
      // Disconnect from voice on other instance first
      const otherState = this.connections.get(currentVoiceInstance);
      if (otherState?.livekitRoom) {
        await otherState.livekitRoom.disconnect();
        otherState.livekitRoom = null;
      }
    }

    // Join voice on requested instance
    const state = this.connections.get(instanceId);
    if (!state) throw new Error('Not connected to instance');

    // ... LiveKit join logic
  }
}

export const connectionManager = new ConnectionManager();
```

### 3.6 Instance-Scoped Stores

All existing stores need to be refactored to be instance-aware:

```typescript
// src/store/createInstanceStore.ts

// Factory to create instance-scoped stores
export function createInstanceScopedStore<T>(
  createStore: (instanceId: string) => T
) {
  const stores = new Map<string, T>();

  return {
    get(instanceId: string): T {
      if (!stores.has(instanceId)) {
        stores.set(instanceId, createStore(instanceId));
      }
      return stores.get(instanceId)!;
    },

    clear(instanceId: string): void {
      stores.delete(instanceId);
    },

    clearAll(): void {
      stores.clear();
    },
  };
}

// Example: Instance-scoped space store
export const spaceStores = createInstanceScopedStore((instanceId) =>
  create<SpaceState>((set, get) => ({
    spaces: [],
    // ... store implementation, all API calls use instanceId for routing
  }))
);

// Usage in components
function SpaceList() {
  const activeInstanceId = useInstanceStore((s) => s.activeInstanceId);
  const spaceStore = spaceStores.get(activeInstanceId!);
  const spaces = spaceStore((s) => s.spaces);
  // ...
}
```

### 3.7 Tab Bar Component

```typescript
// src/components/layout/InstanceTabBar.tsx

import { useInstanceStore } from '@/store/instanceStore';
import { Plus, X } from 'lucide-react';
import { cn } from '@/lib/utils';

export function InstanceTabBar() {
  const { instances, activeInstanceId, setActiveInstance, removeInstance } = useInstanceStore();
  const [showAddDialog, setShowAddDialog] = useState(false);

  return (
    <div className="flex items-center h-10 bg-zinc-900 border-b border-zinc-800">
      {instances.map((instance) => (
        <div
          key={instance.id}
          className={cn(
            'group flex items-center gap-2 px-3 h-full cursor-pointer',
            'border-r border-zinc-800 hover:bg-zinc-800',
            activeInstanceId === instance.id && 'bg-zinc-800 border-b-2 border-b-blue-500'
          )}
          onClick={() => setActiveInstance(instance.id)}
        >
          {instance.iconUrl ? (
            <img src={instance.iconUrl} className="w-5 h-5 rounded" alt="" />
          ) : (
            <div className="w-5 h-5 rounded bg-zinc-700 flex items-center justify-center text-xs">
              {instance.name.charAt(0).toUpperCase()}
            </div>
          )}
          <span className="text-sm truncate max-w-[120px]">{instance.name}</span>
          <button
            onClick={(e) => {
              e.stopPropagation();
              removeInstance(instance.id);
            }}
            className="opacity-0 group-hover:opacity-100 p-0.5 hover:bg-zinc-700 rounded"
          >
            <X className="w-3 h-3" />
          </button>
        </div>
      ))}

      <button
        onClick={() => setShowAddDialog(true)}
        className="flex items-center justify-center w-10 h-full hover:bg-zinc-800"
        title="Add instance"
      >
        <Plus className="w-4 h-4" />
      </button>

      <AddInstanceDialog open={showAddDialog} onOpenChange={setShowAddDialog} />
    </div>
  );
}
```

### 3.8 Add Instance Flow

```typescript
// src/components/instance/AddInstanceDialog.tsx

interface AddInstanceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function AddInstanceDialog({ open, onOpenChange }: AddInstanceDialogProps) {
  const [url, setUrl] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isValidating, setIsValidating] = useState(false);
  const [serverInfo, setServerInfo] = useState<ServerInfo | null>(null);
  const { addInstance, setActiveInstance } = useInstanceStore();

  const handleValidate = async () => {
    setIsValidating(true);
    setError(null);

    try {
      // Check if URL is a valid Redoubt server
      const info = await validateRedoubtServer(url);
      setServerInfo(info);
    } catch (err) {
      setError('Could not connect to server. Please check the URL.');
    } finally {
      setIsValidating(false);
    }
  };

  const handleAdd = async () => {
    try {
      const instance = await addInstance(url);
      setActiveInstance(instance.id);
      onOpenChange(false);
      // Navigate to login/register for this instance
      navigate(`/instance/${instance.id}/auth`);
    } catch (err) {
      setError('Failed to add instance');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add Redoubt Instance</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="url">Server URL</Label>
            <div className="flex gap-2">
              <Input
                id="url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://redoubt.example.com"
              />
              <Button onClick={handleValidate} disabled={isValidating || !url}>
                {isValidating ? 'Checking...' : 'Check'}
              </Button>
            </div>
            {error && <p className="text-sm text-red-400">{error}</p>}
          </div>

          {serverInfo && (
            <div className="p-4 bg-zinc-800 rounded-lg space-y-2">
              <div className="flex items-center gap-3">
                {serverInfo.iconUrl && (
                  <img src={serverInfo.iconUrl} className="w-10 h-10 rounded" alt="" />
                )}
                <div>
                  <p className="font-medium">{serverInfo.name}</p>
                  <p className="text-sm text-zinc-400">{serverInfo.version}</p>
                </div>
              </div>
              <Button onClick={handleAdd} className="w-full">
                Add Instance
              </Button>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
```

---

## 4. Audio Device Management

### 4.1 Device Selection

```typescript
// src/components/settings/AudioDeviceSettings.tsx

import { useAudioDevices } from '@/hooks/useAudioDevices';
import { useSettingsStore } from '@/store/settingsStore';

export function AudioDeviceSettings() {
  const { audioInputs, audioOutputs, refresh } = useAudioDevices();
  const {
    audioInputDevice,
    audioOutputDevice,
    setAudioInputDevice,
    setAudioOutputDevice,
  } = useSettingsStore();

  return (
    <div className="space-y-6">
      {/* Input Device */}
      <div className="space-y-2">
        <Label className="flex items-center gap-2">
          <Mic className="h-4 w-4" />
          Input Device
        </Label>
        <Select value={audioInputDevice || ''} onValueChange={setAudioInputDevice}>
          <SelectTrigger>
            <SelectValue placeholder="Select microphone" />
          </SelectTrigger>
          <SelectContent>
            {audioInputs.map((device) => (
              <SelectItem key={device.deviceId} value={device.deviceId}>
                {device.label || 'Unknown Device'}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Input Level Meter */}
        <InputLevelMeter deviceId={audioInputDevice} />
      </div>

      {/* Output Device */}
      <div className="space-y-2">
        <Label className="flex items-center gap-2">
          <Volume2 className="h-4 w-4" />
          Output Device
        </Label>
        <Select value={audioOutputDevice || ''} onValueChange={setAudioOutputDevice}>
          <SelectTrigger>
            <SelectValue placeholder="Select speakers" />
          </SelectTrigger>
          <SelectContent>
            {audioOutputs.map((device) => (
              <SelectItem key={device.deviceId} value={device.deviceId}>
                {device.label || 'Unknown Device'}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Output Test */}
        <OutputTestButton deviceId={audioOutputDevice} />
      </div>

      {/* Noise Suppression */}
      <div className="flex items-center justify-between">
        <div>
          <Label>Noise Suppression</Label>
          <p className="text-sm text-zinc-400">Reduce background noise</p>
        </div>
        <Switch
          checked={useSettingsStore((s) => s.noiseSuppression)}
          onCheckedChange={(v) => useSettingsStore.getState().setNoiseSuppression(v)}
        />
      </div>
    </div>
  );
}
```

### 4.2 Input Level Meter

```typescript
// src/components/audio/InputLevelMeter.tsx

import { useEffect, useRef, useState } from 'react';

interface InputLevelMeterProps {
  deviceId: string | null;
  className?: string;
}

export function InputLevelMeter({ deviceId, className }: InputLevelMeterProps) {
  const [level, setLevel] = useState(0);
  const analyserRef = useRef<AnalyserNode | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const animationRef = useRef<number>();

  useEffect(() => {
    if (!deviceId) return;

    let cancelled = false;

    async function setup() {
      try {
        const stream = await navigator.mediaDevices.getUserMedia({
          audio: { deviceId: { exact: deviceId } },
        });

        if (cancelled) {
          stream.getTracks().forEach((t) => t.stop());
          return;
        }

        streamRef.current = stream;

        const audioContext = new AudioContext();
        const source = audioContext.createMediaStreamSource(stream);
        const analyser = audioContext.createAnalyser();
        analyser.fftSize = 256;
        source.connect(analyser);
        analyserRef.current = analyser;

        const dataArray = new Uint8Array(analyser.frequencyBinCount);

        function updateLevel() {
          if (cancelled) return;
          analyser.getByteFrequencyData(dataArray);
          const average = dataArray.reduce((a, b) => a + b, 0) / dataArray.length;
          setLevel(average / 255);
          animationRef.current = requestAnimationFrame(updateLevel);
        }

        updateLevel();
      } catch (err) {
        console.error('Failed to access microphone:', err);
      }
    }

    setup();

    return () => {
      cancelled = true;
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
      }
      if (streamRef.current) {
        streamRef.current.getTracks().forEach((t) => t.stop());
      }
    };
  }, [deviceId]);

  return (
    <div className={cn('h-2 bg-zinc-800 rounded-full overflow-hidden', className)}>
      <div
        className="h-full bg-green-500 transition-all duration-75"
        style={{ width: `${level * 100}%` }}
      />
    </div>
  );
}
```

### 4.3 Output Test Button

```typescript
// src/components/audio/OutputTestButton.tsx

import { useState, useRef } from 'react';
import { Volume2 } from 'lucide-react';
import { Button } from '@/components/ui/button';

// Base64 encoded short test tone (or load from assets)
const TEST_TONE_URL = '/sounds/test-tone.mp3';

interface OutputTestButtonProps {
  deviceId: string | null;
}

export function OutputTestButton({ deviceId }: OutputTestButtonProps) {
  const [isPlaying, setIsPlaying] = useState(false);
  const audioRef = useRef<HTMLAudioElement | null>(null);

  const handleTest = async () => {
    if (isPlaying) return;

    setIsPlaying(true);

    try {
      const audio = new Audio(TEST_TONE_URL);
      audioRef.current = audio;

      // Set output device if supported
      if (deviceId && 'setSinkId' in audio) {
        await (audio as any).setSinkId(deviceId);
      }

      audio.onended = () => setIsPlaying(false);
      await audio.play();
    } catch (err) {
      console.error('Failed to play test tone:', err);
      setIsPlaying(false);
    }
  };

  return (
    <Button
      variant="outline"
      size="sm"
      onClick={handleTest}
      disabled={isPlaying}
    >
      <Volume2 className="h-4 w-4 mr-2" />
      {isPlaying ? 'Playing...' : 'Test'}
    </Button>
  );
}
```

---

## 5. Sound Effects System

### 5.1 Sound Manager

```typescript
// src/lib/soundManager.ts

type SoundType =
  | 'user_join'
  | 'user_leave'
  | 'self_mute'
  | 'self_unmute'
  | 'self_deafen'
  | 'self_undeafen'
  | 'disconnect';

const SOUND_FILES: Record<SoundType, string> = {
  user_join: '/sounds/join.mp3',
  user_leave: '/sounds/leave.mp3',
  self_mute: '/sounds/mute.mp3',
  self_unmute: '/sounds/unmute.mp3',
  self_deafen: '/sounds/deafen.mp3',
  self_undeafen: '/sounds/undeafen.mp3',
  disconnect: '/sounds/disconnect.mp3',
};

class SoundManager {
  private audioElements: Map<SoundType, HTMLAudioElement> = new Map();
  private enabled: boolean = true;
  private volume: number = 0.5;
  private outputDeviceId: string | null = null;

  constructor() {
    // Preload all sounds
    for (const [type, url] of Object.entries(SOUND_FILES)) {
      const audio = new Audio(url);
      audio.preload = 'auto';
      this.audioElements.set(type as SoundType, audio);
    }
  }

  setEnabled(enabled: boolean): void {
    this.enabled = enabled;
  }

  setVolume(volume: number): void {
    this.volume = Math.max(0, Math.min(1, volume));
    // Update all audio elements
    for (const audio of this.audioElements.values()) {
      audio.volume = this.volume;
    }
  }

  async setOutputDevice(deviceId: string | null): Promise<void> {
    this.outputDeviceId = deviceId;

    // Update output device on all audio elements
    for (const audio of this.audioElements.values()) {
      if (deviceId && 'setSinkId' in audio) {
        try {
          await (audio as any).setSinkId(deviceId);
        } catch (err) {
          console.error('Failed to set output device:', err);
        }
      }
    }
  }

  play(type: SoundType): void {
    if (!this.enabled) return;

    const audio = this.audioElements.get(type);
    if (!audio) return;

    // Clone and play to allow overlapping sounds
    const clone = audio.cloneNode() as HTMLAudioElement;
    clone.volume = this.volume;

    if (this.outputDeviceId && 'setSinkId' in clone) {
      (clone as any).setSinkId(this.outputDeviceId).then(() => {
        clone.play().catch(console.error);
      });
    } else {
      clone.play().catch(console.error);
    }
  }
}

export const soundManager = new SoundManager();
```

### 5.2 Integration with Voice Events

```typescript
// src/hooks/useVoiceSounds.ts

import { useEffect } from 'react';
import { soundManager } from '@/lib/soundManager';
import { useVoiceStore } from '@/store/voiceStore';
import { usePresenceStore } from '@/store/presenceStore';

export function useVoiceSounds() {
  const { isMuted, isDeafened, currentChannelId } = useVoiceStore();
  const prevMuted = useRef(isMuted);
  const prevDeafened = useRef(isDeafened);
  const prevChannelId = useRef(currentChannelId);

  // Self mute/unmute sounds
  useEffect(() => {
    if (prevMuted.current !== isMuted) {
      soundManager.play(isMuted ? 'self_mute' : 'self_unmute');
    }
    prevMuted.current = isMuted;
  }, [isMuted]);

  // Self deafen/undeafen sounds
  useEffect(() => {
    if (prevDeafened.current !== isDeafened) {
      soundManager.play(isDeafened ? 'self_deafen' : 'self_undeafen');
    }
    prevDeafened.current = isDeafened;
  }, [isDeafened]);

  // Disconnect sound
  useEffect(() => {
    if (prevChannelId.current && !currentChannelId) {
      soundManager.play('disconnect');
    }
    prevChannelId.current = currentChannelId;
  }, [currentChannelId]);

  // Other users join/leave - handled via presence events
  useEffect(() => {
    const unsubscribe = usePresenceStore.subscribe(
      (state) => state.presence,
      (presence, prevPresence) => {
        // Detect voice channel changes for other users
        for (const [userId, userPresence] of presence) {
          const prev = prevPresence.get(userId);

          // User joined voice
          if (userPresence.voiceChannelId && !prev?.voiceChannelId) {
            soundManager.play('user_join');
          }

          // User left voice
          if (!userPresence.voiceChannelId && prev?.voiceChannelId) {
            soundManager.play('user_leave');
          }
        }
      }
    );

    return unsubscribe;
  }, []);
}
```

---

## 6. Speaking Indicators

### 6.1 Speaking Visualizer Component

```typescript
// src/components/voice/SpeakingIndicator.tsx

import { useEffect, useState } from 'react';
import { cn } from '@/lib/utils';

interface SpeakingIndicatorProps {
  audioLevel: number;  // 0-1
  isActive: boolean;
  size?: 'sm' | 'md';
}

export function SpeakingIndicator({
  audioLevel,
  isActive,
  size = 'sm'
}: SpeakingIndicatorProps) {
  const barCount = size === 'sm' ? 3 : 5;
  const heights = generateBarHeights(audioLevel, barCount);

  if (!isActive) return null;

  return (
    <div className={cn(
      'flex items-end gap-0.5',
      size === 'sm' ? 'h-3' : 'h-4'
    )}>
      {heights.map((height, i) => (
        <div
          key={i}
          className={cn(
            'w-0.5 bg-green-500 rounded-full transition-all duration-75',
            size === 'sm' ? 'min-h-[2px]' : 'min-h-[3px]'
          )}
          style={{ height: `${height * 100}%` }}
        />
      ))}
    </div>
  );
}

function generateBarHeights(level: number, count: number): number[] {
  const heights: number[] = [];
  const baseHeight = 0.2;

  for (let i = 0; i < count; i++) {
    // Create wave pattern with slight variations
    const variation = Math.sin((Date.now() / 100) + i) * 0.1;
    const height = baseHeight + (level * 0.8) + variation;
    heights.push(Math.max(0.1, Math.min(1, height)));
  }

  return heights;
}
```

### 6.2 Self Speaking State Hook

```typescript
// src/hooks/useSelfSpeaking.ts

import { useEffect, useState, useRef } from 'react';
import { useVoiceStore } from '@/store/voiceStore';

export function useSelfSpeaking() {
  const [isSpeaking, setIsSpeaking] = useState(false);
  const [audioLevel, setAudioLevel] = useState(0);
  const room = useVoiceStore((s) => s.room);

  useEffect(() => {
    if (!room) {
      setIsSpeaking(false);
      setAudioLevel(0);
      return;
    }

    const localParticipant = room.localParticipant;

    const handleIsSpeakingChanged = (speaking: boolean) => {
      setIsSpeaking(speaking);
    };

    // Subscribe to audio level updates
    let animationFrame: number;
    const updateAudioLevel = () => {
      const audioTrack = localParticipant.audioTrackPublications.values().next().value;
      if (audioTrack?.track) {
        // Get audio level from track (LiveKit provides this)
        const level = audioTrack.track.audioLevel ?? 0;
        setAudioLevel(level);
      }
      animationFrame = requestAnimationFrame(updateAudioLevel);
    };

    localParticipant.on('isSpeakingChanged', handleIsSpeakingChanged);
    updateAudioLevel();

    return () => {
      localParticipant.off('isSpeakingChanged', handleIsSpeakingChanged);
      cancelAnimationFrame(animationFrame);
    };
  }, [room]);

  return { isSpeaking, audioLevel };
}
```

### 6.3 Participant with Speaking Indicator

```typescript
// src/components/voice/ParticipantItem.tsx

import { SpeakingIndicator } from './SpeakingIndicator';
import { useSelfSpeaking } from '@/hooks/useSelfSpeaking';
import { useRemoteSpeaking } from '@/hooks/useRemoteSpeaking';
import { Mic, MicOff, Headphones } from 'lucide-react';

interface ParticipantItemProps {
  participant: ParticipantInfo;
  isLocal: boolean;
}

export function ParticipantItem({ participant, isLocal }: ParticipantItemProps) {
  const selfSpeaking = useSelfSpeaking();
  const remoteSpeaking = useRemoteSpeaking(participant.identity);

  const { isSpeaking, audioLevel } = isLocal ? selfSpeaking : remoteSpeaking;

  return (
    <div className="flex items-center gap-2 px-2 py-1 rounded hover:bg-zinc-800">
      {/* Avatar */}
      <div className={cn(
        'w-8 h-8 rounded-full bg-zinc-700 flex items-center justify-center',
        isSpeaking && 'ring-2 ring-green-500'
      )}>
        {participant.avatarUrl ? (
          <img src={participant.avatarUrl} className="w-full h-full rounded-full" />
        ) : (
          <span className="text-sm">{participant.name.charAt(0)}</span>
        )}
      </div>

      {/* Name */}
      <span className="flex-1 truncate text-sm">
        {participant.name}
        {isLocal && ' (You)'}
      </span>

      {/* Speaking Indicator */}
      <SpeakingIndicator
        audioLevel={audioLevel}
        isActive={isSpeaking && !participant.isMuted}
      />

      {/* Mute/Deafen icons */}
      {participant.isMuted && (
        <MicOff className="w-4 h-4 text-red-400" />
      )}
      {participant.isDeafened && (
        <Headphones className="w-4 h-4 text-red-400" />
      )}
    </div>
  );
}
```

---

## 7. Voice Controls Polish

### 7.1 Enhanced Voice Controls Bar

```typescript
// src/components/voice/VoiceControlsBar.tsx

import { useSelfSpeaking } from '@/hooks/useSelfSpeaking';
import { useVoiceStore } from '@/store/voiceStore';
import { SpeakingIndicator } from './SpeakingIndicator';
import { ConnectionQualityIcon } from './ConnectionQualityIcon';
import {
  Mic, MicOff, Headphones, HeadphoneOff,
  Video, VideoOff, Monitor, PhoneOff, Settings
} from 'lucide-react';

export function VoiceControlsBar() {
  const {
    isMuted,
    isDeafened,
    isVideoEnabled,
    isScreenSharing,
    inputMode,
    isPttActive,
    connectionQuality,
    toggleMute,
    toggleDeafen,
    toggleVideo,
    toggleScreenShare,
    disconnect,
  } = useVoiceStore();

  const { isSpeaking, audioLevel } = useSelfSpeaking();

  return (
    <div className="flex items-center gap-3 px-4 py-3 bg-zinc-900 border-t border-zinc-800">
      {/* Self info with speaking indicator */}
      <div className="flex items-center gap-2 flex-1">
        <div className="flex items-center gap-2">
          <InputLevelIndicator level={audioLevel} />
          <span className="text-sm text-zinc-300">Voice Connected</span>
        </div>

        {inputMode === 'ptt' && (
          <span className={cn(
            'px-2 py-0.5 text-xs rounded',
            isPttActive ? 'bg-green-600' : 'bg-zinc-700'
          )}>
            PTT {isPttActive ? 'ON' : 'OFF'}
          </span>
        )}
      </div>

      {/* Control buttons */}
      <div className="flex items-center gap-1">
        <ControlButton
          active={!isMuted}
          onClick={toggleMute}
          icon={isMuted ? MicOff : Mic}
          activeColor="green"
          tooltip={isMuted ? 'Unmute' : 'Mute'}
        />

        <ControlButton
          active={!isDeafened}
          onClick={toggleDeafen}
          icon={isDeafened ? HeadphoneOff : Headphones}
          activeColor="green"
          tooltip={isDeafened ? 'Undeafen' : 'Deafen'}
        />

        <div className="w-px h-6 bg-zinc-700 mx-1" />

        <ControlButton
          active={isVideoEnabled}
          onClick={toggleVideo}
          icon={isVideoEnabled ? Video : VideoOff}
          tooltip={isVideoEnabled ? 'Turn off camera' : 'Turn on camera'}
        />

        <ControlButton
          active={isScreenSharing}
          onClick={toggleScreenShare}
          icon={Monitor}
          tooltip={isScreenSharing ? 'Stop sharing' : 'Share screen'}
        />

        <div className="w-px h-6 bg-zinc-700 mx-1" />

        <ConnectionQualityIcon quality={connectionQuality} />

        <ControlButton
          onClick={() => {/* open settings */}}
          icon={Settings}
          tooltip="Voice settings"
        />

        <ControlButton
          onClick={disconnect}
          icon={PhoneOff}
          destructive
          tooltip="Disconnect"
        />
      </div>
    </div>
  );
}
```

### 7.2 Input Level Indicator (Always Visible)

```typescript
// src/components/voice/InputLevelIndicator.tsx

interface InputLevelIndicatorProps {
  level: number;  // 0-1
}

export function InputLevelIndicator({ level }: InputLevelIndicatorProps) {
  // Show 5 bars that light up based on level
  const bars = 5;
  const litBars = Math.ceil(level * bars);

  return (
    <div className="flex items-end gap-0.5 h-4">
      {Array.from({ length: bars }).map((_, i) => (
        <div
          key={i}
          className={cn(
            'w-1 rounded-sm transition-colors',
            i < litBars ? 'bg-green-500' : 'bg-zinc-700'
          )}
          style={{ height: `${((i + 1) / bars) * 100}%` }}
        />
      ))}
    </div>
  );
}
```

### 7.3 Connection Quality Icon

```typescript
// src/components/voice/ConnectionQualityIcon.tsx

import { Signal, SignalLow, SignalMedium, SignalHigh } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';

type Quality = 'excellent' | 'good' | 'poor' | 'unknown';

interface ConnectionQualityIconProps {
  quality: Quality;
}

const qualityConfig: Record<Quality, { icon: any; color: string; label: string }> = {
  excellent: { icon: SignalHigh, color: 'text-green-500', label: 'Excellent' },
  good: { icon: SignalMedium, color: 'text-yellow-500', label: 'Good' },
  poor: { icon: SignalLow, color: 'text-red-500', label: 'Poor' },
  unknown: { icon: Signal, color: 'text-zinc-500', label: 'Unknown' },
};

export function ConnectionQualityIcon({ quality }: ConnectionQualityIconProps) {
  const { icon: Icon, color, label } = qualityConfig[quality];

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className={cn('p-2', color)}>
          <Icon className="w-4 h-4" />
        </div>
      </TooltipTrigger>
      <TooltipContent>
        <p>Connection: {label}</p>
      </TooltipContent>
    </Tooltip>
  );
}
```

---

## 8. Global Keyboard Shortcuts

### 8.1 Tauri Shortcut Registration

```rust
// src-tauri/src/commands/shortcuts.rs

use tauri::{AppHandle, Manager};
use tauri_plugin_global_shortcut::{GlobalShortcutExt, Shortcut, ShortcutState};

#[tauri::command]
pub async fn register_shortcut(
    app: AppHandle,
    action: String,
    shortcut: String,
) -> Result<(), String> {
    let shortcut: Shortcut = shortcut.parse().map_err(|e| format!("{}", e))?;

    app.global_shortcut()
        .on_shortcut(shortcut, move |app, _shortcut, event| {
            let event_name = match action.as_str() {
                "ptt" => match event.state() {
                    ShortcutState::Pressed => "ptt-pressed",
                    ShortcutState::Released => "ptt-released",
                },
                "toggle-mute" => {
                    if event.state() == ShortcutState::Pressed {
                        "toggle-mute"
                    } else {
                        return;
                    }
                }
                "toggle-deafen" => {
                    if event.state() == ShortcutState::Pressed {
                        "toggle-deafen"
                    } else {
                        return;
                    }
                }
                _ => return,
            };

            let _ = app.emit(event_name, ());
        })
        .map_err(|e| format!("{}", e))?;

    Ok(())
}

#[tauri::command]
pub async fn unregister_shortcut(app: AppHandle, shortcut: String) -> Result<(), String> {
    let shortcut: Shortcut = shortcut.parse().map_err(|e| format!("{}", e))?;

    app.global_shortcut()
        .unregister(shortcut)
        .map_err(|e| format!("{}", e))?;

    Ok(())
}

#[tauri::command]
pub async fn unregister_all_shortcuts(app: AppHandle) -> Result<(), String> {
    app.global_shortcut()
        .unregister_all()
        .map_err(|e| format!("{}", e))?;

    Ok(())
}
```

### 8.2 Shortcut Hook

```typescript
// src/hooks/useGlobalShortcuts.ts

import { useEffect } from 'react';
import { listen } from '@tauri-apps/api/event';
import { invoke } from '@tauri-apps/api/core';
import { useVoiceStore } from '@/store/voiceStore';
import { useSettingsStore } from '@/store/settingsStore';

export function useGlobalShortcuts() {
  const { toggleMute, toggleDeafen, setPttActive, currentChannelId } = useVoiceStore();
  const { shortcuts } = useSettingsStore();

  // Register shortcuts on mount
  useEffect(() => {
    async function register() {
      try {
        if (shortcuts.ptt) {
          await invoke('register_shortcut', { action: 'ptt', shortcut: shortcuts.ptt });
        }
        if (shortcuts.toggleMute) {
          await invoke('register_shortcut', { action: 'toggle-mute', shortcut: shortcuts.toggleMute });
        }
        if (shortcuts.toggleDeafen) {
          await invoke('register_shortcut', { action: 'toggle-deafen', shortcut: shortcuts.toggleDeafen });
        }
      } catch (err) {
        console.error('Failed to register shortcuts:', err);
      }
    }

    register();

    return () => {
      invoke('unregister_all_shortcuts').catch(console.error);
    };
  }, [shortcuts]);

  // Listen for shortcut events
  useEffect(() => {
    if (!currentChannelId) return;

    const unlisten: (() => void)[] = [];

    listen('ptt-pressed', () => {
      setPttActive(true);
    }).then((fn) => unlisten.push(fn));

    listen('ptt-released', () => {
      setPttActive(false);
    }).then((fn) => unlisten.push(fn));

    listen('toggle-mute', () => {
      toggleMute();
    }).then((fn) => unlisten.push(fn));

    listen('toggle-deafen', () => {
      toggleDeafen();
    }).then((fn) => unlisten.push(fn));

    return () => {
      unlisten.forEach((fn) => fn());
    };
  }, [currentChannelId, toggleMute, toggleDeafen, setPttActive]);
}
```

### 8.3 Shortcut Settings UI

```typescript
// src/components/settings/ShortcutSettings.tsx

import { useState } from 'react';
import { useSettingsStore } from '@/store/settingsStore';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';

export function ShortcutSettings() {
  const { shortcuts, setShortcut } = useSettingsStore();

  return (
    <div className="space-y-4">
      <ShortcutInput
        label="Push to Talk"
        value={shortcuts.ptt}
        onChange={(v) => setShortcut('ptt', v)}
      />
      <ShortcutInput
        label="Toggle Mute"
        value={shortcuts.toggleMute}
        onChange={(v) => setShortcut('toggleMute', v)}
      />
      <ShortcutInput
        label="Toggle Deafen"
        value={shortcuts.toggleDeafen}
        onChange={(v) => setShortcut('toggleDeafen', v)}
      />
    </div>
  );
}

interface ShortcutInputProps {
  label: string;
  value: string | null;
  onChange: (value: string | null) => void;
}

function ShortcutInput({ label, value, onChange }: ShortcutInputProps) {
  const [isRecording, setIsRecording] = useState(false);

  const handleRecord = () => {
    setIsRecording(true);

    const handleKeyDown = (e: KeyboardEvent) => {
      e.preventDefault();

      const parts: string[] = [];
      if (e.ctrlKey) parts.push('Ctrl');
      if (e.altKey) parts.push('Alt');
      if (e.shiftKey) parts.push('Shift');
      if (e.metaKey) parts.push('Super');

      if (e.key && !['Control', 'Alt', 'Shift', 'Meta'].includes(e.key)) {
        parts.push(e.key.toUpperCase());
      }

      if (parts.length > 0) {
        onChange(parts.join('+'));
      }

      setIsRecording(false);
      window.removeEventListener('keydown', handleKeyDown);
    };

    window.addEventListener('keydown', handleKeyDown);
  };

  return (
    <div className="flex items-center justify-between">
      <Label>{label}</Label>
      <div className="flex items-center gap-2">
        <div className="px-3 py-1.5 bg-zinc-800 rounded text-sm min-w-[120px] text-center">
          {isRecording ? 'Press keys...' : value || 'Not set'}
        </div>
        <Button variant="outline" size="sm" onClick={handleRecord}>
          {isRecording ? 'Recording' : 'Set'}
        </Button>
        {value && (
          <Button variant="ghost" size="sm" onClick={() => onChange(null)}>
            Clear
          </Button>
        )}
      </div>
    </div>
  );
}
```

---

## 9. Connection & Reconnection

### 9.1 Reconnection Manager

```typescript
// src/lib/reconnectionManager.ts

type ConnectionType = 'websocket' | 'livekit';

interface ReconnectionState {
  type: ConnectionType;
  status: 'disconnected' | 'reconnecting' | 'connected' | 'failed';
  attempts: number;
  maxAttempts: number;
  nextRetryAt: Date | null;
}

const BACKOFF_DELAYS = [1000, 2000, 5000, 10000, 30000];
const MAX_ATTEMPTS = 10;

class ReconnectionManager {
  private states: Map<string, ReconnectionState> = new Map();
  private listeners: Set<(states: Map<string, ReconnectionState>) => void> = new Set();

  getState(key: string): ReconnectionState | null {
    return this.states.get(key) ?? null;
  }

  async reconnect(
    key: string,
    type: ConnectionType,
    connectFn: () => Promise<void>
  ): Promise<void> {
    const state: ReconnectionState = {
      type,
      status: 'reconnecting',
      attempts: 0,
      maxAttempts: MAX_ATTEMPTS,
      nextRetryAt: null,
    };
    this.states.set(key, state);
    this.notify();

    while (state.attempts < MAX_ATTEMPTS) {
      try {
        await connectFn();
        state.status = 'connected';
        state.nextRetryAt = null;
        this.notify();
        return;
      } catch (err) {
        state.attempts++;

        if (state.attempts >= MAX_ATTEMPTS) {
          state.status = 'failed';
          this.notify();
          throw new Error(`Failed to reconnect after ${MAX_ATTEMPTS} attempts`);
        }

        const delay = BACKOFF_DELAYS[Math.min(state.attempts - 1, BACKOFF_DELAYS.length - 1)];
        state.nextRetryAt = new Date(Date.now() + delay);
        this.notify();

        await new Promise((resolve) => setTimeout(resolve, delay));
      }
    }
  }

  reset(key: string): void {
    this.states.delete(key);
    this.notify();
  }

  subscribe(listener: (states: Map<string, ReconnectionState>) => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private notify(): void {
    for (const listener of this.listeners) {
      listener(new Map(this.states));
    }
  }
}

export const reconnectionManager = new ReconnectionManager();
```

### 9.2 Reconnection Overlay

```typescript
// src/components/overlay/ReconnectionOverlay.tsx

import { useEffect, useState } from 'react';
import { reconnectionManager } from '@/lib/reconnectionManager';
import { Loader2, WifiOff, RefreshCw } from 'lucide-react';
import { Button } from '@/components/ui/button';

export function ReconnectionOverlay() {
  const [states, setStates] = useState(reconnectionManager.states);
  const [countdown, setCountdown] = useState<number | null>(null);

  useEffect(() => {
    return reconnectionManager.subscribe(setStates);
  }, []);

  // Find active reconnection
  const activeReconnection = Array.from(states.values()).find(
    (s) => s.status === 'reconnecting' || s.status === 'failed'
  );

  // Update countdown
  useEffect(() => {
    if (!activeReconnection?.nextRetryAt) {
      setCountdown(null);
      return;
    }

    const updateCountdown = () => {
      const remaining = Math.max(0, activeReconnection.nextRetryAt!.getTime() - Date.now());
      setCountdown(Math.ceil(remaining / 1000));
    };

    updateCountdown();
    const interval = setInterval(updateCountdown, 100);
    return () => clearInterval(interval);
  }, [activeReconnection?.nextRetryAt]);

  if (!activeReconnection) return null;

  return (
    <div className="fixed inset-0 bg-black/80 flex items-center justify-center z-50">
      <div className="bg-zinc-900 rounded-lg p-6 max-w-sm text-center space-y-4">
        {activeReconnection.status === 'reconnecting' ? (
          <>
            <Loader2 className="w-12 h-12 mx-auto text-blue-500 animate-spin" />
            <h2 className="text-lg font-semibold">Reconnecting...</h2>
            <p className="text-sm text-zinc-400">
              Attempt {activeReconnection.attempts} of {activeReconnection.maxAttempts}
            </p>
            {countdown !== null && countdown > 0 && (
              <p className="text-sm text-zinc-500">
                Retrying in {countdown}s
              </p>
            )}
          </>
        ) : (
          <>
            <WifiOff className="w-12 h-12 mx-auto text-red-500" />
            <h2 className="text-lg font-semibold">Connection Failed</h2>
            <p className="text-sm text-zinc-400">
              Unable to connect after {activeReconnection.maxAttempts} attempts
            </p>
            <Button onClick={() => window.location.reload()}>
              <RefreshCw className="w-4 h-4 mr-2" />
              Retry
            </Button>
          </>
        )}
      </div>
    </div>
  );
}
```

---

## 10. Error Handling UI

### 10.1 Error Types

```typescript
// src/lib/errors.ts

export type ErrorType =
  | 'network_disconnected'
  | 'server_unreachable'
  | 'auth_expired'
  | 'microphone_permission_denied'
  | 'camera_permission_denied'
  | 'device_access_failed'
  | 'rate_limited'
  | 'validation_error'
  | 'api_error';

interface AppError {
  type: ErrorType;
  message: string;
  details?: string;
  retryAfter?: number;  // For rate limiting
  fieldErrors?: { field: string; message: string }[];  // For validation
}

export function createAppError(type: ErrorType, details?: any): AppError {
  switch (type) {
    case 'network_disconnected':
      return { type, message: 'Network connection lost', details: 'Check your internet connection' };
    case 'server_unreachable':
      return { type, message: 'Server unreachable', details: 'The server may be down or your connection may be blocked' };
    case 'auth_expired':
      return { type, message: 'Session expired', details: 'Please log in again' };
    case 'microphone_permission_denied':
      return { type, message: 'Microphone access denied', details: 'Please allow microphone access in your system settings' };
    case 'camera_permission_denied':
      return { type, message: 'Camera access denied', details: 'Please allow camera access in your system settings' };
    case 'device_access_failed':
      return { type, message: 'Device access failed', details: details?.message || 'Could not access the audio/video device' };
    case 'rate_limited':
      return { type, message: 'Too many requests', details: `Please wait ${details?.retryAfter || 'a moment'} before trying again`, retryAfter: details?.retryAfter };
    case 'validation_error':
      return { type, message: 'Validation error', fieldErrors: details?.errors };
    case 'api_error':
      return { type, message: details?.title || 'An error occurred', details: details?.detail };
    default:
      return { type: 'api_error', message: 'An unexpected error occurred' };
  }
}
```

### 10.2 Error Toast Component

```typescript
// src/components/ui/error-toast.tsx

import { useEffect } from 'react';
import { X, AlertCircle, WifiOff, Clock, ShieldX } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { AppError } from '@/lib/errors';

interface ErrorToastProps {
  error: AppError;
  onDismiss: () => void;
}

const iconMap: Record<string, any> = {
  network_disconnected: WifiOff,
  server_unreachable: WifiOff,
  auth_expired: ShieldX,
  rate_limited: Clock,
  default: AlertCircle,
};

export function ErrorToast({ error, onDismiss }: ErrorToastProps) {
  const Icon = iconMap[error.type] || iconMap.default;

  // Auto-dismiss after 5 seconds (unless rate limited with timer)
  useEffect(() => {
    if (error.type === 'rate_limited' && error.retryAfter) {
      return; // Don't auto-dismiss rate limit errors
    }

    const timer = setTimeout(onDismiss, 5000);
    return () => clearTimeout(timer);
  }, [error, onDismiss]);

  return (
    <div className={cn(
      'flex items-start gap-3 p-4 rounded-lg shadow-lg',
      'bg-red-950 border border-red-900 text-red-100',
      'animate-in slide-in-from-right-full'
    )}>
      <Icon className="w-5 h-5 text-red-400 flex-shrink-0 mt-0.5" />

      <div className="flex-1 space-y-1">
        <p className="font-medium">{error.message}</p>
        {error.details && (
          <p className="text-sm text-red-300">{error.details}</p>
        )}
        {error.fieldErrors && (
          <ul className="text-sm text-red-300 list-disc list-inside">
            {error.fieldErrors.map((e, i) => (
              <li key={i}>{e.field}: {e.message}</li>
            ))}
          </ul>
        )}
      </div>

      <button onClick={onDismiss} className="text-red-400 hover:text-red-300">
        <X className="w-4 h-4" />
      </button>
    </div>
  );
}
```

### 10.3 Error Store

```typescript
// src/store/errorStore.ts

import { create } from 'zustand';
import type { AppError } from '@/lib/errors';

interface ErrorStore {
  errors: AppError[];
  addError: (error: AppError) => void;
  dismissError: (index: number) => void;
  clearAll: () => void;
}

export const useErrorStore = create<ErrorStore>((set) => ({
  errors: [],

  addError: (error) => set((state) => ({
    errors: [...state.errors, error],
  })),

  dismissError: (index) => set((state) => ({
    errors: state.errors.filter((_, i) => i !== index),
  })),

  clearAll: () => set({ errors: [] }),
}));
```

---

## 11. Settings Panel

### 11.1 Settings Dialog

```typescript
// src/components/settings/SettingsDialog.tsx

import { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { AudioDeviceSettings } from './AudioDeviceSettings';
import { VoiceSettings } from './VoiceSettings';
import { ShortcutSettings } from './ShortcutSettings';
import { NotificationSettings } from './NotificationSettings';
import { AboutSection } from './AboutSection';
import { Mic, Keyboard, Bell, Info, Volume2 } from 'lucide-react';

interface SettingsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function SettingsDialog({ open, onOpenChange }: SettingsDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[80vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>Settings</DialogTitle>
        </DialogHeader>

        <Tabs defaultValue="audio" className="flex-1 flex flex-col overflow-hidden">
          <TabsList className="grid grid-cols-5 w-full">
            <TabsTrigger value="audio" className="flex items-center gap-2">
              <Mic className="w-4 h-4" />
              <span className="hidden sm:inline">Audio</span>
            </TabsTrigger>
            <TabsTrigger value="voice" className="flex items-center gap-2">
              <Volume2 className="w-4 h-4" />
              <span className="hidden sm:inline">Voice</span>
            </TabsTrigger>
            <TabsTrigger value="shortcuts" className="flex items-center gap-2">
              <Keyboard className="w-4 h-4" />
              <span className="hidden sm:inline">Shortcuts</span>
            </TabsTrigger>
            <TabsTrigger value="notifications" className="flex items-center gap-2">
              <Bell className="w-4 h-4" />
              <span className="hidden sm:inline">Notifications</span>
            </TabsTrigger>
            <TabsTrigger value="about" className="flex items-center gap-2">
              <Info className="w-4 h-4" />
              <span className="hidden sm:inline">About</span>
            </TabsTrigger>
          </TabsList>

          <div className="flex-1 overflow-y-auto p-4">
            <TabsContent value="audio">
              <AudioDeviceSettings />
            </TabsContent>
            <TabsContent value="voice">
              <VoiceSettings />
            </TabsContent>
            <TabsContent value="shortcuts">
              <ShortcutSettings />
            </TabsContent>
            <TabsContent value="notifications">
              <NotificationSettings />
            </TabsContent>
            <TabsContent value="about">
              <AboutSection />
            </TabsContent>
          </div>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
```

### 11.2 Voice Settings

```typescript
// src/components/settings/VoiceSettings.tsx

import { useSettingsStore } from '@/store/settingsStore';
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Switch } from '@/components/ui/switch';
import { Slider } from '@/components/ui/slider';

export function VoiceSettings() {
  const {
    inputMode,
    setInputMode,
    soundEffectsEnabled,
    setSoundEffectsEnabled,
    soundEffectsVolume,
    setSoundEffectsVolume,
  } = useSettingsStore();

  return (
    <div className="space-y-6">
      {/* Input Mode */}
      <div className="space-y-3">
        <Label>Input Mode</Label>
        <RadioGroup value={inputMode} onValueChange={setInputMode}>
          <div className="flex items-center space-x-2">
            <RadioGroupItem value="vad" id="vad" />
            <Label htmlFor="vad" className="font-normal">
              Voice Activity Detection
              <span className="block text-sm text-zinc-400">
                Automatically transmit when you speak
              </span>
            </Label>
          </div>
          <div className="flex items-center space-x-2">
            <RadioGroupItem value="ptt" id="ptt" />
            <Label htmlFor="ptt" className="font-normal">
              Push to Talk
              <span className="block text-sm text-zinc-400">
                Hold a key to transmit
              </span>
            </Label>
          </div>
        </RadioGroup>
      </div>

      {/* Sound Effects */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <div>
            <Label>Sound Effects</Label>
            <p className="text-sm text-zinc-400">
              Play sounds for join, leave, mute, etc.
            </p>
          </div>
          <Switch
            checked={soundEffectsEnabled}
            onCheckedChange={setSoundEffectsEnabled}
          />
        </div>

        {soundEffectsEnabled && (
          <div className="space-y-2">
            <Label className="text-sm">Volume</Label>
            <Slider
              value={[soundEffectsVolume * 100]}
              onValueChange={([v]) => setSoundEffectsVolume(v / 100)}
              max={100}
              step={1}
            />
          </div>
        )}
      </div>
    </div>
  );
}
```

### 11.3 Notification Settings

```typescript
// src/components/settings/NotificationSettings.tsx

import { useSettingsStore } from '@/store/settingsStore';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';

export function NotificationSettings() {
  const {
    notificationsEnabled,
    setNotificationsEnabled,
  } = useSettingsStore();

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <Label>Desktop Notifications</Label>
          <p className="text-sm text-zinc-400">
            Show desktop notifications for messages and mentions
          </p>
        </div>
        <Switch
          checked={notificationsEnabled}
          onCheckedChange={setNotificationsEnabled}
        />
      </div>
    </div>
  );
}
```

---

## 12. Cross-Platform Validation

### 12.1 Platform Testing Matrix

| Feature | macOS (Apple Silicon) | macOS (Intel) | Windows 10/11 | Linux (Ubuntu) |
|---------|----------------------|---------------|---------------|----------------|
| Audio device selection | | | | |
| Input level meter | | | | |
| Output test | | | | |
| Global shortcuts | | | | |
| Sound effects | | | | |
| Voice connection | | | | |
| Video | | | | |
| Screen share | | | | |
| Reconnection | | | | |
| Multi-instance | | | | |

### 12.2 Known Platform Differences

Document any platform-specific behaviors discovered during testing:

```markdown
## macOS
- Global shortcuts require Accessibility permission
- Audio device IDs may change between sessions

## Windows
- Some audio devices report duplicate entries
- Global shortcuts work without special permissions

## Linux
- PulseAudio vs PipeWire may affect device enumeration
- Global shortcuts may require X11 (Wayland limitations)
```

---

## 13. Implementation Tasks

### Milestone 1: Multi-Instance Infrastructure

**Data Model & Storage:**
- [x] Define Instance model and types
- [x] Create instance registry storage
- [x] Implement per-instance secure token storage
- [ ] Implement per-instance E2EE key storage (for Phase 4)
- [x] Implement instance metadata cache

**State Management Refactor:**
- [x] Create instanceStore
- [x] Refactor authStore to be instance-scoped
- [x] Refactor spaceStore to be instance-scoped
- [x] Refactor channelStore to be instance-scoped
- [x] Refactor chatStore to be instance-scoped
- [x] Refactor presenceStore to be instance-scoped
- [x] Refactor voiceStore (single active instance for voice)
- [x] Create factory for instance-scoped stores
- [x] Implement store switching on instance change

**Connection Management:**
- [x] Create ConnectionManager class
- [x] Implement on-demand WebSocket connection
- [x] Implement WebSocket disconnect on instance switch
- [x] Implement connection state tracking per instance
- [ ] Handle reconnection for active instance only
- [x] Create API client factory for instance URLs

**Voice Instance Handling:**
- [ ] Detect voice join on different instance
- [ ] Auto-disconnect from current voice
- [ ] Show confirmation for active voice call
- [x] Track active voice instance

### Milestone 2: Multi-Instance UI

**Tab Bar:**
- [x] Create InstanceTabBar component
- [x] Instance icon + name display
- [x] Active instance highlighting
- [x] "Add Instance" button
- [x] Tab context menu (remove)
- [ ] Tab reordering (drag and drop)

**Instance Management:**
- [x] Add Instance dialog
- [x] URL validation (check if valid Redoubt server)
- [x] Login/Register choice after adding
- [x] Remove instance confirmation
- [ ] Instance settings (rename)

**Navigation:**
- [x] Update routes to be instance-scoped
- [x] Remember last space/channel per instance
- [x] Persist and restore last active instance on launch

### Milestone 3: Audio Device Management

- [x] Implement audio input device dropdown
- [x] Implement audio output device dropdown
- [x] Persist device selections
- [ ] Implement input level meter
- [ ] Implement output test button
- [ ] Implement noise suppression toggle
- [x] Handle device enumeration
- [ ] Handle device disconnection gracefully

### Milestone 4: Sound Effects

- [x] Source/create sound files (join, leave, mute, unmute, disconnect) - join/leave done
- [x] Implement SoundManager class
- [x] Play sound on user joins voice (self)
- [x] Play sound on user leaves voice (self)
- [x] Play sound on other user joins
- [x] Play sound on other user leaves
- [ ] Play sound on mute/unmute toggle
- [ ] Play sound on disconnect
- [x] Add volume control for sounds
- [x] Add toggle to disable sounds
- [x] Route sounds through selected output device

### Milestone 5: Speaking Indicators

- [ ] Implement audio level detection (self)
- [x] Create SpeakingIndicator component (ring around avatar when speaking)
- [ ] Display visualizer next to own name
- [x] Receive speaking state from LiveKit
- [x] Display indicator for other participants
- [x] Smooth animation (not jumpy)
- [ ] Threshold tuning for background noise

### Milestone 6: Voice Controls Polish

- [ ] Input level indicator (always visible during calls)
- [x] Mute button visual feedback
- [x] Deafen button visual feedback
- [x] Connection quality icon (green/yellow/red)
- [ ] PTT active indicator
- [x] Enhanced VoiceControlsBar component

### Milestone 7: Global Keyboard Shortcuts

- [x] Implement Tauri shortcut registration (Rust)
- [x] Implement PTT key (press/release events)
- [x] Implement toggle mute shortcut
- [x] Implement toggle deafen shortcut
- [x] Shortcuts work when app not focused
- [ ] Settings UI for configuring shortcuts
- [ ] Handle shortcut conflicts

### Milestone 8: Reconnection Handling

- [ ] Detect WebSocket disconnection
- [ ] Detect LiveKit disconnection
- [ ] Create ReconnectionManager
- [ ] Show reconnecting overlay UI
- [ ] Implement exponential backoff
- [ ] Show connection status during reconnection
- [ ] Auto-rejoin voice after reconnect
- [ ] Handle auth expiry during reconnect
- [ ] Toast on successful reconnect

### Milestone 9: Error Handling UI

- [ ] Define error types
- [ ] Network disconnected state
- [ ] Server unreachable error
- [ ] Auth token expired (redirect to login)
- [ ] Microphone permission denied
- [ ] Camera permission denied
- [ ] Device access failed
- [ ] Rate limit exceeded (show timer)
- [ ] Validation errors (inline)
- [ ] Generic API error handler
- [ ] Error toast component
- [ ] Error store

### Milestone 10: Settings Panel

- [x] Create SettingsDialog component (AudioSettings dialog)
- [x] Audio settings tab (device selection, input mode)
- [x] Voice settings tab (input mode, sounds) - partial (input mode only)
- [ ] Shortcuts settings tab
- [ ] Notifications settings tab (on/off)
- [ ] About section (version, links)

### Milestone 11: Cross-Platform Testing

- [ ] Test on macOS (Apple Silicon)
- [ ] Test on macOS (Intel)
- [ ] Test on Windows 10/11
- [ ] Test on Linux (Ubuntu/Fedora)
- [ ] Document platform-specific behaviors
- [ ] Verify audio devices on all platforms
- [ ] Verify global shortcuts on all platforms

---

## 14. Feature Validation Checklist

### Authentication
- [x] Login works
- [x] Registration with invite code works
- [ ] Email verification flow works
- [ ] Password reset works
- [x] Token refresh works
- [x] Logout works

### Multi-Instance
- [x] Can add new instance
- [x] Can switch between instances
- [ ] Voice disconnects when switching to different instance voice
- [x] Credentials persist per instance
- [x] Instance removal clears all data
- [x] Last active instance restored on launch

### Spaces & Channels
- [x] Space list displays correctly
- [x] Channel list displays correctly
- [x] Create space works (admin)
- [x] Create channel works (admin)
- [x] Channel reordering works
- [x] Member list displays correctly

### Voice
- [x] Join voice channel works
- [x] Leave voice channel works
- [x] Audio transmitted and received
- [x] Mute works (no audio sent)
- [x] Deafen works (no audio received + muted)
- [x] PTT mode works
- [x] VAD mode works
- [x] Video toggle works
- [x] Screen share works
- [x] Device selection persists
- [x] Speaking indicator shows correctly
- [x] Connection quality indicator accurate
- [ ] Input level meter works in settings
- [ ] Output test works
- [ ] Noise suppression toggle works

### Sound Effects
- [x] Join sound plays (self)
- [x] Leave sound plays (self)
- [x] Join sound plays (others)
- [x] Leave sound plays (others)
- [ ] Mute/unmute sounds play
- [ ] Disconnect sound plays
- [x] Volume control works
- [x] Disable toggle works

### Keyboard Shortcuts
- [x] PTT shortcut works (press and release)
- [x] Toggle mute shortcut works
- [x] Toggle deafen shortcut works
- [x] Shortcuts work when app not focused
- [ ] Shortcut configuration works

### Text Chat (if Phase 3 complete)
- [x] Messages load on channel open
- [x] Send message works
- [x] Messages appear in real-time
- [x] Edit message works
- [x] Delete message works
- [x] Reactions work
- [ ] Threading works
- [x] Markdown renders correctly
- [ ] Unread indicators work
- [x] Typing indicators work

### Connection & Errors
- [ ] Graceful handling of network loss
- [ ] Reconnection overlay shows
- [ ] Auto-reconnect works
- [ ] Rejoins voice after reconnect
- [ ] Permission denied handled
- [ ] Rate limit shown clearly
- [ ] API errors shown with friendly messages

### Settings
- [x] Audio device settings work
- [x] Voice settings work (input mode)
- [ ] Shortcut settings work
- [ ] Notification toggle works

---

## Summary

This phase ensures the Tauri desktop client is production-ready with:

- **Multi-instance support** — Connect to multiple Redoubt servers with tabbed UI
- **Full audio device management** — Input/output selection, level meters, output test
- **Sound effects** — Essential audio feedback for voice actions
- **Speaking indicators** — Visual feedback when users are speaking
- **Voice controls polish** — Input level display, connection quality, PTT indicator
- **Global keyboard shortcuts** — PTT, mute, deafen work outside app focus
- **Robust reconnection** — Auto-reconnect with UI feedback
- **Comprehensive error handling** — User-friendly error states for all scenarios
- **Centralized settings** — All configuration in one place
- **Cross-platform validation** — Works reliably on macOS, Windows, Linux

This parallel track can be worked on alongside other phases to ensure the desktop client is polished before shipping.
