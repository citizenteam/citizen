import { createContext } from 'preact';
import { useContext, useState, useEffect, useCallback } from 'preact/hooks';
import axios from 'axios';
import { 
  cookieDebugLog, 
  sessionDebugLog, 
  authDebugLog, 
  requestDebugLog,
  errorLog,
  auditLog,
  securityLog,
  debugLog
} from '../utils/debug';

// Helper to get cookie by name
const getCookie = (name: string): string | null => {
  cookieDebugLog('getCookie called for:', name);
  if (typeof document === 'undefined') {
    cookieDebugLog('getCookie: document is undefined');
    return null;
  }
  const value = `; ${document.cookie}`;
  cookieDebugLog('getCookie: document.cookie =', document.cookie);
  const parts = value.split(`; ${name}=`);
  cookieDebugLog('getCookie: parts =', parts);
  if (parts.length === 2) {
    const cookie = parts.pop()?.split(';').shift();
    const result = cookie ? decodeURIComponent(cookie) : null;
    cookieDebugLog('getCookie: result =', result);
    return result;
  }
  cookieDebugLog('getCookie: cookie not found');
  return null;
};

// Helper to set cookie with proper settings for development
const setCookie = (name: string, value: string, options: { path?: string; secure?: boolean; sameSite?: string } = {}) => {
  cookieDebugLog('setCookie called with:', { name, value, options });
  
  if (typeof document === 'undefined') {
    cookieDebugLog('setCookie: document is undefined');
    return;
  }

  const isLocalhost = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
  const isSecure = window.location.protocol === 'https:';
  
  // For localhost development, don't use SameSite=Lax as it can cause issues
  let cookieString = `${name}=${encodeURIComponent(value)}`;
  
  if (options.path) {
    cookieString += `; path=${options.path}`;
  }
  
  if (options.secure && isSecure) {
    cookieString += '; secure';
  }
  
  // Only add SameSite for non-localhost environments
  if (!isLocalhost && options.sameSite) {
    cookieString += `; samesite=${options.sameSite}`;
  }
  
  cookieDebugLog('setCookie: Final cookie string:', cookieString);
  cookieDebugLog('setCookie: Before setting, document.cookie =', document.cookie);
  
  document.cookie = cookieString;
  
  cookieDebugLog('setCookie: After setting, document.cookie =', document.cookie);
  
  // Test if we can read it back immediately
  const testRead = getCookie(name);
  cookieDebugLog('setCookie: Test read result:', testRead);
  
  return testRead;
};

