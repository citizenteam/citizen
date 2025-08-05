import { useState, useEffect } from 'preact/hooks';
import { useLocation } from 'wouter';
import MinimalLayout from '../components/layout/MinimalLayout';
import { useAuth } from '../context/AuthContext';
import { useApi } from '../hooks/useApi';
import GitHubOAuth from '../components/GitHubOAuth';
import DockerConnection from '../components/DockerConnection';
import { errorLog } from '../utils/debug';

interface User {
  id: number;
  email: string;
  created_at: string;
  github_connected: boolean;
  github_username?: string;
}

export default function ProfilePage() {
  const [, setLocation] = useLocation();
  const { ssoSession } = useAuth();
  const { request } = useApi();
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!ssoSession) {
      setLocation('/login');
      return;
    }

    loadUserProfile();
  }, [ssoSession]);

  const loadUserProfile = async () => {
    try {
      const response = await request({ url: '/citizen/profile' });
      setUser(response);
    } catch (error) {
      errorLog('Failed to load user profile:', error);
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <MinimalLayout>
        <div className="flex items-center justify-center min-h-screen">
          <div className="text-center">
            <div 
              className="w-6 h-6 border-2 border-gray-300 border-t-gray-800 rounded-full animate-spin mx-auto mb-4"
            />
            <p className="text-gray-600">Loading profile...</p>
          </div>
        </div>
      </MinimalLayout>
    );
  }

  return (
    <MinimalLayout>
      <div className="max-w-4xl mx-auto p-6">
        {/* Header */}
        <div className="mb-8">
          <button
            onClick={() => setLocation('/')}
            className="inline-flex items-center gap-2 text-gray-600 hover:text-gray-900 transition-colors mb-4"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
            Back to Apps
          </button>
          
          <h1 className="text-3xl font-bold text-gray-900 mb-2">Profile</h1>
          <p className="text-gray-600">Manage your account settings and integrations</p>
        </div>

        {/* Profile Info */}
        <div 
          className="rounded-2xl p-6 mb-8"
          style={{
            backgroundColor: '#ffffff',
            border: '1px solid #e2e8f0',
            boxShadow: '0 1px 3px rgba(0, 0, 0, 0.1)'
          }}
        >
          <div className="flex items-center gap-4 mb-6">
            <div 
              className="w-16 h-16 rounded-full flex items-center justify-center"
              style={{
                backgroundColor: '#f1f5f9',
                border: '2px solid #e2e8f0'
              }}
            >
              <svg className="w-8 h-8 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
              </svg>
            </div>
            <div>
              <h3 className="text-xl font-semibold text-gray-900">
                {user?.github_username || 'User'}
              </h3>
              <p className="text-gray-600">{user?.email}</p>
              <p className="text-sm text-gray-500">
                Member since {user?.created_at ? new Date(user.created_at).toLocaleDateString() : ''}
              </p>
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <p className="text-sm text-gray-500 mb-1">Account ID</p>
              <p className="text-gray-900 font-medium">#{user?.id}</p>
            </div>
            <div>
              <p className="text-sm text-gray-500 mb-1">GitHub Status</p>
              <div className="flex items-center gap-2">
                <div 
                  className={`w-2 h-2 rounded-full ${user?.github_connected ? 'bg-green-500' : 'bg-gray-300'}`}
                />
                <span className={`text-sm font-medium ${user?.github_connected ? 'text-green-700' : 'text-gray-500'}`}>
                  {user?.github_connected ? 'Connected' : 'Not Connected'}
                </span>
              </div>
            </div>
          </div>
        </div>

        {/* GitHub Integration */}
        <div 
          className="rounded-2xl p-6 mb-8"
          style={{
            backgroundColor: '#ffffff',
            border: '1px solid #e2e8f0',
            boxShadow: '0 1px 3px rgba(0, 0, 0, 0.1)'
          }}
        >
          <h2 className="text-xl font-semibold text-gray-900 mb-6">GitHub Integration</h2>
          <GitHubOAuth />
        </div>

        {/* Docker Hub Integration */}
        <div 
          className="rounded-2xl p-6"
          style={{
            backgroundColor: '#ffffff',
            border: '1px solid #e2e8f0',
            boxShadow: '0 1px 3px rgba(0, 0, 0, 0.1)'
          }}
        >
          <DockerConnection />
        </div>
      </div>
    </MinimalLayout>
  );
} 