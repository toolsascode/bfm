// Authentication service

// Get runtime config from window (injected at runtime)
const getRuntimeConfig = () => {
  if (typeof window !== "undefined") {
    return (window as any).__RUNTIME_CONFIG__ || {};
  }
  return {};
};

// Get auth config dynamically (checks runtime config each time)
const getAuthEnabled = (): boolean => {
  const runtimeConfig = getRuntimeConfig();
  return (
    runtimeConfig.BFM_AUTH_ENABLED === "true" ||
    import.meta.env.VITE_AUTH_ENABLED === "true"
  );
};

const getAuthUsername = (): string => {
  const runtimeConfig = getRuntimeConfig();
  return (
    runtimeConfig.BFM_AUTH_USERNAME ||
    import.meta.env.VITE_AUTH_USERNAME ||
    "admin"
  );
};

const getAuthPassword = (): string => {
  const runtimeConfig = getRuntimeConfig();
  return (
    runtimeConfig.BFM_AUTH_PASSWORD ||
    import.meta.env.VITE_AUTH_PASSWORD ||
    "admin123"
  );
};

export interface AuthCredentials {
  username: string;
  password: string;
}

export class AuthService {
  private isAuthenticated: boolean = false;

  constructor() {
    // Check if already authenticated (from localStorage)
    // Note: We check auth enabled dynamically in case runtime config loads later
    if (!getAuthEnabled()) {
      // Auth disabled, always authenticated
      this.isAuthenticated = true;
      localStorage.setItem("auth_authenticated", "true");
    } else {
      // Auth enabled, check localStorage
      const authStatus = localStorage.getItem("auth_authenticated");
      this.isAuthenticated = authStatus === "true";
    }
  }

  isAuthEnabled(): boolean {
    // Check dynamically each time (runtime config may load after module initialization)
    return getAuthEnabled();
  }

  async login(credentials: AuthCredentials): Promise<boolean> {
    if (!getAuthEnabled()) {
      this.isAuthenticated = true;
      localStorage.setItem("auth_authenticated", "true");
      return true;
    }

    if (
      credentials.username === getAuthUsername() &&
      credentials.password === getAuthPassword()
    ) {
      this.isAuthenticated = true;
      localStorage.setItem("auth_authenticated", "true");
      return true;
    }

    return false;
  }

  logout(): void {
    this.isAuthenticated = false;
    localStorage.removeItem("auth_authenticated");
  }

  getAuthenticated(): boolean {
    return this.isAuthenticated;
  }
}

export const authService = new AuthService();
