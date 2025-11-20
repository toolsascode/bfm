import { Outlet, Link, useLocation } from 'react-router-dom';
import './Layout.css';

interface LayoutProps {
  onLogout: () => void;
}

export default function Layout({ onLogout }: LayoutProps) {
  const location = useLocation();

  const isActive = (path: string) => location.pathname === path;

  return (
    <div className="layout">
      <nav className="sidebar">
        <div className="sidebar-header">
          <h1>FFM</h1>
          <p>Frontend For Migrations</p>
        </div>
        <ul className="nav-menu">
          <li>
            <Link to="/dashboard" className={isActive('/dashboard') ? 'active' : ''}>
              Dashboard
            </Link>
          </li>
          <li>
            <Link to="/migrations" className={isActive('/migrations') ? 'active' : ''}>
              Migrations
            </Link>
          </li>
          <li>
            <Link to="/execute" className={isActive('/execute') ? 'active' : ''}>
              Execute Migration
            </Link>
          </li>
        </ul>
        <div className="sidebar-footer">
          <button onClick={onLogout} className="logout-button">
            Logout
          </button>
        </div>
      </nav>
      <main className="main-content">
        <Outlet />
      </main>
    </div>
  );
}

