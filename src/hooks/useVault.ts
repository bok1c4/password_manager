import { create } from 'zustand';
import { api } from '../lib/api';

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
  last_seen?: number;
}

export interface ApprovalRequest {
  device_id: string;
  name?: string;
  fingerprint?: string;
  status?: string;
}

interface VaultState {
  vaults: VaultInfo[];
  activeVault: string;
  initialized: boolean;
  unlocked: boolean;
  entries: Entry[];
  devices: Device[];
  p2pStatus: P2PStatus;
  peers: Peer[];
  approvals: ApprovalRequest[];
  loading: boolean;
  error: string | null;
  
  checkInitialized: () => Promise<void>;
  fetchVaults: () => Promise<void>;
  switchVault: (vault: string) => Promise<void>;
  createVault: (name: string) => Promise<void>;
  deleteVault: (name: string) => Promise<void>;
  initVault: (name: string, password: string, vault?: string) => Promise<void>;
  unlock: (password: string) => Promise<void>;
  lock: () => Promise<void>;
  fetchEntries: () => Promise<void>;
  addEntry: (site: string, username: string, password: string, notes: string) => Promise<void>;
  updateEntry: (id: string, site: string, username: string, password: string, notes: string) => Promise<void>;
  deleteEntry: (id: string) => Promise<void>;
  getPassword: (id: string) => Promise<string>;
  fetchDevices: () => Promise<void>;
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
  p2pSync: (fullSync?: boolean) => Promise<void>;
  pairingGenerate: () => Promise<{code: string, device_name: string, expires_in: number}>;
  pairingJoin: (code: string, deviceName: string) => Promise<void>;
  clearError: () => void;
}

export const useVault = create<VaultState>((set, get) => ({
  vaults: [],
  activeVault: '',
  initialized: false,
  unlocked: false,
  entries: [],
  devices: [],
  p2pStatus: { running: false, peer_id: null },
  peers: [],
  approvals: [],
  loading: false,
  error: null,

  checkInitialized: async () => {
    try {
      const result = await api.isInitialized();
      const data = result.data;
      set({ initialized: data.initialized });
      await get().fetchVaults();
    } catch (e) {
      console.error('Failed to check initialized:', e);
    }
  },

  fetchVaults: async () => {
    try {
      const result = await api.getVaults();
      set({ vaults: result.data, activeVault: result.data.find((v: VaultInfo) => v.active)?.name || '' });
    } catch (e) {
      console.error('Failed to fetch vaults:', e);
    }
  },

  switchVault: async (vault: string) => {
    set({ loading: true, error: null });
    try {
      await api.useVault(vault);
      set({ activeVault: vault, unlocked: false, entries: [], devices: [] });
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
      await api.createVault(name);
      set({ activeVault: '', unlocked: false, entries: [], devices: [], initialized: false });
      await get().fetchVaults();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  deleteVault: async (name: string) => {
    set({ loading: true, error: null });
    try {
      await api.deleteVault(name, true);
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
      await invoke('init_vault', { name, password, vault });
      set({ initialized: true });
      await get().fetchVaults();
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
      await api.unlock(password);
      set({ unlocked: true, error: null });
      await get().fetchEntries();
      await get().fetchDevices();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  lock: async () => {
    try {
      await api.lock();
      set({ unlocked: false, entries: [], devices: [] });
    } catch (e) {
      console.error('Failed to lock:', e);
    }
  },

  fetchEntries: async () => {
    try {
      const result = await api.getEntries();
      set({ entries: result.data || [] });
    } catch (e) {
      console.error('Failed to fetch entries:', e);
      set({ entries: [], error: 'Failed to fetch entries. Make sure the server is running.' });
    }
  },

  addEntry: async (site: string, username: string, password: string, notes: string) => {
    set({ loading: true, error: null });
    try {
      await api.addEntry(site, username, password, notes);
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
      await api.updateEntry(id, site, username, password, notes);
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
      await api.deleteEntry(id);
      await get().fetchEntries();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  getPassword: async (id: string): Promise<string> => {
    try {
      const result = await api.getPassword(id);
      return result.data.password;
    } catch (e) {
      console.error('Failed to get password:', e);
      throw e;
    }
  },

  fetchDevices: async () => {
    try {
      const result = await api.getDevices();
      set({ devices: result.data || [] });
    } catch (e) {
      console.error('Failed to fetch devices:', e);
    }
  },

  generatePassword: async (length: number): Promise<string> => {
    try {
      const result = await api.generatePassword(length);
      return result.data.password;
    } catch (e) {
      console.error('Failed to generate password:', e);
      throw e;
    }
  },

  fetchP2PStatus: async () => {
    try {
      const result = await api.p2pStatus();
      const data = result.data;
      set({ p2pStatus: { running: data.running, peer_id: data.peer_id } });
    } catch (e) {
      console.error('Failed to fetch P2P status:', e);
    }
  },

  startP2P: async () => {
    set({ loading: true, error: null });
    try {
      await api.p2pStart();
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
      await api.p2pStop();
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
      await api.p2pConnect(address);
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
      await api.p2pDisconnect(peerId);
      await get().fetchPeers();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  fetchPeers: async () => {
    try {
      const result = await api.p2pPeers();
      set({ peers: result.data || [] });
    } catch (e) {
      console.error('Failed to fetch peers:', e);
    }
  },

  fetchApprovals: async () => {
    try {
      const result = await api.p2pApprovals();
      set({ approvals: result.data || [] });
    } catch (e) {
      console.error('Failed to fetch approvals:', e);
    }
  },

  approveDevice: async (deviceId: string) => {
    set({ loading: true, error: null });
    try {
      await api.p2pApprove(deviceId);
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
      await api.p2pReject(deviceId, reason);
      await get().fetchApprovals();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  p2pSync: async (fullSync: boolean = false) => {
    set({ loading: true, error: null });
    try {
      await api.p2pSync(fullSync);
      await get().fetchEntries();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  pairingGenerate: async () => {
    const result = await api.pairingGenerate();
    return result.data;
  },

  pairingJoin: async (code: string, deviceName: string) => {
    await api.pairingJoin(code, deviceName);
    await get().fetchDevices();
  },

  clearError: () => {
    set({ error: null });
  },
}));
