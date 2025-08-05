import { useState, useEffect, useCallback } from 'preact/hooks';
import { useLocation } from 'wouter';
import { useApi } from '../hooks/useApi';
import MinimalLayout from '../components/layout/MinimalLayout';
import { errorLog, componentDebugLog, warnLog } from '../utils/debug';
import LogViewer from '../components/LogViewer';
import CustomSelect from '../components/CustomSelect';
import GitHubRepositorySelector from '../components/GitHubRepositorySelector';

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

import type { AppInfo } from '../types';

interface AppDetailsMinimalProps {}

const BUILDER_OPTIONS = [
  { value: 'herokuish', name: 'Herokuish', description: 'Default Citizen builder' },
  { value: 'pack', name: 'Cloud Native Buildpacks', description: 'Modern buildpack system' },
  { value: 'dockerfile', name: 'Dockerfile', description: 'Build from Dockerfile' }
];

const POPULAR_BUILDPACKS = [
  { name: 'Node.js', url: 'heroku/nodejs' },
  { name: 'Python', url: 'heroku/python' }, 
  { name: 'PHP', url: 'heroku/php' },
  { name: 'Ruby', url: 'heroku/ruby' },
  { name: 'Java', url: 'heroku/java' },
  { name: 'Go', url: 'heroku/go' },
  { name: 'Static', url: 'heroku/static' },
  { name: 'Multi', url: 'heroku/multi' }
];

