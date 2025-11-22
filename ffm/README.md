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
- **Tailwind CSS** for styling and responsive design
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

Start the development server with hot-reload (HMR):

```bash
npm run dev
```

The application will be available at `http://localhost:4040`

**Hot Module Replacement (HMR)** is enabled by default. Changes to React components, CSS files, and TypeScript files will automatically reload in the browser without losing application state.

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

## Styling with Tailwind CSS

This project uses **Tailwind CSS** for all styling. All components use Tailwind utility classes instead of custom CSS files.

### Custom BfM Colors

The following custom colors are available as Tailwind utilities:

- `bfm-teal` / `bfm-teal-dark` - Teal accent colors
- `bfm-green` / `bfm-green-dark` - Green accent colors
- `bfm-blue` / `bfm-blue-dark` / `bfm-dark-blue` - Blue accent colors
- `bfm-sidebar-bg` - Sidebar background color
- `bfm-sidebar-hover` - Sidebar hover state
- `bfm-active-accent` - Active navigation accent

**Usage examples:**

```tsx
<div className="bg-bfm-teal text-white">Teal background</div>
<button className="hover:bg-bfm-blue-dark">Blue button</button>
```

### Responsive Design

All components are fully responsive with mobile-first design:

- Mobile: Single column layouts, collapsible sidebar
- Tablet: 2-column grids where appropriate
- Desktop: Full multi-column layouts

Breakpoints: `sm:` (640px), `md:` (768px), `lg:` (1024px), `xl:` (1280px)

## Project Structure

```
ffm/
├── src/
│   ├── components/       # React components (all use Tailwind CSS)
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
│   └── index.css         # Tailwind directives and base styles
├── index.html
├── package.json
├── tailwind.config.js    # Tailwind configuration
├── postcss.config.js     # PostCSS configuration
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

See LICENSE file in the root of the bfm project.
