// Debug utility for frontend
// Provides environment-aware logging with different categories

/**
 * Environment detection
 */
export const isProductionEnvironment = (): boolean => {
  return import.meta.env.PROD || import.meta.env.MODE === 'production';
};

export const isDevelopmentEnvironment = (): boolean => {
  return import.meta.env.DEV || import.meta.env.MODE === 'development';
};

/**
 * Base logging function with timestamp and category
 */
const logWithCategory = (category: string, level: 'log' | 'warn' | 'error' | 'info', ...args: any[]) => {
  const timestamp = new Date().toLocaleTimeString();
  const prefix = `[${timestamp}] [${category}]`;
  
  switch (level) {
    case 'error':
      console.error(prefix, ...args);
      break;
    case 'warn':
      console.warn(prefix, ...args);
      break;
    case 'info':
      console.info(prefix, ...args);
      break;
    default:
      console.log(prefix, ...args);
  }
};

/**
 * General debug logging (development only)
 */
export const debugLog = (...args: any[]) => {
  if (isDevelopmentEnvironment()) {
    logWithCategory('DEBUG', 'log', ...args);
  }
};

/**
 * Always logged functions (both dev and prod)
 */
export const infoLog = (...args: any[]) => {
  logWithCategory('INFO', 'info', ...args);
};

export const errorLog = (...args: any[]) => {
  logWithCategory('ERROR', 'error', ...args);
};

export const warnLog = (...args: any[]) => {
  logWithCategory('WARN', 'warn', ...args);
};

/**
 * Category-specific debug functions (development only)
 */
export const authDebugLog = (...args: any[]) => {
  if (isDevelopmentEnvironment()) {
    logWithCategory('AUTH', 'log', ...args);
  }
};

export const cookieDebugLog = (...args: any[]) => {
  if (isDevelopmentEnvironment()) {
    logWithCategory('COOKIE', 'log', ...args);
  }
};

export const sessionDebugLog = (...args: any[]) => {
  if (isDevelopmentEnvironment()) {
    logWithCategory('SESSION', 'log', ...args);
  }
};

export const requestDebugLog = (...args: any[]) => {
  if (isDevelopmentEnvironment()) {
    logWithCategory('REQUEST', 'log', ...args);
  }
};

export const routerDebugLog = (...args: any[]) => {
  if (isDevelopmentEnvironment()) {
    logWithCategory('ROUTER', 'log', ...args);
  }
};

export const apiDebugLog = (...args: any[]) => {
  if (isDevelopmentEnvironment()) {
    logWithCategory('API', 'log', ...args);
  }
};

export const hmrDebugLog = (...args: any[]) => {
  if (isDevelopmentEnvironment()) {
    logWithCategory('HMR', 'log', ...args);
  }
};

export const componentDebugLog = (...args: any[]) => {
  if (isDevelopmentEnvironment()) {
    logWithCategory('COMPONENT', 'log', ...args);
  }
};

/**
 * Always logged security and audit functions
 */
export const securityLog = (...args: any[]) => {
  logWithCategory('SECURITY', 'warn', ...args);
};

export const auditLog = (...args: any[]) => {
  logWithCategory('AUDIT', 'info', ...args);
};

/**
 * Performance logging (development only)
 */
export const perfDebugLog = (...args: any[]) => {
  if (isDevelopmentEnvironment()) {
    logWithCategory('PERF', 'log', ...args);
  }
};

/**
 * Utility functions for common debug scenarios
 */
export const logEnvironmentInfo = () => {
  if (isDevelopmentEnvironment()) {
    debugLog('Environment Info:', {
      mode: import.meta.env.MODE,
      dev: import.meta.env.DEV,
      prod: import.meta.env.PROD,
      baseUrl: import.meta.env.BASE_URL,
      apiUrl: import.meta.env.VITE_API_URL
    });
  }
};

export const logUserAgent = () => {
  if (isDevelopmentEnvironment()) {
    debugLog('User Agent:', navigator.userAgent);
  }
};

export const logWindowLocation = () => {
  if (isDevelopmentEnvironment()) {
    debugLog('Window Location:', {
      href: window.location.href,
      hostname: window.location.hostname,
      pathname: window.location.pathname,
      search: window.location.search,
      hash: window.location.hash
    });
  }
}; 