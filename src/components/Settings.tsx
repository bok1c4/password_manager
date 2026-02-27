import { useState, useEffect } from 'react';
import { useVault } from '../hooks/useVault';

export function Settings() {
  const { devices, syncStatus, lock, syncNow, syncPull, syncPush, initSync, p2pStatus, peers, approvals, startP2P, stopP2P, connectPeer, disconnectPeer, fetchP2PStatus, fetchPeers, fetchApprovals, approveDevice, rejectDevice } = useVault();
  const [showInitSync, setShowInitSync] = useState(false);
  const [remoteUrl, setRemoteUrl] = useState('');
  const [connectAddress, setConnectAddress] = useState('');

  useEffect(() => {
    fetchP2PStatus();
    fetchPeers();
    fetchApprovals();
    const interval = setInterval(() => {
      if (p2pStatus.running) {
        fetchPeers();
        fetchApprovals();
      }
    }, 5000);
    return () => clearInterval(interval);
  }, [p2pStatus.running]);

  const handleInitSync = async () => {
    if (remoteUrl) {
      await initSync(remoteUrl);
      setShowInitSync(false);
      setRemoteUrl('');
    }
  };

  const handleConnect = async () => {
    if (connectAddress) {
      await connectPeer(connectAddress);
      setConnectAddress('');
    }
  };

  return (
    <div className="p-4 space-y-6">
      <section>
        <h2 className="text-lg font-bold mb-3">Devices</h2>
        <div className="space-y-2">
          {devices.map((device) => (
            <div
              key={device.id}
              className="flex items-center justify-between p-3 bg-white dark:bg-surface-dark rounded"
            >
              <div>
                <p className="font-medium">{device.name}</p>
                <p className="text-xs text-gray-500">{device.id}</p>
              </div>
              {device.trusted ? (
                <span className="text-xs bg-green-100 text-green-800 px-2 py-1 rounded">
                  Trusted
                </span>
              ) : (
                <span className="text-xs bg-yellow-100 text-yellow-800 px-2 py-1 rounded">
                  Pending
                </span>
              )}
            </div>
          ))}
        </div>
      </section>

      <section>
        <h2 className="text-lg font-bold mb-3">P2P Sync</h2>
        <div className="p-3 bg-white dark:bg-surface-dark rounded space-y-3">
          <div className="flex items-center justify-between">
            <p>Status: {p2pStatus.running ? 'Running' : 'Stopped'}</p>
            {p2pStatus.running ? (
              <button
                onClick={stopP2P}
                className="px-3 py-1 bg-red-500 text-white rounded text-sm"
              >
                Stop
              </button>
            ) : (
              <button
                onClick={startP2P}
                className="px-3 py-1 bg-green-500 text-white rounded text-sm"
              >
                Start
              </button>
            )}
          </div>

          {p2pStatus.running && (
            <>
              <div className="space-y-2">
                <p className="text-sm font-medium">Connect to Peer</p>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={connectAddress}
                    onChange={(e) => setConnectAddress(e.target.value)}
                    placeholder="/ip4/192.168.1.x/tcp/0/p2p/..."
                    className="flex-1 px-3 py-2 rounded border dark:bg-gray-700 dark:border-gray-600 text-sm"
                  />
                  <button
                    onClick={handleConnect}
                    className="px-3 py-2 bg-primary text-white rounded text-sm"
                  >
                    Connect
                  </button>
                </div>
              </div>

              <div className="space-y-2">
                <p className="text-sm font-medium">Connected Peers</p>
                {peers.length === 0 ? (
                  <p className="text-sm text-gray-500">No peers connected</p>
                ) : (
                  <div className="space-y-2">
                    {peers.map((peer) => (
                      <div key={peer.peer_id} className="flex items-center justify-between p-2 bg-gray-50 dark:bg-gray-700 rounded">
                        <div>
                          <p className="text-sm font-medium">{peer.name}</p>
                          <p className="text-xs text-gray-500">{peer.peer_id.slice(0, 16)}...</p>
                        </div>
                        <button
                          onClick={() => disconnectPeer(peer.peer_id)}
                          className="px-2 py-1 bg-gray-300 dark:bg-gray-600 rounded text-xs"
                        >
                          Disconnect
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {approvals.length > 0 && (
                <div className="space-y-2">
                  <p className="text-sm font-medium">Pending Approvals</p>
                  <div className="space-y-2">
                    {approvals.map((approval) => (
                      <div key={approval.device_id} className="p-2 bg-yellow-50 dark:bg-yellow-900/20 rounded">
                        <p className="text-sm font-medium">{approval.name}</p>
                        <p className="text-xs text-gray-500">{approval.fingerprint}</p>
                        <div className="flex gap-2 mt-2">
                          <button
                            onClick={() => approveDevice(approval.device_id)}
                            className="px-2 py-1 bg-green-500 text-white rounded text-xs"
                          >
                            Approve
                          </button>
                          <button
                            onClick={() => rejectDevice(approval.device_id, 'Rejected')}
                            className="px-2 py-1 bg-red-500 text-white rounded text-xs"
                          >
                            Reject
                          </button>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      </section>

      <section>
        <h2 className="text-lg font-bold mb-3">Sync</h2>
        <div className="p-3 bg-white dark:bg-surface-dark rounded space-y-3">
          <p>Status: {syncStatus.initialized ? 'Connected' : 'Not configured'}</p>
          {syncStatus.last_sync && (
            <p className="text-sm text-gray-500">
              Last sync: {new Date(syncStatus.last_sync).toLocaleString()}
            </p>
          )}
          
          {syncStatus.initialized ? (
            <div className="flex gap-2">
              <button
                onClick={syncPull}
                className="px-3 py-2 bg-gray-200 dark:bg-gray-600 rounded text-sm"
              >
                Pull
              </button>
              <button
                onClick={syncPush}
                className="px-3 py-2 bg-gray-200 dark:bg-gray-600 rounded text-sm"
              >
                Push
              </button>
              <button
                onClick={syncNow}
                className="px-3 py-2 bg-primary text-white rounded text-sm"
              >
                Sync Now
              </button>
            </div>
          ) : (
            <div className="space-y-2">
              {!showInitSync ? (
                <button
                  onClick={() => setShowInitSync(true)}
                  className="px-3 py-2 bg-primary text-white rounded text-sm"
                >
                  Initialize Sync
                </button>
              ) : (
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={remoteUrl}
                    onChange={(e) => setRemoteUrl(e.target.value)}
                    placeholder="git@github.com:user/repo.git"
                    className="flex-1 px-3 py-2 rounded border dark:bg-gray-700 dark:border-gray-600 text-sm"
                  />
                  <button
                    onClick={handleInitSync}
                    className="px-3 py-2 bg-primary text-white rounded text-sm"
                  >
                    Init
                  </button>
                  <button
                    onClick={() => setShowInitSync(false)}
                    className="px-3 py-2 bg-gray-200 dark:bg-gray-600 rounded text-sm"
                  >
                    Cancel
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      </section>

      <section>
        <h2 className="text-lg font-bold mb-3">Security</h2>
        <button
          onClick={lock}
          className="w-full px-4 py-2 bg-red-500 text-white rounded"
        >
          Lock Vault
        </button>
      </section>
    </div>
  );
}
