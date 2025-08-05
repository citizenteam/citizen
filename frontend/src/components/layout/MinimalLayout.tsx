import { useLocation } from 'wouter';
import { useAuth } from '../../context/AuthContext';
import { errorLog } from '../../utils/debug';

interface MinimalLayoutProps {
  children: preact.ComponentChildren;
  showSystemStatus?: boolean;
  showLogout?: boolean;
}

export default function MinimalLayout({ 
  children, 
  showSystemStatus = true, 
  showLogout = true 
}: MinimalLayoutProps) {
  const [, setLocation] = useLocation();
  const { logout } = useAuth();

  const handleLogout = async () => {
    try {
      await logout();
    } catch (error) {
      errorLog('Logout failed:', error);
      setLocation('/login');
    }
  };

  return (
    <div 
      className="min-h-screen relative"
      style={{
        backgroundColor: '#fafafa',
        fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
        color: '#0f172a'
      }}
    >

      {/* Main Content */}
      <main className="pt-1">
        {children}
      </main>

      {/* System Status - Bottom Left */}
      {showSystemStatus && (
        <div 
          className="fixed bottom-6 left-6 text-xs flex items-center gap-2 px-3 py-2 rounded-full z-50"
          style={{ 
            color: '#64748b',
            backgroundColor: 'rgba(255, 255, 255, 0.6)',
            backdropFilter: 'blur(10px)',
            border: '1px solid rgba(0, 0, 0, 0.04)'
          }}
        >
          <div 
            className="w-1.5 h-1.5 rounded-full"
            style={{ 
              backgroundColor: '#22c55e',
              animation: 'pulse 2s infinite'
            }}
          ></div>
          <span style={{ fontSize: '11px', fontWeight: '500' }}>live</span>
        </div>
      )}

      {/* Profile & Logout Buttons - Bottom Right */}
      {showLogout && (
        <div className="fixed bottom-6 right-6 flex flex-col gap-2 z-50">
          {/* Profile Button */}
          <button
            onClick={() => setLocation('/profile')}
            className="text-xs px-4 py-2 rounded-full transition-all duration-300"
            style={{
              backgroundColor: 'rgba(255, 255, 255, 0.6)',
              border: '1px solid rgba(0, 0, 0, 0.06)',
              color: '#64748b',
              backdropFilter: 'blur(10px)',
              fontSize: '12px',
              fontWeight: '500',
              cursor: 'pointer'
            }}
            onMouseEnter={(e) => {
              (e.target as HTMLElement).style.backgroundColor = 'rgba(0, 0, 0, 0.04)';
              (e.target as HTMLElement).style.color = '#0f172a';
            }}
            onMouseLeave={(e) => {
              (e.target as HTMLElement).style.backgroundColor = 'rgba(255, 255, 255, 0.6)';
              (e.target as HTMLElement).style.color = '#64748b';
            }}
          >
            profile
          </button>

          {/* Logout Button */}
          <button
            onClick={handleLogout}
            className="text-xs px-4 py-2 rounded-full transition-all duration-300"
            style={{
              backgroundColor: 'rgba(255, 255, 255, 0.6)',
              border: '1px solid rgba(0, 0, 0, 0.06)',
              color: '#64748b',
              backdropFilter: 'blur(10px)',
              fontSize: '12px',
              fontWeight: '500',
              cursor: 'pointer'
            }}
            onMouseEnter={(e) => {
              (e.target as HTMLElement).style.backgroundColor = 'rgba(0, 0, 0, 0.04)';
              (e.target as HTMLElement).style.color = '#0f172a';
            }}
            onMouseLeave={(e) => {
              (e.target as HTMLElement).style.backgroundColor = 'rgba(255, 255, 255, 0.6)';
              (e.target as HTMLElement).style.color = '#64748b';
            }}
          >
            logout
          </button>
        </div>
      )}

      {/* Animations */}
      <style>{`
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.4; }
        }
      `}</style>
    </div>
  );
}