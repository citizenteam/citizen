import { useState, useEffect } from 'preact/hooks';
import { useApi } from '../hooks/useApi';
import { componentDebugLog, errorLog } from '../utils/debug';

interface GitHubConfig {
  client_id: string;
  redirect_uri: string;
  is_active: boolean;
  configured_at: string;
}

interface GitHubConfigResponse {
  configured: boolean;
  client_id?: string;
  redirect_uri?: string;
  is_active?: boolean;
  configured_at?: string;
}

export default function GitHubOAuth() {
  const { request } = useApi();
  const [githubConfigured, setGithubConfigured] = useState(false);
  const [githubConnected, setGithubConnected] = useState(false);
  const [githubUsername, setGithubUsername] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [config, setConfig] = useState<GitHubConfig | null>(null);
  const [showSetup, setShowSetup] = useState(false);
  const [saving, setSaving] = useState(false);
  const [formData, setFormData] = useState({
    client_id: '',
    client_secret: '',
    redirect_uri: `${window.location.origin}/api/v1/github/auth/callback`
  });

  // Check GitHub connection status on mount
  useEffect(() => {
    checkGitHubStatus();
    loadConfig();
    
    // If we're in OAuth callback, start polling for status updates
    if (window.location.search.includes('code=')) {
      const pollInterval = setInterval(() => {
        checkGitHubStatus();
      }, 2000); // Check every 2 seconds
      
      // Stop polling after 30 seconds
      setTimeout(() => {
        clearInterval(pollInterval);
      }, 30000);
      
      return () => clearInterval(pollInterval);
    }
  }, []);

  const loadConfig = async () => {
    try {
      const response = await request({ url: '/github/config' }) as GitHubConfigResponse;
      componentDebugLog('Config response:', response);
      if (response && response.configured) {
        setConfig({
          client_id: response.client_id || '',
          redirect_uri: response.redirect_uri || '',
          is_active: response.is_active || false,
          configured_at: response.configured_at || ''
        });
      }
    } catch (error) {
      errorLog('Failed to load GitHub config:', error);
    }
  };

  const checkGitHubStatus = async () => {
    try {
      const response = await request({ url: '/github/status' });
      if (response) {
        setGithubConfigured(response.github_configured);
        setGithubConnected(response.github_connected);
        setGithubUsername(response.github_username);
      }
    } catch (error) {
      errorLog('Failed to check GitHub status:', error);
    }
  };

  const initGitHubAuth = async () => {
    // Check if configured first
    if (!githubConfigured) {
      setShowSetup(true);
      return;
    }

    try {
      setLoading(true);
      const response = await request({ url: '/github/auth/init' });
      if (response) {
        if (response.setup_required) {
          // Show setup instructions
          setShowSetup(true);
          return;
        }
        
        // Open GitHub OAuth in popup
        window.open(response.auth_url, 'github-oauth', 'width=600,height=700');
        
        // Listen for OAuth completion
        const handleMessage = (event: MessageEvent) => {
          if (event.data.type === 'github-oauth-success') {
            setGithubConnected(true);
            setGithubUsername(event.data.username);
            window.removeEventListener('message', handleMessage);
          }
        };
        window.addEventListener('message', handleMessage);
        
        // Also listen for URL changes (in case popup redirects back)
        const checkAuthStatus = () => {
          // Check if we're in a popup and handle OAuth result
          if (window.opener && window.location.search.includes('code=')) {
            // We're in the popup, notify parent and close
            window.opener.postMessage({
              type: 'github-oauth-success',
              username: 'GitHub User' // This will be updated by the parent's status check
            }, '*');
            window.close();
          }
        };
        
        // Check immediately
        checkAuthStatus();
        
        // Also set up a periodic check
        const authCheckInterval = setInterval(checkAuthStatus, 1000);
        setTimeout(() => clearInterval(authCheckInterval), 30000); // Stop after 30 seconds
      }
    } catch (error) {
      errorLog('Failed to initiate GitHub auth:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleSetup = async (e: Event) => {
    e.preventDefault();
    setSaving(true);

    try {
      await request({
        url: '/github/config',
        method: 'POST',
        data: formData
      });

      alert('GitHub OAuth configuration successful!');
      setShowSetup(false);
      loadConfig();
      checkGitHubStatus();
    } catch (error) {
      errorLog('Failed to save GitHub config:', error);
      alert('GitHub OAuth configuration failed!');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!confirm('Are you sure you want to delete the GitHub OAuth configuration?')) {
      return;
    }

    try {
      await request({ url: '/github/config', method: 'DELETE' });
      alert('GitHub OAuth configuration deleted!');
      setConfig(null);
      setGithubConfigured(false);
      setGithubConnected(false);
      setGithubUsername(null);
    } catch (error) {
      errorLog('Failed to delete GitHub config:', error);
      alert('Failed to delete GitHub OAuth configuration!');
    }
  };

  if (loading) {
    return (
      <div style={{ 
        backgroundColor: '#1e293b', 
        borderRadius: '16px', 
        padding: '24px', 
        border: '1px solid #334155',
        marginBottom: '24px'
      }}>
        <div className="flex items-center justify-center py-4">
          <div style={{ 
            width: '20px', 
            height: '20px', 
            border: '2px solid #64748b', 
            borderTop: '2px solid #f8fafc', 
            borderRadius: '50%'
          }} className="animate-spin"></div>
          <span style={{ marginLeft: '12px', color: '#cbd5e1' }}>Loading GitHub configuration...</span>
        </div>
      </div>
    );
  }

  return (
    <div style={{ 
      backgroundColor: '#1e293b', 
      borderRadius: '16px', 
      padding: '24px', 
      border: '1px solid #334155',
      marginBottom: '24px'
    }}>
      <div className="flex items-center gap-3 mb-4">
        <div style={{ 
          width: '32px', 
          height: '32px', 
          backgroundColor: config ? 'rgba(34, 197, 94, 0.2)' : 'rgba(239, 68, 68, 0.2)', 
          borderRadius: '8px', 
          display: 'flex', 
          alignItems: 'center', 
          justifyContent: 'center' 
        }}>
          {config ? (
            <svg style={{ width: '16px', height: '16px', color: '#22c55e' }} fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
            </svg>
          ) : (
            <svg style={{ width: '16px', height: '16px', color: '#ef4444' }} fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
            </svg>
          )}
        </div>
        <div className="flex-1">
          <h3 style={{ fontSize: '18px', fontWeight: '600', color: '#f8fafc', marginBottom: '4px' }}>
            GitHub OAuth Configuration
          </h3>
          <p style={{ fontSize: '14px', color: '#cbd5e1' }}>
            {config ? 'GitHub OAuth is configured and ready' : 'Configure GitHub OAuth for repository integration'}
          </p>
        </div>
        {config && (
          <div className="flex gap-2">
            <button
              onClick={() => setShowSetup(true)}
              style={{
                padding: '8px 12px',
                backgroundColor: 'rgba(100, 116, 139, 0.2)',
                color: '#cbd5e1',
                borderRadius: '8px',
                border: '1px solid #475569',
                fontSize: '14px',
                fontWeight: '500',
                cursor: 'pointer',
                transition: 'all 0.2s ease'
              }}
              onMouseEnter={(e) => {
                (e.target as HTMLElement).style.backgroundColor = 'rgba(100, 116, 139, 0.3)';
              }}
              onMouseLeave={(e) => {
                (e.target as HTMLElement).style.backgroundColor = 'rgba(100, 116, 139, 0.2)';
              }}
            >
              Update
            </button>
            <button
              onClick={handleDelete}
              style={{
                padding: '8px 12px',
                backgroundColor: 'rgba(239, 68, 68, 0.2)',
                color: '#fca5a5',
                borderRadius: '8px',
                border: '1px solid #dc2626',
                fontSize: '14px',
                fontWeight: '500',
                cursor: 'pointer',
                transition: 'all 0.2s ease'
              }}
              onMouseEnter={(e) => {
                (e.target as HTMLElement).style.backgroundColor = 'rgba(239, 68, 68, 0.3)';
              }}
              onMouseLeave={(e) => {
                (e.target as HTMLElement).style.backgroundColor = 'rgba(239, 68, 68, 0.2)';
              }}
            >
              Delete
            </button>
          </div>
        )}
      </div>

      {config ? (
        <div style={{ 
          padding: '16px', 
          backgroundColor: 'rgba(15, 23, 42, 0.5)', 
          borderRadius: '12px', 
          border: '1px solid #475569',
          marginBottom: '16px'
        }}>
          <div className="grid grid-cols-1 gap-3">
            <div>
              <p style={{ fontSize: '12px', color: '#94a3b8', marginBottom: '4px' }}>Client ID</p>
              <p style={{ fontSize: '14px', color: '#f8fafc', fontFamily: 'monospace' }}>{config.client_id}</p>
            </div>
            <div>
              <p style={{ fontSize: '12px', color: '#94a3b8', marginBottom: '4px' }}>Redirect URI</p>
              <p style={{ fontSize: '14px', color: '#f8fafc', fontFamily: 'monospace', wordBreak: 'break-all' }}>{config.redirect_uri}</p>
            </div>
            <div>
              <p style={{ fontSize: '12px', color: '#94a3b8', marginBottom: '4px' }}>Configured</p>
              <p style={{ fontSize: '14px', color: '#f8fafc' }}>{new Date(config.configured_at).toLocaleDateString()}</p>
            </div>
          </div>
        </div>
      ) : (
        <div style={{ textAlign: 'center', padding: '20px 0', marginBottom: '16px' }}>
          <p style={{ fontSize: '14px', color: '#94a3b8', marginBottom: '16px' }}>
            No GitHub OAuth configuration found. Set up your GitHub App to enable repository integration.
          </p>
          <button
            onClick={() => setShowSetup(true)}
            style={{
              padding: '12px 24px',
              backgroundColor: '#475569',
              color: '#f8fafc',
              borderRadius: '12px',
              border: 'none',
              fontSize: '14px',
              fontWeight: '600',
              cursor: 'pointer',
              transition: 'all 0.2s ease'
            }}
            onMouseEnter={(e) => {
              (e.target as HTMLElement).style.backgroundColor = '#64748b';
            }}
            onMouseLeave={(e) => {
              (e.target as HTMLElement).style.backgroundColor = '#475569';
            }}
          >
            Configure GitHub OAuth
          </button>
        </div>
      )}

      {/* GitHub Account Connection */}
      {config && (
        <div style={{ 
          padding: '16px', 
          backgroundColor: 'rgba(15, 23, 42, 0.5)', 
          borderRadius: '12px', 
          border: '1px solid #475569'
        }}>
          <h4 style={{ fontSize: '16px', fontWeight: '600', color: '#f8fafc', marginBottom: '12px' }}>
            GitHub Account
          </h4>
          
          {githubConnected ? (
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div style={{ 
                  width: '24px', 
                  height: '24px', 
                  backgroundColor: 'rgba(34, 197, 94, 0.2)', 
                  borderRadius: '6px', 
                  display: 'flex', 
                  alignItems: 'center', 
                  justifyContent: 'center' 
                }}>
                  <svg style={{ width: '12px', height: '12px', color: '#22c55e' }} fill="currentColor" viewBox="0 0 20 20">
                    <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                  </svg>
                </div>
                <div>
                  <p style={{ fontSize: '14px', color: '#f8fafc', fontWeight: '500' }}>
                    Connected as @{githubUsername}
                  </p>
                  <p style={{ fontSize: '12px', color: '#94a3b8' }}>
                    GitHub account is connected and ready
                  </p>
                </div>
              </div>
            </div>
          ) : (
            <div style={{ textAlign: 'center' }}>
              <p style={{ fontSize: '14px', color: '#94a3b8', marginBottom: '16px' }}>
                Connect your GitHub account to access repositories
              </p>
              <button
                onClick={initGitHubAuth}
                disabled={loading}
                style={{
                  padding: '12px 24px',
                  backgroundColor: loading ? '#475569' : '#64748b',
                  color: '#f8fafc',
                  borderRadius: '8px',
                  border: 'none',
                  fontSize: '14px',
                  fontWeight: '600',
                  cursor: loading ? 'not-allowed' : 'pointer',
                  opacity: loading ? '0.7' : '1',
                  transition: 'all 0.2s ease'
                }}
                onMouseEnter={(e) => {
                  if (!loading) {
                    (e.target as HTMLElement).style.backgroundColor = '#94a3b8';
                  }
                }}
                onMouseLeave={(e) => {
                  if (!loading) {
                    (e.target as HTMLElement).style.backgroundColor = '#64748b';
                  }
                }}
              >
                {loading ? 'Connecting...' : 'Connect GitHub Account'}
              </button>
            </div>
          )}
        </div>
      )}

      {showSetup && (
        <div style={{ 
          marginTop: '20px',
          padding: '24px',
          backgroundColor: '#0f172a',
          borderRadius: '12px',
          border: '1px solid #475569'
        }}>
          <h4 style={{ fontSize: '16px', fontWeight: '600', color: '#f8fafc', marginBottom: '16px' }}>
            GitHub OAuth Settings
          </h4>
          
          <form onSubmit={handleSetup}>
            <div style={{ marginBottom: '16px' }}>
              <label style={{ 
                display: 'block', 
                fontSize: '14px', 
                fontWeight: '500', 
                color: '#f8fafc', 
                marginBottom: '6px' 
              }}>
                Client ID
              </label>
              <input
                type="text"
                value={formData.client_id}
                onChange={(e) => setFormData({...formData, client_id: (e.target as HTMLInputElement).value})}
                required
                placeholder="GitHub OAuth App Client ID"
                style={{
                  width: '100%',
                  padding: '12px 16px',
                  border: '1px solid #475569',
                  borderRadius: '8px',
                  fontSize: '14px',
                  backgroundColor: '#1e293b',
                  color: '#f8fafc',
                  outline: 'none',
                  transition: 'all 0.2s ease'
                }}
                onFocus={(e) => {
                  (e.target as HTMLElement).style.borderColor = '#64748b';
                }}
                onBlur={(e) => {
                  (e.target as HTMLElement).style.borderColor = '#475569';
                }}
              />
            </div>

            <div style={{ marginBottom: '16px' }}>
              <label style={{ 
                display: 'block', 
                fontSize: '14px', 
                fontWeight: '500', 
                color: '#f8fafc', 
                marginBottom: '6px' 
              }}>
                Client Secret
              </label>
              <input
                type="password"
                value={formData.client_secret}
                onChange={(e) => setFormData({...formData, client_secret: (e.target as HTMLInputElement).value})}
                required
                placeholder="GitHub OAuth App Client Secret"
                style={{
                  width: '100%',
                  padding: '12px 16px',
                  border: '1px solid #475569',
                  borderRadius: '8px',
                  fontSize: '14px',
                  backgroundColor: '#1e293b',
                  color: '#f8fafc',
                  outline: 'none',
                  transition: 'all 0.2s ease'
                }}
                onFocus={(e) => {
                  (e.target as HTMLElement).style.borderColor = '#64748b';
                }}
                onBlur={(e) => {
                  (e.target as HTMLElement).style.borderColor = '#475569';
                }}
              />
            </div>

            <div style={{ marginBottom: '20px' }}>
              <label style={{ 
                display: 'block', 
                fontSize: '14px', 
                fontWeight: '500', 
                color: '#f8fafc', 
                marginBottom: '6px' 
              }}>
                Redirect URI
              </label>
              <input
                type="url"
                value={formData.redirect_uri}
                onChange={(e) => setFormData({...formData, redirect_uri: (e.target as HTMLInputElement).value})}
                required
                placeholder="OAuth callback URL"
                style={{
                  width: '100%',
                  padding: '12px 16px',
                  border: '1px solid #475569',
                  borderRadius: '8px',
                  fontSize: '14px',
                  backgroundColor: '#1e293b',
                  color: '#f8fafc',
                  outline: 'none',
                  transition: 'all 0.2s ease'
                }}
                onFocus={(e) => {
                  (e.target as HTMLElement).style.borderColor = '#64748b';
                }}
                onBlur={(e) => {
                  (e.target as HTMLElement).style.borderColor = '#475569';
                }}
              />
            </div>

            <div className="flex gap-3 mb-6">
              <button 
                type="submit" 
                disabled={saving}
                style={{
                  flex: '1',
                  padding: '12px 16px',
                  backgroundColor: saving ? '#475569' : '#64748b',
                  color: '#f8fafc',
                  borderRadius: '8px',
                  border: 'none',
                  fontSize: '14px',
                  fontWeight: '600',
                  cursor: saving ? 'not-allowed' : 'pointer',
                  opacity: saving ? '0.7' : '1',
                  transition: 'all 0.2s ease'
                }}
                onMouseEnter={(e) => {
                  if (!saving) {
                    (e.target as HTMLElement).style.backgroundColor = '#94a3b8';
                  }
                }}
                onMouseLeave={(e) => {
                  if (!saving) {
                    (e.target as HTMLElement).style.backgroundColor = '#64748b';
                  }
                }}
              >
                {saving ? 'Saving...' : 'Save Configuration'}
              </button>
              <button 
                type="button" 
                onClick={() => setShowSetup(false)}
                style={{
                  padding: '12px 16px',
                  backgroundColor: 'rgba(100, 116, 139, 0.2)',
                  color: '#cbd5e1',
                  borderRadius: '8px',
                  border: '1px solid #475569',
                  fontSize: '14px',
                  fontWeight: '600',
                  cursor: 'pointer',
                  transition: 'all 0.2s ease'
                }}
                onMouseEnter={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = 'rgba(100, 116, 139, 0.3)';
                }}
                onMouseLeave={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = 'rgba(100, 116, 139, 0.2)';
                }}
              >
                Cancel
              </button>
            </div>

            <div style={{ 
              padding: '16px',
              backgroundColor: 'rgba(71, 85, 105, 0.2)',
              borderRadius: '8px',
              border: '1px solid #475569'
            }}>
              <h5 style={{ fontSize: '14px', fontWeight: '600', color: '#f8fafc', marginBottom: '12px' }}>
                How to create a GitHub OAuth App:
              </h5>
              <ol style={{ color: '#cbd5e1', fontSize: '13px', lineHeight: '1.5', paddingLeft: '16px' }}>
                <li style={{ marginBottom: '4px' }}>Go to GitHub Settings → Developer settings → OAuth Apps</li>
                <li style={{ marginBottom: '4px' }}>Click "New OAuth App"</li>
                <li style={{ marginBottom: '4px' }}>Fill in application details</li>
                <li style={{ marginBottom: '4px' }}>Set Authorization callback URL to the Redirect URI above</li>
                <li>Copy the Client ID and Client Secret values</li>
              </ol>
            </div>
          </form>
        </div>
      )}
    </div>
  );
} 