export default function AppDetailsMinimal() {
  const [, setLocation] = useLocation();
  const [appName, setAppName] = useState<string>('');
  const [activeTab, setActiveTab] = useState<'dashboard' | 'settings' | 'logs'>('dashboard');
  const [activeSettingsTab, setActiveSettingsTab] = useState<'domains' | 'environment' | 'access' | 'danger'>('domains');
  const [showLogsModal, setShowLogsModal] = useState(false);
  const [showDeployModal, setShowDeployModal] = useState(false);
  const [deployLogs, setDeployLogs] = useState<string>('');
  const [visibleEnvVars, setVisibleEnvVars] = useState<{[key: string]: boolean}>({});
  const [deleteConfirm, setDeleteConfirm] = useState<{show: boolean, type: 'env' | 'domain', key: string}>({show: false, type: 'env', key: ''});
  const [autoDeployEnabled, setAutoDeployEnabled] = useState(false);
  const [manualDeployEnabled, setManualDeployEnabled] = useState(true);
  const [activities, setActivities] = useState<any[]>([]);
  const [appInfo, setAppInfo] = useState<AppInfo | null>(null);
  const [message, setMessage] = useState<{ text: string; type: 'success' | 'error' } | null>(null);
  
  // Form states
  const [newDomain, setNewDomain] = useState('');
  const [envKey, setEnvKey] = useState('');
  const [envValue, setEnvValue] = useState('');
  const [envVars, setEnvVars] = useState<{[key: string]: string}>({});
  
  // Deploy config states
  const [selectedBuilder, setSelectedBuilder] = useState('herokuish');
  const [selectedBuildpack, setSelectedBuildpack] = useState('');
  const [deploySource, setDeploySource] = useState<'github' | 'remote'>('github');
  const [gitUrl, setGitUrl] = useState('');
  const [gitBranch, setGitBranch] = useState('main');
  
  // Log states
  const [logs, setLogs] = useState<string>('');
  
  // Public access states
  const [isPublic, setIsPublic] = useState(false);
  const [publicSettingExists, setPublicSettingExists] = useState(false);
  
  const { request: fetchAppInfo, loading: appInfoLoading } = useApi<AppInfo>();
  const { request: addDomain, loading: domainLoading } = useApi();
  const { request: removeDomain, loading: removeDomainLoading } = useApi();
  const { request: setEnvVar, loading: envLoading } = useApi();
  const { request: removeEnvVar, loading: removeEnvLoading } = useApi();
  const { request: fetchEnvVars, loading: fetchEnvLoading } = useApi<{[key: string]: string}>();
  const { request: deleteApp, loading: deleteLoading } = useApi();
  const { request: fetchLogs } = useApi<{ logs: string; type: string; tail: number }>();
  const { request: deployApp, loading: deployLoading } = useApi();
  const { request: restartApp, loading: restartLoading } = useApi();
  const { request: fetchActivities } = useApi<{activities: any[], total: number}>();
  const { request: fetchPublicSetting, loading: publicSettingLoading } = useApi<any>();
  const { request: updatePublicSetting, loading: updatePublicLoading } = useApi();

  useEffect(() => {
    const path = window.location.pathname;
    const pathParts = path.split('/');
    const appNameFromPath = pathParts[pathParts.length - 1];
    setAppName(appNameFromPath);
  }, []);

  useEffect(() => {
    if (appName) {
      loadAppInfo();
      loadEnvVars();
      loadActivities();
      loadPublicSetting();
    }
  }, [appName]);

  const loadAppInfo = async () => {
    if (!appName) return;
    
    try {
      const info = await fetchAppInfo({ url: `/citizen/apps/${appName}` });
      if (info) {
        setAppInfo(info);
      }
    } catch (error) {
      errorLog('Failed to load app info:', error);
      showMessage('Failed to load app info', 'error');
    }
  };

  const loadEnvVars = async () => {
    if (!appName) return;
    
    try {
      const vars = await fetchEnvVars({ url: `/citizen/apps/${appName}/env` });
      if (vars) {
        setEnvVars(vars);
      }
    } catch (error) {
      errorLog('Failed to load env vars:', error);
      showMessage('Failed to load environment variables', 'error');
    }
  };

  const loadActivities = async () => {
    if (!appName) return;
    
    try {
      const result = await fetchActivities({ url: `/citizen/apps/${appName}/activities` });
      
      // useApi already returns only the data part, we should look for .activities
      if (result && result.activities) {
        setActivities(result.activities);
      } else {
        // If activities comes as an array, use it directly
        if (Array.isArray(result)) {
          setActivities(result);
        } else {
          setActivities([]);
        }
      }
    } catch (error) {
      errorLog('Failed to load activities:', error);
      setActivities([]);
    }
  };

  const loadPublicSetting = async () => {
    if (!appName) return;
    
    try {
      const response = await fetchPublicSetting({ url: `/citizen/apps/${appName}/public-setting` });
      if (response) {
        setIsPublic(response.is_public);
        setPublicSettingExists(true);
      } else {
        setIsPublic(false);
        setPublicSettingExists(false);
      }
    } catch (error) {
      errorLog('Failed to load public setting:', error);
      setIsPublic(false);
      setPublicSettingExists(false);
    }
  };

  const handleUpdatePublicSetting = async (isPublic: boolean) => {
    if (!appName) return;
    
    try {
      const response = await updatePublicSetting({
        url: `/citizen/apps/${appName}/public-setting`,
        method: 'POST',
        data: { is_public: isPublic }
      });
      
      if (response) {
        setIsPublic(isPublic);
        setPublicSettingExists(true);
        showMessage(
          isPublic ? 'App is now public (no authentication required)' : 'App is now private (authentication required)',
          'success'
        );
      } else {
        showMessage('Failed to update public setting', 'error');
      }
    } catch (error) {
      errorLog('Failed to update public setting:', error);
      showMessage('Failed to update public setting', 'error');
    }
  };

  const showMessage = (text: string, type: 'success' | 'error') => {
    setMessage({ text, type });
    setTimeout(() => setMessage(null), 4000);
  };

  const toggleEnvVisibility = (key: string) => {
    setVisibleEnvVars(prev => ({
      ...prev,
      [key]: !prev[key]
    }));
  };

  const handleDeleteConfirm = (type: 'env' | 'domain', key: string) => {
    setDeleteConfirm({ show: true, type, key });
  };

  const executeDelete = () => {
    if (deleteConfirm.type === 'env') {
      handleRemoveEnv(deleteConfirm.key);
    } else {
      handleRemoveDomain(deleteConfirm.key);
    }
    setDeleteConfirm({ show: false, type: 'env', key: '' });
  };

  const getAppStatus = () => {
    if (appInfoLoading) return { status: 'loading', color: '#fbbf24', text: 'Loading', pulse: true };
    if (!appInfo) return { status: 'unknown', color: '#6b7280', text: 'Unknown', pulse: false };
    
    if (appInfo.running && appInfo.deployed) {
      return { status: 'running', color: '#10b981', text: 'Live', pulse: true };
    } else if (appInfo.deployed) {
      return { status: 'deployed', color: '#f59e0b', text: 'Deploying', pulse: true };
    } else {
      return { status: 'stopped', color: '#ef4444', text: 'Stopped', pulse: false };
    }
  };

  const getAppUrl = () => {
    if (appInfo?.domains && appInfo.domains.length > 0) {
      return `http://${appInfo.domains[0]}`;
    }
    return null;
  };

  const getActivityTypeText = (type: string) => {
    switch (type) {
      case 'deploy':
        return 'DEPLOY';
      case 'restart':
        return 'RESTART';
      case 'domain':
        return 'DOMAIN';
      case 'config':
        return 'CONFIG';
      case 'build':
        return 'BUILD';
      default:
        return 'ACTIVITY';
    }
  };

  const getActivityStatusText = (status: string) => {
    switch (status) {
      case 'success':
        return 'SUCCESS';
      case 'error':
        return 'ERROR';
      case 'warning':
        return 'WARNING';
      case 'info':
        return 'INFO';
      default:
        return 'UNKNOWN';
    }
  };

  const formatRelativeTime = (timestamp: string) => {
    const now = new Date();
    const time = new Date(timestamp);
    const diff = now.getTime() - time.getTime();
    
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);
    
    if (minutes < 1) return 'now';
    if (minutes < 60) return `${minutes}m ago`;
    if (hours < 24) return `${hours}h ago`;
    return `${days}d ago`;
  };

  const status = getAppStatus();
  const appUrl = getAppUrl();

  const tabs = [
    { id: 'dashboard', label: 'Dashboard' },
    { id: 'settings', label: 'Settings' },
    { id: 'logs', label: 'Logs' }
  ];

  const handleAddDomain = async (e: Event) => {
    e.preventDefault();
    if (!newDomain.trim()) return;

    try {
      const result = await addDomain({
        url: `/citizen/apps/${appName}/custom-domain`,
        method: 'POST',
        data: { domain: newDomain }
      });

      if (result !== null) {
        showMessage('Custom domain added successfully', 'success');
        // Add domain activity
        const newActivity = {
          id: Date.now(),
          type: 'domain',
          message: `Custom domain added: ${newDomain}`,
          timestamp: new Date().toISOString(),
          status: 'info'
        };
        setActivities(prev => [newActivity, ...prev.slice(0, 4)]);
        setNewDomain('');
        loadAppInfo();
      } else {
        showMessage('Failed to add custom domain', 'error');
      }
    } catch (error) {
      errorLog('Add custom domain error:', error);
      showMessage('Failed to add custom domain', 'error');
    }
  };

  const handleRemoveDomain = async (domain: string) => {
    if (!confirm(`Are you sure you want to remove domain: ${domain}?`)) return;

    try {
      const result = await removeDomain({
        url: `/citizen/apps/${appName}/custom-domain`,
        method: 'DELETE',
        data: { domain }
      });

      if (result !== null) {
        showMessage('Custom domain removed successfully', 'success');
        // Add domain removal activity
        const newActivity = {
          id: Date.now(),
          type: 'domain',
          message: `Custom domain removed: ${domain}`,
          timestamp: new Date().toISOString(),
          status: 'warning'
        };
        setActivities(prev => [newActivity, ...prev.slice(0, 4)]);
        loadAppInfo();
      } else {
        showMessage('Failed to remove custom domain', 'error');
      }
    } catch (error) {
      errorLog('Remove custom domain error:', error);
      showMessage('Failed to remove custom domain', 'error');
    }
  };

  const handleSetEnv = async (e: Event) => {
    e.preventDefault();
    if (!envKey.trim() || !envValue.trim()) return;
    
    // Prevent multiple submissions
    if (envLoading) return;

    // Check if key already exists for update message
    const isUpdate = envVars.hasOwnProperty(envKey);
    const action = isUpdate ? 'updated' : 'set';

    try {
      const result = await setEnvVar({
        url: `/citizen/apps/${appName}/env`,
        method: 'POST',
        data: { 
          env_vars: {
            [envKey]: envValue
          }
        }
      });

      if (result !== null) {
        showMessage(`Environment variable ${action} successfully`, 'success');
        // Add env variable activity
        const newActivity = {
          id: Date.now(),
          type: 'config',
          message: `Environment variable ${action}: ${envKey.trim()}`,
          timestamp: new Date().toISOString(),
          status: 'info'
        };
        setActivities(prev => [newActivity, ...prev.slice(0, 4)]);
        setEnvKey('');
        setEnvValue('');
        loadEnvVars();
      } else {
        showMessage('Failed to set environment variable', 'error');
      }
    } catch (error) {
      errorLog('Set env error:', error);
      showMessage('Failed to set environment variable', 'error');
    }
  };

  const handleRemoveEnv = async (key: string) => {
    if (!confirm(`Are you sure you want to remove environment variable: ${key}?`)) return;

    try {
      const result = await removeEnvVar({
        url: `/citizen/apps/${appName}/env`,
        method: 'DELETE',
        data: { key }
      });

      if (result !== null) {
        showMessage('Environment variable removed successfully', 'success');
        // Add env variable removal activity
        const newActivity = {
          id: Date.now(),
          type: 'config',
          message: `Environment variable removed: ${key}`,
          timestamp: new Date().toISOString(),
          status: 'warning'
        };
        setActivities(prev => [newActivity, ...prev.slice(0, 4)]);
        loadEnvVars();
      } else {
        showMessage('Failed to remove environment variable', 'error');
      }
    } catch (error) {
      errorLog('Remove env error:', error);
      showMessage('Failed to remove environment variable', 'error');
    }
  };

  const handleDeleteApp = async () => {
    if (!confirm(`Are you sure you want to delete the app "${appName}"? This action cannot be undone.`)) {
      return;
    }

    const confirmation = prompt('This will permanently delete all data associated with this app. Type "DELETE" to confirm:');
    if (confirmation !== 'DELETE') {
      return;
    }

    const result = await deleteApp({
      url: `/citizen/apps/${appName}`,
      method: 'DELETE'
    });

    if (result !== null) {
      showMessage('App deleted successfully', 'success');
      setTimeout(() => setLocation('/'), 2000);
    } else {
      showMessage('Failed to delete app', 'error');
    }
  };

  const handleRepositorySelect = useCallback((repo: GitHubRepository) => {
            componentDebugLog('Repository selected:', repo);
    setGitUrl(repo.clone_url);
    setGitBranch(repo.default_branch);
    // Repository selected message removed
  }, []);

  const handleDeploy = async () => {
    if (!gitUrl.trim()) return;
    
    // Prevent multiple simultaneous deploys
    if (deployLoading) {
      showMessage('Deployment is already in progress', 'error');
      return;
    }

    try {
      setShowDeployModal(true);
      setDeployLogs('🚀 Starting deployment...\n');
      
      const result = await deployApp({
        url: `/citizen/apps/${appName}/deploy`,
        method: 'POST',
        data: {
          git_url: gitUrl,
          git_branch: gitBranch,
          builder: selectedBuilder,
          buildpack: selectedBuildpack
        }
      });

      if (result !== null) {
        showMessage('Deployment started successfully', 'success');
        // Set deploy logs from response if available
        if (result.output) {
          setDeployLogs(result.output);
        }
        // Add deploy activity
        const newActivity = {
          id: Date.now(),
          type: 'deploy',
          message: `Deployment started from ${deploySource === 'github' ? 'GitHub' : 'Git URL'}`,
          timestamp: new Date().toISOString(),
          status: 'info'
        };
        setActivities(prev => [newActivity, ...prev.slice(0, 4)]);
        loadAppInfo();
        
        // Start live log monitoring if deployment is successful
        startLiveLogMonitoring();
      } else {
        showMessage('Failed to start deployment', 'error');
        setShowDeployModal(false);
      }
    } catch (error) {
      errorLog('Deploy error:', error);
      showMessage('Failed to start deployment', 'error');
      setShowDeployModal(false);
    }
  };

  // Live log monitoring for deploy and build logs
  const startLiveLogMonitoring = () => {
    let logInterval: NodeJS.Timeout;
    let attempts = 0;
    const maxAttempts = 60; // 5 minutes max
    
    const checkLogs = async () => {
      attempts++;
      try {
        // Use new live build logs endpoint that includes container detection
        const logsResult = await fetchLogs({ 
          url: `/citizen/apps/${appName}/logs/live-build` 
        });
        
        if (logsResult && logsResult.logs) {
          setDeployLogs(logsResult.logs);
          
          // If we detect completion indicators, stop monitoring sooner
          const logs = logsResult.logs.toLowerCase();
          if (logs.includes('application deployed') || 
              logs.includes('deployment complete') ||
              logs.includes('build complete') ||
              logs.includes('successfully built')) {
            if (logInterval) clearInterval(logInterval);
            setTimeout(() => loadAppInfo(), 2000); // Refresh app info after completion
            return;
          }
        }
        
        // Stop monitoring after max attempts or if deployment finished
        if (attempts >= maxAttempts || !deployLoading) {
          if (logInterval) clearInterval(logInterval);
          return;
        }
      } catch (error) {
        warnLog('Log monitoring error:', error);
        // If backend returns error, try fallback to regular logs
        try {
          const fallbackResult = await fetchLogs({ 
            url: `/citizen/apps/${appName}/logs?tail=100` 
          });
          if (fallbackResult && fallbackResult.logs) {
            setDeployLogs(fallbackResult.logs);
          }
        } catch (fallbackError) {
          warnLog('Fallback log monitoring error:', fallbackError);
        }
      }
    };
    
    // Start monitoring every 3 seconds for better real-time feel
    logInterval = setInterval(checkLogs, 3000);
    
    // Initial check
    checkLogs();
  };

  const handleRestartApp = async () => {
    try {
      const result = await restartApp({
        url: `/citizen/apps/${appName}/restart`,
        method: 'POST'
      });

      if (result !== null) {
        showMessage('App restarted successfully', 'success');
        // Add restart activity
        const newActivity = {
          id: Date.now(),
          type: 'restart',
          message: 'App restarted successfully',
          timestamp: new Date().toISOString(),
          status: 'warning'
        };
        setActivities(prev => [newActivity, ...prev.slice(0, 4)]);
        setTimeout(() => {
          loadAppInfo();
        }, 1000);
      } else {
        showMessage('Failed to restart app', 'error');
      }
    } catch (error) {
      errorLog('Restart error:', error);
      showMessage('Failed to restart app', 'error');
    }
  };

  const loadLogs = async () => {
    if (!appName) return;
    
    try {
      const result = await fetchLogs({ url: `/citizen/apps/${appName}/logs?tail=1000` });
      if (result && result.logs) {
        setLogs(result.logs);
      }
    } catch (error) {
      errorLog('Failed to load logs:', error);
      setLogs('Failed to load logs');
    }
  };

  if (appInfoLoading && !appInfo) {
    return (
      <div 
        className="min-h-screen flex flex-col items-center justify-center"
        style={{
          backgroundColor: '#fafafa',
          fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
          color: '#0f172a',
          padding: '0 24px'
        }}
      >
        <button
          onClick={() => setLocation('/')}
          className="fixed top-8 left-8 z-10 transition-all duration-300"
          style={{
            backgroundColor: '#000000',
            border: 'none',
            borderRadius: '50%',
            width: '44px',
            height: '44px',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)'
          }}
        >
          <svg 
            className="w-4 h-4" 
            style={{ color: '#ffffff' }}
            fill="none" 
            stroke="currentColor" 
            viewBox="0 0 24 24"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
          </svg>
        </button>

        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-gray-900 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading application details...</p>
        </div>
      </div>
    );
  }

  return (
    <MinimalLayout showSystemStatus={false} showLogout={false}>
      <div 
        className="min-h-screen"
        style={{
          padding: '0 24px'
        }}
      >
        {/* Header - Tab Navigation + Back Button + App Info */}
        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between pt-3 pb-3 lg:pt-6 lg:pb-6 gap-3 lg:gap-4">
          {/* Left: Back Button + App Info */}
          <div className="flex items-center gap-3">
            <button
              onClick={() => setLocation('/')}
              className="transition-all duration-300"
              style={{
                backgroundColor: '#000000',
                border: 'none',
                borderRadius: '50%',
                width: '32px',
                height: '32px',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)'
              }}
            >
              <svg 
                className="w-3 h-3" 
                style={{ color: '#ffffff' }}
                fill="none" 
                stroke="currentColor" 
                viewBox="0 0 24 24"
              >
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
              </svg>
            </button>
            
            <div className="flex items-center gap-2">
              <h1 
                style={{ 
                  fontSize: '13px',
                  fontWeight: '500',
                  color: '#0f172a',
                  margin: '0'
                }}
              >
                {appName}
              </h1>
              
              <div 
                className="inline-flex items-center gap-1 px-2 py-1 rounded-full"
                style={{
                  backgroundColor: status.status === 'running' ? 'rgba(34, 197, 94, 0.08)' : 
                                  status.status === 'deployed' ? 'rgba(245, 158, 11, 0.08)' : 
                                  'rgba(239, 68, 68, 0.08)',
                  border: `1px solid ${status.status === 'running' ? 'rgba(34, 197, 94, 0.12)' : 
                                     status.status === 'deployed' ? 'rgba(245, 158, 11, 0.12)' : 
                                     'rgba(239, 68, 68, 0.12)'}`
                }}
              >
                <div 
                  className="w-1.5 h-1.5 rounded-full"
                  style={{ 
                    backgroundColor: status.color,
                    animation: status.pulse ? 'pulse 2s infinite' : 'none'
                  }}
                ></div>
                <span 
                  style={{ 
                    fontSize: '11px',
                    fontWeight: '500',
                    color: status.color,
                    textTransform: 'lowercase'
                  }}
                >
                  {status.text}
                </span>
              </div>
            </div>
          </div>

          {/* Center: Tab Navigation */}
          <div>
            {/* Mobile: Select Dropdown */}
            <div className="block lg:hidden">
              <CustomSelect
                options={tabs.map(tab => ({ value: tab.id, label: tab.label }))}
                value={activeTab}
                onChange={(newTab) => {
                  setActiveTab(newTab as any);
                  if (newTab === 'logs') {
                    loadLogs();
                  }
                }}
                placeholder="Select Tab"
                className="w-full"
              />
            </div>

            {/* Desktop: Tab Buttons */}
            <div className="hidden lg:flex gap-2 justify-center">
              {tabs.map((tab) => (
                <button
                  key={tab.id}
                  onClick={() => {
                    setActiveTab(tab.id as any);
                    if (tab.id === 'logs') {
                      loadLogs();
                    }
                  }}
                  className="px-4 py-2 rounded-full transition-all duration-300 whitespace-nowrap"
                  style={{
                    backgroundColor: activeTab === tab.id ? 'rgba(0, 0, 0, 0.08)' : 'transparent',
                    color: activeTab === tab.id ? '#0f172a' : '#64748b',
                    border: '1px solid transparent',
                    fontSize: '13px',
                    fontWeight: '500',
                    cursor: 'pointer'
                  }}
                >
                  {tab.label}
                </button>
              ))}
            </div>
          </div>

          {/* Right: Empty for balance - hidden on mobile */}
          <div className="hidden lg:block" style={{ width: '100px' }}></div>
        </div>

        {/* Message */}
        {message && (
          <div className="flex justify-center mb-6">
            <div 
              className="px-4 py-3 rounded-full text-center max-w-md"
              style={{
                backgroundColor: message.type === 'success' ? 'rgba(34, 197, 94, 0.08)' : 'rgba(239, 68, 68, 0.08)',
                color: message.type === 'success' ? '#22c55e' : '#ef4444',
                border: `1px solid ${message.type === 'success' ? 'rgba(34, 197, 94, 0.12)' : 'rgba(239, 68, 68, 0.12)'}`,
                fontSize: '13px',
                fontWeight: '500'
              }}
            >
              {message.text}
            </div>
          </div>
        )}

        {/* Content - Sabit Grid */}
        <div className="flex justify-center items-center min-h-[400px] md:min-h-[500px] px-0 md:px-0">
          <div 
            className="w-full max-w-4xl"
            style={{
              minHeight: '400px'
            }}
          >
            {activeTab === 'dashboard' && (
              <div 
                className="flex flex-col lg:flex-row gap-6 lg:gap-8 w-full max-w-6xl"
                style={{
                  minHeight: '500px'
                }}
              >
                {/* Sol Panel - Ana Bilgiler */}
                <div className="flex-1 space-y-4">
                  {/* App Status Card */}
                  <div 
                    className="p-4 lg:p-5 rounded-xl lg:rounded-2xl"
                    style={{
                      backgroundColor: 'rgba(255, 255, 255, 0.7)',
                      border: '1px solid rgba(0, 0, 0, 0.06)'
                    }}
                  >
                    <div className="flex items-center justify-between mb-3">
                      <h3 className="text-sm font-semibold text-gray-900">App Status</h3>
                      <div className="flex items-center gap-2">
                        <div 
                          className="w-1.5 h-1.5 rounded-full"
                          style={{
                            backgroundColor: status.color,
                            animation: status.pulse ? 'pulse 2s infinite' : 'none'
                          }}
                        ></div>
                        <span className="text-xs font-medium" style={{ color: status.color }}>
                          {status.text}
                        </span>
                      </div>
                    </div>
                    <div className="grid grid-cols-3 gap-3 text-center">
                      <div>
                        <div className="text-lg font-bold text-gray-900">
                          {appInfo?.domains?.length || 0}
                        </div>
                        <div className="text-xs text-gray-500">Domains</div>
                      </div>
                      <div>
                        <div className="text-lg font-bold text-gray-900 font-mono">
                          {envVars.PORT || appInfo?.port || appInfo?.ports?.http || '5000'}
                        </div>
                        <div className="text-xs text-gray-500">Port</div>
                      </div>
                      <div>
                        <div className="text-lg font-bold text-gray-900">
                          {appInfo?.git_url ? 'Git' : 'Manual'}
                        </div>
                        <div className="text-xs text-gray-500">Deploy</div>
                      </div>
                    </div>
                  </div>

                  {/* Deployment Configuration */}
                  <div 
                    className="p-4 lg:p-5 rounded-xl lg:rounded-2xl"
                    style={{
                      backgroundColor: 'rgba(255, 255, 255, 0.7)',
                      border: '1px solid rgba(0, 0, 0, 0.06)'
                    }}
                  >
                    <h3 className="text-sm font-semibold text-gray-900 mb-3">Deployment Configuration</h3>
                    
                    {/* Source Selection */}
                    <div className="mb-4">
                      <div className="text-xs text-gray-600 mb-2">Source</div>
                      <div className="grid grid-cols-2 gap-2">
                        <button
                          onClick={() => setDeploySource('github')}
                          className="p-3 rounded-lg text-left transition-all duration-300 border"
                          style={{
                            backgroundColor: deploySource === 'github' ? 'rgba(59, 130, 246, 0.05)' : 'rgba(255, 255, 255, 0.6)',
                            borderColor: deploySource === 'github' ? '#3b82f6' : 'rgba(0, 0, 0, 0.06)',
                            cursor: 'pointer'
                          }}
                        >
                          <div className="text-xs font-semibold text-gray-900 mb-1">GitHub</div>
                          <div className="text-xs text-gray-600">Connected repos</div>
                        </button>
                        <button
                          onClick={() => setDeploySource('remote')}
                          className="p-3 rounded-lg text-left transition-all duration-300 border"
                          style={{
                            backgroundColor: deploySource === 'remote' ? 'rgba(59, 130, 246, 0.05)' : 'rgba(255, 255, 255, 0.6)',
                            borderColor: deploySource === 'remote' ? '#3b82f6' : 'rgba(0, 0, 0, 0.06)',
                            cursor: 'pointer'
                          }}
                        >
                          <div className="text-xs font-semibold text-gray-900 mb-1">Git URL</div>
                          <div className="text-xs text-gray-600">Any Git repo</div>
                        </button>
                      </div>
                    </div>

                    {/* Deploy Mode */}
                    <div className="space-y-2">
                      <div className="text-xs text-gray-600 mb-2">Deploy Mode</div>
                      <div className="flex items-center gap-4">
                        <label className="flex items-center gap-2 cursor-not-allowed opacity-50">
                          <input
                            type="checkbox"
                            checked={autoDeployEnabled}
                            onChange={(e) => setAutoDeployEnabled((e.target as HTMLInputElement).checked)}
                            className="w-3 h-3 text-blue-600 rounded border-gray-300"
                            disabled
                          />
                          <span className="text-xs text-gray-700">Auto Deploy (Coming Soon)</span>
                        </label>
                        <label className="flex items-center gap-2 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={manualDeployEnabled}
                            onChange={(e) => setManualDeployEnabled((e.target as HTMLInputElement).checked)}
                            className="w-3 h-3 text-blue-600 rounded border-gray-300"
                          />
                          <span className="text-xs text-gray-700">Manual Deploy</span>
                        </label>
                      </div>
                    </div>
                  </div>

                  {/* Repository & Build Settings */}
                  <div 
                    className="p-4 lg:p-5 rounded-xl lg:rounded-2xl"
                    style={{
                      backgroundColor: 'rgba(255, 255, 255, 0.7)',
                      border: '1px solid rgba(0, 0, 0, 0.06)'
                    }}
                  >
                    <h3 className="text-sm font-semibold text-gray-900 mb-3">Repository & Build Settings</h3>
                    
                    {/* Repository */}
                    <div className="mb-4">
                      <div className="text-xs text-gray-600 mb-2">Repository</div>
                      {deploySource === 'github' ? (
                        <GitHubRepositorySelector
                          appName={appName}
                          onRepositorySelect={handleRepositorySelect}
                        />
                      ) : (
                        <div className="space-y-2">
                          <input
                            type="url"
                            value={gitUrl}
                            onInput={(e) => setGitUrl((e.target as HTMLInputElement).value)}
                            placeholder="https://github.com/username/repo.git"
                            className="w-full px-3 py-2 rounded-lg transition-all duration-300 text-xs"
                            style={{
                              backgroundColor: 'rgba(255, 255, 255, 0.8)',
                              border: '1px solid rgba(0, 0, 0, 0.06)',
                              color: '#0f172a',
                              outline: 'none'
                            }}
                          />
                          <input
                            type="text"
                            value={gitBranch}
                            onInput={(e) => setGitBranch((e.target as HTMLInputElement).value)}
                            placeholder="main"
                            className="w-full px-3 py-2 rounded-lg transition-all duration-300 text-xs"
                            style={{
                              backgroundColor: 'rgba(255, 255, 255, 0.8)',
                              border: '1px solid rgba(0, 0, 0, 0.06)',
                              color: '#0f172a',
                              outline: 'none'
                            }}
                          />
                        </div>
                      )}
                    </div>

                    {/* Buildpack */}
                    <div>
                      <div className="text-xs text-gray-600 mb-2">Buildpack</div>
                      <div className="grid grid-cols-2 gap-2">
                        <button
                          onClick={() => setSelectedBuilder('pack')}
                          className="p-3 rounded-lg text-left transition-all duration-300 border"
                          style={{
                            backgroundColor: selectedBuilder === 'pack' ? 'rgba(0, 0, 0, 0.04)' : 'rgba(255, 255, 255, 0.6)',
                            borderColor: selectedBuilder === 'pack' ? '#000000' : 'rgba(0, 0, 0, 0.06)',
                            cursor: 'pointer'
                          }}
                        >
                          <div className="text-xs font-semibold text-gray-900 mb-1">Cloud Native</div>
                          <div className="text-xs text-gray-600">Auto-detect</div>
                        </button>
                        <button
                          onClick={() => setSelectedBuilder('dockerfile')}
                          className="p-3 rounded-lg text-left transition-all duration-300 border"
                          style={{
                            backgroundColor: selectedBuilder === 'dockerfile' ? 'rgba(0, 0, 0, 0.04)' : 'rgba(255, 255, 255, 0.6)',
                            borderColor: selectedBuilder === 'dockerfile' ? '#000000' : 'rgba(0, 0, 0, 0.06)',
                            cursor: 'pointer'
                          }}
                        >
                          <div className="text-xs font-semibold text-gray-900 mb-1">Dockerfile</div>
                          <div className="text-xs text-gray-600">Custom build</div>
                        </button>
                      </div>
                    </div>
                  </div>

                  {/* Deploy Actions */}
                  <div 
                    className="p-4 lg:p-5 rounded-xl lg:rounded-2xl"
                    style={{
                      backgroundColor: 'rgba(255, 255, 255, 0.7)',
                      border: '1px solid rgba(0, 0, 0, 0.06)'
                    }}
                  >
                    <h3 className="text-sm font-semibold text-gray-900 mb-3">Actions</h3>
                    <div className="flex flex-col sm:flex-row gap-2">
                      <button
                        onClick={() => {
                          if (!deployLoading && gitUrl.trim()) {
                            handleDeploy();
                          }
                        }}
                        disabled={deployLoading || !gitUrl.trim()}
                        className="flex-1 py-2 px-4 rounded-lg transition-all duration-300 text-xs font-semibold"
                        style={{
                          backgroundColor: deployLoading || !gitUrl.trim() ? 'rgba(0, 0, 0, 0.3)' : '#000000',
                          color: '#ffffff',
                          border: 'none',
                          cursor: deployLoading || !gitUrl.trim() ? 'not-allowed' : 'pointer',
                          opacity: deployLoading ? 0.7 : 1
                        }}
                      >
                        {deployLoading ? (
                          <div style={{ display: 'flex', alignItems: 'center', gap: '6px', justifyContent: 'center' }}>
                            <div style={{ 
                              width: '10px', 
                              height: '10px', 
                              border: '2px solid rgba(255,255,255,0.3)', 
                              borderTop: '2px solid white', 
                              borderRadius: '50%',
                              animation: 'spin 1s linear infinite'
                            }}></div>
                            Deploying...
                          </div>
                        ) : 'Deploy'}
                      </button>
                      <button
                        onClick={() => handleRestartApp()}
                        className="flex-1 py-2 px-4 rounded-lg transition-all duration-300 text-xs font-semibold"
                        style={{
                          backgroundColor: '#ffffff',
                          color: '#000000',
                          border: '1px solid rgba(0, 0, 0, 0.2)',
                          cursor: 'pointer'
                        }}
                      >
                        Restart
                      </button>
                    </div>
                  </div>
                </div>

                {/* Right Panel - Recent Activities */}
                <div className="w-full lg:w-80 space-y-4">


                  {/* Last Activities */}
                  <div 
                    className="p-4 lg:p-5 rounded-xl lg:rounded-2xl"
                    style={{
                      backgroundColor: 'rgba(255, 255, 255, 0.7)',
                      border: '1px solid rgba(0, 0, 0, 0.06)'
                    }}
                  >
                    <h3 className="text-sm font-semibold text-gray-900 mb-3">Last Activities</h3>
                    <div className="space-y-2">
                      {activities.length > 0 ? (
                        activities.map((activity) => (
                          <div 
                            key={activity.id}
                            className="flex items-center gap-2 p-2 rounded-lg bg-gray-50 border border-gray-200"
                          >
                            <div className="flex-1 min-w-0">
                              <div className="text-xs font-medium text-gray-900 truncate">
                                [{getActivityTypeText(activity.type)}] {activity.message}
                              </div>
                              <div className="text-xs text-gray-500 mt-1">
                                {formatRelativeTime(activity.timestamp)} - {getActivityStatusText(activity.status)}
                              </div>
                            </div>
                          </div>
                        ))
                      ) : (
                        <div className="text-center py-4 text-gray-500">
                          <div className="text-xs">No recent activities</div>
                          <div className="text-xs mt-1 opacity-60">Activities will appear here</div>
                        </div>
                      )}
                    </div>
                  </div>


                </div>
              </div>
            )}

            {/* Deploy tab removed - merged into dashboard */}
            {false && (
              <div 
                className="flex flex-col items-center justify-center space-y-4 md:space-y-6"
                style={{
                  minHeight: '400px'
                }}
              >
                                  {/* Deploy Source Selection - Centered */}
                <div className="w-full max-w-2xl">
                  <h3 className="text-base md:text-lg font-semibold text-gray-900 mb-4 md:mb-6 text-center">
                    Deploy Source
                  </h3>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-3 md:gap-4">
                    <button
                      onClick={() => setDeploySource('github')}
                      className="p-4 md:p-6 rounded-lg md:rounded-xl text-left transition-all duration-300"
                      style={{
                        backgroundColor: deploySource === 'github' ? 'rgba(59, 130, 246, 0.05)' : 'rgba(255, 255, 255, 0.6)',
                        border: `2px solid ${deploySource === 'github' ? '#3b82f6' : 'rgba(0, 0, 0, 0.06)'}`,
                        cursor: 'pointer'
                      }}
                    >
                      <div className="text-sm md:text-base font-semibold text-gray-900 mb-1 md:mb-2">
                        GitHub Repository
                      </div>
                      <div className="text-xs md:text-sm text-gray-600">
                        Deploy from connected GitHub repos
                      </div>
                    </button>

                    <button
                      onClick={() => setDeploySource('remote')}
                      className="p-4 md:p-6 rounded-lg md:rounded-xl text-left transition-all duration-300"
                      style={{
                        backgroundColor: deploySource === 'remote' ? 'rgba(59, 130, 246, 0.05)' : 'rgba(255, 255, 255, 0.6)',
                        border: `2px solid ${deploySource === 'remote' ? '#3b82f6' : 'rgba(0, 0, 0, 0.06)'}`,
                        cursor: 'pointer'
                      }}
                    >
                      <div className="text-sm md:text-base font-semibold text-gray-900 mb-1 md:mb-2">
                        Git Remote URL
                      </div>
                      <div className="text-xs md:text-sm text-gray-600">
                        Deploy from any Git URL
                      </div>
                    </button>
                  </div>
                </div>

                                  {/* Builder Selection - Centered */}
                <div className="w-full max-w-2xl">
                  <h3 className="text-base md:text-lg font-semibold text-gray-900 mb-4 md:mb-6 text-center">
                    Builder
                  </h3>
                  <div className="grid grid-cols-1 gap-3 md:gap-4">
                    {BUILDER_OPTIONS.map((builder) => (
                      <button
                        key={builder.value}
                        onClick={() => setSelectedBuilder(builder.value)}
                        className="p-4 md:p-6 rounded-lg md:rounded-xl text-left transition-all duration-300"
                        style={{
                          backgroundColor: selectedBuilder === builder.value ? 'rgba(0, 0, 0, 0.04)' : 'rgba(255, 255, 255, 0.6)',
                          border: `2px solid ${selectedBuilder === builder.value ? '#000000' : 'rgba(0, 0, 0, 0.06)'}`,
                          cursor: 'pointer'
                        }}
                      >
                        <div className="text-sm md:text-base font-semibold text-gray-900 mb-1 md:mb-2">
                          {builder.name}
                        </div>
                        <div className="text-xs md:text-sm text-gray-600">
                          {builder.description}
                        </div>
                      </button>
                    ))}
                  </div>
                </div>

                                  {/* Repository Configuration - Centered */}
                <div className="w-full max-w-2xl">
                  <h3 className="text-base md:text-lg font-semibold text-gray-900 mb-4 md:mb-6 text-center">
                    Repository
                  </h3>
                  {deploySource === 'github' ? (
                    <GitHubRepositorySelector
                      appName={appName}
                      onRepositorySelect={handleRepositorySelect}
                    />
                  ) : (
                    <div className="space-y-3 md:space-y-4">
                      <input
                        type="url"
                        value={gitUrl}
                        onInput={(e) => setGitUrl((e.target as HTMLInputElement).value)}
                        placeholder="https://github.com/username/repo.git"
                        className="w-full px-3 md:px-4 py-3 md:py-4 rounded-lg md:rounded-xl transition-all duration-300 text-sm md:text-base"
                        style={{
                          backgroundColor: 'rgba(255, 255, 255, 0.8)',
                          border: '1px solid rgba(0, 0, 0, 0.06)',
                          color: '#0f172a',
                          outline: 'none'
                        }}
                      />
                      <input
                        type="text"
                        value={gitBranch}
                        onInput={(e) => setGitBranch((e.target as HTMLInputElement).value)}
                        placeholder="main"
                        className="w-full px-3 md:px-4 py-3 md:py-4 rounded-lg md:rounded-xl transition-all duration-300 text-sm md:text-base"
                        style={{
                          backgroundColor: 'rgba(255, 255, 255, 0.8)',
                          border: '1px solid rgba(0, 0, 0, 0.06)',
                          color: '#0f172a',
                          outline: 'none'
                        }}
                      />
                    </div>
                  )}
                </div>

                                  {/* Deploy Button - Centered */}
                <div className="w-full max-w-sm md:max-w-md">
                  <button
                    onClick={handleDeploy}
                    disabled={deployLoading || !gitUrl.trim()}
                    className="w-full py-3 md:py-4 rounded-lg md:rounded-xl transition-all duration-300 text-sm md:text-base font-semibold"
                    style={{
                      backgroundColor: deployLoading || !gitUrl.trim() ? 'rgba(0, 0, 0, 0.3)' : '#22c55e',
                      color: '#ffffff',
                      border: 'none',
                      cursor: deployLoading || !gitUrl.trim() ? 'not-allowed' : 'pointer',
                      opacity: deployLoading ? 0.7 : 1
                    }}
                  >
                    {deployLoading ? (
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', justifyContent: 'center' }}>
                        <div style={{ 
                          width: '14px', 
                          height: '14px', 
                          border: '2px solid rgba(255,255,255,0.3)', 
                          borderTop: '2px solid white', 
                          borderRadius: '50%',
                          animation: 'spin 1s linear infinite'
                        }}></div>
                        Deploying Application...
                      </div>
                    ) : 'Deploy Application'}
                  </button>
                </div>
              </div>
            )}

            {activeTab === 'settings' && (
              <div 
                className="flex justify-center items-center px-0 md:px-0"
                style={{
                  minHeight: '500px'
                }}
              >
                <div 
                  className="flex flex-col lg:flex-row gap-4 lg:gap-8 w-full max-w-6xl"
                  style={{
                    minHeight: '400px'
                  }}
                >
                  {/* Settings Sidebar */}
                  <div 
                    className="w-full lg:w-48 p-4 lg:p-5 rounded-xl lg:rounded-2xl"
                    style={{
                      backgroundColor: 'rgba(255, 255, 255, 0.7)',
                      border: '1px solid rgba(0, 0, 0, 0.06)',
                      height: 'fit-content'
                    }}
                  >
                    <h3 style={{ fontSize: '16px', fontWeight: '600', color: '#0f172a', marginBottom: '12px' }}>
                      Settings
                    </h3>
                    
                    {/* Mobile: Settings Select */}
                    <div className="block lg:hidden">
                      <CustomSelect
                        options={[
                          { value: 'domains', label: 'Domains' },
                          { value: 'environment', label: 'Environment' },
                          { value: 'access', label: 'Access' },
                          { value: 'danger', label: 'Danger Zone' }
                        ]}
                        value={activeSettingsTab}
                        onChange={(value) => setActiveSettingsTab(value as any)}
                        placeholder="Select Settings"
                        className="w-full"
                      />
                    </div>

                    {/* Desktop: Settings Buttons */}
                    <div className="hidden lg:flex lg:flex-col gap-2 lg:space-y-2">
                      <button
                        onClick={() => setActiveSettingsTab('domains')}
                        className="w-full text-left px-4 py-3 rounded-xl transition-all duration-300 text-sm"
                        style={{
                          backgroundColor: activeSettingsTab === 'domains' ? '#000000' : 'transparent',
                          color: activeSettingsTab === 'domains' ? '#ffffff' : '#64748b',
                          border: 'none',
                          fontWeight: '500',
                          cursor: 'pointer'
                        }}
                      >
                        Domains
                      </button>
                      <button
                        onClick={() => setActiveSettingsTab('environment')}
                        className="w-full text-left px-4 py-3 rounded-xl transition-all duration-300 text-sm"
                        style={{
                          backgroundColor: activeSettingsTab === 'environment' ? '#000000' : 'transparent',
                          color: activeSettingsTab === 'environment' ? '#ffffff' : '#64748b',
                          border: 'none',
                          fontWeight: '500',
                          cursor: 'pointer'
                        }}
                      >
                        Environment
                      </button>
                      <button
                        onClick={() => setActiveSettingsTab('access')}
                        className="w-full text-left px-4 py-3 rounded-xl transition-all duration-300 text-sm"
                        style={{
                          backgroundColor: activeSettingsTab === 'access' ? '#000000' : 'transparent',
                          color: activeSettingsTab === 'access' ? '#ffffff' : '#64748b',
                          border: 'none',
                          fontWeight: '500',
                          cursor: 'pointer'
                        }}
                      >
                        Access
                      </button>
                      <button
                        onClick={() => setActiveSettingsTab('danger')}
                        className="w-full text-left px-4 py-3 rounded-xl transition-all duration-300 text-sm"
                        style={{
                          backgroundColor: activeSettingsTab === 'danger' ? '#000000' : 'transparent',
                          color: activeSettingsTab === 'danger' ? '#ffffff' : '#64748b',
                          border: 'none',
                          fontWeight: '500',
                          cursor: 'pointer'
                        }}
                      >
                        Danger Zone
                      </button>
                    </div>
                  </div>

                  {/* Settings Content */}
                  <div className="flex-1 min-w-0">
                    {/* Domain Management */}
                    {activeSettingsTab === 'domains' && (
                      <div className="p-5 lg:p-6 rounded-xl lg:rounded-2xl"
                        style={{
                          backgroundColor: 'rgba(255, 255, 255, 0.7)',
                          border: '1px solid rgba(0, 0, 0, 0.06)',
                          minHeight: '400px'
                        }}
                      >
                        <h3 style={{ fontSize: '18px', fontWeight: '600', color: '#0f172a', marginBottom: '16px' }}>
                          Domain Management
                        </h3>
                        
                        {/* Current Domains */}
                        <div className="mb-4">
                          <h4 style={{ fontSize: '14px', fontWeight: '600', color: '#64748b', marginBottom: '8px' }}>
                            Active Domains
                          </h4>
                          {appInfo?.domains && appInfo.domains.length > 0 ? (
                            <div className="space-y-2 overflow-x-auto">
                              {appInfo.domains.map((domain, index) => (
                                <div 
                                  key={domain}
                                  className="flex flex-col sm:flex-row sm:items-center sm:justify-between p-3 md:p-4 rounded-lg gap-3 min-w-0"
                                  style={{
                                    backgroundColor: 'rgba(255, 255, 255, 0.6)',
                                    border: '1px solid rgba(0, 0, 0, 0.06)'
                                  }}
                                >
                                  <div className="flex items-center gap-3 min-w-0 flex-1">
                                    <div className="w-2 h-2 bg-green-500 rounded-full flex-shrink-0"></div>
                                    <span 
                                      className="truncate"
                                      style={{ 
                                        fontSize: '12px',
                                        fontWeight: '500',
                                        color: '#0f172a',
                                        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace'
                                      }}
                                    >
                                      {domain}
                                    </span>
                                    {index === 0 && (
                                      <span 
                                        className="px-2 py-1 rounded-full text-xs flex-shrink-0"
                                        style={{
                                          backgroundColor: 'rgba(59, 130, 246, 0.1)',
                                          color: '#3b82f6',
                                          fontSize: '8px',
                                          fontWeight: '500'
                                        }}
                                      >
                                        Primary
                                      </span>
                                    )}
                                  </div>
                                  <div className="flex items-center gap-3 justify-end sm:justify-start flex-shrink-0">
                                    <a
                                      href={`http://${domain}`}
                                      target="_blank"
                                      rel="noopener noreferrer"
                                      className="text-green-600 hover:text-green-700 transition-colors p-1"
                                      title="Open domain"
                                    >
                                      <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                                      </svg>
                                    </a>
                                    <button
                                      onClick={() => handleDeleteConfirm('domain', domain)}
                                      className="p-1 text-red-500 hover:text-red-700 hover:bg-red-50 rounded transition-colors"
                                      title="Remove domain"
                                    >
                                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                                      </svg>
                                    </button>
                                  </div>
                                </div>
                              ))}
                            </div>
                          ) : (
                            <p style={{ fontSize: '13px', color: '#64748b' }}>No domains configured</p>
                          )}
                        </div>

                        {/* Add Domain Form */}
                        <div>
                          <h4 style={{ fontSize: '14px', fontWeight: '600', color: '#64748b', marginBottom: '8px' }}>
                            Add New Domain
                          </h4>
                          <form onSubmit={handleAddDomain} className="flex flex-col sm:flex-row gap-3">
                            <input
                              type="text"
                              value={newDomain}
                              onInput={(e) => setNewDomain((e.target as HTMLInputElement).value)}
                              placeholder="example.com"
                              className="flex-1 px-3 py-2 rounded-lg transition-all duration-300"
                              style={{
                                backgroundColor: 'rgba(255, 255, 255, 0.8)',
                                border: '1px solid rgba(0, 0, 0, 0.06)',
                                color: '#0f172a',
                                outline: 'none',
                                fontSize: '13px'
                              }}
                              disabled={domainLoading}
                            />
                            <button
                              type="submit"
                              disabled={domainLoading || !newDomain.trim()}
                              className="px-4 py-2 rounded-lg transition-all duration-300 whitespace-nowrap"
                              style={{
                                backgroundColor: '#000000',
                                color: '#ffffff',
                                border: 'none',
                                fontSize: '13px',
                                fontWeight: '500',
                                opacity: domainLoading || !newDomain.trim() ? '0.5' : '1',
                                cursor: domainLoading || !newDomain.trim() ? 'not-allowed' : 'pointer'
                              }}
                            >
                              {domainLoading ? 'Adding...' : 'Add'}
                            </button>
                          </form>
                        </div>
                      </div>
                    )}

                    {/* Environment Variables */}
                    {activeSettingsTab === 'environment' && (
                      <div className="p-5 lg:p-6 rounded-xl lg:rounded-2xl"
                        style={{
                          backgroundColor: 'rgba(255, 255, 255, 0.7)',
                          border: '1px solid rgba(0, 0, 0, 0.06)',
                          minHeight: '400px'
                        }}
                      >
                        <h3 style={{ fontSize: '18px', fontWeight: '600', color: '#0f172a', marginBottom: '16px' }}>
                          Environment Variables
                        </h3>
                        
                        {/* Add Environment Variable Form */}
                        <div className="mb-4">
                          <h4 style={{ fontSize: '14px', fontWeight: '600', color: '#64748b', marginBottom: '8px' }}>
                            Add New Variable
                          </h4>
                          <form onSubmit={handleSetEnv} className="space-y-3">
                            <div className="flex flex-col sm:flex-row gap-3">
                              <input
                                type="text"
                                value={envKey}
                                onInput={(e) => setEnvKey((e.target as HTMLInputElement).value)}
                                placeholder="Variable name"
                                className="flex-1 px-3 py-2 rounded-lg transition-all duration-300"
                                style={{
                                  backgroundColor: 'rgba(255, 255, 255, 0.8)',
                                  border: `1px solid ${envKey.trim() === 'PORT' ? '#ef4444' : envVars.hasOwnProperty(envKey.trim()) && envKey.trim() ? '#f59e0b' : 'rgba(0, 0, 0, 0.06)'}`,
                                  color: '#0f172a',
                                  outline: 'none',
                                  fontSize: '13px'
                                }}
                                disabled={envLoading}
                              />
                              <input
                                type="text"
                                value={envValue}
                                onInput={(e) => setEnvValue((e.target as HTMLInputElement).value)}
                                placeholder="Variable value"
                                className="flex-1 px-3 py-2 rounded-lg transition-all duration-300"
                                style={{
                                  backgroundColor: 'rgba(255, 255, 255, 0.8)',
                                  border: '1px solid rgba(0, 0, 0, 0.06)',
                                  color: '#0f172a',
                                  outline: 'none',
                                  fontSize: '13px'
                                }}
                                disabled={envLoading}
                              />
                            </div>
                            {envKey.trim() === 'PORT' ? (
                              <div 
                                style={{ 
                                  fontSize: '11px', 
                                  color: '#ef4444', 
                                  marginTop: '4px',
                                  fontWeight: '500'
                                }}
                              >
                                PORT cannot be modified manually
                              </div>
                            ) : envVars.hasOwnProperty(envKey.trim()) && envKey.trim() && (
                              <div 
                                style={{ 
                                  fontSize: '11px', 
                                  color: '#f59e0b', 
                                  marginTop: '4px',
                                  fontWeight: '500'
                                }}
                              >
                                This will update existing variable
                              </div>
                            )}
                            <button
                              type="submit"
                              disabled={envLoading || !envKey.trim() || !envValue.trim() || envKey.trim() === 'PORT'}
                              className="w-full py-2 rounded-lg transition-all duration-300"
                              style={{
                                backgroundColor: envLoading || !envKey.trim() || !envValue.trim() || envKey.trim() === 'PORT' ? 'rgba(0, 0, 0, 0.3)' : '#000000',
                                color: '#ffffff',
                                border: 'none',
                                fontSize: '13px',
                                fontWeight: '500',
                                cursor: envLoading || !envKey.trim() || !envValue.trim() || envKey.trim() === 'PORT' ? 'not-allowed' : 'pointer'
                              }}
                            >
                              {envLoading ? 'Setting...' : 
                               envVars.hasOwnProperty(envKey.trim()) && envKey.trim() ? 'Update Variable' : 'Set Variable'}
                            </button>
                          </form>
                        </div>

                        {/* Current Environment Variables */}
                        <div>
                          <h4 className="text-sm font-semibold text-gray-600 mb-3">
                            Current Variables ({Object.keys(envVars).length})
                          </h4>
                          <div className="space-y-3">
                            {Object.keys(envVars).length > 0 ? (
                              Object.entries(envVars).map(([key, value]) => {
                                const isPortVariable = key === 'PORT';
                                const isVisible = visibleEnvVars[key];
                                return (
                                  <div key={key} className="space-y-2">
                                    {/* Key Input */}
                                    <div className="flex items-center gap-2">
                                      <input
                                        type="text"
                                        value={key}
                                        readOnly
                                        className="flex-1 px-3 py-2 rounded-lg text-sm font-mono bg-gray-50 border"
                                        style={{
                                          borderColor: isPortVariable ? '#3b82f6' : '#e5e7eb',
                                          backgroundColor: isPortVariable ? 'rgba(59, 130, 246, 0.05)' : '#f9fafb',
                                          color: '#374151'
                                        }}
                                      />
                                      <div className="flex items-center gap-1">
                                        {isPortVariable && (
                                          <span className="px-2 py-1 bg-blue-100 text-blue-600 text-xs rounded-full font-medium">
                                            system
                                          </span>
                                        )}
                                        {!isPortVariable ? (
                                          <button
                                            onClick={() => handleDeleteConfirm('env', key)}
                                            className="p-1 text-red-500 hover:text-red-700 hover:bg-red-50 rounded transition-colors"
                                            title="Remove variable"
                                          >
                                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                                            </svg>
                                          </button>
                                        ) : (
                                          <span className="px-2 py-1 text-gray-400 text-xs">
                                            Protected
                                          </span>
                                        )}
                                      </div>
                                    </div>

                                    {/* Value Input */}
                                    <div className="relative">
                                      <input
                                        type={isVisible ? "text" : "password"}
                                        value={value || ''}
                                        readOnly
                                        placeholder="Empty"
                                        className="w-full px-3 py-2 pr-10 rounded-lg text-sm font-mono bg-gray-50 border border-gray-200 text-gray-700"
                                      />
                                      <button
                                        onClick={() => toggleEnvVisibility(key)}
                                        className="absolute right-2 top-1/2 transform -translate-y-1/2 p-1 text-gray-400 hover:text-gray-600 transition-colors"
                                      >
                                        {isVisible ? (
                                          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" />
                                          </svg>
                                        ) : (
                                          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                                          </svg>
                                        )}
                                      </button>
                                    </div>

                                    {isPortVariable && (
                                      <p className="text-xs text-gray-500 italic">
                                        Automatically set during deployment
                                      </p>
                                    )}
                                  </div>
                                );
                              })
                            ) : (
                              <div 
                                className="text-center py-4 rounded-lg"
                                style={{
                                  backgroundColor: 'rgba(255, 255, 255, 0.6)',
                                  border: '1px solid rgba(0, 0, 0, 0.06)'
                                }}
                              >
                                <p style={{ fontSize: '13px', color: '#64748b' }}>No environment variables set</p>
                                <p style={{ fontSize: '11px', color: '#9ca3af', marginTop: '4px' }}>
                                  Add your first environment variable above
                                </p>
                              </div>
                            )}
                          </div>
                        </div>
                      </div>
                    )}

                    {/* Access Control */}
                    {activeSettingsTab === 'access' && (
                      <div className="p-5 lg:p-6 rounded-xl lg:rounded-2xl"
                        style={{
                          backgroundColor: 'rgba(255, 255, 255, 0.7)',
                          border: '1px solid rgba(0, 0, 0, 0.06)',
                          minHeight: '400px'
                        }}
                      >
                        <h3 style={{ fontSize: '18px', fontWeight: '600', color: '#0f172a', marginBottom: '16px' }}>
                          Access Control
                        </h3>
                        
                        <div className="mb-6">
                          <h4 style={{ fontSize: '14px', fontWeight: '600', color: '#64748b', marginBottom: '8px' }}>
                            Public Access
                          </h4>
                          <p style={{ fontSize: '13px', color: '#64748b', marginBottom: '16px' }}>
                            Control whether this app requires authentication to access. Public apps can be accessed by anyone without logging in.
                          </p>
                          
                          <div className="flex items-center gap-4">
                            <label className="flex items-center gap-3 cursor-pointer">
                              <input
                                type="radio"
                                name="publicAccess"
                                checked={!isPublic}
                                onChange={() => handleUpdatePublicSetting(false)}
                                disabled={updatePublicLoading}
                                className="w-4 h-4 text-blue-600 border-gray-300 focus:ring-blue-500"
                              />
                              <div>
                                <span className="text-sm font-medium text-gray-900">Private</span>
                                <span className="block text-xs text-gray-500">Requires authentication</span>
                              </div>
                            </label>
                            
                            <label className="flex items-center gap-3 cursor-pointer">
                              <input
                                type="radio"
                                name="publicAccess"
                                checked={isPublic}
                                onChange={() => handleUpdatePublicSetting(true)}
                                disabled={updatePublicLoading}
                                className="w-4 h-4 text-blue-600 border-gray-300 focus:ring-blue-500"
                              />
                              <div>
                                <span className="text-sm font-medium text-gray-900">Public</span>
                                <span className="block text-xs text-gray-500">No authentication required</span>
                              </div>
                            </label>
                          </div>
                          
                          {updatePublicLoading && (
                            <div className="mt-4 flex items-center gap-2">
                              <div className="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                              <span className="text-sm text-gray-600">Updating access settings...</span>
                            </div>
                          )}
                        </div>

                        {/* Current Status */}
                        <div 
                          className="p-4 rounded-lg"
                          style={{
                            backgroundColor: isPublic ? 'rgba(34, 197, 94, 0.05)' : 'rgba(59, 130, 246, 0.05)',
                            border: `1px solid ${isPublic ? 'rgba(34, 197, 94, 0.2)' : 'rgba(59, 130, 246, 0.2)'}`
                          }}
                        >
                          <div className="flex items-center gap-3">
                            <div 
                              className="w-3 h-3 rounded-full"
                              style={{
                                backgroundColor: isPublic ? '#22c55e' : '#3b82f6'
                              }}
                            ></div>
                            <div>
                              <h5 className="text-sm font-medium text-gray-900">
                                {isPublic ? 'Public Access Enabled' : 'Private Access (Default)'}
                              </h5>
                              <p className="text-xs text-gray-600">
                                {isPublic 
                                  ? 'Anyone can access this app without logging in'
                                  : 'Users must authenticate to access this app'
                                }
                              </p>
                            </div>
                          </div>
                        </div>

                        {/* Warning for Public Apps */}
                        {isPublic && (
                          <div 
                            className="p-4 rounded-lg mt-4"
                            style={{
                              backgroundColor: 'rgba(245, 158, 11, 0.05)',
                              border: '1px solid rgba(245, 158, 11, 0.2)'
                            }}
                          >
                            <div className="flex items-start gap-3">
                              <svg className="w-5 h-5 text-amber-500 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
                              </svg>
                              <div>
                                <h6 className="text-sm font-medium text-amber-800">Security Notice</h6>
                                <p className="text-xs text-amber-700 mt-1">
                                  Public apps are accessible to anyone on the internet. Make sure your app doesn't contain sensitive data or functionality that should be protected.
                                </p>
                              </div>
                            </div>
                          </div>
                        )}
                      </div>
                    )}

                    {/* Danger Zone */}
                    {activeSettingsTab === 'danger' && (
                      <div 
                        className="p-5 lg:p-6 rounded-xl lg:rounded-2xl"
                        style={{
                          backgroundColor: 'rgba(239, 68, 68, 0.05)',
                          border: '1px solid rgba(239, 68, 68, 0.1)',
                          minHeight: '400px'
                        }}
                      >
                        <h3 style={{ fontSize: '18px', fontWeight: '600', color: '#ef4444', marginBottom: '8px' }}>
                          Danger Zone
                        </h3>
                        <p style={{ fontSize: '14px', color: '#64748b', marginBottom: '16px' }}>
                          This action cannot be undone. All data will be permanently deleted.
                        </p>
                        <button
                          onClick={handleDeleteApp}
                          disabled={deleteLoading}
                          className="px-6 py-3 rounded-xl transition-all duration-300"
                          style={{
                            backgroundColor: '#ef4444',
                            color: '#ffffff',
                            border: 'none',
                            fontSize: '14px',
                            fontWeight: '500',
                            opacity: deleteLoading ? '0.5' : '1',
                            cursor: deleteLoading ? 'not-allowed' : 'pointer'
                          }}
                        >
                          {deleteLoading ? 'Deleting...' : 'Delete App'}
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            )}

            {activeTab === 'logs' && (
              <div 
                className="flex justify-center items-center px-0 md:px-0"
                style={{
                  minHeight: '500px'
                }}
              >
                <div className="w-full max-w-4xl lg:max-w-5xl">
                  <h3 className="text-center mb-3 md:mb-6" style={{ fontSize: '16px', fontWeight: '600', color: '#0f172a' }}>
                    Application Logs
                  </h3>
                  <div 
                    className="rounded-lg md:rounded-2xl overflow-hidden"
                    style={{
                      backgroundColor: 'rgba(255, 255, 255, 0.6)',
                      border: '1px solid rgba(0, 0, 0, 0.06)',
                      minHeight: '300px',
                      height: 'auto'
                    }}
                  >
                    <LogViewer 
                      logs={logs}
                      isLive={false}
                      title=""
                    />
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Delete Confirmation Modal */}
        {deleteConfirm.show && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
            <div className="bg-white rounded-xl shadow-xl max-w-sm w-full mx-4">
              <div className="p-6">
                <div className="flex items-center gap-3 mb-4">
                  <div className="w-8 h-8 bg-red-100 rounded-full flex items-center justify-center">
                    <svg className="w-4 h-4 text-red-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
                    </svg>
                  </div>
                  <div>
                    <h3 className="text-lg font-semibold text-gray-900">
                      {deleteConfirm.type === 'env' ? 'Remove Variable' : 'Remove Domain'}
                    </h3>
                  </div>
                </div>
                
                <p className="text-gray-600 mb-6">
                  Are you sure you want to remove{' '}
                  <span className="font-mono font-medium text-gray-900">
                    {deleteConfirm.key}
                  </span>
                  ? This action cannot be undone.
                </p>
                
                <div className="flex gap-3">
                  <button
                    onClick={() => setDeleteConfirm({ show: false, type: 'env', key: '' })}
                    className="flex-1 px-4 py-2 text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-lg transition-colors"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={executeDelete}
                    className="flex-1 px-4 py-2 text-white bg-red-600 hover:bg-red-700 rounded-lg transition-colors"
                  >
                    Remove
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Deploy Logs Modal */}
        {showDeployModal && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
            <div className="bg-white rounded-xl shadow-xl max-w-4xl w-full mx-4" style={{ maxHeight: '80vh' }}>
              <div className="p-6">
                <div className="flex items-center justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <div className="w-8 h-8 bg-green-100 rounded-full flex items-center justify-center">
                      <svg className="w-4 h-4 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 3l14 9-14 9V3z" />
                      </svg>
                    </div>
                    <div>
                      <h3 className="text-lg font-semibold text-gray-900">
                        Deploy Logs
                      </h3>
                      <p className="text-sm text-gray-600">Real-time deployment progress</p>
                    </div>
                  </div>
                  <button
                    onClick={() => setShowDeployModal(false)}
                    className="p-2 text-gray-400 hover:text-gray-600 rounded-lg transition-colors"
                  >
                    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>
                
                <div className="h-96 overflow-hidden">
                  <LogViewer 
                    logs={deployLogs || 'Starting deployment...'}
                    isLive={deployLoading}
                    title=""
                  />
                </div>
                
                <div className="flex justify-end mt-4">
                  <button
                    onClick={() => setShowDeployModal(false)}
                    className="px-4 py-2 text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-lg transition-colors"
                  >
                    Close
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Animations */}
        <style>{`
          @keyframes spin {
            from { transform: rotate(0deg); }
            to { transform: rotate(360deg); }
          }
          @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.4; }
          }
        `}</style>
      </div>
    </MinimalLayout>
  );
}