import { useState, useEffect } from 'preact/hooks';
import { useApi } from '../hooks/useApi';
import { componentDebugLog, errorLog } from '../utils/debug';

interface DockerConnectionResponse {
  connected: boolean;
  username?: string;
}

interface DockerConnectionProps {
  onStatusChange?: (connected: boolean) => void;
}

export default function DockerConnection({ onStatusChange }: DockerConnectionProps) {
  const { request } = useApi();
  const [connected, setConnected] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [connectionData, setConnectionData] = useState<DockerConnectionResponse | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [message, setMessage] = useState<{ text: string; type: 'success' | 'error' } | null>(null);
  
  const [formData, setFormData] = useState({
    username: '',
    access_token: ''
  });

  useEffect(() => {
    loadConnectionStatus();
  }, []);

  const loadConnectionStatus = async () => {
    try {
      setLoading(true);
      const response = await request({ url: '/citizen/docker/connection' });
      componentDebugLog('Docker connection status response:', response);
      
      if (response && response.connected === true) {
        componentDebugLog('Setting connected to true');
        setConnectionData(response);
        setConnected(true);
        onStatusChange?.(true);
      } else {
        componentDebugLog('Setting connected to false, response:', response);
        setConnectionData(null);
        setConnected(false);
        onStatusChange?.(false);
      }
    } catch (error) {
      errorLog('Failed to load Docker connection status:', error);
      setConnectionData(null);
      setConnected(false);
      onStatusChange?.(false);
    } finally {
      setLoading(false);
    }
  };

  const showMessage = (text: string, type: 'success' | 'error') => {
    setMessage({ text, type });
    setTimeout(() => setMessage(null), 5000);
  };

  const handleConnect = async (e: Event) => {
    e.preventDefault();
    setSaving(true);

    try {
      componentDebugLog('Sending connect request...');
      const response = await request({
        url: '/citizen/docker/connection',
        method: 'POST',
        data: formData
      });

      componentDebugLog('Connect response:', response);

      if (response && response.connected) {
        componentDebugLog('Connect successful, updating state...');
        setConnected(true);
        setConnectionData(response);
        setShowForm(false);
        setFormData({ username: '', access_token: '' });
        showMessage('Docker Hub connection established successfully!', 'success');
        onStatusChange?.(true);
        componentDebugLog('State updated after connect');
      } else {
        componentDebugLog('Connect failed, response:', response);
        showMessage('Failed to establish Docker Hub connection', 'error');
      }
    } catch (error: any) {
      errorLog('Failed to connect Docker Hub:', error);
      const errorMessage = error?.response?.data?.message || 'Failed to establish Docker Hub connection';
      showMessage(errorMessage, 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleDisconnect = async () => {
    if (!confirm('Are you sure you want to disconnect from Docker Hub?')) {
      return;
    }

    setSaving(true);
    try {
      const response = await request({
        url: '/citizen/docker/connection',
        method: 'DELETE'
      });

      if (response) {
        // Reload connection status after disconnect
        await loadConnectionStatus();
        showMessage('Docker Hub disconnected successfully', 'success');
      } else {
        showMessage('Failed to disconnect Docker Hub', 'error');
      }
    } catch (error: any) {
      errorLog('Failed to disconnect Docker Hub:', error);
      const errorMessage = error?.response?.data?.message || 'Failed to disconnect Docker Hub';
      showMessage(errorMessage, 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleTest = async () => {
    if (!formData.username || !formData.access_token) {
      showMessage('Please enter both username and access token', 'error');
      return;
    }

    setTesting(true);
    try {
      const response = await request({
        url: '/citizen/docker/test',
        method: 'POST',
        data: formData
      });

      if (response) {
        showMessage('Docker Hub connection test successful!', 'success');
      } else {
        showMessage('Docker Hub connection test failed', 'error');
      }
    } catch (error: any) {
      errorLog('Docker Hub test failed:', error);
      const errorMessage = error?.response?.data?.message || 'Docker Hub connection test failed';
      showMessage(errorMessage, 'error');
    } finally {
      setTesting(false);
    }
  };

  if (loading) {
    return (
      <div className="text-center py-4">
        <div className="w-6 h-6 border-2 border-gray-300 border-t-gray-800 rounded-full animate-spin mx-auto mb-2"></div>
        <p className="text-sm text-gray-600">Loading Docker connection...</p>
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <div>
          <h3 className="text-lg font-semibold text-gray-900">Docker Hub Connection</h3>
          <p className="text-sm text-gray-600">Connect your Docker Hub account for container operations</p>
        </div>
        <div className="flex items-center gap-2">
          <div className={`w-3 h-3 rounded-full ${connected ? 'bg-green-500' : 'bg-gray-300'}`}></div>
          <span className={`text-sm font-medium ${connected ? 'text-green-700' : 'text-gray-500'}`}>
            {connected ? 'Connected' : 'Not Connected'}
          </span>
        </div>
      </div>

      {message && (
        <div className={`p-3 rounded-md mb-4 ${
          message.type === 'success' ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-800'
        }`}>
          <p className="text-sm">{message.text}</p>
        </div>
      )}

      {connected ? (
        <div className="bg-green-50 border border-green-200 rounded-lg p-4 mb-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-green-800">
                {connectionData?.username ? `Connected as: ${connectionData.username}` : 'Docker Hub Connected'}
              </p>
              <p className="text-xs text-green-600">
                Ready for container operations
              </p>
            </div>
            <button
              onClick={handleDisconnect}
              disabled={saving}
              className="px-4 py-2 bg-red-600 text-white text-sm rounded-md hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {saving ? 'Disconnecting...' : 'Disconnect'}
            </button>
          </div>
        </div>
      ) : (
        <div>
          {!showForm ? (
            <div className="bg-gray-50 border border-gray-200 rounded-lg p-4 text-center">
              <div className="mb-4">
                <svg className="w-12 h-12 text-gray-400 mx-auto mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                </svg>
                <p className="text-gray-600 mb-4">Connect your Docker Hub account to enable container operations</p>
              </div>
              <button
                onClick={() => setShowForm(true)}
                className="px-6 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 transition-colors"
              >
                Connect Docker Hub
              </button>
            </div>
          ) : (
            <div className="bg-white border border-gray-200 rounded-lg p-6">
              <div className="mb-4">
                <h4 className="text-md font-medium text-gray-900 mb-2">Connect Docker Hub Account</h4>
                <p className="text-sm text-gray-600 mb-4">
                  Enter your Docker Hub username and personal access token. 
                  <a href="https://docs.docker.com/docker-hub/access-tokens/" target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline ml-1">
                    Learn how to create an access token
                  </a>
                </p>
              </div>

              <form onSubmit={handleConnect}>
                <div className="space-y-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1">
                      Docker Hub Username
                    </label>
                    <input
                      type="text"
                      value={formData.username}
                      onInput={(e) => setFormData({...formData, username: (e.target as HTMLInputElement).value})}
                      placeholder="your-docker-username"
                      required
                      className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                      disabled={saving || testing}
                    />
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1">
                      Personal Access Token
                    </label>
                    <input
                      type="password"
                      value={formData.access_token}
                      onInput={(e) => setFormData({...formData, access_token: (e.target as HTMLInputElement).value})}
                      placeholder="dckr_pat_..."
                      required
                      className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                      disabled={saving || testing}
                    />
                    <p className="text-xs text-gray-500 mt-1">
                      Your access token will be encrypted and stored securely
                    </p>
                  </div>

                  <div className="flex gap-3">
                    <button
                      type="button"
                      onClick={handleTest}
                      disabled={testing || saving || !formData.username || !formData.access_token}
                      className="px-4 py-2 bg-gray-600 text-white rounded-md hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                    >
                      {testing ? 'Testing...' : 'Test Connection'}
                    </button>
                    
                    <button
                      type="submit"
                      disabled={saving || testing || !formData.username || !formData.access_token}
                      className="flex-1 px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                    >
                      {saving ? 'Connecting...' : 'Connect Docker Hub'}
                    </button>
                    
                    <button
                      type="button"
                      onClick={() => {
                        setShowForm(false);
                        setFormData({ username: '', access_token: '' });
                      }}
                      className="px-4 py-2 bg-gray-300 text-gray-700 rounded-md hover:bg-gray-400 transition-colors"
                      disabled={saving || testing}
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              </form>
            </div>
          )}
        </div>
      )}
    </div>
  );
} 