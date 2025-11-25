# Phase 4: SaaS UI Frontend

**Duration**: 2 weeks  
**Goal**: Build core React frontend - authentication, event timeline, basic clip viewing

**Scope**: Simplified UI for PoC - essential features only, no advanced configuration

**Milestone 2 Target**: End of this phase - first clip viewing (UI → SaaS → KVM VM → Edge → Stream)

**Note**: This phase is **deferred for PoC**. The PoC focuses on Edge Appliance ↔ User VM API communication, with Edge Web UI providing the user interface. SaaS UI will be implemented post-PoC.

---

### Epic 4.1: Frontend Project Setup

**Priority: P0**

**Note**: SaaS frontend is a private repository (git submodule in meta repo).

#### Step 4.1.1: React Project Structure
- **Substep 4.1.1.1**: Initialize React + TypeScript project
  - **Status**: ⬜ TODO
  - Create React app with Vite or Create React App
  - TypeScript configuration
  - Tailwind CSS setup
- **Substep 4.1.1.2**: Project structure
  - **Status**: ⬜ TODO
  - `src/components/` - React components
  - `src/pages/` - Page components
  - `src/hooks/` - Custom hooks
  - `src/services/` - API services
  - `src/store/` - State management (Zustand)
  - `src/utils/` - Utility functions
- **Substep 4.1.1.3**: Development tooling
  - **Status**: ⬜ TODO
  - ESLint configuration
  - Prettier configuration
  - Testing setup (Vitest/Jest)

#### Step 4.1.2: API Client Setup
- **Substep 4.1.2.1**: API client implementation
  - **Status**: ⬜ TODO
  - Axios or fetch wrapper
  - Request/response interceptors
  - Error handling
- **Substep 4.1.2.2**: API service layer
  - **Status**: ⬜ TODO
  - Event API service
  - User API service
  - Camera API service
  - Subscription API service
- **Substep 4.1.2.3**: Unit tests for frontend project setup
  - **Status**: ⬜ TODO
  - **P0**: Test API client implementation (request/response interceptors, error handling)
  - **P0**: Test API service layer (event, user, camera, subscription services)
  - **P1**: Test React component structure and utilities

### Epic 4.2: Authentication UI

**Priority: P0**

#### Step 4.2.1: Auth0 Integration
- **Substep 4.2.1.1**: Auth0 React SDK setup
  - **Status**: ⬜ TODO
  - Install and configure Auth0 React SDK
  - Auth0Provider setup
  - Configuration
- **Substep 4.2.1.2**: Authentication flows
  - **Status**: ⬜ TODO
  - Login page
  - Logout functionality
  - Protected route wrapper
  - Token management

#### Step 4.2.2: User Profile UI
- **Substep 4.2.2.1**: User profile page
  - **Status**: ⬜ TODO
  - Profile information display
  - Profile editing
  - User preferences
- **Substep 4.2.2.2**: User settings
  - **Status**: ⬜ TODO
  - Settings page
  - Notification preferences
  - Account management
- **Substep 4.2.2.3**: Unit tests for authentication UI
  - **Status**: ⬜ TODO
  - **P0**: Test Auth0 React SDK integration
  - **P0**: Test login/logout flows
  - **P0**: Test protected route wrapper
  - **P0**: Test token management
  - **P1**: Test user profile and settings components

### Epic 4.3: Dashboard & Navigation

**Priority: P0**

#### Step 4.3.1: Main Layout
- **Substep 4.3.1.1**: Layout component
  - **Status**: ⬜ TODO
  - Header with user info
  - Navigation sidebar
  - Main content area
  - Responsive design
- **Substep 4.3.1.2**: Navigation
  - **Status**: ⬜ TODO
  - Route configuration (React Router)
  - Navigation menu
  - Active route highlighting
  - Mobile navigation

#### Step 4.3.2: Dashboard Page (Basic)
- **Substep 4.3.2.1**: Basic dashboard
  - **Status**: ⬜ TODO
  - **P0**: Simple "Events" nav item
  - **P0**: Basic camera status label (e.g., "Cameras: 2 online")
  - **P1**: Dashboard widgets (camera overview, recent events, health indicators)
- **Substep 4.3.2.2**: Updates
  - **Status**: ⬜ TODO
  - **P0**: Basic polling refresh
  - **P1**: SSE connection for live updates
- **Substep 4.3.2.3**: Unit tests for dashboard and navigation
  - **Status**: ⬜ TODO
  - **P0**: Test layout component (header, sidebar, responsive)
  - **P0**: Test navigation and routing
  - **P0**: Test dashboard page (basic polling, camera status)
  - **P1**: Test SSE connection (if implemented)

### Epic 4.4: Event Timeline UI

**Priority: P0**

#### Step 4.4.1: Timeline Component
- **Substep 4.4.1.1**: Timeline layout
  - **Status**: ⬜ TODO
  - **P0**: Simple table/list of events
  - **P0**: Event card rendering
  - **P0**: Basic pagination
  - **P1**: Date grouping
  - **P1**: Infinite scroll
- **Substep 4.4.1.2**: Event cards
  - **Status**: ⬜ TODO
  - **P0**: Event metadata display
  - **P0**: Event type indicators
  - **P0**: Timestamp formatting
  - **P1**: Event thumbnail display

