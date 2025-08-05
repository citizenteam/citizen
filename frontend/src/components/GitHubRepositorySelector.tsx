import { errorLog, componentDebugLog } from "../utils/debug";
import { useState, useEffect, useCallback } from 'preact/hooks';
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

interface GitHubRepositorySelectorProps {
  appName: string;
  onRepositorySelect?: (repo: GitHubRepository) => void;
}

export default function GitHubRepositorySelector({ appName, onRepositorySelect }: GitHubRepositorySelectorProps) {
  const { request } = useApi();
  const [githubConnected, setGithubConnected] = useState(false);
  const [githubUsername, setGithubUsername] = useState<string | null>(null);
  const [repositories, setRepositories] = useState<GitHubRepository[]>([]);
  const [loading, setLoading] = useState(false);
  const [showRepoSelector, setShowRepoSelector] = useState(false);
  const [selectedRepo, setSelectedRepo] = useState<GitHubRepository | null>(null);
  const [autoDeploy, setAutoDeploy] = useState(true);
  const [deployBranch, setDeployBranch] = useState('main');

  // Check GitHub connection status on mount
  useEffect(() => {
    checkGitHubStatus();
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

  const checkConnectedRepository = useCallback(async () => {
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
          setAutoDeploy(connectedRepo.auto_deploy);
          setDeployBranch(connectedRepo.deploy_branch || connectedRepo.default_branch); // Set deploy branch from connection
          
          // Notify parent component immediately about connected repository
          if (onRepositorySelect) {
            componentDebugLog('Auto-notifying parent about connected repository:', repoData);
            onRepositorySelect(repoData);
          }
        }
      }
    } catch (error) {
      errorLog('Failed to check connected repository:', error);
    }
  }, [request, appName, onRepositorySelect]);

  const checkGitHubStatus = async () => {
    try {
      const response = await request({ url: '/github/status' });
      if (response) {
        setGithubConnected(response.github_connected);
        setGithubUsername(response.github_username);
      }
    } catch (error) {
      errorLog('Failed to check GitHub status:', error);
    }
  };

  const loadRepositories = async () => {
    try {
      setLoading(true);
      const response = await request({ url: '/github/repositories' });
      if (response && response.repositories) {
        setRepositories(response.repositories);
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
          deploy_branch: deployBranch
        }
      });

      if (response) {
        setShowRepoSelector(false);
        if (onRepositorySelect) {
          onRepositorySelect(selectedRepo);
        }
      }
    } catch (error) {
      errorLog('Failed to connect repository:', error);
    } finally {
      setLoading(false);
    }
  };

  const disconnectRepository = async () => {
    if (!selectedRepo) return;

    try {
      setLoading(true);
      await request({
        url: `/github/apps/${appName}/disconnect`,
        method: 'DELETE'
      });

      setSelectedRepo(null);
      setShowRepoSelector(false);
    } catch (error) {
      errorLog('Failed to disconnect repository:', error);
    } finally {
      setLoading(false);
    }
  };

  if (!githubConnected) {
    return (
      <div style={{ 
        backgroundColor: '#ffffff', 
        borderRadius: '12px', 
        padding: '20px', 
        border: '1px solid #e2e8f0',
        textAlign: 'center'
      }}>
        <div style={{ 
          width: '48px', 
          height: '48px', 
          backgroundColor: '#f1f5f9', 
          borderRadius: '12px', 
          display: 'flex', 
          alignItems: 'center', 
          justifyContent: 'center',
          margin: '0 auto 16px'
        }}>
          <svg style={{ width: '24px', height: '24px', color: '#64748b' }} fill="currentColor" viewBox="0 0 20 20">
            <path fillRule="evenodd" d="M10 0C4.477 0 0 4.484 0 10.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0110 4.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.203 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.942.359.31.678.921.678 1.856 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0020 10.017C20 4.484 15.522 0 10 0z" clipRule="evenodd" />
          </svg>
        </div>
        <h3 style={{ fontSize: '16px', fontWeight: '600', color: '#1f2937', marginBottom: '8px' }}>
          GitHub Account Required
        </h3>
        <p style={{ fontSize: '14px', color: '#6b7280', marginBottom: '16px' }}>
          Connect your GitHub account in Profile page to select repositories
        </p>
        <button
          onClick={() => window.location.href = '/profile'}
          style={{
            padding: '12px 24px',
            backgroundColor: '#1f2937',
            color: '#ffffff',
            borderRadius: '8px',
            border: 'none',
            fontSize: '14px',
            fontWeight: '600',
            cursor: 'pointer',
            transition: 'all 0.2s ease'
          }}
          onMouseEnter={(e) => {
            (e.target as HTMLElement).style.backgroundColor = '#374151';
          }}
          onMouseLeave={(e) => {
            (e.target as HTMLElement).style.backgroundColor = '#1f2937';
          }}
        >
          Go to Profile
        </button>
      </div>
    );
  }

  return (
    <div style={{ 
      backgroundColor: '#ffffff', 
      borderRadius: '12px', 
      padding: '20px', 
      border: '1px solid #e2e8f0'
    }}>
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <div style={{ 
            width: '32px', 
            height: '32px', 
            backgroundColor: '#f1f5f9', 
            borderRadius: '8px', 
            display: 'flex', 
            alignItems: 'center', 
            justifyContent: 'center' 
          }}>
            <svg style={{ width: '16px', height: '16px', color: '#64748b' }} fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M10 0C4.477 0 0 4.484 0 10.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0110 4.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.203 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.942.359.31.678.921.678 1.856 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0020 10.017C20 4.484 15.522 0 10 0z" clipRule="evenodd" />
            </svg>
          </div>
          <div>
            <h3 style={{ fontSize: '16px', fontWeight: '600', color: '#1f2937', marginBottom: '2px' }}>
              Repository Selection
            </h3>
            <p style={{ fontSize: '12px', color: '#6b7280' }}>
              Connected as @{githubUsername}
            </p>
          </div>
        </div>
      </div>

      {selectedRepo ? (
        <div style={{ 
          padding: '16px', 
          backgroundColor: '#f8fafc', 
          borderRadius: '8px', 
          border: '1px solid #e2e8f0'
        }}>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div style={{ 
                width: '24px', 
                height: '24px', 
                backgroundColor: 'rgba(34, 197, 94, 0.1)', 
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
                <p style={{ fontSize: '14px', color: '#1f2937', fontWeight: '600' }}>
                  {selectedRepo.full_name}
                </p>
                <p style={{ fontSize: '12px', color: '#6b7280' }}>
                  Branch: {deployBranch} • {selectedRepo.private ? 'Private' : 'Public'}
                </p>
              </div>
            </div>
            <div className="flex gap-2">
              <button
                onClick={() => setShowRepoSelector(true)}
                style={{
                  padding: '6px 12px',
                  backgroundColor: '#f1f5f9',
                  color: '#64748b',
                  borderRadius: '6px',
                  border: '1px solid #e2e8f0',
                  fontSize: '12px',
                  fontWeight: '500',
                  cursor: 'pointer',
                  transition: 'all 0.2s ease'
                }}
                onMouseEnter={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = '#e2e8f0';
                }}
                onMouseLeave={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = '#f1f5f9';
                }}
              >
                Change
              </button>
              <button
                onClick={disconnectRepository}
                disabled={loading}
                style={{
                  padding: '6px 12px',
                  backgroundColor: 'rgba(239, 68, 68, 0.1)',
                  color: '#ef4444',
                  borderRadius: '6px',
                  border: '1px solid rgba(239, 68, 68, 0.2)',
                  fontSize: '12px',
                  fontWeight: '500',
                  cursor: loading ? 'not-allowed' : 'pointer',
                  opacity: loading ? '0.7' : '1',
                  transition: 'all 0.2s ease'
                }}
                onMouseEnter={(e) => {
                  if (!loading) {
                    (e.target as HTMLElement).style.backgroundColor = 'rgba(239, 68, 68, 0.2)';
                  }
                }}
                onMouseLeave={(e) => {
                  if (!loading) {
                    (e.target as HTMLElement).style.backgroundColor = 'rgba(239, 68, 68, 0.1)';
                  }
                }}
              >
                {loading ? 'Removing...' : 'Remove'}
              </button>
            </div>
          </div>
        </div>
      ) : (
        <div style={{ textAlign: 'center', padding: '20px 0' }}>
          <p style={{ fontSize: '14px', color: '#6b7280', marginBottom: '16px' }}>
            No repository selected for this app
          </p>
          <button
            onClick={() => setShowRepoSelector(true)}
            style={{
              padding: '12px 24px',
              backgroundColor: '#1f2937',
              color: '#ffffff',
              borderRadius: '8px',
              border: 'none',
              fontSize: '14px',
              fontWeight: '600',
              cursor: 'pointer',
              transition: 'all 0.2s ease'
            }}
            onMouseEnter={(e) => {
              (e.target as HTMLElement).style.backgroundColor = '#374151';
            }}
            onMouseLeave={(e) => {
              (e.target as HTMLElement).style.backgroundColor = '#1f2937';
            }}
          >
            Select Repository
          </button>
        </div>
      )}

      {showRepoSelector && (
        <div style={{ 
          marginTop: '20px',
          padding: '20px',
          backgroundColor: '#f8fafc',
          borderRadius: '8px',
          border: '1px solid #e2e8f0'
        }}>
          <div className="flex items-center justify-between mb-4">
            <h4 style={{ fontSize: '14px', fontWeight: '600', color: '#1f2937' }}>
              Select Repository
            </h4>
            <button
              onClick={() => setShowRepoSelector(false)}
              style={{
                padding: '4px',
                backgroundColor: 'transparent',
                color: '#64748b',
                border: 'none',
                borderRadius: '4px',
                cursor: 'pointer'
              }}
            >
              <svg style={{ width: '16px', height: '16px' }} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          {loading ? (
            <div className="flex items-center justify-center py-8">
              <div style={{ 
                width: '20px', 
                height: '20px', 
                border: '2px solid #e2e8f0', 
                borderTop: '2px solid #1f2937', 
                borderRadius: '50%'
              }} className="animate-spin"></div>
              <span style={{ marginLeft: '12px', color: '#6b7280' }}>Loading repositories...</span>
            </div>
          ) : (
            <div style={{ maxHeight: '300px', overflowY: 'auto' }}>
              {repositories.length > 0 ? (
                <div className="space-y-2">
                  {repositories.map((repo) => (
                    <div
                      key={repo.id}
                      onClick={() => {
                        setSelectedRepo(repo);
                        setDeployBranch(repo.default_branch); // Set deploy branch to repository's default branch
                      }}
                      style={{
                        padding: '12px',
                        backgroundColor: selectedRepo?.id === repo.id ? '#e0f2fe' : '#ffffff',
                        border: `1px solid ${selectedRepo?.id === repo.id ? '#0ea5e9' : '#e2e8f0'}`,
                        borderRadius: '8px',
                        cursor: 'pointer',
                        transition: 'all 0.2s ease'
                      }}
                      onMouseEnter={(e) => {
                        if (selectedRepo?.id !== repo.id) {
                          (e.target as HTMLElement).style.backgroundColor = '#f8fafc';
                        }
                      }}
                      onMouseLeave={(e) => {
                        if (selectedRepo?.id !== repo.id) {
                          (e.target as HTMLElement).style.backgroundColor = '#ffffff';
                        }
                      }}
                    >
                      <div className="flex items-center justify-between">
                        <div>
                          <p style={{ fontSize: '14px', color: '#1f2937', fontWeight: '600' }}>
                            {repo.full_name}
                          </p>
                          <p style={{ fontSize: '12px', color: '#6b7280' }}>
                            {repo.description || 'No description available'}
                          </p>
                          <div className="flex items-center gap-4 mt-1">
                            <span style={{ fontSize: '11px', color: '#64748b' }}>
                              Branch: {repo.default_branch}
                            </span>
                            <span style={{ fontSize: '11px', color: '#64748b' }}>
                              {repo.private ? 'Private' : 'Public'}
                            </span>
                          </div>
                        </div>
                        {selectedRepo?.id === repo.id && (
                          <div style={{ 
                            width: '20px', 
                            height: '20px', 
                            backgroundColor: '#0ea5e9', 
                            borderRadius: '50%', 
                            display: 'flex', 
                            alignItems: 'center', 
                            justifyContent: 'center' 
                          }}>
                            <svg style={{ width: '12px', height: '12px', color: '#ffffff' }} fill="currentColor" viewBox="0 0 20 20">
                              <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                            </svg>
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div style={{ textAlign: 'center', padding: '40px 0' }}>
                  <p style={{ fontSize: '14px', color: '#6b7280' }}>
                    No repositories found
                  </p>
                </div>
              )}
            </div>
          )}

          {selectedRepo && (
            <div style={{ marginTop: '16px', paddingTop: '16px', borderTop: '1px solid #e2e8f0' }}>
              <div>
                <p style={{ fontSize: '14px', color: '#1f2937', fontWeight: '600', marginBottom: '8px' }}>
                  {selectedRepo.full_name}
                </p>
                
                {/* Branch Selection */}
                <div style={{ marginBottom: '16px' }}>
                  <label style={{ fontSize: '12px', color: '#6b7280', display: 'block', marginBottom: '4px' }}>
                    Deploy Branch
                  </label>
                  <select
                    value={deployBranch}
                    onChange={(e) => setDeployBranch((e.target as HTMLSelectElement).value)}
                    style={{
                      width: '100%',
                      padding: '8px 12px',
                      fontSize: '14px',
                      backgroundColor: '#ffffff',
                      border: '1px solid #e2e8f0',
                      borderRadius: '6px',
                      color: '#1f2937',
                      cursor: 'pointer'
                    }}
                  >
                    <option value="main">main</option>
                    <option value="master">master</option>
                    <option value="develop">develop</option>
                    <option value="dev">dev</option>
                    {/* Show default branch only if it's not already in the list */}
                    {selectedRepo.default_branch && 
                     !['main', 'master', 'develop', 'dev'].includes(selectedRepo.default_branch) && (
                      <option value={selectedRepo.default_branch}>
                        {selectedRepo.default_branch} (default)
                      </option>
                    )}
                  </select>
                </div>

                {/* Auto Deploy Toggle */}
                <div style={{ marginBottom: '16px' }}>
                  <label style={{ 
                    display: 'flex', 
                    alignItems: 'center', 
                    gap: '8px',
                    fontSize: '14px',
                    color: '#1f2937',
                    cursor: 'pointer'
                  }}>
                    <input
                      type="checkbox"
                      checked={autoDeploy}
                      onChange={(e) => setAutoDeploy((e.target as HTMLInputElement).checked)}
                      style={{ 
                        width: '16px', 
                        height: '16px',
                        cursor: 'pointer'
                      }}
                    />
                    Auto-deploy on push to {deployBranch}
                  </label>
                </div>

                <button
                  onClick={connectRepository}
                  disabled={loading}
                  style={{
                    width: '100%',
                    padding: '12px 16px',
                    backgroundColor: loading ? '#9ca3af' : '#1f2937',
                    color: '#ffffff',
                    borderRadius: '6px',
                    border: 'none',
                    fontSize: '14px',
                    fontWeight: '600',
                    cursor: loading ? 'not-allowed' : 'pointer',
                    transition: 'all 0.2s ease'
                  }}
                  onMouseEnter={(e) => {
                    if (!loading) {
                      (e.target as HTMLElement).style.backgroundColor = '#374151';
                    }
                  }}
                  onMouseLeave={(e) => {
                    if (!loading) {
                      (e.target as HTMLElement).style.backgroundColor = '#1f2937';
                    }
                  }}
                >
                  {loading ? 'Connecting...' : 'Connect Repository'}
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
} 