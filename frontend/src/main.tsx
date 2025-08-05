import { render } from 'preact'
import './index.css'
import AppRouter from './router'
import { AuthProvider } from './context/AuthContext'
import { hmrDebugLog, debugLog } from './utils/debug'

// Vite HMR control - prevent auto refresh
if (import.meta.hot) {
  hmrDebugLog('HMR: Hot reload enabled, auto refresh disabled');
  
  // HMR update'lerini manuel olarak handle et
  import.meta.hot.on('vite:beforeUpdate', () => {
    hmrDebugLog('HMR: Component updating without page refresh');
  });
  
  // Prevent full reload - only do component updates
  import.meta.hot.on('vite:beforeFullReload', (payload) => {
    hmrDebugLog('HMR: Preventing full page reload', payload);
    // Full reload'u engelle, sadece hot update yap
    return false;
  });
  
  // Error durumunda da full reload yapma
  import.meta.hot.on('vite:error', (payload) => {
    hmrDebugLog('HMR: Error occurred but preventing full reload', payload);
  });
}

render(
  <AuthProvider>
    <AppRouter />
  </AuthProvider>, 
  document.getElementById('app')!
)
