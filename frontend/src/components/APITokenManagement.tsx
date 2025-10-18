import { useState, useEffect } from 'preact/hooks';
import { useApi } from '../hooks/useApi';
import type { APIToken, APITokenRequest, APITokenResponse } from '../types';

interface APITokenManagementProps {
  onMessage: (message: { text: string; type: 'success' | 'error' | 'info' }) => void;
}

export function APITokenManagement({ onMessage }: APITokenManagementProps) {
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [loading, setLoading] = useState(false);
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [createdToken, setCreatedToken] = useState<string | null>(null);
  const [formData, setFormData] = useState<APITokenRequest>({
    name: '',
    description: '',
  });

  const { request } = useApi();

  // Load tokens
  const loadTokens = async () => {
    setLoading(true);
    try {
      const tokens = await request({ url: '/citizen/api-tokens', method: 'GET' });
      if (tokens) {
        setTokens(tokens);
      }
    } catch (error) {
      onMessage({ text: 'Failed to load tokens: ' + error, type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  // Create token
  const createToken = async (e: Event) => {
    e.preventDefault();
    if (!formData.name.trim()) {
      onMessage({ text: 'Token name is required', type: 'error' });
      return;
    }

    setLoading(true);
    try {
      const tokenData = await request({
        url: '/citizen/api-tokens',
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        data: formData,
      });

      if (tokenData) {
        const tokenResponse = tokenData as APITokenResponse;
        setCreatedToken(tokenResponse.token);
        setFormData({ name: '', description: '' });
        setShowCreateForm(false);
        onMessage({ text: 'API token created successfully!', type: 'success' });
        loadTokens();
      }
    } catch (error) {
      onMessage({ text: 'Failed to create token: ' + error, type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  // Delete token
  const deleteToken = async (tokenId: number, tokenName: string) => {
    if (!confirm(`Are you sure you want to delete the "${tokenName}" token?`)) {
      return;
    }

    setLoading(true);
    try {
      await request({
        url: `/citizen/api-tokens/${tokenId}`,
        method: 'DELETE',
      });

      onMessage({ text: 'Token deleted successfully', type: 'success' });
      loadTokens();
    } catch (error) {
      onMessage({ text: 'Failed to delete token: ' + error, type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  // Copy token to clipboard
  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      onMessage({ text: 'Token copied to clipboard!', type: 'success' });
    } catch (error) {
      onMessage({ text: 'Failed to copy to clipboard', type: 'error' });
    }
  };

  // Format date
  const formatDate = (dateString: string | undefined) => {
    if (!dateString) return 'Never used';
    return new Date(dateString).toLocaleString();
  };

  // Load tokens on mount
  useEffect(() => {
    loadTokens();
  }, []);

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h3 className="text-lg font-medium text-gray-900">API Token Management</h3>
        <button
          onClick={() => setShowCreateForm(true)}
          className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
        >
          <svg className="-ml-1 mr-2 h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
          </svg>
          New Token
        </button>
      </div>

      {/* Created Token Display */}
      {createdToken && (
        <div className="bg-green-50 border border-green-200 rounded-md p-4">
          <div className="flex items-start">
            <div className="flex-shrink-0">
              <svg className="h-5 w-5 text-green-400" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
              </svg>
            </div>
            <div className="ml-3 flex-1">
              <h4 className="text-sm font-medium text-green-800">Token Created!</h4>
              <p className="mt-1 text-sm text-green-700">
                This token is only shown once. Make sure to save it in a secure location.
              </p>
              <div className="mt-3 flex items-center space-x-2">
                <code className="bg-white px-3 py-2 rounded border text-sm font-mono break-all">
                  {createdToken}
                </code>
                <button
                  onClick={() => copyToClipboard(createdToken)}
                  className="inline-flex items-center px-3 py-2 border border-green-300 text-sm font-medium rounded-md text-green-700 bg-white hover:bg-green-50"
                >
                  Copy
                </button>
              </div>
              <button
                onClick={() => setCreatedToken(null)}
                className="mt-3 text-sm text-green-600 hover:text-green-500"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Create Token Form */}
      {showCreateForm && (
        <div className="bg-white shadow rounded-lg p-6">
          <h4 className="text-lg font-medium text-gray-900 mb-4">New API Token</h4>
          <form onSubmit={createToken} className="space-y-4">
            <div>
              <label htmlFor="token-name" className="block text-sm font-medium text-gray-700">
                Token Name *
              </label>
              <input
                type="text"
                id="token-name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: (e.target as HTMLInputElement).value })}
                className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                placeholder="e.g. CI/CD Pipeline"
                required
              />
            </div>
            <div>
              <label htmlFor="token-description" className="block text-sm font-medium text-gray-700">
                Description
              </label>
              <textarea
                id="token-description"
                value={formData.description}
                onChange={(e) => setFormData({ ...formData, description: (e.target as HTMLTextAreaElement).value })}
                rows={2}
                className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                placeholder="Describe what this token will be used for..."
              />
            </div>
            <div className="flex justify-end space-x-3">
              <button
                type="button"
                onClick={() => setShowCreateForm(false)}
                className="inline-flex items-center px-4 py-2 border border-gray-300 text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={loading}
                className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50"
              >
                {loading ? 'Creating...' : 'Create'}
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Tokens List */}
      <div className="bg-white shadow rounded-lg">
        <div className="px-6 py-4 border-b border-gray-200">
          <h4 className="text-lg font-medium text-gray-900">Current Tokens</h4>
        </div>
        
        {loading ? (
          <div className="p-6 text-center">
            <div className="inline-block animate-spin rounded-full h-6 w-6 border-b-2 border-indigo-600"></div>
            <p className="mt-2 text-sm text-gray-500">Loading...</p>
          </div>
        ) : tokens.length === 0 ? (
          <div className="p-6 text-center">
            <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M15 7a2 2 0 012 2m0 0a2 2 0 012 2v6a2 2 0 01-2 2h-6a2 2 0 01-2-2V9a2 2 0 012-2m0 0V7a2 2 0 012-2m-6 2a2 2 0 00-2 2v6a2 2 0 002 2h6a2 2 0 002-2v-2m-6 0a2 2 0 002 2h2a2 2 0 002-2" />
            </svg>
            <h3 className="mt-2 text-sm font-medium text-gray-900">No tokens yet</h3>
            <p className="mt-1 text-sm text-gray-500">Create a new token for API access.</p>
          </div>
        ) : (
          <div className="divide-y divide-gray-200">
            {tokens.map((token) => (
              <div key={token.id} className="p-6">
                <div className="flex items-center justify-between">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center space-x-3">
                      <h4 className="text-sm font-medium text-gray-900 truncate">{token.name}</h4>
                      <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                        Active
                      </span>
                    </div>
                    {token.description && (
                      <p className="mt-1 text-sm text-gray-500 truncate">{token.description}</p>
                    )}
                    <div className="mt-2 flex items-center space-x-4 text-xs text-gray-500">
                      <span>Token: <code className="bg-gray-100 px-1 rounded">{token.token_prefix}...</code></span>
                      <span>Usage: {token.usage_count} times</span>
                      <span>Last used: {formatDate(token.last_used_at)}</span>
                      <span>Created: {formatDate(token.created_at)}</span>
                    </div>
                  </div>
                  <div className="flex-shrink-0">
                    <button
                      onClick={() => deleteToken(token.id, token.name)}
                      className="inline-flex items-center px-3 py-1 border border-transparent text-sm font-medium rounded text-red-700 bg-red-100 hover:bg-red-200 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}