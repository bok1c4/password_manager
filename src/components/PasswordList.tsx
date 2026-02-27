import { useState } from 'react';
import { useVault, Entry } from '../hooks/useVault';
import { PasswordForm } from './PasswordForm';

export function PasswordList() {
  const { entries, deleteEntry, getPassword, loading } = useVault();
  const [showForm, setShowForm] = useState(false);
  const [search, setSearch] = useState('');
  const [selectedEntry, setSelectedEntry] = useState<Entry | undefined>(undefined);
  const [copyingId, setCopyingId] = useState<string | null>(null);
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const filteredEntries = entries.filter(e => 
    e.site.toLowerCase().includes(search.toLowerCase()) ||
    e.username.toLowerCase().includes(search.toLowerCase())
  );

  const handleEdit = (entry: Entry) => {
    setSelectedEntry(entry);
    setShowForm(true);
  };

  const handleCloseForm = () => {
    setShowForm(false);
    setSelectedEntry(undefined);
  };

  const handleDelete = async (id: string) => {
    if (confirm('Are you sure you want to delete this password?')) {
      await deleteEntry(id);
    }
  };

  const handleCopy = async (id: string) => {
    try {
      setCopyingId(id);
      const password = await getPassword(id);
      const { writeText } = await import('@tauri-apps/plugin-clipboard-manager');
      await writeText(password);
      setCopiedId(id);
      setTimeout(() => setCopiedId(null), 1500);
      
      // Auto-clear clipboard after 30 seconds
      setTimeout(async () => {
        try {
          const { readText } = await import('@tauri-apps/plugin-clipboard-manager');
          const currentClipboard = await readText();
          if (currentClipboard === password) {
            await writeText('');
          }
        } catch (e) {
          // Ignore clipboard read errors
        }
      }, 30000);
    } catch (e) {
      console.error('Failed to copy:', e);
      alert('Failed to copy password. Make sure the vault is unlocked.');
    } finally {
      setCopyingId(null);
    }
  };

  return (
    <div className="p-4">
      <div className="flex gap-4 mb-4">
        <input
          type="text"
          placeholder="Search passwords..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="flex-1 px-4 py-2 rounded-lg border dark:bg-gray-700 dark:border-gray-600"
        />
        <button
          onClick={() => {
            setSelectedEntry(undefined);
            setShowForm(true);
          }}
          className="px-4 py-2 bg-primary text-white rounded-lg"
        >
          + Add
        </button>
      </div>

      {loading && <p className="text-center py-4">Loading...</p>}

      <div className="space-y-2">
        {filteredEntries.map((entry) => (
          <div
            key={entry.id}
            className="flex items-center justify-between p-4 bg-white dark:bg-surface-dark rounded-lg shadow"
          >
            <div className="flex-1">
              <h3 className="font-medium">{entry.site}</h3>
              <p className="text-sm text-gray-500">{entry.username}</p>
            </div>
            <div className="flex gap-2">
              <button
                onClick={() => handleCopy(entry.id)}
                disabled={copyingId === entry.id}
                className={`px-3 py-1 text-sm rounded ${
                  copiedId === entry.id
                    ? 'bg-green-500 text-white'
                    : 'border border-gray-300 rounded hover:bg-gray-100'
                }`}
              >
                {copyingId === entry.id ? 'Copying...' : copiedId === entry.id ? 'Copied!' : 'Copy'}
              </button>
              <button
                onClick={() => handleEdit(entry)}
                className="px-3 py-1 text-sm border rounded"
              >
                Edit
              </button>
              <button
                onClick={() => handleDelete(entry.id)}
                className="px-3 py-1 text-sm text-red-500 border border-red-500 rounded"
              >
                Delete
              </button>
            </div>
          </div>
        ))}
      </div>

      {filteredEntries.length === 0 && !loading && (
        <p className="text-center py-8 text-gray-500">
          No passwords yet. Click "Add" to get started.
        </p>
      )}

      {showForm && (
        <PasswordForm
          key={selectedEntry?.id || 'new'}
          entry={selectedEntry}
          onClose={handleCloseForm}
        />
      )}
    </div>
  );
}
