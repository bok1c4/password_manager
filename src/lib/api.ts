import { invoke } from '@tauri-apps/api/core';

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

export interface SyncStatus {
  initialized: boolean;
  last_sync?: number;
  pending_changes: number;
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

export class VaultAPI {
  // Vault lifecycle
  static async isInitialized(): Promise<boolean> {
    return await invoke('is_initialized');
  }

  static async initVault(name: string, password: string, vault?: string): Promise<string> {
    return await invoke('init_vault', { name, password, vault });
  }

  static async unlockVault(password: string): Promise<boolean> {
    return await invoke('unlock_vault', { password });
  }

  static async lockVault(): Promise<boolean> {
    return await invoke('lock_vault');
  }

  static async isUnlocked(): Promise<boolean> {
    return await invoke('is_unlocked');
  }

  // Vault management
  static async getVaults(): Promise<VaultInfo[]> {
    return await invoke('get_vaults');
  }

  static async useVault(vault: string): Promise<boolean> {
    return await invoke('use_vault', { vault });
  }

  static async createVault(name: string): Promise<boolean> {
    return await invoke('create_vault', { name });
  }

  static async deleteVault(name: string, password: string): Promise<boolean> {
    return await invoke('delete_vault', { name, password });
  }

  // Entry management
  static async getEntries(): Promise<Entry[]> {
    return await invoke('get_entries');
  }

  static async addEntry(site: string, username: string, password: string, notes: string): Promise<string> {
    return await invoke('add_entry', { site, username, password, notes });
  }

  static async updateEntry(id: string, site: string, username: string, password: string, notes: string): Promise<string> {
    return await invoke('update_entry', { id, site, username, password, notes });
  }

  static async deleteEntry(id: string): Promise<boolean> {
    return await invoke('delete_entry', { id });
  }

  static async getPassword(id: string): Promise<string> {
    return await invoke('get_password', { id });
  }

  // Devices
  static async getDevices(): Promise<Device[]> {
    return await invoke('get_devices');
  }

  // Sync
  static async getSyncStatus(): Promise<SyncStatus> {
    return await invoke('get_sync_status');
  }

  static async initSync(remote: string): Promise<boolean> {
    return await invoke('init_sync', { remote });
  }

  static async syncNow(): Promise<boolean> {
    return await invoke('sync_now');
  }

  // P2P
  static async p2pStatus(): Promise<boolean> {
    return await invoke('p2p_status');
  }

  static async p2pStart(): Promise<boolean> {
    return await invoke('p2p_start');
  }

  static async p2pStop(): Promise<boolean> {
    return await invoke('p2p_stop');
  }

  static async p2pPeers(): Promise<any[]> {
    return await invoke('p2p_peers');
  }

  static async p2pConnect(address: string): Promise<boolean> {
    return await invoke('p2p_connect', { address });
  }

  static async p2pDisconnect(peerId: string): Promise<boolean> {
    return await invoke('p2p_disconnect', { peerId });
  }

  static async p2pApprovals(): Promise<any[]> {
    return await invoke('p2p_approvals');
  }

  static async p2pApprove(deviceId: string): Promise<boolean> {
    return await invoke('p2p_approve', { deviceId });
  }

  static async p2pReject(deviceId: string, reason: string): Promise<boolean> {
    return await invoke('p2p_reject', { deviceId, reason });
  }

  static async p2pSync(fullSync: boolean = false): Promise<boolean> {
    return await invoke('p2p_sync', { fullSync });
  }

  // Password generation
  static async generatePassword(length: number = 16): Promise<string> {
    return await invoke('generate_password', { length });
  }

  // Pairing
  static async pairingGenerate(): Promise<{code: string, device_name: string, expires_in: number}> {
    return await invoke('pairing_generate');
  }

  static async pairingJoin(code: string, deviceName: string, password: string): Promise<boolean> {
    return await invoke('pairing_join', { code, deviceName, password });
  }

  static async pairingStatus(): Promise<any> {
    return await invoke('pairing_status');
  }
}
