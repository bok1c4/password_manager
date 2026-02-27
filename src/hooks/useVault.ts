import { create } from 'zustand';

export interface Entry {
  id: string;
  site: string;
  username: string;
  password?: string;
  encrypted_password?: string;
  notes?: string;
  created_at?: string;
  updated_at?: string;
}

export interface Device {
  id: string;
  name: string;
  public_key?: string;
  fingerprint?: string;
  trusted: boolean;
  created_at?: string;
}

export interface SyncStatus {
  initialized: boolean;
  last_sync: number | null;
  pending_changes: number;
}

export interface VaultInfo {
  name: string;
  active: boolean;
  initialized: boolean;
}

export interface P2PStatus {
  running: boolean;
  peer_id: string | null;
}

export interface Peer {
  peer_id: string;
  name: string;
  connected: boolean;
  last_seen: number;
}

export interface ApprovalRequest {
  device_id: string;
  name: string;
  fingerprint: string;
  requested_at: number;
}

interface VaultState {
  vaults: VaultInfo[];
  activeVault: string;
  initialized: boolean;
  unlocked: boolean;
  entries: Entry[];
  devices: Device[];
  syncStatus: SyncStatus;
  p2pStatus: P2PStatus;
  peers: Peer[];
  approvals: ApprovalRequest[];
  loading: boolean;
  error: string | null;
  
  checkInitialized: () => Promise<void>;
  fetchVaults: () => Promise<void>;
  switchVault: (vault: string) => Promise<void>;
  createVault: (name: string) => Promise<void>;
  initVault: (name: string, password: string, vault?: string) => Promise<void>;
  unlock: (password: string) => Promise<void>;
  lock: () => Promise<void>;
  fetchEntries: () => Promise<void>;
  fetchEntriesOnce: () => Promise<void>;
  addEntry: (site: string, username: string, password: string, notes: string) => Promise<void>;
  updateEntry: (id: string, site: string, username: string, password: string, notes: string) => Promise<void>;
  deleteEntry: (id: string) => Promise<void>;
  getPassword: (id: string) => Promise<string>;
  fetchDevices: () => Promise<void>;
  fetchSyncStatus: () => Promise<void>;
  syncNow: () => Promise<void>;
  syncPush: () => Promise<void>;
  syncPull: () => Promise<void>;
  initSync: (remote: string) => Promise<void>;
  generatePassword: (length: number) => Promise<string>;
  fetchP2PStatus: () => Promise<void>;
  startP2P: () => Promise<void>;
  stopP2P: () => Promise<void>;
  connectPeer: (address: string) => Promise<void>;
  disconnectPeer: (peerId: string) => Promise<void>;
  fetchPeers: () => Promise<void>;
  fetchApprovals: () => Promise<void>;
  approveDevice: (deviceId: string) => Promise<void>;
  rejectDevice: (deviceId: string, reason: string) => Promise<void>;
}

