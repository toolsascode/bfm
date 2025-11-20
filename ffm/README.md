# FFM - Frontend For Migrations

A modern web frontend for managing and monitoring database migrations via the BFM (Backend For Migrations) API.

## Features

- 📊 **Dashboard**: Overview of migration statistics and metrics
- 📋 **Migration List**: Browse all available migrations with filtering
- 🔍 **Migration Details**: View detailed information about each migration
- ▶️ **Execute Migrations**: Run migrations manually with progress tracking
- 📈 **Real-time Stats**: Track migration progress and overall metrics
- 🔐 **Authentication**: Basic username/password authentication (configurable)

## Tech Stack

- **React 18** with TypeScript
- **Vite** for fast development and building
- **React Router** for navigation
- **Axios** for API communication
- **Recharts** for data visualization
- **date-fns** for date formatting

## Getting Started

### Prerequisites

- Node.js 18+ and npm/yarn/pnpm
- BFM server running and accessible

### Installation

```bash
cd ffm
npm install
```

### Configuration

Copy `.env.example` to `.env` and configure:

```bash
cp .env.example .env
```

Edit `.env`:

```env
# BFM API Configuration
VITE_BFM_API_URL=http://localhost:7070

# Authentication (set to disable auth)
VITE_AUTH_ENABLED=true
VITE_AUTH_USERNAME=admin
VITE_AUTH_PASSWORD=admin123
```

### Development

```bash
npm run dev
```

The application will be available at `http://localhost:3000`

### Building for Production

```bash
npm run build
```

The built files will be in the `dist` directory.

### Preview Production Build

```bash
npm run preview
```

## Authentication

Authentication can be enabled or disabled via environment variables:

- `VITE_AUTH_ENABLED=true` - Enable authentication
- `VITE_AUTH_ENABLED=false` - Disable authentication (default: no login required)

When enabled, use the credentials from `VITE_AUTH_USERNAME` and `VITE_AUTH_PASSWORD`.

## API Integration

The frontend communicates with the BFM server via HTTP API:

- `GET /api/v1/migrations` - List all migrations
- `GET /api/v1/migrations/:id` - Get migration details
- `GET /api/v1/migrations/:id/status` - Get migration status
- `POST /api/v1/migrate` - Execute migrations
- `POST /api/v1/migrations/:id/rollback` - Rollback a migration
- `GET /api/v1/health` - Health check

The BFM API token should be set via the BFM server's `BFM_API_TOKEN` environment variable. The frontend will use this token for authentication.

## Project Structure

```
ffm/
├── src/
│   ├── components/       # React components
│   │   ├── Dashboard.tsx
│   │   ├── Login.tsx
│   │   ├── Layout.tsx
│   │   ├── MigrationList.tsx
│   │   ├── MigrationDetail.tsx
│   │   └── MigrationExecute.tsx
│   ├── services/         # API and auth services
│   │   ├── api.ts
│   │   └── auth.ts
│   ├── types/            # TypeScript types
│   │   └── api.ts
│   ├── App.tsx           # Main app component
│   ├── main.tsx          # Entry point
│   └── index.css         # Global styles
├── index.html
├── package.json
├── tsconfig.json
└── vite.config.ts
```

## Features in Detail

### Dashboard

- Total migrations count
- Applied/Pending/Failed statistics
- Status distribution chart
- Migrations by backend chart
- Migrations by connection chart
- Recent migrations table
- Health status indicator

### Migration List

- Filter by backend, schema, connection, status
- Sortable columns
- View migration details
- Real-time status updates

### Migration Details

- Complete migration metadata
- Current status information
- Rollback functionality
- Error messages display

### Execute Migration

- Configure connection and schema
- Set migration target filters
- Dry run option
- Real-time progress tracking
- Detailed execution results

## License

See LICENSE file in the root of the mops project.

