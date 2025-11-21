import { useState } from 'react';
import { Outlet, Link, useLocation } from 'react-router-dom';

interface LayoutProps {
  onLogout: () => void;
}

export default function Layout({ onLogout }: LayoutProps) {
  const location = useLocation();
  const [sidebarOpen, setSidebarOpen] = useState(false);

  const isActive = (path: string) => location.pathname === path;

  return (
    <div className="flex min-h-screen bg-gray-50">
      {/* Mobile menu button */}
      <button
        onClick={() => setSidebarOpen(!sidebarOpen)}
        className="md:hidden fixed top-4 left-4 z-50 p-2 bg-bfm-sidebar-bg text-white rounded-lg shadow-lg"
        aria-label="Toggle menu"
      >
        <svg
          className="w-6 h-6"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          {sidebarOpen ? (
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M6 18L18 6M6 6l12 12"
            />
          ) : (
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M4 6h16M4 12h16M4 18h16"
            />
          )}
        </svg>
      </button>

      {/* Sidebar overlay for mobile */}
      {sidebarOpen && (
        <div
          className="md:hidden fixed inset-0 bg-black/50 z-40"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      <nav
        className={`w-64 bg-gradient-to-b from-bfm-sidebar-bg to-[#1a2f4a] text-white flex flex-col fixed h-screen overflow-y-auto shadow-lg transition-all duration-300 ease-in-out z-40 ${
          sidebarOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0'
        }`}
      >
        <div className="p-6 border-b border-white/10 flex flex-col items-center text-center">
          <img src="/favicon.png" alt="BfM Logo" className="w-16 h-16 mb-4 drop-shadow-md" />
          <h1 className="text-2xl mb-1 bg-gradient-to-br from-bfm-teal via-bfm-green to-bfm-blue bg-clip-text text-transparent">
            BfM
          </h1>
          <p className="text-sm text-white/70">Backend For Migrations</p>
        </div>
        <ul className="list-none py-4 flex-1">
          <li className="my-1">
            <Link
              to="/dashboard"
              className={`block px-6 py-3 text-white/80 no-underline transition-all duration-200 ${
                isActive('/dashboard')
                  ? 'bg-gradient-to-r from-bfm-teal/20 to-bfm-blue/10 text-white border-l-4 border-bfm-active-accent'
                  : 'hover:bg-bfm-sidebar-hover hover:text-white hover:translate-x-1'
              }`}
              onClick={() => setSidebarOpen(false)}
            >
              Dashboard
            </Link>
          </li>
          <li className="my-1">
            <Link
              to="/migrations"
              className={`block px-6 py-3 text-white/80 no-underline transition-all duration-200 ${
                isActive('/migrations')
                  ? 'bg-gradient-to-r from-bfm-teal/20 to-bfm-blue/10 text-white border-l-4 border-bfm-active-accent'
                  : 'hover:bg-bfm-sidebar-hover hover:text-white hover:translate-x-1'
              }`}
              onClick={() => setSidebarOpen(false)}
            >
              Migrations
            </Link>
          </li>
        </ul>
        <div className="p-4 pt-4 border-t border-white/10">
          <button
            onClick={onLogout}
            className="w-full py-2 bg-gradient-to-br from-bfm-blue to-bfm-blue-dark text-white border-none rounded text-sm transition-all duration-200 font-medium hover:from-bfm-blue-dark hover:to-bfm-dark-blue hover:-translate-y-0.5 hover:shadow-lg hover:shadow-bfm-blue/30 active:scale-95"
          >
            Logout
          </button>
        </div>
      </nav>
      <main className="flex-1 md:ml-64 p-4 md:p-8 min-h-screen">
        <Outlet />
      </main>
    </div>
  );
}

