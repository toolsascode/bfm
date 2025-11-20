import { Routes, Route, Navigate } from 'react-router-dom';
import { useState, useEffect } from 'react';
import Login from './components/Login';
import Layout from './components/Layout';
import MigrationList from './components/MigrationList';
import MigrationDetail from './components/MigrationDetail';
import MigrationExecute from './components/MigrationExecute';
import Dashboard from './components/Dashboard';
import { authService } from './services/auth';

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState(authService.getAuthenticated());

  useEffect(() => {
    setIsAuthenticated(authService.getAuthenticated());
  }, []);

  const handleLogin = () => {
    setIsAuthenticated(true);
  };

  const handleLogout = () => {
    authService.logout();
    setIsAuthenticated(false);
  };

  if (!authService.isAuthEnabled()) {
    // Auth disabled, allow access
    return (
      <Routes>
        <Route path="/" element={<Layout onLogout={handleLogout} />}>
          <Route index element={<Navigate to="/dashboard" replace />} />
          <Route path="dashboard" element={<Dashboard />} />
          <Route path="migrations" element={<MigrationList />} />
          <Route path="migrations/:id" element={<MigrationDetail />} />
          <Route path="execute" element={<MigrationExecute />} />
        </Route>
      </Routes>
    );
  }

  if (!isAuthenticated) {
    return <Login onLogin={handleLogin} />;
  }

  return (
    <Routes>
      <Route path="/" element={<Layout onLogout={handleLogout} />}>
        <Route index element={<Navigate to="/dashboard" replace />} />
        <Route path="dashboard" element={<Dashboard />} />
        <Route path="migrations" element={<MigrationList />} />
        <Route path="migrations/:id" element={<MigrationDetail />} />
        <Route path="execute" element={<MigrationExecute />} />
      </Route>
    </Routes>
  );
}

export default App;

