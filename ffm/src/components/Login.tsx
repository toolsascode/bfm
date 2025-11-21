import { useState, FormEvent } from 'react';
import { authService } from '../services/auth';
import { toastService } from '../services/toast';

interface LoginProps {
  onLogin: () => void;
}

export default function Login({ onLogin }: LoginProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const success = await authService.login({ username, password });
      if (success) {
        toastService.success('Login successful!');
        setTimeout(() => onLogin(), 300); // Small delay for toast visibility
      } else {
        const errorMsg = 'Invalid username or password';
        setError(errorMsg);
        toastService.error(errorMsg);
      }
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : 'Login failed. Please try again.';
      setError(errorMsg);
      toastService.error(errorMsg);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex justify-center items-center min-h-screen bg-gradient-to-br from-bfm-teal via-bfm-green via-bfm-blue to-bfm-blue-dark relative overflow-hidden p-4">
      <div className="absolute inset-0 pointer-events-none">
        <div className="absolute top-[30%] left-[20%] w-96 h-96 bg-bfm-teal/30 rounded-full blur-3xl animate-pulse-slow" />
        <div className="absolute bottom-[30%] right-[20%] w-96 h-96 bg-bfm-blue/30 rounded-full blur-3xl animate-pulse-slow" style={{ animationDelay: '1s' }} />
      </div>
      <div className="bg-white p-6 md:p-8 rounded-xl shadow-2xl w-full max-w-md relative z-10 animate-scale-in">
        <div className="flex justify-center mb-6">
          <img src="/favicon.png" alt="BfM Logo" className="w-20 h-20 drop-shadow-lg" />
        </div>
        <h1 className="bg-gradient-to-br from-bfm-teal via-bfm-green to-bfm-blue bg-clip-text text-transparent mb-2 text-2xl font-semibold text-center">
          BfM - Backend For Migrations
        </h1>
        <h2 className="text-gray-600 mb-6 text-xl font-normal text-center">Login</h2>
        <form onSubmit={handleSubmit}>
          <div className="mb-4">
            <label htmlFor="username" className="block mb-2 text-gray-800 font-medium">
              Username
            </label>
            <input
              id="username"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
              autoFocus
              className="w-full px-3 py-3 border border-gray-300 rounded text-base focus:outline-none focus:border-bfm-teal focus:ring-2 focus:ring-bfm-teal/20"
            />
          </div>
          <div className="mb-4">
            <label htmlFor="password" className="block mb-2 text-gray-800 font-medium">
              Password
            </label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              className="w-full px-3 py-3 border border-gray-300 rounded text-base focus:outline-none focus:border-bfm-teal focus:ring-2 focus:ring-bfm-teal/20"
            />
          </div>
          {error && (
            <div className="text-red-600 mb-4 p-2 bg-red-50 rounded text-sm">{error}</div>
          )}
          <button
            type="submit"
            disabled={loading}
            className="w-full py-3 bg-gradient-to-br from-bfm-teal to-bfm-green text-white border-none rounded-md text-base font-medium cursor-pointer transition-all shadow-lg shadow-bfm-teal/30 hover:from-bfm-teal-dark hover:to-bfm-green-dark hover:-translate-y-0.5 hover:shadow-xl hover:shadow-bfm-teal/40 disabled:opacity-60 disabled:cursor-not-allowed disabled:hover:translate-y-0"
          >
            {loading ? 'Logging in...' : 'Login'}
          </button>
        </form>
      </div>
    </div>
  );
}