// Helper to get session from cookie and localStorage (development fallback)
const getSession = (name: string): string | null => {
  sessionDebugLog('getSession called for:', name);
  
  // First try cookie storage
  const cookieValue = getCookie(name);
  if (cookieValue) {
    sessionDebugLog('getSession: Found in cookie:', cookieValue);
    return cookieValue;
  }
  
  // For development: fallback to localStorage if running on localhost
  const isLocalhost = typeof window !== 'undefined' && 
    (window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1');
  
  if (isLocalhost) {
    const localStorageValue = localStorage.getItem(name);
    if (localStorageValue) {
      sessionDebugLog('getSession: Found in localStorage (development):', localStorageValue);
      return localStorageValue;
    }
  }
  
  sessionDebugLog('getSession: No session found');
  return null;
};

// Helper to set session in cookie and localStorage (development fallback)
const setSession = (name: string, value: string) => {
  sessionDebugLog('setSession called with:', { name, value });
  
  // Set in cookie first
  const cookieResult = setCookie(name, value, { path: '/', secure: true, sameSite: 'lax' });
  
  // For development: also set in localStorage if running on localhost
  const isLocalhost = typeof window !== 'undefined' && 
    (window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1');
  
  if (isLocalhost) {
    localStorage.setItem(name, value);
    sessionDebugLog('setSession: Also saved to localStorage (development)');
  }
  
  // Verify session was set properly
  const verification = getSession(name);
  sessionDebugLog('setSession: Verification result:', verification);
  
  return verification === value;
};

// Helper to clear session from cookie and localStorage
const clearSession = (name: string) => {
  sessionDebugLog('clearSession called for:', name);
  
  // Clear cookie
  setCookie(name, '', { path: '/' });
  
  // Additional cookie clearing with expiry for thorough cleanup
  const isSecure = window.location.protocol === 'https:';
  const secureFlag = isSecure ? '; secure' : '';
  document.cookie = `${name}=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT${secureFlag}`;
  
  // For development: also clear from localStorage if running on localhost
  const isLocalhost = typeof window !== 'undefined' && 
    (window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1');
  
  if (isLocalhost) {
    localStorage.removeItem(name);
    sessionDebugLog('clearSession: Also cleared from localStorage (development)');
  }
  
  sessionDebugLog('clearSession: Session cleared');
};

interface User {
  id: number;
  username: string;
}

interface AuthContextType {
  // Auth durumu
  isAuthenticated: boolean;
  isLoading: boolean;
  
  // User bilgisi
  user: User | null;
  
  // SSO session
  ssoSession: string | null;
  
  // Actions
  login: (session: string, user: User) => void;
  setSSOSession: (session: string | null) => void;
  logout: () => Promise<void>;
  checkAuth: () => Promise<boolean>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: preact.ComponentChildren }) {
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [user, setUser] = useState<User | null>(null);
  const [loginCalled, setLoginCalled] = useState<boolean>(false);
  const [authChecked, setAuthChecked] = useState<boolean>(false);
  const [ssoSession, _setSSOSession] = useState<string | null>(() => {
    authDebugLog('AuthContext: Initializing ssoSession state from cookie only...');
    
    // Initialize from secure cookie only (no URL parameters for security)
    const storedSession = getSession('sso_session');
    authDebugLog('AuthContext: Cookie session result:', storedSession);
    if (storedSession) {
        authDebugLog('AuthContext: Returning cookie session:', storedSession);
        return storedSession;
    }
    authDebugLog('AuthContext: No session found in cookie, returning null');
    return null;
  });

  // Setup axios defaults
  useEffect(() => {
    axios.defaults.withCredentials = true;
    axios.defaults.baseURL = import.meta.env.VITE_API_URL || '/api/v1';
  }, []);

  // Clear auth state
  const clearAuthState = useCallback(() => {
    authDebugLog('🔴 clearAuthState called');
    setIsAuthenticated(false);
    setUser(null);
    _setSSOSession(null);
    setAuthChecked(false);
    
    // Clear session from all storage methods
    clearSession('sso_session');
    auditLog('User session cleared');
  }, []);

  // Auth validation function - memoized to prevent unnecessary re-renders
  const checkAuth = useCallback(async (): Promise<boolean> => {
    authDebugLog('🔵 checkAuth called');
    try {
      requestDebugLog('🔵 Making request to /auth/token-validate...');
      const response = await axios.get('/auth/token-validate');
      authDebugLog('🔵 checkAuth response:', response.data);
      
      if (response.data && response.data.success) {
        const userData = response.data.data;
        authDebugLog('🔵 checkAuth successful, user data:', userData);
        setUser({ 
          id: userData.user_id, 
          username: userData.username || 'Unknown',
        });
        setIsAuthenticated(true);
        auditLog('User authentication validated successfully');
        return true;
      } else {
        authDebugLog('🔴 checkAuth failed - response not successful');
        securityLog('Authentication validation failed - invalid response');
        clearAuthState();
      }
    } catch (error) {
      authDebugLog('🔴 checkAuth failed with error:', error);
      securityLog('Authentication validation failed - request error:', error);
      // Clear invalid session
      clearAuthState();
    }
    return false;
  }, []);

  const login = useCallback((session: string, user: User) => {
    authDebugLog('AuthContext.login called with:', { session, user });
    
    // Set session using helper function
    const sessionSet = setSession('sso_session', session);
    
    if (sessionSet) {
      authDebugLog('AuthContext.login - Session set successfully');
    } else {
      errorLog('AuthContext.login - Session setting failed');
    }
    
    // Set all states at once to prevent multiple re-renders
    setLoginCalled(true);
    setAuthChecked(true); // Mark as checked to prevent useEffect from running
    _setSSOSession(session);
    setUser(user);
    setIsAuthenticated(true);
    setIsLoading(false);
    auditLog('User logged in successfully:', user.username);
    authDebugLog('AuthContext.login completed - isAuthenticated should be true');
  }, []);

  // Wrapper for setSSOSession
  const setSSOSession = useCallback((newSession: string | null) => {
    authDebugLog('setSSOSession called with:', newSession);
    setLoginCalled(false);
    setAuthChecked(false); // Reset auth check flag
    _setSSOSession(newSession);
    if (newSession) {
      // When SSO session is set, validate auth
      checkAuth().then((isValid) => {
        setAuthChecked(true);
        if (!isValid) {
          clearAuthState();
        }
      });
    } else {
      clearAuthState();
    }
  }, []); // Remove dependencies

  // Initial auth check when component mounts or ssoSession changes
  useEffect(() => {
    authDebugLog('AuthContext useEffect triggered - ssoSession:', ssoSession, 'loginCalled:', loginCalled, 'authChecked:', authChecked);
    
    // Skip auth check if login was called directly OR if we already checked
    if (loginCalled || authChecked) {
      authDebugLog('Skipping auth check - loginCalled:', loginCalled, 'authChecked:', authChecked);
      if (loginCalled) {
        setLoginCalled(false); // Reset the flag
      }
      return;
    }

    const performAuthCheck = async () => {
      authDebugLog('Performing auth check...');
      setIsLoading(true);
      
      if (ssoSession) {
        authDebugLog('SSO session exists, validating...');
        const isValid = await checkAuth();
        if (!isValid) {
          authDebugLog('SSO session invalid, clearing state');
          clearAuthState();
        }
      } else {
        // Double-check storage in case state is out of sync
        const storedSession = getSession('sso_session');
        if (storedSession) {
          authDebugLog('Found stored session, updating state:', storedSession);
          _setSSOSession(storedSession);
          const isValid = await checkAuth();
          if (!isValid) {
            authDebugLog('Stored session invalid, clearing state');
            clearAuthState();
          }
        } else {
          authDebugLog('No SSO session, setting unauthenticated');
          setIsAuthenticated(false);
          setUser(null);
        }
      }
      
      setAuthChecked(true); // Mark as checked
      setIsLoading(false);
    };

    performAuthCheck();
  }, [ssoSession]); // Only depend on ssoSession

  // Periodic auth check (every 5 minutes) - DISABLED for now to prevent refresh issues
  useEffect(() => {
    if (!isAuthenticated) {
      return;
    }

    // TEMPORARILY DISABLED - causing too frequent refreshes
    authDebugLog('⏰ Periodic auth check DISABLED to prevent refresh issues');
    return;

    // This code is temporarily commented out
    /*
    authDebugLog('⏰ Setting up periodic auth check (5 minutes)');
    const interval = setInterval(() => {
      authDebugLog('⏰ Periodic auth check triggered');
      if (ssoSession) {
        authDebugLog('⏰ SSO session exists, checking auth...');
        checkAuth().then((isValid) => {
          if (!isValid) {
            authDebugLog('🔴 Periodic auth check failed, logging out');
            securityLog('Periodic authentication check failed - session expired');
            clearAuthState();
          } else {
            authDebugLog('✅ Periodic auth check successful');
          }
        });
      } else {
        authDebugLog('⏰ No SSO session for periodic check');
      }
    }, 5 * 60 * 1000); // 5 minutes

    return () => {
      authDebugLog('⏰ Clearing periodic auth check interval');
      clearInterval(interval);
    };
    */
  }, [isAuthenticated]); // Only depend on isAuthenticated

  const logout = useCallback(async () => {
    try {
      await axios.post('/auth/logout');
      auditLog('User logout API call successful');
    } catch (error) {
      errorLog('Logout API error:', error);
    }
    
    clearAuthState();
    auditLog('User logged out');
    
    // Redirect to login page
    window.location.href = '/login';
  }, []);

  const contextValue: AuthContextType = {
    isAuthenticated,
    isLoading,
    user,
    ssoSession,
    setSSOSession,
    logout,
    checkAuth,
    login
  };

  return (
    <AuthContext.Provider value={contextValue}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}