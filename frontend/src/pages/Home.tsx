import { useState, useEffect } from 'preact/hooks';
import { useLocation } from 'wouter';
import { useApi } from '../hooks/useApi';
import MinimalLayout from '../components/layout/MinimalLayout';
import type { App, AppInfo } from '../types';
import { errorLog } from '../utils/debug';

export default function Home() {
  const [, setLocation] = useLocation();
  const [apps, setApps] = useState<App[]>([]);
  const [appInfos, setAppInfos] = useState<Record<string, AppInfo>>({});
  const [showContent, setShowContent] = useState(false);
  const { data: rawApps, error, loading, request: fetchApps } = useApi<string[]>();
  const { request: fetchAppInfo } = useApi<AppInfo>();
  
  useEffect(() => {
    fetchApps({ url: '/citizen/apps' });
  }, []);

  // Transform string array to App object array
  useEffect(() => {
    if (rawApps) {
      const transformedApps: App[] = rawApps.map((appName: string) => ({
        name: appName,
        status: undefined
      }));
      setApps(transformedApps);
      // Show content immediately for better UX
      setShowContent(true);
    } else if (error) {
      // Show content immediately on error
      setShowContent(true);
    }
  }, [rawApps, error]);

  // Fetch detailed info for all apps at once - performance improvement
  useEffect(() => {
    if (apps && apps.length > 0) {
      const fetchAllAppsInfo = async () => {
        try {
          const allInfo = await fetchAppInfo({ url: '/citizen/apps-info' });
          if (allInfo) {
            setAppInfos(allInfo as unknown as Record<string, AppInfo>);
          }
        } catch (error) {
          errorLog('Failed to fetch all apps info:', error);
          // Set default info for all apps if API fails
          const defaultInfos: Record<string, AppInfo> = {};
          apps.forEach(app => {
            if (app && app.name) {
              defaultInfos[app.name] = {
                domains: [],
                running: false,
                deployed: true,
                ports: { http: '5000' },
                raw: {}
              };
            }
          });
          setAppInfos(defaultInfos);
        }
      };
      
      fetchAllAppsInfo();
    }
  }, [apps]);

  const handleCreateApp = () => {
    setLocation('/apps/new');
  };

  const handleAppClick = (appName: string) => {
    setLocation(`/apps/${appName}`);
  };

  const getAppStatus = (appName: string) => {
    const info = appInfos[appName];
    // If app is newly loaded and no info yet, show a default status
    if (!info) {
      return { status: 'unknown', color: '#9ca3af', text: 'checking', pulse: true };
    }
    
    if (info.running && info.deployed) {
      return { status: 'running', color: '#22c55e', text: 'live', pulse: true };
    } else if (info.deployed) {
      return { status: 'deployed', color: '#f59e0b', text: 'deploying', pulse: true };
    } else {
      return { status: 'stopped', color: '#ef4444', text: 'stopped', pulse: false };
    }
  };

  const getAppUrl = (appName: string) => {
    const info = appInfos[appName];
    if (info?.domains && info.domains.length > 0) {
      return `http://${info.domains[0]}`;
    }
    return null;
  };

  // Don't show anything until we have attempted to load
  if (!showContent) {
    return (
      <MinimalLayout showSystemStatus={true} showLogout={true}>
        <div 
          className="min-h-screen flex flex-col items-center justify-center px-6"
          style={{
            background: 'linear-gradient(135deg, #fafafa 0%, #f8fafc 100%)',
            fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'
          }}
        >
          {/* Create App Button - Floating - ALWAYS VISIBLE */}
          <button
            onClick={handleCreateApp}
            className="fixed top-8 right-8 z-10 group"
            style={{
              background: 'linear-gradient(135deg, #1a1a1a 0%, #000000 100%)',
              border: 'none',
              borderRadius: '50%',
              width: '56px',
              height: '56px',
              boxShadow: '0 8px 32px rgba(0, 0, 0, 0.12)',
              cursor: 'pointer',
              transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)'
            }}
            onMouseEnter={(e) => {
              (e.target as HTMLElement).style.transform = 'scale(1.05)';
              (e.target as HTMLElement).style.boxShadow = '0 12px 40px rgba(0, 0, 0, 0.2)';
            }}
            onMouseLeave={(e) => {
              (e.target as HTMLElement).style.transform = 'scale(1)';
              (e.target as HTMLElement).style.boxShadow = '0 8px 32px rgba(0, 0, 0, 0.12)';
            }}
          >
            <svg 
              className="w-6 h-6 text-white" 
              fill="none" 
              stroke="currentColor" 
              viewBox="0 0 24 24"
              style={{ 
                position: 'absolute',
                top: '50%',
                left: '50%',
                transform: 'translate(-50%, -50%)'
              }}
            >
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
            </svg>
          </button>

          {/* Header */}
          <div className="text-center mb-12">
            <div 
              className="font-semibold text-xl mb-2 cursor-pointer select-none transition-all duration-200"
              onClick={() => setLocation('/')}
              style={{ 
                letterSpacing: '-0.02em',
                color: '#0f172a'
              }}
            >
              citizen
            </div>
            
            <div 
              className="font-mono text-xs select-none transition-all duration-300"
              style={{ 
                color: '#64748b',
                letterSpacing: '0.01em',
                fontWeight: '400',
                padding: '6px 12px',
                backgroundColor: 'rgba(100, 116, 139, 0.08)',
                border: '1px solid rgba(100, 116, 139, 0.12)',
                borderRadius: '8px',
                display: 'inline-block',
                cursor: 'pointer'
              }}
              onMouseEnter={(e) => {
                (e.target as HTMLElement).style.backgroundColor = 'rgba(100, 116, 139, 0.12)';
                (e.target as HTMLElement).style.borderColor = 'rgba(100, 116, 139, 0.2)';
                (e.target as HTMLElement).style.color = '#475569';
              }}
              onMouseLeave={(e) => {
                (e.target as HTMLElement).style.backgroundColor = 'rgba(100, 116, 139, 0.08)';
                (e.target as HTMLElement).style.borderColor = 'rgba(100, 116, 139, 0.12)';
                (e.target as HTMLElement).style.color = '#64748b';
              }}
            >
              @vibe://citizen.build/deploy?share=private
            </div>
          </div>

          {/* Loading content placeholder - invisible but maintains layout */}
          <div className="w-full max-w-2xl opacity-0">
            <div className="space-y-4">
              <div style={{ padding: '16px 20px', borderRadius: '12px', height: '60px' }}></div>
              <div style={{ padding: '16px 20px', borderRadius: '12px', height: '60px' }}></div>
            </div>
          </div>
        </div>
      </MinimalLayout>
    );
  }

  return (
    <MinimalLayout>
      <div 
        className="min-h-screen flex flex-col items-center justify-center"
        style={{
          padding: '0 24px'
        }}
      >
        {/* Header */}
        <div className="text-center mb-12">
          <div 
            className="font-semibold text-xl mb-2 cursor-pointer select-none transition-all duration-200"
            onClick={() => setLocation('/')}
            style={{ 
              letterSpacing: '-0.02em',
              color: '#0f172a'
            }}
            onMouseEnter={(e) => {
              (e.target as HTMLElement).style.color = '#1a1a1a';
            }}
            onMouseLeave={(e) => {
              (e.target as HTMLElement).style.color = '#0f172a';
            }}
          >
            citizen
          </div>
          
          <div 
            className="font-mono text-xs select-none transition-all duration-300"
            style={{ 
              color: '#64748b',
              letterSpacing: '0.01em',
              fontWeight: '400',
              padding: '6px 12px',
              backgroundColor: 'rgba(100, 116, 139, 0.08)',
              border: '1px solid rgba(100, 116, 139, 0.12)',
              borderRadius: '8px',
              display: 'inline-block',
              cursor: 'pointer'
            }}
            onMouseEnter={(e) => {
              (e.target as HTMLElement).style.backgroundColor = 'rgba(100, 116, 139, 0.12)';
              (e.target as HTMLElement).style.borderColor = 'rgba(100, 116, 139, 0.2)';
              (e.target as HTMLElement).style.color = '#475569';
            }}
            onMouseLeave={(e) => {
              (e.target as HTMLElement).style.backgroundColor = 'rgba(100, 116, 139, 0.08)';
              (e.target as HTMLElement).style.borderColor = 'rgba(100, 116, 139, 0.12)';
              (e.target as HTMLElement).style.color = '#64748b';
            }}
          >
            @vibe://citizen.build/deploy?share=private
          </div>
        </div>

        {/* Create App Button - Floating */}
        <button
          onClick={handleCreateApp}
          className="fixed top-8 right-8 z-10 group"
          style={{
            background: 'linear-gradient(135deg, #1a1a1a 0%, #000000 100%)',
            border: 'none',
            borderRadius: '50%',
            width: '56px',
            height: '56px',
            boxShadow: '0 8px 32px rgba(0, 0, 0, 0.12)',
            cursor: 'pointer',
            transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)'
          }}
          onMouseEnter={(e) => {
            (e.target as HTMLElement).style.transform = 'scale(1.05)';
            (e.target as HTMLElement).style.boxShadow = '0 12px 40px rgba(0, 0, 0, 0.2)';
          }}
          onMouseLeave={(e) => {
            (e.target as HTMLElement).style.transform = 'scale(1)';
            (e.target as HTMLElement).style.boxShadow = '0 8px 32px rgba(0, 0, 0, 0.12)';
          }}
        >
          <svg 
            className="w-6 h-6 text-white" 
            fill="none" 
            stroke="currentColor" 
            viewBox="0 0 24 24"
            style={{ 
              position: 'absolute',
              top: '50%',
              left: '50%',
              transform: 'translate(-50%, -50%)'
            }}
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
          </svg>
        </button>

        {/* Main Content */}
        <div className="w-full max-w-2xl">
          {error ? (
            <div className="text-center py-12">
              <div 
                className="inline-flex items-center gap-2 px-4 py-2 rounded-full mb-4"
                style={{
                  backgroundColor: 'rgba(239, 68, 68, 0.08)',
                  border: '1px solid rgba(239, 68, 68, 0.12)'
                }}
              >
                <div className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: '#ef4444' }}></div>
                <span style={{ color: '#ef4444', fontSize: '13px', fontWeight: '500' }}>
                  failed to load apps
                </span>
              </div>
              <button 
                onClick={() => fetchApps({ url: '/citizen/apps' })}
                className="px-4 py-2 rounded-full transition-all duration-300"
                style={{
                  backgroundColor: '#000000',
                  color: '#ffffff',
                  border: 'none',
                  fontSize: '13px',
                  fontWeight: '500',
                  cursor: 'pointer'
                }}
                onMouseEnter={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = '#1a1a1a';
                }}
                onMouseLeave={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = '#000000';
                }}
              >
                try again
              </button>
            </div>
          ) : apps && apps.length > 0 ? (
            <div className="space-y-4">
              {apps.map((app) => {
                if (!app || !app.name) return null;
                
                const status = getAppStatus(app.name);
                const appUrl = getAppUrl(app.name);
                
                return (
                  <div 
                    key={app.name}
                    onClick={() => handleAppClick(app.name)}
                    className="group cursor-pointer transition-all duration-300"
                    style={{
                      padding: '16px 20px',
                      borderRadius: '12px',
                      backgroundColor: 'transparent',
                      border: '1px solid transparent',
                      transform: 'translateY(0)',
                    }}
                    onMouseEnter={(e) => {
                      (e.currentTarget as HTMLElement).style.transform = 'translateY(-2px)';
                      (e.currentTarget as HTMLElement).style.backgroundColor = 'rgba(255, 255, 255, 0.8)';
                      (e.currentTarget as HTMLElement).style.borderColor = 'rgba(0, 0, 0, 0.06)';
                      (e.currentTarget as HTMLElement).style.boxShadow = '0 8px 32px rgba(0, 0, 0, 0.12)';
                    }}
                    onMouseLeave={(e) => {
                      (e.currentTarget as HTMLElement).style.transform = 'translateY(0)';
                      (e.currentTarget as HTMLElement).style.backgroundColor = 'transparent';
                      (e.currentTarget as HTMLElement).style.borderColor = 'transparent';
                      (e.currentTarget as HTMLElement).style.boxShadow = 'none';
                    }}
                  >
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-4">
                        {/* Status Light */}
                        <div className="flex items-center gap-2">
                          <div 
                            className="w-2 h-2 rounded-full"
                            style={{ 
                              backgroundColor: status.color,
                              animation: status.pulse ? 'pulse 2s infinite' : 'none'
                            }}
                          ></div>
                          <span 
                            style={{ 
                              fontSize: '12px',
                              fontWeight: '500',
                              color: status.color,
                              letterSpacing: '0.01em',
                              textTransform: 'lowercase'
                            }}
                          >
                            {status.text}
                          </span>
                        </div>
                        
                        {/* App Name */}
                        <h3 
                          style={{ 
                            fontSize: '16px',
                            fontWeight: '600',
                            color: '#0f172a',
                            letterSpacing: '-0.01em'
                          }}
                        >
                          {app.name}
                        </h3>
                      </div>

                      <div className="flex items-center gap-2">
                        {/* URL */}
                        {appUrl && (
                          <span 
                            style={{ 
                              fontSize: '13px',
                              color: '#64748b',
                              fontWeight: '400',
                              fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace'
                            }}
                          >
                            {appUrl.replace('http://', '')}
                          </span>
                        )}
                        
                        {/* External Link */}
                        {appUrl && (
                          <a
                            href={appUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                            onClick={(e) => e.stopPropagation()}
                            className="opacity-0 group-hover:opacity-100 transition-all duration-300"
                            style={{
                              padding: '4px',
                              borderRadius: '6px',
                              color: '#64748b'
                            }}
                            onMouseEnter={(e) => {
                              (e.currentTarget as HTMLElement).style.color = '#22c55e';
                            }}
                            onMouseLeave={(e) => {
                              (e.currentTarget as HTMLElement).style.color = '#64748b';
                            }}
                          >
                            <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                            </svg>
                          </a>
                        )}
                        
                        {/* Arrow */}
                        <div 
                          className="opacity-0 group-hover:opacity-100 transition-all duration-300"
                          style={{ 
                            color: '#64748b',
                            transform: 'translateX(2px)'
                          }}
                        >
                          <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                          </svg>
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          ) : !error ? (
            <div className="text-center py-12">
              <div 
                className="w-16 h-16 mx-auto mb-6 rounded-full flex items-center justify-center"
                style={{ 
                  backgroundColor: 'rgba(0, 0, 0, 0.04)',
                  border: '1px solid rgba(0, 0, 0, 0.06)'
                }}
              >
                <svg 
                  className="w-6 h-6" 
                  style={{ color: '#a3a3a3' }} 
                  fill="none" 
                  stroke="currentColor" 
                  viewBox="0 0 24 24"
                >
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                </svg>
              </div>
              
              <h3 
                style={{ 
                  fontSize: '18px',
                  fontWeight: '600',
                  color: '#0f172a',
                  marginBottom: '6px',
                  letterSpacing: '-0.01em'
                }}
              >
                no apps yet
              </h3>
              
              <p 
                style={{ 
                  fontSize: '14px',
                  color: '#64748b',
                  marginBottom: '24px',
                  fontWeight: '400'
                }}
              >
                create your first app to get started
              </p>
              
              <button
                onClick={handleCreateApp}
                className="inline-flex items-center gap-2 px-5 py-2 rounded-full transition-all duration-300"
                style={{
                  backgroundColor: '#000000',
                  color: '#ffffff',
                  border: 'none',
                  fontSize: '13px',
                  fontWeight: '500',
                  cursor: 'pointer'
                }}
                onMouseEnter={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = '#1a1a1a';
                  (e.target as HTMLElement).style.transform = 'translateY(-1px)';
                }}
                onMouseLeave={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = '#000000';
                  (e.target as HTMLElement).style.transform = 'translateY(0)';
                }}
              >
                <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
                </svg>
                create your first app
              </button>
            </div>
          ) : null}
        </div>

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