export const useVault = create<VaultState>((set, get) => ({
  vaults: [],
  activeVault: '',
  initialized: false,
  unlocked: false,
  entries: [],
  devices: [],
  syncStatus: { initialized: false, last_sync: null, pending_changes: 0 },
  p2pStatus: { running: false, peer_id: null },
  peers: [],
  approvals: [],
  loading: false,
  error: null,

  checkInitialized: async () => {
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      const result = await invoke<boolean>('is_initialized');
      set({ initialized: result });
      await get().fetchVaults();
    } catch (e) {
      console.error('Failed to check initialized:', e);
    }
  },

  fetchVaults: async () => {
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      const vaults = await invoke<VaultInfo[]>('get_vaults');
      set({ vaults, activeVault: vaults.find(v => v.active)?.name || '' });
    } catch (e) {
      console.error('Failed to fetch vaults:', e);
    }
  },

  switchVault: async (vault: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      // Lock the current vault first
      await invoke('lock_vault');
      // Switch to new vault
      await invoke('use_vault', { vault });
      // Clear all local state for the new vault
      set({ 
        activeVault: vault, 
        unlocked: false, 
        entries: [], 
        devices: [],
        syncStatus: { initialized: false, last_sync: null, pending_changes: 0 }
      });
      await get().fetchVaults();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  createVault: async (name: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('create_vault', { name });
      // Clear all state after creating new vault
      set({ 
        activeVault: '', 
        unlocked: false, 
        entries: [], 
        devices: [],
        initialized: false,
        syncStatus: { initialized: false, last_sync: null, pending_changes: 0 }
      });
      await get().fetchVaults();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  initVault: async (name: string, password: string, vault?: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      // Clear all state before init
      set({ 
        entries: [], 
        devices: [],
        initialized: false,
        syncStatus: { initialized: false, last_sync: null, pending_changes: 0 }
      });
      await invoke('init_vault', { name, password, vault });
      set({ initialized: true });
      await get().fetchVaults();
      // Auto-unlock after initialization
      await get().unlock(password);
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  unlock: async (password: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      const result = await invoke<boolean>('unlock_vault', { password });
      if (result) {
        set({ unlocked: true, error: null });
        await get().fetchEntriesOnce();
        await get().fetchDevices();
        await get().fetchSyncStatus();
      } else {
        set({ error: 'Failed to unlock vault' });
      }
    } catch (e: any) {
      const errorMsg = e.toString();
      set({ error: errorMsg.includes('wrong password') ? 'Wrong password' : errorMsg });
    } finally {
      set({ loading: false });
    }
  },

  lock: async () => {
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('lock_vault');
      set({ unlocked: false, entries: [], devices: [] });
    } catch (e) {
      console.error('Failed to lock:', e);
    }
  },

  fetchEntries: async () => {
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      const entries = await invoke<Entry[]>('get_entries');
      set({ entries });
    } catch (e) {
      console.error('Failed to fetch entries:', e);
      set({ entries: [], error: 'Failed to fetch entries. Make sure the server is running.' });
    }
  },

  fetchEntriesOnce: async () => {
    const state = get();
    if (state.entries.length > 0) return; // Already fetched
    await get().fetchEntries();
  },

  addEntry: async (site: string, username: string, password: string, notes: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('add_entry', { site, username, password, notes });
      await get().fetchEntries();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  updateEntry: async (id: string, site: string, username: string, password: string, notes: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('update_entry', { id, site, username, password, notes });
      await get().fetchEntries();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  deleteEntry: async (id: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('delete_entry', { id });
      await get().fetchEntries();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  getPassword: async (id: string): Promise<string> => {
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      const password = await invoke<string>('get_password', { id });
      return password;
    } catch (e) {
      console.error('Failed to get password:', e);
      throw e;
    }
  },

  fetchDevices: async () => {
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      const devices = await invoke<Device[]>('get_devices');
      set({ devices });
    } catch (e) {
      console.error('Failed to fetch devices:', e);
    }
  },

  fetchSyncStatus: async () => {
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      const syncStatus = await invoke<SyncStatus>('get_sync_status');
      set({ syncStatus });
    } catch (e) {
      console.error('Failed to fetch sync status:', e);
    }
  },

  syncNow: async () => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('sync_now');
      await get().fetchSyncStatus();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  syncPush: async () => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('sync_push');
      await get().fetchSyncStatus();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  syncPull: async () => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('sync_pull');
      await get().fetchEntries();
      await get().fetchSyncStatus();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  initSync: async (remote: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('init_sync', { remote });
      await get().fetchSyncStatus();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  generatePassword: async (length: number) => {
    const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()_+-=';
    let password = '';
    const array = new Uint32Array(length);
    crypto.getRandomValues(array);
    for (let i = 0; i < length; i++) {
      password += chars[array[i] % chars.length];
    }
    return password;
  },

  fetchP2PStatus: async () => {
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      const running = await invoke<boolean>('p2p_status');
      set({ p2pStatus: { running, peer_id: null } });
    } catch (e) {
      console.error('Failed to fetch P2P status:', e);
    }
  },

  startP2P: async () => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('p2p_start');
      await get().fetchP2PStatus();
      await get().fetchPeers();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  stopP2P: async () => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('p2p_stop');
      set({ p2pStatus: { running: false, peer_id: null }, peers: [] });
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  connectPeer: async (address: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('p2p_connect', { address });
      await get().fetchPeers();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  disconnectPeer: async (peerId: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('p2p_disconnect', { peerId });
      await get().fetchPeers();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  fetchPeers: async () => {
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      const peers = await invoke<Peer[]>('p2p_peers');
      set({ peers });
    } catch (e) {
      console.error('Failed to fetch peers:', e);
    }
  },

  fetchApprovals: async () => {
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      const approvals = await invoke<ApprovalRequest[]>('p2p_approvals');
      set({ approvals });
    } catch (e) {
      console.error('Failed to fetch approvals:', e);
    }
  },

  approveDevice: async (deviceId: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('p2p_approve', { deviceId });
      await get().fetchApprovals();
      await get().fetchDevices();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  rejectDevice: async (deviceId: string, reason: string) => {
    set({ loading: true, error: null });
    try {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('p2p_reject', { deviceId, reason });
      await get().fetchApprovals();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },
}));
