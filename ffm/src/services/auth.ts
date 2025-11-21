// Authentication service

const AUTH_ENABLED = import.meta.env.VITE_AUTH_ENABLED === 'true';
const AUTH_USERNAME = import.meta.env.VITE_AUTH_USERNAME || 'admin';
const AUTH_PASSWORD = import.meta.env.VITE_AUTH_PASSWORD || 'admin123';

export interface AuthCredentials {
  username: string;
  password: string;
}

export class AuthService {
  private isAuthenticated: boolean = false;

  constructor() {
    // Check if already authenticated (from localStorage)
    if (!AUTH_ENABLED) {
      // Auth disabled, always authenticated
      this.isAuthenticated = true;
      localStorage.setItem('auth_authenticated', 'true');
    } else {
      // Auth enabled, check localStorage
      const authStatus = localStorage.getItem('auth_authenticated');
      this.isAuthenticated = authStatus === 'true';
    }
  }

  isAuthEnabled(): boolean {
    return AUTH_ENABLED;
  }

  async login(credentials: AuthCredentials): Promise<boolean> {
    if (!AUTH_ENABLED) {
      this.isAuthenticated = true;
      localStorage.setItem('auth_authenticated', 'true');
      return true;
    }

    if (
      credentials.username === AUTH_USERNAME &&
      credentials.password === AUTH_PASSWORD
    ) {
      this.isAuthenticated = true;
      localStorage.setItem('auth_authenticated', 'true');
      return true;
    }

    return false;
  }

  logout(): void {
    this.isAuthenticated = false;
    localStorage.removeItem('auth_authenticated');
  }

  getAuthenticated(): boolean {
    return this.isAuthenticated;
  }
}

export const authService = new AuthService();

