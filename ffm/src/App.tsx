import { Routes, Route, Navigate } from "react-router-dom";
import { useState, useEffect } from "react";
import Login from "./components/Login";
import Layout from "./components/Layout";
import MigrationList from "./components/MigrationList";
import MigrationDetail from "./components/MigrationDetail";
import Dashboard from "./components/Dashboard";
import ToastContainer from "./components/ToastContainer";
import { authService } from "./services/auth";

function App() {
  // Initialize from auth service (which checks localStorage)
  const [isAuthenticated, setIsAuthenticated] = useState(() =>
    authService.getAuthenticated(),
  );

  useEffect(() => {
    // Re-check authentication on mount/reload
    const checkAuth = () => {
      setIsAuthenticated(authService.getAuthenticated());
    };
    checkAuth();

    // Also check on storage events (in case of multiple tabs)
    const handleStorageChange = (e: StorageEvent) => {
      if (e.key === "auth_authenticated") {
        checkAuth();
      }
    };
    window.addEventListener("storage", handleStorageChange);
    return () => window.removeEventListener("storage", handleStorageChange);
  }, []);

  const handleLogin = () => {
    setIsAuthenticated(true);
  };

  const handleLogout = () => {
    authService.logout();
    setIsAuthenticated(false);
    // When auth is enabled, the component will automatically re-render
    // and show the Login page due to the conditional rendering
  };

  if (!authService.isAuthEnabled()) {
    // Auth disabled, allow access
    return (
      <>
        <ToastContainer />
        <Routes>
          <Route path="/" element={<Layout onLogout={handleLogout} />}>
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="dashboard" element={<Dashboard />} />
            <Route path="migrations" element={<MigrationList />} />
            <Route path="migrations/:id" element={<MigrationDetail />} />
          </Route>
        </Routes>
      </>
    );
  }

  if (!isAuthenticated) {
    return (
      <>
        <ToastContainer />
        <Login onLogin={handleLogin} />
      </>
    );
  }

  return (
    <>
      <ToastContainer />
      <Routes>
        <Route path="/" element={<Layout onLogout={handleLogout} />}>
          <Route index element={<Navigate to="/dashboard" replace />} />
          <Route path="dashboard" element={<Dashboard />} />
          <Route path="migrations" element={<MigrationList />} />
          <Route path="migrations/:id" element={<MigrationDetail />} />
        </Route>
      </Routes>
    </>
  );
}

export default App;