#### Step 4.4.2: Event Filtering & Search
- **Substep 4.4.2.1**: Filter UI
  - **Status**: ⬜ TODO
  - **P0**: Basic filters (camera, type, date range)
  - **P0**: Simple filter state management
  - **P1**: Advanced date range picker
- **Substep 4.4.2.2**: Search functionality
  - **Status**: ⬜ TODO
  - **P1**: Search input and basic search
  - **P1**: Search API integration
  - **P2**: Search history

#### Step 4.4.3: Event Details View
- **Substep 4.4.3.1**: Event detail modal/page
  - **Status**: ⬜ TODO
  - Event metadata display
  - Thumbnail/snapshot display
  - Detection details (bounding boxes if available)
  - Camera information
- **Substep 4.4.3.2**: Event actions
  - **Status**: ⬜ TODO
  - "View Clip" button
  - "Download" button
  - "Archive" button (if applicable)
  - Event deletion (if allowed)
- **Substep 4.4.3.3**: Unit tests for event timeline UI
  - **Status**: ⬜ TODO
  - **P0**: Test timeline component (event list, cards, pagination)
  - **P0**: Test event filtering (camera, type, date range)
  - **P0**: Test event details view and actions
  - **P1**: Test event search functionality
  - **P1**: Test date grouping and infinite scroll

### Epic 4.5: Clip Viewing UI

**Priority: P0**

#### Step 4.5.1: Video Player Component
- **Substep 4.5.1.1**: Video player integration
  - **Status**: ⬜ TODO
  - **P0**: Standard HTML5 `<video>` element with HTTP URL
  - **P0**: React video player component using HTTP progressive download
  - **P0**: Basic playback controls (play/pause, seek)
  - **P1**: WebRTC stream handling (if WebRTC implemented)
  - **P1**: Fullscreen support
- **Substep 4.5.1.2**: Stream request flow
  - **Status**: ⬜ TODO
  - **P0**: "View Clip" button requests HTTP URL from SaaS
  - **P0**: Loading states and error handling
  - **P1**: WebRTC connection management (if WebRTC implemented)

#### Step 4.5.2: Clip Player Features
- **Substep 4.5.2.1**: Playback features
  - **Status**: ⬜ TODO
  - Play/pause
  - Seek
  - Volume control
  - Playback speed
- **Substep 4.5.2.2**: Clip information
  - **Status**: ⬜ TODO
  - Clip metadata display
  - Camera information
  - Timestamp display
  - Download option
- **Substep 4.5.2.3**: Unit tests for clip viewing UI
  - **Status**: ⬜ TODO
  - **P0**: Test video player component (HTTP progressive download)
  - **P0**: Test stream request flow (loading states, error handling)
  - **P0**: Test playback features (play/pause, seek, volume)
  - **P1**: Test WebRTC stream handling (if implemented)

### Epic 4.6: Camera Management UI (Basic)

**Priority: P0** (Simplified for PoC)

#### Step 4.6.1: Basic Camera List
- **Substep 4.6.1.1**: Camera list page (simple)
  - **Status**: ⬜ TODO
  - **P0**: Display discovered cameras from Edge
  - Camera status indicators (online/offline)
  - Basic camera information
  - **P2**: Camera thumbnail/preview, advanced actions
- **Substep 4.6.1.2**: Basic camera configuration
  - **Status**: ⬜ TODO
  - **P0**: Camera naming and labeling
  - **P2**: Detection zones, schedules, advanced settings
- **Substep 4.6.1.3**: Unit tests for camera management UI
  - **Status**: ⬜ TODO
  - **P0**: Test camera list page (display, status indicators)
  - **P0**: Test camera configuration (naming, labeling)
  - **P2**: Test advanced camera settings (if implemented)

### Epic 4.7: Subscription & Billing UI

**Priority: P2** (Defer to post-PoC)

#### Step 4.7.1: Basic Plan Display (PoC)
- **Substep 4.7.1.1**: Simple plan indicator
  - **Status**: ⬜ TODO
  - **P0**: Display "Free Plan" or plan name (hard-coded for PoC)
  - **P2**: Full subscription management UI, plan comparison, upgrade/downgrade
- **Substep 4.7.1.2**: Billing UI
  - **Status**: ⬜ TODO
  - **P2**: Payment method management, billing history, Stripe integration

### Epic 4.8: Onboarding & ISO Download

**Priority: P1** (Can be simplified for PoC)

#### Step 4.8.1: Onboarding Flow
- **Substep 4.8.1.1**: Onboarding wizard
  - **Status**: ⬜ TODO
  - Welcome screen
  - Plan selection
  - ISO download instructions
  - Setup guide
- **Substep 4.8.1.2**: ISO download page
  - **Status**: ⬜ TODO
  - ISO download button
  - Download instructions
  - Installation guide
  - Troubleshooting tips
- **Substep 4.8.1.3**: Unit tests for onboarding and ISO download
  - **Status**: ⬜ TODO
  - **P1**: Test onboarding flow components
  - **P1**: Test ISO download page

---

## Success Criteria

### Phase 4 Success Criteria (SaaS UI)

**PoC Must-Have:**
- ✅ Users can log in via Auth0
- ✅ Event timeline displaying events in UI
- ✅ Basic filtering and search
- ✅ Users can view clips on-demand
- ✅ **Milestone 2**: First clip viewing flow working
- ✅ Basic camera list/status page

**Stretch Goals:**
- Subscription management UI
- Advanced camera configuration UI
- Rich dashboard with statistics

