# View Guard Edge UI

React-based frontend for the Edge Appliance web interface.

## Technology Stack

- **React 18** with TypeScript
- **Vite** for build tooling
- **Tailwind CSS** for styling
- **React Router** for navigation
- **Lucide React** for icons
- **Axios** for API calls
- **Recharts** for charts (optional)

## Development

### Prerequisites

- Node.js 18+ and npm

### Setup

```bash
cd edge/orchestrator/internal/web/frontend
npm install
```

### Development Server

```bash
npm run dev
```

This will start the Vite dev server on `http://localhost:5173` with hot module replacement.

The dev server is configured to proxy API requests to `http://localhost:8080` (the Go backend server).

### Building for Production

```bash
npm run build
```

This will build the frontend and output the files to `../static/` directory, which is then embedded in the Go binary using `//go:embed`.

## Project Structure

```
frontend/
├── src/
│   ├── components/      # Reusable UI components
│   │   ├── Layout.tsx   # Main layout with sidebar
│   │   ├── Sidebar.tsx  # Navigation sidebar
│   │   ├── Header.tsx   # Top header with status
│   │   ├── Button.tsx   # Button component
│   │   ├── Input.tsx    # Input field component
│   │   ├── Select.tsx   # Select dropdown component
│   │   ├── Card.tsx     # Card container component
│   │   ├── Loading.tsx  # Loading spinner
│   │   └── ErrorBoundary.tsx # Error boundary
│   ├── pages/           # Page components
│   │   ├── Dashboard.tsx
│   │   ├── Cameras.tsx
│   │   ├── Events.tsx
│   │   └── Configuration.tsx
│   ├── styles/          # CSS styles
│   │   └── index.css    # Tailwind imports and custom styles
│   ├── utils/           # Utility functions
│   │   └── api.ts       # API client (axios wrapper)
│   ├── App.tsx          # Main app component
│   └── main.tsx         # Entry point
├── index.html           # HTML template
├── package.json         # Dependencies
├── tsconfig.json        # TypeScript config
├── vite.config.ts       # Vite config
├── tailwind.config.js   # Tailwind config
└── postcss.config.js   # PostCSS config
```

## Components

### Layout Components

- **Layout**: Main layout wrapper with sidebar and header
- **Sidebar**: Responsive navigation sidebar with mobile menu
- **Header**: Top header showing system status

### Form Components

- **Button**: Styled button with variants (primary, secondary, danger) and sizes
- **Input**: Text input with label and error handling
- **Select**: Dropdown select with label and error handling

### UI Components

- **Card**: Container component for content sections
- **Loading**: Loading spinner component
- **ErrorBoundary**: React error boundary for error handling

## Styling

The project uses Tailwind CSS with custom utility classes defined in `src/styles/index.css`:

- `.btn` - Base button styles
- `.btn-primary`, `.btn-secondary`, `.btn-danger` - Button variants
- `.input` - Input field styles
- `.card` - Card container styles

## API Integration

The `utils/api.ts` file provides a typed API client using Axios. All API calls are automatically prefixed with `/api` and errors are handled consistently.

Example usage:

```typescript
import { api } from '../utils/api'

const status = await api.get<SystemStatus>('/api/status')
```

## Building and Embedding

The frontend is built into the `static/` directory and embedded in the Go binary using Go's `embed` package. The build process:

1. Run `npm run build` in the frontend directory
2. Vite outputs to `../static/`
3. Go's `//go:embed static/*` in `server.go` includes the files in the binary

