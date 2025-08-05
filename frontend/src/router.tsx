import { h } from 'preact';
import { Router, Route, Switch, Redirect } from 'wouter';
import { useAuth } from './context/AuthContext';

// Pages
import LoginPageMinimal from './pages/LoginPageMinimal';
import HomePage from './pages/Home';
import CreateAppMinimal from './pages/CreateAppMinimal';
import AppDetailsMinimal from './pages/AppDetailsMinimal';
import ProfilePage from './pages/ProfilePage';

// Auth check component
const PrivateRoute = ({ component: Component, ...props }: any) => {
  const { isAuthenticated, isLoading } = useAuth();
  
  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading...</p>
        </div>
      </div>
    );
  }
  
  if (!isAuthenticated) {
    return <Redirect to="/login" />;
  }
  
  return <Component {...props} />;
};

export default function AppRouter() {
  const { isAuthenticated, isLoading } = useAuth();

  return (
    <Router>
      <Switch>
        <Route path="/login">
          <LoginPageMinimal />
        </Route>
        <Route path="/profile">
          <PrivateRoute component={ProfilePage} />
        </Route>
        <Route path="/apps/new">
          <PrivateRoute component={CreateAppMinimal} />
        </Route>
        <Route path="/apps/:appName">
          {(params) => <PrivateRoute component={AppDetailsMinimal} appName={params.appName} />}
        </Route>
        <Route path="/apps">
          <PrivateRoute component={HomePage} />
        </Route>
        <Route path="/">
          <PrivateRoute component={HomePage} />
        </Route>
      </Switch>
    </Router>
  );
}