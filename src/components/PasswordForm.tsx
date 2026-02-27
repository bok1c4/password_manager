import { useState, useEffect } from 'react';
import { useVault, Entry } from '../hooks/useVault';

interface PasswordFormProps {
  entry?: Entry;
  onClose: () => void;
}

export function PasswordForm({ entry, onClose }: PasswordFormProps) {
  const { addEntry, updateEntry, generatePassword, getPassword } = useVault();
  const [site, setSite] = useState(entry?.site || '');
  const [username, setUsername] = useState(entry?.username || '');
  const [password, setPassword] = useState('');
  const [notes, setNotes] = useState(entry?.notes || '');
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingPassword, setLoadingPassword] = useState(false);

  useEffect(() => {
    if (entry?.id) {
      setLoadingPassword(true);
      getPassword(entry.id)
        .then(setPassword)
        .catch(() => setPassword(''))
        .finally(() => setLoadingPassword(false));
    }
  }, [entry?.id]);

  const handleGenerate = async () => {
    const pwd = await generatePassword(20);
    setPassword(pwd);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      if (entry?.id) {
        await updateEntry(entry.id, site, username, password, notes);
      } else {
        await addEntry(site, username, password, notes);
      }
      onClose();
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center p-4">
      <div className="bg-white dark:bg-surface-dark rounded-lg w-full max-w-md p-6">
        <h2 className="text-xl font-bold mb-4">
          {entry ? 'Edit Password' : 'Add Password'}
        </h2>
        
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1">Site</label>
            <input
              type="text"
              value={site}
              onChange={(e) => setSite(e.target.value)}
              className="w-full px-3 py-2 rounded border dark:bg-gray-700 dark:border-gray-600"
              required
            />
          </div>
          
          <div>
            <label className="block text-sm font-medium mb-1">Username</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full px-3 py-2 rounded border dark:bg-gray-700 dark:border-gray-600"
            />
          </div>
          
          <div>
            <label className="block text-sm font-medium mb-1">Password</label>
            <div className="flex gap-2">
              <input
                type={showPassword ? 'text' : 'password'}
                value={loadingPassword ? 'Loading...' : password}
                onChange={(e) => setPassword(e.target.value)}
                className="flex-1 px-3 py-2 rounded border dark:bg-gray-700 dark:border-gray-600"
                disabled={loadingPassword}
                required
              />
              <button
                type="button"
                onClick={handleGenerate}
                className="px-3 py-2 bg-gray-200 dark:bg-gray-600 rounded"
              >
                Generate
              </button>
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="px-3 py-2 bg-gray-200 dark:bg-gray-600 rounded"
              >
                Show
              </button>
            </div>
          </div>
          
          <div>
            <label className="block text-sm font-medium mb-1">Notes</label>
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              className="w-full px-3 py-2 rounded border dark:bg-gray-700 dark:border-gray-600"
              rows={3}
            />
          </div>
          
          <div className="flex gap-2 justify-end">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 border rounded"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={loading}
              className="px-4 py-2 bg-primary text-white rounded disabled:opacity-50"
            >
              {loading ? 'Saving...' : 'Save'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
