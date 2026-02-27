import { useState, useEffect } from 'react';
import { useVault } from './hooks/useVault';
import { PasswordList } from './components/PasswordList';
import { Settings } from './components/Settings';

type Tab = 'passwords' | 'settings';

function App() {
  const { initialized, unlocked, loading, error, vaults, activeVault, checkInitialized, initVault, unlock, lock, switchVault, createVault } = useVault();
  const [password, setPassword] = useState('');
  const [deviceName, setDeviceName] = useState('');
  const [newVaultName, setNewVaultName] = useState('');
  const [showCreateVault, setShowCreateVault] = useState(false);
  const [activeTab, setActiveTab] = useState<Tab>('passwords');
  const [selectedVaultForInit, setSelectedVaultForInit] = useState<string | null>(null);

  useEffect(() => {
    checkInitialized();
  }, []);

  const handleInit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (password.length < 8) {
      return;
    }
    const vault = selectedVaultForInit || newVaultName.trim() || undefined;
    await initVault(deviceName, password, vault);
    setSelectedVaultForInit(null);
    setDeviceName('');
    setPassword('');
  };

  const handleCreateVault = async () => {
    if (newVaultName.trim()) {
      await createVault(newVaultName.trim());
      setSelectedVaultForInit(newVaultName.trim());
      setNewVaultName('');
      setShowCreateVault(false);
    }
  };

  const handleUnlock = async (e: React.FormEvent) => {
    e.preventDefault();
    await unlock(password);
  };

  const handleSwitchToUninitializedVault = async (vaultName: string) => {
    await switchVault(vaultName);
    setSelectedVaultForInit(vaultName);
  };

  // Initialize screen
  if (!initialized || selectedVaultForInit) {
    const uninitializedVaults = vaults.filter(v => !v.initialized);
    const showVaultSelect = uninitializedVaults.length > 0;
    
    return (
      <div className="min-h-screen bg-background-light dark:bg-background-dark flex items-center justify-center p-4">
        <div className="w-full max-w-md">
          <div className="text-center mb-8">
            <div className="text-6xl mb-4">Password Manager</div>
            <h1 className="text-2xl font-bold">Password Manager</h1>
            <p className="text-gray-500 mt-2">
              {selectedVaultForInit ? `Set up "${selectedVaultForInit}" vault` : (showVaultSelect ? 'Set up your vault' : 'Create your vault')}
            </p>
          </div>

          <form onSubmit={handleInit} className="space-y-4">
            {!selectedVaultForInit && showVaultSelect ? (
              <div>
                <label className="block text-sm font-medium mb-1">Select Vault</label>
                <select
                  value={newVaultName}
                  onChange={(e) => setNewVaultName(e.target.value)}
                  className="w-full px-4 py-3 rounded-lg border dark:bg-gray-700"
                >
                  <option value="">Choose a vault...</option>
                  {uninitializedVaults.map(v => (
                    <option key={v.name} value={v.name}>{v.name}</option>
                  ))}
                </select>
              </div>
            ) : (
              <div>
                <label className="block text-sm font-medium mb-1">Vault Name</label>
                <input
                  type="text"
                  value={selectedVaultForInit || newVaultName}
                  onChange={(e) => setNewVaultName(e.target.value)}
                  placeholder="e.g., work, personal"
                  className="w-full px-4 py-3 rounded-lg border dark:bg-gray-700"
                  readOnly={!!selectedVaultForInit}
                />
              </div>
            )}
            <div>
              <input
                type="text"
                value={deviceName}
                onChange={(e) => setDeviceName(e.target.value)}
                placeholder="Device name (e.g., MacBook Pro)"
                className="w-full px-4 py-3 rounded-lg border dark:bg-gray-700"
                required
              />
            </div>
            <div>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Master password (min 8 characters)"
                className="w-full px-4 py-3 rounded-lg border dark:bg-gray-700"
                minLength={8}
                required
              />
            </div>
            {error && <p className="text-red-500 text-sm">{error}</p>}
            <button
              type="submit"
              disabled={loading || !deviceName || password.length < 8}
              className="w-full py-3 bg-primary text-white rounded-lg font-medium disabled:opacity-50"
            >
              {loading ? 'Creating...' : 'Create Vault'}
            </button>
          </form>
        </div>
      </div>
    );
  }

  // Unlock screen
  if (!unlocked) {
    return (
      <div className="min-h-screen bg-background-light dark:bg-background-dark flex items-center justify-center p-4">
        <div className="w-full max-w-md">
          <div className="text-center mb-8">
            <div className="text-6xl mb-4">Password Manager</div>
            <h1 className="text-2xl font-bold">Password Manager</h1>
            <p className="text-gray-500 mt-2">Unlock your vault</p>
          </div>

          <div className="mb-4">
            <label className="block text-sm font-medium mb-1">Vault</label>
            <select
              value={activeVault}
              onChange={(e) => switchVault(e.target.value)}
              className="w-full px-4 py-3 rounded-lg border dark:bg-gray-700"
            >
              {vaults.filter(v => v.initialized).map(v => (
                <option key={v.name} value={v.name}>{v.name}</option>
              ))}
            </select>
          </div>

          {vaults.filter(v => !v.initialized).length > 0 && (
            <div className="mb-4">
              <p className="text-sm text-gray-500 mb-2">Uninitialized vaults:</p>
              {vaults.filter(v => !v.initialized).map(v => (
                <button
                  key={v.name}
                  onClick={() => handleSwitchToUninitializedVault(v.name)}
                  className="w-full py-2 mb-2 border rounded text-sm"
                >
                  Set up "{v.name}" vault
                </button>
              ))}
            </div>
          )}

          <form onSubmit={handleUnlock} className="space-y-4">
            <div>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Master password"
                className="w-full px-4 py-3 rounded-lg border dark:bg-gray-700"
                autoFocus
              />
            </div>
            {error && <p className="text-red-500 text-sm">{error}</p>}
            <button
              type="submit"
              disabled={loading || !password}
              className="w-full py-3 bg-primary text-white rounded-lg font-medium disabled:opacity-50"
            >
              {loading ? 'Unlocking...' : 'Unlock'}
            </button>
          </form>
        </div>
      </div>
    );
  }

  // Main app
  return (
    <div className="min-h-screen bg-background-light dark:bg-background-dark">
      {showCreateVault && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-surface-dark p-6 rounded-lg shadow-lg">
            <h2 className="text-lg font-bold mb-4">Create New Vault</h2>
            <input
              type="text"
              value={newVaultName}
              onChange={(e) => setNewVaultName(e.target.value)}
              placeholder="Vault name (e.g., work, personal)"
              className="w-full px-4 py-2 mb-4 rounded-lg border dark:bg-gray-700"
              autoFocus
            />
            <div className="flex gap-2 justify-end">
              <button
                onClick={() => setShowCreateVault(false)}
                className="px-4 py-2 border rounded"
              >
                Cancel
              </button>
              <button
                onClick={handleCreateVault}
                disabled={!newVaultName.trim() || loading}
                className="px-4 py-2 bg-primary text-white rounded disabled:opacity-50"
              >
                Create
              </button>
            </div>
          </div>
        </div>
      )}

      <header className="flex items-center justify-between p-4 bg-white dark:bg-surface-dark border-b">
        <div className="flex items-center gap-4">
          <h1 className="text-xl font-bold">Password Manager</h1>
          <select
            value={activeVault}
            onChange={(e) => switchVault(e.target.value)}
            className="px-2 py-1 text-sm border rounded dark:bg-gray-700"
          >
            {vaults.map(v => (
              <option key={v.name} value={v.name}>
                {v.name} {v.initialized ? '' : '(not initialized)'}
              </option>
            ))}
          </select>
          <button
            onClick={() => setShowCreateVault(true)}
            className="px-2 py-1 text-sm border rounded"
            title="Create new vault"
          >
            + New
          </button>
        </div>
        <div className="flex gap-2">
          <button
            onClick={lock}
            className="px-3 py-1 text-sm border rounded"
          >
            Lock
          </button>
        </div>
      </header>

      <nav className="flex border-b">
        <button
          onClick={() => setActiveTab('passwords')}
          className={`flex-1 py-3 text-center ${activeTab === 'passwords' ? 'border-b-2 border-primary font-medium' : ''}`}
        >
          Passwords
        </button>
        <button
          onClick={() => setActiveTab('settings')}
          className={`flex-1 py-3 text-center ${activeTab === 'settings' ? 'border-b-2 border-primary font-medium' : ''}`}
        >
          Settings
        </button>
      </nav>

      <main>
        {activeTab === 'passwords' ? <PasswordList /> : <Settings />}
      </main>
    </div>
  );
}

export default App;
