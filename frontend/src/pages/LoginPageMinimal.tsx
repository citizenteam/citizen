import { useState, useEffect } from 'preact/hooks';
import { useLocation } from 'wouter';
import MinimalLayout from '../components/layout/MinimalLayout';
import { useAuth } from '../context/AuthContext';
import { useApi } from '../hooks/useApi';
import { authDebugLog, routerDebugLog, requestDebugLog, errorLog } from '../utils/debug';

interface LoginRequest {
  username: string;
  password: string;
}

interface LoginResponse {
  sso_session: string;
  redirect_url?: string;
  user?: {
    user_id: number;
    username: string;
  }
}

export default function LoginPageMinimal() {
  const { isAuthenticated, isLoading, login } = useAuth();
  const [, setLocation] = useLocation();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const { data, error, loading, request } = useApi<LoginResponse>();

  const getRedirectUrl = () => {
    const params = new URLSearchParams(window.location.search);
    return params.get('redirect') || '/';
  };

  // Only redirect if already authenticated and not in the middle of a login process
  useEffect(() => {
    if (isAuthenticated && !loading) {
      authDebugLog('User already authenticated, redirecting...');
      const redirectUrl = getRedirectUrl();
      if (redirectUrl.startsWith('http')) {
        window.location.href = redirectUrl;
      } else {
        setLocation(redirectUrl);
      }
    }
  }, [isAuthenticated, loading, setLocation]);

  // Show loading while auth is being checked
  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900 mx-auto mb-4"></div>
          <p className="text-gray-600">Checking authentication...</p>
        </div>
      </div>
    );
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault();
    
    authDebugLog('Login form submitted', { username, password: '***' });
    
    if (!username || !password) {
      authDebugLog('Username or password missing');
      return;
    }

    try {
      requestDebugLog('Making login request...');
      const response = await request({
        url: '/auth/login',
        method: 'POST',
        data: { username, password }
      });

      authDebugLog('Login response received:', response);

      if (response?.sso_session && response.user) {
        authDebugLog('Login successful, calling auth.login...');
        login(response.sso_session, {
          id: response.user.user_id,
          username: response.user.username,
        });
        
        // Wait a bit for state to update, then redirect
        setTimeout(() => {
          const redirectUrl = getRedirectUrl();
          routerDebugLog('Redirecting to:', redirectUrl);
          if (redirectUrl.startsWith('http')) {
            window.location.href = redirectUrl;
          } else {
            setLocation(redirectUrl);
          }
        }, 100);
      } else {
        authDebugLog('Login response missing required data:', response);
      }
    } catch (err) {
      errorLog('Login failed:', err);
    }
  };

  return (
    <MinimalLayout showSystemStatus={false} showLogout={false}>
      <div 
        className="min-h-screen flex flex-col items-center justify-center px-6"
        style={{
          background: 'linear-gradient(135deg, #fafafa 0%, #f8fafc 100%)',
          fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'
        }}
      >
        {/* Header */}
        <div className="text-center mb-12">
          <div 
            className="font-semibold text-xl mb-2 select-none"
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

        {/* Login Form */}
        <div className="w-full max-w-sm">
          <form onSubmit={handleSubmit} className="space-y-4">
            {/* Username */}
            <div>
              <input
                type="text"
                placeholder="username"
                value={username}
                onChange={(e) => setUsername((e.target as HTMLInputElement).value)}
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

            {/* Password */}
            <div>
              <input
                type="password"
                placeholder="password"
                value={password}
                onChange={(e) => setPassword((e.target as HTMLInputElement).value)}
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
                invalid credentials
              </div>
            )}

            {/* Submit Button */}
            <button
              type="submit"
              disabled={loading || !username || !password}
              className="w-full py-3 rounded-lg transition-all duration-300"
              style={{
                backgroundColor: loading || !username || !password ? '#e5e7eb' : '#000000',
                color: loading || !username || !password ? '#9ca3af' : '#ffffff',
                border: 'none',
                fontSize: '14px',
                fontWeight: '500',
                cursor: loading || !username || !password ? 'not-allowed' : 'pointer'
              }}
              onMouseEnter={(e) => {
                if (!loading && username && password) {
                  (e.target as HTMLElement).style.backgroundColor = '#1a1a1a';
                  (e.target as HTMLElement).style.transform = 'translateY(-1px)';
                }
              }}
              onMouseLeave={(e) => {
                if (!loading && username && password) {
                  (e.target as HTMLElement).style.backgroundColor = '#000000';
                  (e.target as HTMLElement).style.transform = 'translateY(0)';
                }
              }}
            >
              {loading ? 'signing in...' : 'sign in'}
            </button>
          </form>
        </div>
      </div>
    </MinimalLayout>
  );
}