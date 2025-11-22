import { useState } from "react";
import { Outlet, Link, useLocation } from "react-router-dom";

interface LayoutProps {
  onLogout: () => void;
}

export default function Layout({ onLogout }: LayoutProps) {
  const location = useLocation();
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

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
        className={`${
          sidebarCollapsed ? "w-20" : "w-64"
        } bg-gradient-to-b from-bfm-sidebar-bg to-[#1a2f4a] text-white flex flex-col fixed h-screen overflow-y-auto shadow-lg transition-all duration-300 ease-in-out z-40 ${
          sidebarOpen ? "translate-x-0" : "-translate-x-full md:translate-x-0"
        }`}
      >
        <div
          className={`p-6 border-b border-white/10 flex flex-col items-center text-center relative ${
            sidebarCollapsed ? "p-4" : ""
          }`}
        >
          <button
            onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
            className={`hidden md:block p-1.5 text-white/70 hover:text-white hover:bg-white/10 rounded transition-all duration-200 ${
              sidebarCollapsed
                ? "absolute top-2 right-2"
                : "absolute top-4 right-4"
            }`}
            aria-label="Toggle sidebar"
          >
            <svg
              className="w-5 h-5"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              {sidebarCollapsed ? (
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M13 5l7 7-7 7M5 5l7 7-7 7"
                />
              ) : (
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M11 19l-7-7 7-7m8 14l-7-7 7-7"
                />
              )}
            </svg>
          </button>
          <img
            src="/favicon.png"
            alt="BfM Logo"
            className={`drop-shadow-md transition-all duration-300 ${
              sidebarCollapsed ? "w-10 h-10 mb-2 mt-8" : "w-16 h-16 mb-4"
            }`}
          />
          {!sidebarCollapsed && (
            <>
              <h1 className="text-2xl mb-1 bg-gradient-to-br from-bfm-teal via-bfm-green to-bfm-blue bg-clip-text text-transparent">
                BfM
              </h1>
              <p className="text-sm text-white/70">Backend For Migrations</p>
            </>
          )}
        </div>
        <ul className="list-none py-4 flex-1">
          <li className="my-1">
            <Link
              to="/dashboard"
              className={`flex items-center ${
                sidebarCollapsed ? "justify-center px-3 py-3" : "px-6 py-3"
              } text-white/80 no-underline transition-all duration-200 ${
                isActive("/dashboard")
                  ? sidebarCollapsed
                    ? "bg-gradient-to-r from-bfm-teal/30 to-bfm-blue/20 text-white"
                    : "bg-gradient-to-r from-bfm-teal/20 to-bfm-blue/10 text-white border-l-4 border-bfm-active-accent"
                  : "hover:bg-bfm-sidebar-hover hover:text-white hover:translate-x-1"
              }`}
              onClick={() => setSidebarOpen(false)}
              title={sidebarCollapsed ? "Dashboard" : undefined}
            >
              <svg
                className="w-5 h-5 flex-shrink-0"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"
                />
              </svg>
              {!sidebarCollapsed && <span className="ml-3">Dashboard</span>}
            </Link>
          </li>
          <li className="my-1">
            <Link
              to="/migrations"
              className={`flex items-center ${
                sidebarCollapsed ? "justify-center px-3 py-3" : "px-6 py-3"
              } text-white/80 no-underline transition-all duration-200 ${
                isActive("/migrations")
                  ? sidebarCollapsed
                    ? "bg-gradient-to-r from-bfm-teal/30 to-bfm-blue/20 text-white"
                    : "bg-gradient-to-r from-bfm-teal/20 to-bfm-blue/10 text-white border-l-4 border-bfm-active-accent"
                  : "hover:bg-bfm-sidebar-hover hover:text-white hover:translate-x-1"
              }`}
              onClick={() => setSidebarOpen(false)}
              title={sidebarCollapsed ? "Migrations" : undefined}
            >
              <svg
                className="w-5 h-5 flex-shrink-0"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
                />
              </svg>
              {!sidebarCollapsed && <span className="ml-3">Migrations</span>}
            </Link>
          </li>
        </ul>
        <div
          className={`p-4 pt-4 border-t border-white/10 ${
            sidebarCollapsed ? "px-2" : ""
          }`}
        >
          <button
            onClick={onLogout}
            className={`w-full py-2 bg-gradient-to-br from-bfm-blue to-bfm-blue-dark text-white border-none rounded text-sm transition-all duration-200 font-medium hover:from-bfm-blue-dark hover:to-bfm-dark-blue hover:-translate-y-0.5 hover:shadow-lg hover:shadow-bfm-blue/30 active:scale-95 ${
              sidebarCollapsed ? "px-2" : ""
            }`}
            title={sidebarCollapsed ? "Logout" : undefined}
          >
            {sidebarCollapsed ? (
              <svg
                className="w-5 h-5 mx-auto"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
                />
              </svg>
            ) : (
              "Logout"
            )}
          </button>
        </div>
      </nav>
      <main
        className={`flex-1 p-4 md:p-8 min-h-screen transition-all duration-300 ${
          sidebarCollapsed ? "md:ml-20" : "md:ml-64"
        }`}
      >
        <Outlet />
      </main>
    </div>
  );
}
