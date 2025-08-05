import { useState } from 'preact/hooks';
import { useLocation } from 'wouter';
import MinimalLayout from '../components/layout/MinimalLayout';
import { useApi } from '../hooks/useApi';
import { errorLog } from '../utils/debug';

export default function CreateAppMinimal() {
  const [, setLocation] = useLocation();
  const [appName, setAppName] = useState('');
  const { data, error, loading, request } = useApi<{ message: string }>();

  const handleSubmit = async (e: Event) => {
    e.preventDefault();
    
    if (!appName.trim()) {
      return;
    }

    try {
      const response = await request({
        url: '/citizen/apps',
        method: 'POST',
        data: { app_name: appName.trim() }
      });

      if (response) {
        setLocation('/');
      }
    } catch (err) {
      errorLog('Failed to create app:', err);
    }
  };

  const handleCancel = () => {
    setLocation('/');
  };

  return (
    <MinimalLayout showSystemStatus={true} showLogout={true}>
      <div 
        className="min-h-screen flex flex-col items-center justify-center px-6"
        style={{
          background: 'linear-gradient(135deg, #fafafa 0%, #f8fafc 100%)',
          fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'
        }}
      >
        {/* Back Button - Top Left */}
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
          onMouseEnter={(e) => {
            (e.target as HTMLElement).style.backgroundColor = '#1a1a1a';
            (e.target as HTMLElement).style.transform = 'translateY(-1px)';
            (e.target as HTMLElement).style.boxShadow = '0 6px 16px rgba(0, 0, 0, 0.2)';
          }}
          onMouseLeave={(e) => {
            (e.target as HTMLElement).style.backgroundColor = '#000000';
            (e.target as HTMLElement).style.transform = 'translateY(0)';
            (e.target as HTMLElement).style.boxShadow = '0 4px 12px rgba(0, 0, 0, 0.15)';
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

        {/* Create App Form */}
        <div className="w-full max-w-sm">
          <div className="text-center mb-8">
            <h2 
              style={{ 
                fontSize: '18px',
                fontWeight: '600',
                color: '#0f172a',
                marginBottom: '6px',
                letterSpacing: '-0.01em'
              }}
            >
              create new app
            </h2>
            <p 
              style={{ 
                fontSize: '14px',
                color: '#64748b',
                fontWeight: '400'
              }}
            >
              choose a unique name for your application
            </p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            {/* App Name Input */}
            <div>
              <input
                type="text"
                placeholder="app name"
                value={appName}
                onChange={(e) => setAppName((e.target as HTMLInputElement).value)}
                className="w-full px-4 py-3 rounded-lg transition-all duration-300 outline-none"
                style={{
                  backgroundColor: 'rgba(255, 255, 255, 0.8)',
                  border: '1px solid rgba(0, 0, 0, 0.06)',
                  fontSize: '14px',
                  fontWeight: '400',
                  color: '#0f172a'
                }}
                onFocus={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = 'rgba(255, 255, 255, 1)';
                  (e.target as HTMLElement).style.borderColor = 'rgba(0, 0, 0, 0.12)';
                  (e.target as HTMLElement).style.boxShadow = '0 0 0 3px rgba(0, 0, 0, 0.04)';
                }}
                onBlur={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = 'rgba(255, 255, 255, 0.8)';
                  (e.target as HTMLElement).style.borderColor = 'rgba(0, 0, 0, 0.06)';
                  (e.target as HTMLElement).style.boxShadow = 'none';
                }}
              />
            </div>

            {/* Error Message */}
            {error && (
              <div 
                className="text-center py-2"
                style={{
                  color: '#ef4444',
                  fontSize: '13px',
                  fontWeight: '500'
                }}
              >
                failed to create app
              </div>
            )}

            {/* Action Buttons */}
            <div className="flex gap-3">
              <button
                type="button"
                onClick={handleCancel}
                className="flex-1 py-3 rounded-lg transition-all duration-300"
                style={{
                  backgroundColor: 'rgba(255, 255, 255, 0.8)',
                  border: '1px solid rgba(0, 0, 0, 0.06)',
                  color: '#64748b',
                  fontSize: '14px',
                  fontWeight: '500',
                  cursor: 'pointer'
                }}
                onMouseEnter={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = 'rgba(255, 255, 255, 1)';
                  (e.target as HTMLElement).style.borderColor = 'rgba(0, 0, 0, 0.12)';
                  (e.target as HTMLElement).style.color = '#475569';
                }}
                onMouseLeave={(e) => {
                  (e.target as HTMLElement).style.backgroundColor = 'rgba(255, 255, 255, 0.8)';
                  (e.target as HTMLElement).style.borderColor = 'rgba(0, 0, 0, 0.06)';
                  (e.target as HTMLElement).style.color = '#64748b';
                }}
              >
                cancel
              </button>

              <button
                type="submit"
                disabled={loading || !appName.trim()}
                className="flex-1 py-3 rounded-lg transition-all duration-300"
                style={{
                  backgroundColor: loading || !appName.trim() ? '#e5e7eb' : '#000000',
                  color: loading || !appName.trim() ? '#9ca3af' : '#ffffff',
                  border: 'none',
                  fontSize: '14px',
                  fontWeight: '500',
                  cursor: loading || !appName.trim() ? 'not-allowed' : 'pointer'
                }}
                onMouseEnter={(e) => {
                  if (!loading && appName.trim()) {
                    (e.target as HTMLElement).style.backgroundColor = '#1a1a1a';
                    (e.target as HTMLElement).style.transform = 'translateY(-1px)';
                  }
                }}
                onMouseLeave={(e) => {
                  if (!loading && appName.trim()) {
                    (e.target as HTMLElement).style.backgroundColor = '#000000';
                    (e.target as HTMLElement).style.transform = 'translateY(0)';
                  }
                }}
              >
                {loading ? 'creating...' : 'create app'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </MinimalLayout>
  );
}