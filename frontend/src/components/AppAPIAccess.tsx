import { useState, useEffect } from 'preact/hooks';
import { useApi } from '../hooks/useApi';
import type { AppAPIAccess, AppAPIAccessRequest, OperationInfo } from '../types';

interface AppAPIAccessProps {
  appName: string;
  onMessage: (message: { text: string; type: 'success' | 'error' | 'info' }) => void;
}

export function AppAPIAccess({ appName, onMessage }: AppAPIAccessProps) {
  const [access, setAccess] = useState<AppAPIAccess | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const { request } = useApi();

  // Load app API access config
  const loadAccess = async () => {
    console.log('Loading access config for app:', appName);
    setLoading(true);
    try {
      const accessData = await request({ url: `/citizen/apps/${appName}/api-access`, method: 'GET' });
      console.log('Loaded access data:', accessData);
      if (accessData) {
        setAccess(accessData);
      } else {
        console.error('No access data returned');
      }
    } catch (error) {
      console.error('Load access error:', error);
      onMessage({ text: 'Failed to load API access settings: ' + error, type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  // Save access configuration
  const saveAccess = async (newAccess: AppAPIAccessRequest) => {
    console.log('Saving access config:', newAccess);
    setSaving(true);
    
    const updatedAccess = await request({
      url: `/citizen/apps/${appName}/api-access`,
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      data: newAccess,
    });

    console.log('API response:', updatedAccess);
    setSaving(false);
    
    if (updatedAccess) {
      setAccess(updatedAccess);
      onMessage({ text: 'API access settings updated successfully!', type: 'success' });
      return updatedAccess;
    } else {
      console.error('No data returned from API');
      onMessage({ text: 'Failed to update settings: No data returned from API', type: 'error' });
      throw new Error('No data returned from API');
    }
  };

  // Toggle API access
  const toggleAPIAccess = async () => {
    if (!access) return;
    
    const newEnabled = !access.api_access_enabled;
    const originalAccess = { ...access };
    
    // Optimistically update UI
    setAccess({
      ...access,
      api_access_enabled: newEnabled
    });
    
    const newAccess: AppAPIAccessRequest = {
      api_access_enabled: newEnabled,
      rate_limit_per_minute: access.rate_limit_per_minute,
    };
    
    try {
      await saveAccess(newAccess);
    } catch (error) {
      console.error('Toggle API access error:', error);
      // Revert on error
      setAccess(originalAccess);
    }
  };

  // Operations are no longer configurable - API tokens have full access when enabled

  // Update rate limit
  const updateRateLimit = async (rateLimit: number) => {
    if (!access) return;
    
    // Optimistically update UI
    const originalAccess = { ...access };
    setAccess({
      ...access,
      rate_limit_per_minute: rateLimit
    });
    
    const newAccess: AppAPIAccessRequest = {
      api_access_enabled: access.api_access_enabled,
      rate_limit_per_minute: rateLimit,
    };
    
    try {
      await saveAccess(newAccess);
    } catch (error) {
      // Revert on error
      setAccess(originalAccess);
    }
  };

  // Load data on mount
  useEffect(() => {
    loadAccess();
  }, [appName]);

  if (loading) {
    return (
      <div className="p-6 text-center">
        <div className="inline-block animate-spin rounded-full h-6 w-6 border-b-2 border-indigo-600"></div>
        <p className="mt-2 text-sm text-gray-500">Loading...</p>
      </div>
    );
  }

  if (!access) {
    return (
      <div className="p-6 text-center">
        <p className="text-sm text-gray-500">Failed to load API access settings.</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h3 className="text-lg font-medium text-gray-900">API Access Settings</h3>
        <div className="flex items-center">
          <span className="mr-3 text-sm font-medium text-gray-700">API Access</span>
          <button
            type="button"
            onClick={toggleAPIAccess}
            disabled={saving}
            className={`${
              access.api_access_enabled ? 'bg-indigo-600' : 'bg-gray-200'
            } relative inline-flex flex-shrink-0 h-6 w-11 border-2 border-transparent rounded-full cursor-pointer transition-colors ease-in-out duration-200 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50`}
          >
            <span
              className={`${
                access.api_access_enabled ? 'translate-x-5' : 'translate-x-0'
              } pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow transform ring-0 transition ease-in-out duration-200`}
            />
          </button>
        </div>
      </div>

      {access.api_access_enabled && (
        <div className="space-y-6">
          {/* Full Access Info */}
          <div className="bg-green-50 border border-green-200 rounded-md p-4">
            <div className="flex">
              <div className="flex-shrink-0">
                <svg className="h-5 w-5 text-green-400" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
              </div>
              <div className="ml-3">
                <h4 className="text-sm font-medium text-green-800">Full API Access Enabled</h4>
                <div className="mt-1 text-sm text-green-700">
                  <p>API tokens have complete access to all operations including:</p>
                  <ul className="mt-2 list-disc list-inside space-y-1">
                    <li>Deploy and restart applications</li>
                    <li>View and manage environment variables</li>
                    <li>Access logs and application status</li>
                    <li>Manage domains and configurations</li>
                    <li>All other application operations</li>
                  </ul>
                </div>
              </div>
            </div>
          </div>

          {/* Rate Limit */}
          <div>
            <label htmlFor="rate-limit" className="block text-sm font-medium text-gray-700 mb-2">
              Rate Limit (requests per minute)
            </label>
            <div className="flex items-center space-x-2">
              <input
                type="number"
                id="rate-limit"
                value={access.rate_limit_per_minute}
                onChange={(e) => {
                  const value = parseInt((e.target as HTMLInputElement).value);
                  if (!isNaN(value) && value > 0) {
                    updateRateLimit(value);
                  }
                }}
                disabled={saving}
                min="1"
                max="1000"
                className="block w-24 border-gray-300 rounded-md shadow-sm focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm disabled:opacity-50"
              />
              <span className="text-sm text-gray-500">requests/minute</span>
            </div>
            <p className="mt-1 text-xs text-gray-500">
              Maximum number of requests that can be made with API tokens per minute
            </p>
          </div>

          {/* Usage Info */}
          <div className="bg-blue-50 border border-blue-200 rounded-md p-4">
            <div className="flex">
              <div className="flex-shrink-0">
                <svg className="h-5 w-5 text-blue-400" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" />
                </svg>
              </div>
              <div className="ml-3">
                <h4 className="text-sm font-medium text-blue-800">API Usage</h4>
                <div className="mt-1 text-sm text-blue-700">
                  <p>API access is enabled for this app. API tokens can access these endpoints:</p>
                  <ul className="mt-2 list-disc list-inside space-y-1">
                    <li><code className="bg-blue-100 px-1 rounded text-xs">GET /api/v1/citizen/apps/{appName}/*</code></li>
                    <li><code className="bg-blue-100 px-1 rounded text-xs">POST /api/v1/citizen/apps/{appName}/*</code></li>
                    <li><code className="bg-blue-100 px-1 rounded text-xs">Authorization: Bearer YOUR_TOKEN</code></li>
                  </ul>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {!access.api_access_enabled && (
        <div className="text-center py-8">
          <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
          </svg>
          <h3 className="mt-2 text-sm font-medium text-gray-900">API Access Disabled</h3>
          <p className="mt-1 text-sm text-gray-500">
            API access is not enabled for this app. Enable the toggle above to allow API token access.
          </p>
        </div>
      )}

      {saving && (
        <div className="fixed inset-0 bg-black bg-opacity-25 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-4 flex items-center space-x-3">
            <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-indigo-600"></div>
            <span className="text-sm text-gray-700">Saving...</span>
          </div>
        </div>
      )}
    </div>
  );
}