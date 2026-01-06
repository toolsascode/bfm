import { useState, useEffect } from "react";
import { Outlet, Link, useLocation } from "react-router-dom";
import { authService } from "../services/auth";

interface LayoutProps {
  onLogout: () => void;
}

export default function Layout({ onLogout }: LayoutProps) {
  const location = useLocation();
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [authEnabled, setAuthEnabled] = useState(() =>
    authService.isAuthEnabled(),
  );

  // Re-check auth enabled status in case runtime config loads after component mount
  useEffect(() => {
    // Check immediately
    setAuthEnabled(authService.isAuthEnabled());

    // Also check after a short delay to catch late-loading runtime config
    const timeout = setTimeout(() => {
      setAuthEnabled(authService.isAuthEnabled());
    }, 100);

    return () => clearTimeout(timeout);
  }, []);

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
          <a
            href="https://github.com/toolsascode/bfm"
            target="_blank"
            rel="noopener noreferrer"
            className={`flex items-center ${
              sidebarCollapsed ? "justify-center px-3 py-3" : "px-6 py-3"
            } text-white/80 no-underline transition-all duration-200 hover:bg-bfm-sidebar-hover hover:text-white hover:translate-x-1 rounded`}
            title={sidebarCollapsed ? "GitHub Repository" : undefined}
          >
            <svg
              className="w-5 h-5 flex-shrink-0"
              fill="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                fillRule="evenodd"
                d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"
                clipRule="evenodd"
              />
            </svg>
            {!sidebarCollapsed && <span className="ml-3">GitHub</span>}
          </a>
        </div>
        {authEnabled && (
          <div
            className={`p-4 pt-4 border-t border-white/10 relative z-10 ${
              sidebarCollapsed ? "px-2" : ""
            }`}
          >
            <button
              type="button"
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                // Close mobile sidebar if open
                if (sidebarOpen) {
                  setSidebarOpen(false);
                }
                if (onLogout && typeof onLogout === "function") {
                  onLogout();
                }
              }}
              className={`w-full py-2 bg-gradient-to-br from-bfm-blue to-bfm-blue-dark text-white border-none rounded text-sm transition-all duration-200 font-medium hover:from-bfm-blue-dark hover:to-bfm-dark-blue hover:-translate-y-0.5 hover:shadow-lg hover:shadow-bfm-blue/30 active:scale-95 cursor-pointer relative z-10 ${
                sidebarCollapsed ? "px-2" : ""
              }`}
              title={sidebarCollapsed ? "Logout" : undefined}
              style={{ pointerEvents: "auto" }}
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
        )}
      </nav>
      <main
        className={`flex-1 p-4 md:p-8 min-h-screen transition-all duration-300 max-w-full overflow-x-hidden ${
          sidebarCollapsed ? "md:ml-20" : "md:ml-64"
        }`}
      >
        <Outlet />
      </main>
    </div>
  );
}
