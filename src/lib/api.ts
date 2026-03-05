const API_BASE = 'http://localhost:18475/api';

// Token management
let authToken: string | null = null;

function setToken(token: string) {
  authToken = token;
}

function getToken(): string | null {
  return authToken;
}

function clearToken() {
  authToken = null;
}

async function apiCall(endpoint: string, options?: RequestInit): Promise<any> {
  const token = getToken();
  
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...options?.headers as Record<string, string>,
  };
  
  // Add auth token if available (and not for public endpoints)
  const publicEndpoints = ['/is_initialized', '/unlock', '/init', '/health', '/vaults'];
  if (token && !publicEndpoints.includes(endpoint)) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  
  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers,
  });
  
  const data = await response.json();
  if (!data.success) {
    throw new Error(data.error || 'API call failed');
  }
  return data;
}

export const api = {
  // Vault
  isInitialized: () => apiCall('/is_initialized'),
  unlock: async (password: string) => {
    const result = await apiCall('/unlock', { method: 'POST', body: JSON.stringify({ password }) });
    // Store the token after successful unlock
    if (result.data?.token) {
      setToken(result.data.token);
    }
    return result;
  },
  lock: async () => {
    const result = await apiCall('/lock', { method: 'POST' });
    // Clear token after lock
    clearToken();
    return result;
  },
  
  // Entries
  getEntries: () => apiCall('/entries'),
  addEntry: (site: string, username: string, password: string, notes: string) => 
    apiCall('/entries/add', { method: 'POST', body: JSON.stringify({ site, username, password, notes }) }),
  updateEntry: (id: string, site: string, username: string, password: string, notes: string) =>
    apiCall('/entries/update', { method: 'POST', body: JSON.stringify({ id, site, username, password, notes }) }),
  deleteEntry: (id: string) => apiCall('/entries/delete', { method: 'POST', body: JSON.stringify({ id }) }),
  getPassword: (id: string) => apiCall('/entries/get_password', { method: 'POST', body: JSON.stringify({ id }) }),
  
  // Devices
  getDevices: () => apiCall('/devices'),
  
  // Vaults
  getVaults: () => apiCall('/vaults'),
  useVault: async (vault: string) => {
    const result = await apiCall('/vaults/use', { method: 'POST', body: JSON.stringify({ vault }) });
    // Clear token when switching vaults (need to re-unlock)
    clearToken();
    return result;
  },
  createVault: (name: string) => apiCall('/vaults/create', { method: 'POST', body: JSON.stringify({ name }) }),
  deleteVault: (name: string, deleteDataDir: boolean = true) => apiCall('/vaults/delete', { method: 'POST', body: JSON.stringify({ name, delete_data_dir: deleteDataDir }) }),
  
  // P2P
  p2pStatus: () => apiCall('/p2p/status'),
  p2pStart: () => apiCall('/p2p/start', { method: 'POST' }),
  p2pStop: () => apiCall('/p2p/stop', { method: 'POST' }),
  p2pPeers: () => apiCall('/p2p/peers'),
  p2pConnect: (address: string) => apiCall('/p2p/connect', { method: 'POST', body: JSON.stringify({ address }) }),
  p2pDisconnect: (peerId: string) => apiCall('/p2p/disconnect', { method: 'POST', body: JSON.stringify({ peer_id: peerId }) }),
  p2pApprovals: () => apiCall('/p2p/approvals'),
  p2pApprove: (deviceId: string) => apiCall('/p2p/approve', { method: 'POST', body: JSON.stringify({ device_id: deviceId }) }),
  p2pReject: (deviceId: string, reason: string) => apiCall('/p2p/reject', { method: 'POST', body: JSON.stringify({ device_id: deviceId, reason }) }),
  p2pSync: (fullSync: boolean = false) => apiCall('/p2p/sync', { method: 'POST', body: JSON.stringify({ full_sync: fullSync }) }),
  
  // Pairing
  pairingGenerate: () => apiCall('/pairing/generate'),
  pairingJoin: (code: string, deviceName: string, password: string) => 
    apiCall('/pairing/join', { method: 'POST', body: JSON.stringify({ code, device_name: deviceName, password }) }),
  pairingStatus: () => apiCall('/pairing/status'),
  
  // Generate password
  generatePassword: (length: number = 16) => apiCall('/generate', { method: 'POST', body: JSON.stringify({ length }) }),
};
