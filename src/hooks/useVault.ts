import { create } from 'zustand';
import { VaultAPI } from '../lib/api';

export interface Entry {
  id: string;
  site: string;
  username: string;
  encrypted_password?: string;
  notes?: string;
  created_at?: string;
  updated_at?: string;
}

export interface Device {
  id: string;
  name: string;
  fingerprint: string;
  trusted: boolean;
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
  deleteVault: (name: string, password: string) => Promise<void>;
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
  pairingJoin: (code: string, deviceName: string, password: string) => Promise<void>;
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
      const initialized = await VaultAPI.isInitialized();
      set({ initialized });
      await get().fetchVaults();
    } catch (e) {
      console.error('Failed to check initialized:', e);
    }
  },

  fetchVaults: async () => {
    try {
      const vaults = await VaultAPI.getVaults();
      set({ vaults, activeVault: vaults.find((v: VaultInfo) => v.active)?.name || '' });
    } catch (e) {
      console.error('Failed to fetch vaults:', e);
    }
  },

  switchVault: async (vault: string) => {
    set({ loading: true, error: null });
    try {
      // First lock if currently unlocked to ensure clean state
      if (get().unlocked) {
        await VaultAPI.lockVault();
      }
      
      await VaultAPI.useVault(vault);
      
      // Clear all vault-related state
      set({ 
        activeVault: vault, 
        unlocked: false, 
        initialized: false,
        entries: [], 
        devices: [],
        error: null 
      });
      
      // Verify the switch worked by fetching vaults
      await get().fetchVaults();
      
      // Double-check the active vault matches
      const currentVaults = get().vaults;
      const switchedVault = currentVaults.find(v => v.name === vault);
      if (!switchedVault?.active) {
        throw new Error("Vault switch verification failed");
      }
    } catch (e: any) {
      set({ error: e.toString() });
      // Re-fetch to ensure we have correct state
      await get().fetchVaults();
      throw e;
    } finally {
      set({ loading: false });
    }
  },

  createVault: async (name: string) => {
    set({ loading: true, error: null });
    try {
      await VaultAPI.createVault(name);
      set({ activeVault: '', unlocked: false, entries: [], devices: [], initialized: false });
      await get().fetchVaults();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  deleteVault: async (name: string, password: string) => {
    set({ loading: true, error: null });
    try {
      await VaultAPI.deleteVault(name, password);
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
      await VaultAPI.initVault(name, password, vault);
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
      await VaultAPI.unlockVault(password);
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
    set({ loading: true, error: null });
    try {
      await VaultAPI.lockVault();
      // Clear all sensitive state
      set({ 
        unlocked: false, 
        entries: [], 
        devices: [],
        error: null 
      });
    } catch (e: any) {
      console.error('Failed to lock:', e);
      set({ error: e.toString() });
      // Even if server call fails, clear local state for security
      set({ unlocked: false, entries: [], devices: [] });
    } finally {
      set({ loading: false });
    }
  },

  fetchEntries: async () => {
    try {
      const entries = await VaultAPI.getEntries();
      set({ entries: entries || [] });
    } catch (e) {
      console.error('Failed to fetch entries:', e);
      set({ entries: [], error: 'Failed to fetch entries. Make sure the server is running.' });
    }
  },

  addEntry: async (site: string, username: string, password: string, notes: string) => {
    set({ loading: true, error: null });
    try {
      await VaultAPI.addEntry(site, username, password, notes);
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
      await VaultAPI.updateEntry(id, site, username, password, notes);
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
      await VaultAPI.deleteEntry(id);
      await get().fetchEntries();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  getPassword: async (id: string): Promise<string> => {
    try {
      const password = await VaultAPI.getPassword(id);
      return password;
    } catch (e) {
      console.error('Failed to get password:', e);
      throw e;
    }
  },

  fetchDevices: async () => {
    try {
      const devices = await VaultAPI.getDevices();
      set({ devices: devices || [] });
    } catch (e) {
      console.error('Failed to fetch devices:', e);
    }
  },

  generatePassword: async (length: number): Promise<string> => {
    try {
      const password = await VaultAPI.generatePassword(length);
      return password;
    } catch (e) {
      console.error('Failed to generate password:', e);
      throw e;
    }
  },

  fetchP2PStatus: async () => {
    try {
      const running = await VaultAPI.p2pStatus();
      set({ p2pStatus: { running, peer_id: null } });
    } catch (e) {
      console.error('Failed to fetch P2P status:', e);
    }
  },

  startP2P: async () => {
    set({ loading: true, error: null });
    try {
      await VaultAPI.p2pStart();
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
      await VaultAPI.p2pStop();
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
      await VaultAPI.p2pConnect(address);
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
      await VaultAPI.p2pDisconnect(peerId);
      await get().fetchPeers();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  fetchPeers: async () => {
    try {
      const peers = await VaultAPI.p2pPeers();
      set({ peers: peers || [] });
    } catch (e) {
      console.error('Failed to fetch peers:', e);
    }
  },

  fetchApprovals: async () => {
    try {
      const approvals = await VaultAPI.p2pApprovals();
      set({ approvals: approvals || [] });
    } catch (e) {
      console.error('Failed to fetch approvals:', e);
    }
  },

  approveDevice: async (deviceId: string) => {
    set({ loading: true, error: null });
    try {
      await VaultAPI.p2pApprove(deviceId);
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
      await VaultAPI.p2pReject(deviceId, reason);
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
      await VaultAPI.p2pSync(fullSync);
      await get().fetchEntries();
    } catch (e: any) {
      set({ error: e.toString() });
    } finally {
      set({ loading: false });
    }
  },

  pairingGenerate: async () => {
    const data = await VaultAPI.pairingGenerate();
    return data;
  },

  pairingJoin: async (code: string, deviceName: string, password: string) => {
    try {
      await VaultAPI.pairingJoin(code, deviceName, password);
      // After joining, we need to unlock the vault
      await get().unlock(password);
      await get().fetchDevices();
    } catch (e) {
      console.error('pairingJoin failed:', e);
      throw e;
    }
  },

  clearError: () => {
    set({ error: null });
  },
}));
