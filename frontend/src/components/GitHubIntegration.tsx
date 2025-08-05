import { errorLog, componentDebugLog } from "../utils/debug";
import { useState, useEffect } from 'preact/hooks';
import { useApi } from '../hooks/useApi';

interface GitHubRepository {
  id: number;
  name: string;
  full_name: string;
  private: boolean;
  html_url: string;
  clone_url: string;
  default_branch: string;
  description: string;
  owner: {
    login: string;
  };
}

interface GitHubIntegrationProps {
  appName: string;
  onRepositoryConnect?: (repo: GitHubRepository, autoDeploy: boolean) => void;
}

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

export default function GitHubIntegration({ appName, onRepositoryConnect }: GitHubIntegrationProps) {
  const { request } = useApi();
  const [githubConfigured, setGithubConfigured] = useState(false);
  const [githubConnected, setGithubConnected] = useState(false);
  const [githubUsername, setGithubUsername] = useState<string | null>(null);
  const [repositories, setRepositories] = useState<GitHubRepository[]>([]);
  const [loading, setLoading] = useState(false);
  const [showRepoSelector, setShowRepoSelector] = useState(false);
  const [selectedRepo, setSelectedRepo] = useState<GitHubRepository | null>(null);
  const [autoDeploy, setAutoDeploy] = useState(true);
  const [deployBranch, setDeployBranch] = useState('main');
  const [showSetupModal, setShowSetupModal] = useState(false);
  const [clientId, setClientId] = useState('');
  const [clientSecret, setClientSecret] = useState('');
  const [redirectUri, setRedirectUri] = useState('');
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

  // Auto-load repositories when GitHub is connected
  useEffect(() => {
    if (githubConnected && repositories.length === 0) {
      loadRepositories();
    }
  }, [githubConnected]);

  // Check if this app has a connected repository
  useEffect(() => {
    if (githubConnected && appName) {
      checkConnectedRepository();
    }
  }, [githubConnected, appName]);

  const checkConnectedRepository = async () => {
    try {
      const response = await request({ url: '/github/connections' });
      if (response && response.connections) {
        const connectedRepo = response.connections.find((conn: any) => conn.app_name === appName);
        if (connectedRepo) {
          // Repository is already connected, show connected state
          const repoData = {
            id: connectedRepo.github_id,
            name: connectedRepo.name,
            full_name: connectedRepo.full_name,
            clone_url: connectedRepo.clone_url,
            html_url: connectedRepo.html_url,
            private: connectedRepo.private,
            default_branch: connectedRepo.default_branch,
            description: `Connected repository`,
            owner: { login: connectedRepo.owner }
          };
          setSelectedRepo(repoData);
          setDeployBranch(connectedRepo.deploy_branch);
          setAutoDeploy(connectedRepo.auto_deploy);
        }
      }
    } catch (error) {
      errorLog('Failed to check connected repository:', error);
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

  const setupGitHubOAuth = async () => {
    try {
      setLoading(true);
      const response = await request({
        url: '/github/setup',
        method: 'POST',
        data: {
          client_id: clientId,
          client_secret: clientSecret,
        },
      });
      
      if (response) {
        setGithubConfigured(true);
        setShowSetupModal(false);
        setRedirectUri(response.redirect_uri);
        // Refresh status
        checkGitHubStatus();
      }
    } catch (error) {
      errorLog('Failed to setup GitHub OAuth:', error);
    } finally {
      setLoading(false);
    }
  };

  const initGitHubAuth = async () => {
    // Check if configured first
    if (!githubConfigured) {
      setShowSetupModal(true);
      return;
    }

    try {
      setLoading(true);
      const response = await request({ url: '/github/auth/init' });
      if (response) {
        if (response.setup_required) {
          // Show setup instructions
          setRedirectUri(response.redirect_uri);
          setShowSetupModal(true);
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
            // Reload repositories when connected (with delay to ensure backend is ready)
            setTimeout(() => {
              loadRepositories();
            }, 500);
          }
        };
        window.addEventListener('message', handleMessage);
        
        // Also listen for URL changes (in case popup redirects back)
        const checkAuthStatus = () => {
          // Check if we're in a popup and handle OAuth result
          if (window.opener && window.location.search.includes('code=')) {
            // We're in OAuth popup, notify parent and close
            window.opener.postMessage({
              type: 'github-oauth-success',
              username: 'GitHub User' // Will be updated by status check
            }, '*');
            window.close();
          } else if (window.location.search.includes('code=')) {
            // Regular page with OAuth callback, refresh to clear URL and update status
            window.history.replaceState({}, document.title, window.location.pathname);
            // Trigger status check after a short delay to ensure backend has processed
            setTimeout(() => {
              checkGitHubStatus();
            }, 1000);
          } else {
            // Regular page, recheck status
            checkGitHubStatus();
          }
        };
        
        // Check on page load
        checkAuthStatus();
      }
    } catch (error) {
      errorLog('Failed to init GitHub OAuth:', error);
    } finally {
      setLoading(false);
    }
  };

  const loadRepositories = async () => {
    try {
      setLoading(true);
      const response = await request({ url: '/github/repositories' });
      if (response) {
        setRepositories(response.repositories);
        setShowRepoSelector(true);
      }
    } catch (error) {
      errorLog('Failed to load repositories:', error);
    } finally {
      setLoading(false);
    }
  };

  const connectRepository = async () => {
    if (!selectedRepo) return;

    try {
      setLoading(true);
      const response = await request({
        url: '/github/connect',
        method: 'POST',
        data: {
          app_name: appName,
          repository_id: selectedRepo.id,
          full_name: selectedRepo.full_name,
          auto_deploy: autoDeploy,
          deploy_branch: deployBranch,
        },
      });
      
      if (response) {
        onRepositoryConnect?.(selectedRepo, autoDeploy);
        setShowRepoSelector(false);
      }
    } catch (error) {
      errorLog('Failed to connect repository:', error);
    } finally {
      setLoading(false);
    }
  };

  const loadConfig = async () => {
    try {
      const response = await request({ url: '/github/config' });
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
    } catch (error) {
      errorLog('Failed to delete GitHub config:', error);
      alert('Failed to delete GitHub OAuth configuration!');
    }
  };

  if (!githubConnected) {
    return (
      <>
        <div style={{ 
          backgroundColor: '#1e293b', 
          borderRadius: '16px', 
          padding: '24px', 
          border: '1px solid #334155' 
        }}>
          <div className="flex items-center gap-4">
            <div style={{ 
              width: '48px', 
              height: '48px', 
              backgroundColor: '#0f172a', 
              borderRadius: '12px', 
              display: 'flex', 
              alignItems: 'center', 
              justifyContent: 'center' 
            }}>
              <svg style={{ width: '24px', height: '24px', color: '#ffffff' }} fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M10 0C4.477 0 0 4.484 0 10.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0110 4.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.203 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.942.359.31.678.921.678 1.856 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0020 10.017C20 4.484 15.522 0 10 0z" clipRule="evenodd" />
              </svg>
            </div>
            <div className="flex-1">
              <h3 style={{ fontSize: '18px', fontWeight: '600', color: '#f8fafc', marginBottom: '4px' }}>
                GitHub Integration
              </h3>
              <p style={{ fontSize: '14px', color: '#cbd5e1' }}>
                Connect your GitHub account to automatically deploy repositories
              </p>
              {!githubConfigured && (
                <p style={{ fontSize: '12px', color: '#fbbf24', marginTop: '4px' }}>
                  ⚙️ GitHub OAuth configuration required
                </p>
              )}
            </div>
            <button
              onClick={initGitHubAuth}
              disabled={loading}
              style={{
                padding: '12px 16px',
                backgroundColor: '#0f172a',
                color: '#ffffff',
                borderRadius: '12px',
                border: 'none',
                fontSize: '14px',
                fontWeight: '500',
                cursor: loading ? 'not-allowed' : 'pointer',
                opacity: loading ? '0.5' : '1',
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                transition: 'all 0.2s ease'
              }}
              onMouseEnter={(e) => {
                if (!loading) {
                  (e.target as HTMLElement).style.backgroundColor = '#1e293b';
                }
              }}
              onMouseLeave={(e) => {
                if (!loading) {
                  (e.target as HTMLElement).style.backgroundColor = '#0f172a';
                }
              }}
            >
              {loading ? (
                <div style={{ 
                  width: '16px', 
                  height: '16px', 
                  border: '2px solid #ffffff', 
                  borderTop: '2px solid transparent', 
                  borderRadius: '50%' 
                }} className="animate-spin"></div>
              ) : (
                <svg style={{ width: '16px', height: '16px' }} fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 0C4.477 0 0 4.484 0 10.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0110 4.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.203 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.942.359.31.678.921.678 1.856 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0020 10.017C20 4.484 15.522 0 10 0z" clipRule="evenodd" />
                </svg>
              )}
              {githubConfigured ? 'Connect to GitHub' : 'Setup GitHub'}
            </button>
          </div>
        </div>

        {/* GitHub Setup Modal */}
        {showSetupModal && (
          <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
            <div className="bg-white rounded-2xl p-6 max-w-md w-full mx-4">
              <div className="flex items-center gap-3 mb-4">
                <div className="w-8 h-8 bg-gray-900 rounded-lg flex items-center justify-center">
                  <svg className="w-4 h-4 text-white" fill="currentColor" viewBox="0 0 20 20">
                    <path fillRule="evenodd" d="M10 0C4.477 0 0 4.484 0 10.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0110 4.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.203 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.942.359.31.678.921.678 1.856 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0020 10.017C20 4.484 15.522 0 10 0z" clipRule="evenodd" />
                  </svg>
                </div>
                <h3 className="text-lg font-semibold text-gray-900">GitHub OAuth Kurulumu</h3>
              </div>

              <div className="space-y-4">
                <div className="p-4 bg-blue-50 rounded-lg border border-blue-200">
                  <p className="text-sm text-blue-800 mb-2">
                    <strong>1. Create GitHub App:</strong>
                  </p>
                  <p className="text-sm text-blue-700 mb-2">
                    GitHub → Settings → Developer settings → OAuth Apps → New OAuth App
                  </p>
                  <div className="text-sm text-blue-700">
                    <p><strong>Redirect URI:</strong></p>
                    <code className="bg-blue-100 px-2 py-1 rounded text-xs break-all">
                      {redirectUri || `${window.location.origin}/api/v1/github/auth/callback`}
                    </code>
                  </div>
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">
                    Client ID
                  </label>
                  <input
                    type="text"
                    value={clientId}
                    onChange={(e) => setClientId((e.target as HTMLInputElement).value)}
                    placeholder="Iv1.xxxxxxxxxxxxxxxx"
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">
                    Client Secret
                  </label>
                  <input
                    type="password"
                    value={clientSecret}
                    onChange={(e) => setClientSecret((e.target as HTMLInputElement).value)}
                    placeholder="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
                    className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                  />
                </div>

                <div className="flex gap-3 pt-4">
                  <button
                    onClick={setupGitHubOAuth}
                    disabled={loading || !clientId || !clientSecret}
                    className="flex-1 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {loading ? 'Kaydediliyor...' : 'Kaydet'}
                  </button>
                  <button
                    onClick={() => setShowSetupModal(false)}
                    className="px-4 py-2 bg-gray-200 text-gray-700 rounded-lg hover:bg-gray-300"
                  >
                                          Cancel
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}
      </>
    );
  }

  return (
    <div class="github-integration">
      <GitHubConfigSetup />
      
      <div class="divider"></div>
      
      <div style={{ 
        backgroundColor: 'rgba(255, 255, 255, 0.8)', 
        borderRadius: '16px', 
        padding: '24px', 
        border: '1px solid rgba(0, 0, 0, 0.06)' 
      }}>
        <div className="flex items-center gap-3 mb-4">
          <div style={{ 
            width: '32px', 
            height: '32px', 
            backgroundColor: 'rgba(34, 197, 94, 0.1)', 
            borderRadius: '8px', 
            display: 'flex', 
            alignItems: 'center', 
            justifyContent: 'center' 
          }}>
            <svg style={{ width: '16px', height: '16px', color: '#22c55e' }} fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
            </svg>
          </div>
          <div className="flex-1">
            <h3 style={{ fontSize: '18px', fontWeight: '600', color: '#0f172a', marginBottom: '4px' }}>
              Select GitHub Repository
            </h3>
            <p style={{ fontSize: '14px', color: '#64748b' }}>
              Select the repository to deploy from @{githubUsername} account
            </p>
          </div>
          <button
            onClick={loadRepositories}
            disabled={loading}
            style={{
              padding: '8px 12px',
              backgroundColor: 'rgba(0, 0, 0, 0.04)',
              color: '#475569',
              borderRadius: '8px',
              border: 'none',
              fontSize: '14px',
              fontWeight: '500',
              cursor: loading ? 'not-allowed' : 'pointer',
              opacity: loading ? '0.5' : '1',
              transition: 'all 0.2s ease'
            }}
            onMouseEnter={(e) => {
              if (!loading) {
                (e.target as HTMLElement).style.backgroundColor = 'rgba(0, 0, 0, 0.08)';
              }
            }}
            onMouseLeave={(e) => {
              if (!loading) {
                (e.target as HTMLElement).style.backgroundColor = 'rgba(0, 0, 0, 0.04)';
              }
            }}
          >
                            {loading ? 'Loading...' : 'Refresh'}
          </button>
        </div>

        {/* Repository Selection */}
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Repository
            </label>
            
            {loading ? (
              <div className="flex items-center justify-center py-4">
                <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                <span className="ml-2 text-gray-600">Repositories loading...</span>
              </div>
            ) : repositories.length === 0 ? (
              <div className="text-center py-4 text-gray-500">
                <p>No repositories found. Click refresh button.</p>
              </div>
            ) : (
              <select
                value={selectedRepo?.id || ''}
                onChange={(e) => {
                  const repoId = parseInt((e.target as HTMLSelectElement).value);
                  const repo = repositories.find(r => r.id === repoId);
                  setSelectedRepo(repo || null);
                }}
                style={{
                  width: '100%',
                  padding: '12px 16px',
                  border: '1px solid rgba(0, 0, 0, 0.1)',
                  borderRadius: '12px',
                  fontSize: '14px',
                  backgroundColor: '#ffffff',
                  color: '#0f172a',
                  outline: 'none',
                  transition: 'all 0.2s ease'
                }}
                onFocus={(e) => {
                  (e.target as HTMLElement).style.borderColor = '#0f172a';
                  (e.target as HTMLElement).style.boxShadow = '0 0 0 3px rgba(15, 23, 42, 0.1)';
                }}
                onBlur={(e) => {
                  (e.target as HTMLElement).style.borderColor = 'rgba(0, 0, 0, 0.1)';
                  (e.target as HTMLElement).style.boxShadow = 'none';
                }}
              >
                <option value="">Select repository...</option>
                {repositories.map((repo) => (
                  <option key={repo.id} value={repo.id}>
                    {repo.name} {repo.private ? '(Private)' : '(Public)'} - {repo.description || 'No description'}
                  </option>
                ))}
              </select>
            )}
          </div>

          {selectedRepo && (
            <div style={{ 
              padding: '20px', 
              backgroundColor: 'rgba(15, 23, 42, 0.02)', 
              borderRadius: '12px', 
              border: '1px solid rgba(15, 23, 42, 0.08)',
              marginTop: '16px'
            }}>
              <div className="flex items-center gap-2 mb-3">
                <svg style={{ width: '20px', height: '20px', color: '#0f172a' }} fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 0C4.477 0 0 4.484 0 10.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0110 4.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.203 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.942.359.31.678.921.678 1.856 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0020 10.017C20 4.484 15.522 0 10 0z" clipRule="evenodd" />
                </svg>
                <h4 style={{ fontSize: '16px', fontWeight: '600', color: '#0f172a' }}>
                  {selectedRepo.name}
                </h4>
                <a 
                  href={selectedRepo.html_url} 
                  target="_blank" 
                  rel="noopener noreferrer"
                  style={{ color: '#64748b', transition: 'color 0.2s ease' }}
                  onMouseEnter={(e) => {
                    (e.target as HTMLElement).style.color = '#0f172a';
                  }}
                  onMouseLeave={(e) => {
                    (e.target as HTMLElement).style.color = '#64748b';
                  }}
                >
                  <svg style={{ width: '16px', height: '16px' }} fill="currentColor" viewBox="0 0 20 20">
                    <path d="M11 3a1 1 0 100 2h2.586l-6.293 6.293a1 1 0 101.414 1.414L15 6.414V9a1 1 0 102 0V4a1 1 0 00-1-1h-5z"></path>
                    <path d="M5 5a2 2 0 00-2 2v8a2 2 0 002 2h8a2 2 0 002-2v-3a1 1 0 10-2 0v3H5V7h3a1 1 0 000-2H5z"></path>
                  </svg>
                </a>
              </div>
              <p style={{ fontSize: '14px', color: '#64748b', marginBottom: '8px' }}>
                {selectedRepo.description || 'No description'}
              </p>
              <p style={{ fontSize: '12px', color: '#64748b', marginBottom: '16px' }}>
                Clone URL: <code style={{ 
                  backgroundColor: 'rgba(15, 23, 42, 0.06)', 
                  padding: '2px 6px', 
                  borderRadius: '4px', 
                  fontSize: '11px' 
                }}>{selectedRepo.clone_url}</code>
              </p>
              
              <div className="grid grid-cols-2 gap-4 mb-5">
                <div>
                  <label style={{ 
                    display: 'block', 
                    fontSize: '14px', 
                    fontWeight: '500', 
                    color: '#0f172a', 
                    marginBottom: '6px' 
                  }}>
                    Deploy Branch
                  </label>
                  <input
                    type="text"
                    value={deployBranch}
                    onChange={(e) => setDeployBranch((e.target as HTMLInputElement).value)}
                    style={{
                      width: '100%',
                      padding: '10px 12px',
                      border: '1px solid rgba(0, 0, 0, 0.1)',
                      borderRadius: '8px',
                      fontSize: '14px',
                      backgroundColor: '#ffffff',
                      color: '#0f172a',
                      outline: 'none',
                      transition: 'all 0.2s ease'
                    }}
                    placeholder={selectedRepo.default_branch}
                    onFocus={(e) => {
                      (e.target as HTMLElement).style.borderColor = '#0f172a';
                      (e.target as HTMLElement).style.boxShadow = '0 0 0 3px rgba(15, 23, 42, 0.1)';
                    }}
                    onBlur={(e) => {
                      (e.target as HTMLElement).style.borderColor = 'rgba(0, 0, 0, 0.1)';
                      (e.target as HTMLElement).style.boxShadow = 'none';
                    }}
                  />
                </div>
                
                <div className="flex items-center">
                  <label className="flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      checked={autoDeploy}
                      onChange={(e) => setAutoDeploy((e.target as HTMLInputElement).checked)}
                      style={{
                        width: '16px',
                        height: '16px',
                        marginRight: '8px',
                        accentColor: '#0f172a'
                      }}
                    />
                    <span style={{ fontSize: '14px', color: '#0f172a', fontWeight: '500' }}>
                      Otomatik Deploy
                    </span>
                  </label>
                </div>
              </div>
              
              <div className="flex gap-3">
                <button
                  onClick={connectRepository}
                  disabled={loading}
                  style={{
                    flex: '1',
                    padding: '12px 16px',
                    backgroundColor: '#0f172a',
                    color: '#ffffff',
                    borderRadius: '12px',
                    border: 'none',
                    fontSize: '14px',
                    fontWeight: '600',
                    cursor: loading ? 'not-allowed' : 'pointer',
                    opacity: loading ? '0.7' : '1',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    gap: '8px',
                    transition: 'all 0.2s ease'
                  }}
                  onMouseEnter={(e) => {
                    if (!loading) {
                      (e.target as HTMLElement).style.backgroundColor = '#1e293b';
                    }
                  }}
                  onMouseLeave={(e) => {
                    if (!loading) {
                      (e.target as HTMLElement).style.backgroundColor = '#0f172a';
                    }
                  }}
                >
                  {loading ? (
                    <div style={{ 
                      width: '16px', 
                      height: '16px', 
                      border: '2px solid #ffffff', 
                      borderTop: '2px solid transparent', 
                      borderRadius: '50%' 
                    }} className="animate-spin"></div>
                  ) : (
                    <svg style={{ width: '16px', height: '16px' }} fill="currentColor" viewBox="0 0 20 20">
                      <path d="M8 9a3 3 0 100-6 3 3 0 000 6zM8 11a6 6 0 016 6H2a6 6 0 016-6zM16 7a1 1 0 10-2 0v1h-1a1 1 0 100 2h1v1a1 1 0 102 0v-1h1a1 1 0 100-2h-1V7z"></path>
                    </svg>
                  )}
                  {selectedRepo && repositories.find(r => r.id === selectedRepo.id && r.full_name === selectedRepo.full_name) 
                    ? 'Update Connection' 
                    : 'Connect Repository'
                  }
                </button>
                
                {selectedRepo && (
                  <button
                    onClick={async () => {
                      try {
                        await request({
                          url: `/github/apps/${appName}/disconnect`,
                          method: 'DELETE'
                        });
                        setSelectedRepo(null);
                        setDeployBranch('main');
                        setAutoDeploy(false);
                      } catch (error) {
                        errorLog('Failed to disconnect repository:', error);
                      }
                    }}
                    style={{
                      padding: '12px 16px',
                      backgroundColor: 'rgba(239, 68, 68, 0.1)',
                      color: '#ef4444',
                      borderRadius: '12px',
                      border: '1px solid rgba(239, 68, 68, 0.2)',
                      fontSize: '14px',
                      fontWeight: '600',
                      cursor: 'pointer',
                      transition: 'all 0.2s ease'
                    }}
                    onMouseEnter={(e) => {
                      (e.target as HTMLElement).style.backgroundColor = 'rgba(239, 68, 68, 0.15)';
                    }}
                    onMouseLeave={(e) => {
                      (e.target as HTMLElement).style.backgroundColor = 'rgba(239, 68, 68, 0.1)';
                    }}
                  >
                    Remove Connection
                  </button>
                )}
              </div>
            </div>
          )}
        </div>
      </div>

      <style jsx>{`
        .config-section {
          margin-bottom: 2rem;
          padding: 1.5rem;
          border: 1px solid #333;
          border-radius: 8px;
          background: #1a1a1a;
        }

        .config-section h3 {
          color: #fff;
          margin-bottom: 1rem;
          border-bottom: 2px solid #333;
          padding-bottom: 0.5rem;
        }

        .config-status {
          display: flex;
          justify-content: space-between;
          align-items: flex-start;
          margin-bottom: 1rem;
        }

        .config-info p {
          margin: 0.5rem 0;
          color: #ccc;
        }

        .config-actions {
          display: flex;
          gap: 0.5rem;
        }

        .config-form {
          margin-top: 1rem;
          padding: 1rem;
          border: 1px solid #444;
          border-radius: 6px;
          background: #222;
        }

        .config-form h4 {
          color: #fff;
          margin-bottom: 1rem;
        }

        .form-group {
          margin-bottom: 1rem;
        }

        .form-group label {
          display: block;
          margin-bottom: 0.5rem;
          color: #fff;
          font-weight: bold;
        }

        .form-group input {
          width: 100%;
          padding: 0.75rem;
          border: 1px solid #555;
          border-radius: 4px;
          background: #333;
          color: #fff;
        }

        .form-group input:focus {
          outline: none;
          border-color: #666;
        }

        .form-actions {
          display: flex;
          gap: 0.5rem;
          margin-top: 1rem;
        }

        .config-help {
          margin-top: 1rem;
          padding: 1rem;
          border: 1px solid #444;
          border-radius: 4px;
          background: #2a2a2a;
        }

        .config-help h5 {
          color: #fff;
          margin-bottom: 0.5rem;
        }

        .config-help ol {
          color: #ccc;
          margin-left: 1.5rem;
        }

        .config-help li {
          margin-bottom: 0.5rem;
        }

        .btn {
          padding: 0.5rem 1rem;
          border: none;
          border-radius: 4px;
          cursor: pointer;
          font-size: 0.9rem;
          transition: all 0.2s;
        }

        .btn-primary {
          background: #333;
          color: #fff;
        }

        .btn-primary:hover {
          background: #444;
        }

        .btn-secondary {
          background: #666;
          color: #fff;
        }

        .btn-secondary:hover {
          background: #777;
        }

        .btn-danger {
          background: #8b0000;
          color: #fff;
        }

        .btn-danger:hover {
          background: #a00000;
        }

        .btn:disabled {
          opacity: 0.5;
          cursor: not-allowed;
        }

        .divider {
          height: 1px;
          background: #333;
          margin: 2rem 0;
        }
      `}</style>
    </div>
  );
}

// GitHub Config Setup Component
function GitHubConfigSetup() {
  const [config, setConfig] = useState<GitHubConfig | null>(null);
  const [showSetup, setShowSetup] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [formData, setFormData] = useState({
    client_id: '',
    client_secret: '',
    redirect_uri: `${window.location.origin}/api/v1/github/auth/callback`
  });

  const { request } = useApi();

  const loadConfig = async () => {
    try {
      const response = await request({ url: '/github/config' }) as GitHubConfigResponse;
      componentDebugLog('Config response:', response); // Debug log
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
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadConfig();
  }, []);

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
          border: '1px solid #475569'
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
        <div style={{ textAlign: 'center', padding: '20px 0' }}>
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