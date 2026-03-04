import { useState, useEffect } from 'react';
import { useVault } from './hooks/useVault';
import { PasswordList } from './components/PasswordList';
import { Settings } from './components/Settings';

type View = 'home' | 'init' | 'unlock' | 'join' | 'app';

function App() {
  const {
    unlocked,
    loading,
    error,
    vaults,
    activeVault,
    checkInitialized,
    initVault,
    unlock,
    lock,
    switchVault,
    deleteVault,
    clearError,
    pairingJoin
  } = useVault();
  
  const [password, setPassword] = useState('');
  const [deviceName, setDeviceName] = useState('');
  const [newVaultName, setNewVaultName] = useState('');
  const [activeTab, setActiveTab] = useState<'passwords' | 'settings'>('passwords');
  const [view, setView] = useState<View>('home');
  const [selectedVault, setSelectedVault] = useState<string | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [pairingCode, setPairingCode] = useState('');
  const [joinDeviceName, setJoinDeviceName] = useState('');
  const [joinPassword, setJoinPassword] = useState('');
  const [joinError, setJoinError] = useState('');
  const [isJoining, setIsJoining] = useState(false);

  useEffect(() => {
    checkInitialized();
  }, []);

  useEffect(() => {
    // If there are initialized vaults, we can show the home screen
    // If a vault is unlocked, show the app view
    if (unlocked) {
      setView('app');
    }
  }, [unlocked]);

  const handleInit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (password.length < 8 || !deviceName.trim()) {
      return;
    }
    const vault = newVaultName.trim() || 'default';
    await initVault(deviceName, password, vault);
    setPassword('');
    setDeviceName('');
    setNewVaultName('');
    setView('home');
  };



  const handleUnlock = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!password) return;
    await unlock(password);
    setPassword('');
  };

  const handleDeleteVault = async (vaultName: string) => {
    await deleteVault(vaultName);
    setDeleteConfirm(null);
  };

  const openUnlock = (vaultName: string) => {
    switchVault(vaultName);
    setSelectedVault(vaultName);
    setView('unlock');
  };

  const openInit = () => {
    setNewVaultName('');
    setDeviceName('');
    setPassword('');
    setView('init');
  };

  const goHome = () => {
    setView('home');
    setSelectedVault(null);
    setPassword('');
    setJoinError('');
    clearError();
  };

  const openJoin = () => {
    setPairingCode('');
    setJoinDeviceName('');
    setJoinError('');
    setView('join');
  };

  const handleJoinVault = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!pairingCode.trim() || !joinDeviceName.trim() || !joinPassword.trim()) return;

    setIsJoining(true);
    setJoinError('');
    try {
      await pairingJoin(pairingCode.trim().toUpperCase(), joinDeviceName.trim(), joinPassword.trim());
      // Join successful - the vault should now be in the list
      await checkInitialized();
      setView('home');
      setPairingCode('');
      setJoinDeviceName('');
      setJoinPassword('');
    } catch (e: any) {
      setJoinError(e.toString());
    } finally {
      setIsJoining(false);
    }
  };

  // Home Screen - List all vaults
  if (view === 'home') {
    return (
      <div className="min-h-screen bg-background-light dark:bg-background-dark flex items-center justify-center p-4">
        <div className="w-full max-w-2xl">
          <div className="text-center mb-8">
            <h1 className="text-3xl font-bold mb-2">Password Manager</h1>
            <p className="text-gray-500">Select a vault or create a new one</p>
          </div>

          {/* Vault List */}
          <div className="space-y-3 mb-6">
            {vaults.length === 0 ? (
              <div className="text-center py-8 text-gray-400">
                <p>No vaults found. Create your first vault to get started.</p>
              </div>
            ) : (
              vaults.map((vault) => (
                <div
                  key={vault.name}
                  className="bg-white dark:bg-surface-dark rounded-lg shadow p-4 flex items-center justify-between"
                >
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center">
                      <span className="text-lg">🔐</span>
                    </div>
                    <div>
                      <h3 className="font-semibold">{vault.name}</h3>
                      <p className="text-sm text-gray-500">
                        {vault.initialized ? 'Initialized' : 'Not initialized'}
                        {vault.active && ' • Active'}
                      </p>
                    </div>
                  </div>
                  <div className="flex gap-2">
                    {vault.initialized ? (
                      <button
                        onClick={() => openUnlock(vault.name)}
                        className="px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition"
                      >
                        Login
                      </button>
                    ) : (
                      <button
                        onClick={() => {
                          setNewVaultName(vault.name);
                          setView('init');
                        }}
                        className="px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition"
                      >
                        Setup
                      </button>
                    )}
                    <button
                      onClick={() => setDeleteConfirm(vault.name)}
                      className="px-4 py-2 border border-red-300 text-red-600 rounded-lg hover:bg-red-50 dark:hover:bg-red-900/20 transition"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>

          {/* Create New Vault Button */}
          <button
            onClick={openInit}
            className="w-full py-4 border-2 border-dashed border-gray-300 dark:border-gray-600 rounded-lg text-gray-500 hover:border-primary hover:text-primary transition flex items-center justify-center gap-2"
          >
            <span className="text-xl">+</span>
            Create New Vault
          </button>

          {/* Join Vault Button */}
          <button
            onClick={openJoin}
            className="w-full py-4 border-2 border-dashed border-gray-300 dark:border-gray-600 rounded-lg text-gray-500 hover:border-green-500 hover:text-green-600 transition flex items-center justify-center gap-2 mt-3"
          >
            <span className="text-xl">🔗</span>
            Join Existing Vault
          </button>

          {/* Delete Confirmation Modal */}
          {deleteConfirm && (
            <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
              <div className="bg-white dark:bg-surface-dark p-6 rounded-lg shadow-lg max-w-md">
                <h3 className="text-lg font-bold mb-2">Delete Vault?</h3>
                <p className="text-gray-500 mb-4">
                  Are you sure you want to delete "{deleteConfirm}"? This action cannot be undone and all passwords will be lost.
                </p>
                <div className="flex gap-3 justify-end">
                  <button
                    onClick={() => setDeleteConfirm(null)}
                    className="px-4 py-2 border rounded"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={() => handleDeleteVault(deleteConfirm)}
                    disabled={loading}
                    className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700 transition"
                  >
                    {loading ? 'Deleting...' : 'Delete'}
                  </button>
                </div>
              </div>
            </div>
          )}

          {error && (
            <div className="mt-4 p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
              <p className="text-red-600 dark:text-red-400 text-sm">{error}</p>
            </div>
          )}
        </div>
      </div>
    );
  }

  // Init Vault Screen
  if (view === 'init') {
    return (
      <div className="min-h-screen bg-background-light dark:bg-background-dark flex items-center justify-center p-4">
        <div className="w-full max-w-md">
          <div className="text-center mb-8">
            <h1 className="text-2xl font-bold">Create New Vault</h1>
            <p className="text-gray-500 mt-2">
              {newVaultName ? `Setting up "${newVaultName}"` : 'Set up your new vault'}
            </p>
          </div>

          <form onSubmit={handleInit} className="space-y-4">
            <div>
              <label className="block text-sm font-medium mb-1">Vault Name</label>
              <input
                type="text"
                value={newVaultName}
                onChange={(e) => setNewVaultName(e.target.value)}
                placeholder="e.g., work, personal"
                className="w-full px-4 py-3 rounded-lg border dark:bg-gray-700"
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Device Name</label>
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
              <label className="block text-sm font-medium mb-1">Master Password</label>
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
            <div className="flex gap-3">
              <button
                type="button"
                onClick={goHome}
                className="flex-1 py-3 border rounded-lg font-medium"
              >
                Back
              </button>
              <button
                type="submit"
                disabled={loading || !deviceName || password.length < 8 || !newVaultName.trim()}
                className="flex-1 py-3 bg-primary text-white rounded-lg font-medium disabled:opacity-50"
              >
                {loading ? 'Creating...' : 'Create Vault'}
              </button>
            </div>
          </form>
        </div>
      </div>
    );
  }

  // Unlock Screen
  if (view === 'unlock') {
    return (
      <div className="min-h-screen bg-background-light dark:bg-background-dark flex items-center justify-center p-4">
        <div className="w-full max-w-md">
          <div className="text-center mb-8">
            <h1 className="text-2xl font-bold">Unlock Vault</h1>
            <p className="text-gray-500 mt-2">
              {selectedVault ? `Unlock "${selectedVault}"` : 'Unlock your vault'}
            </p>
          </div>

          <form onSubmit={handleUnlock} className="space-y-4">
            <div>
              <label className="block text-sm font-medium mb-1">Master Password</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter your master password"
                className="w-full px-4 py-3 rounded-lg border dark:bg-gray-700"
                autoFocus
                required
              />
            </div>
            {error && <p className="text-red-500 text-sm">{error}</p>}
            <div className="flex gap-3">
              <button
                type="button"
                onClick={goHome}
                className="flex-1 py-3 border rounded-lg font-medium"
              >
                Back
              </button>
              <button
                type="submit"
                disabled={loading || !password}
                className="flex-1 py-3 bg-primary text-white rounded-lg font-medium disabled:opacity-50"
              >
                {loading ? 'Unlocking...' : 'Unlock'}
              </button>
            </div>
          </form>
        </div>
      </div>
    );
  }

  // Join Vault Screen
  if (view === 'join') {
    return (
      <div className="min-h-screen bg-background-light dark:bg-background-dark flex items-center justify-center p-4">
        <div className="w-full max-w-md">
          <div className="text-center mb-8">
            <h1 className="text-2xl font-bold">Join Vault</h1>
            <p className="text-gray-500 mt-2">
              Enter the pairing code from another device
            </p>
          </div>

          <form onSubmit={handleJoinVault} className="space-y-4">
            <div>
              <label className="block text-sm font-medium mb-1">Pairing Code</label>
              <input
                type="text"
                value={pairingCode}
                onChange={(e) => setPairingCode(e.target.value.toUpperCase())}
                placeholder="ABC-DEF-GHI"
                className="w-full px-4 py-3 rounded-lg border dark:bg-gray-700 text-center text-lg tracking-wider font-mono"
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Device Name</label>
              <input
                type="text"
                value={joinDeviceName}
                onChange={(e) => setJoinDeviceName(e.target.value)}
                placeholder="Device name (e.g., Laptop, Phone)"
                className="w-full px-4 py-3 rounded-lg border dark:bg-gray-700"
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Vault Password</label>
              <input
                type="password"
                value={joinPassword}
                onChange={(e) => setJoinPassword(e.target.value)}
                placeholder="Enter the vault password from the other device"
                className="w-full px-4 py-3 rounded-lg border dark:bg-gray-700"
                required
              />
            </div>
            {joinError && <p className="text-red-500 text-sm">{joinError}</p>}
            <div className="flex gap-3">
              <button
                type="button"
                onClick={goHome}
                className="flex-1 py-3 border rounded-lg font-medium"
              >
                Back
              </button>
              <button
                type="submit"
                disabled={isJoining || !pairingCode.trim() || !joinDeviceName.trim() || !joinPassword.trim()}
                className="flex-1 py-3 bg-green-600 text-white rounded-lg font-medium disabled:opacity-50"
              >
                {isJoining ? 'Joining...' : 'Join Vault'}
              </button>
            </div>
          </form>
        </div>
      </div>
    );
  }

  // Main App Screen
  return (
    <div className="min-h-screen bg-background-light dark:bg-background-dark">
      <header className="flex items-center justify-between p-4 bg-white dark:bg-surface-dark border-b">
        <div className="flex items-center gap-4">
          <h1 className="text-xl font-bold">Password Manager</h1>
          <span className="text-sm text-gray-500">{activeVault}</span>
        </div>
        <div className="flex gap-2">
          <button
            onClick={goHome}
            className="px-3 py-1 text-sm border rounded"
          >
            Switch Vault
          </button>
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
