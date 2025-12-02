# Frontend Testing Guide

This guide explains how to run and write tests for the frontend React application.

## Setup

### Install Dependencies

First, install the testing dependencies:

```bash
cd edge/orchestrator/internal/web/frontend
npm install
```

This will install:
- **Vitest**: Fast unit test framework (Vite-native)
- **React Testing Library**: React component testing utilities
- **@testing-library/jest-dom**: Custom Jest matchers for DOM assertions
- **@testing-library/user-event**: User interaction simulation
- **jsdom**: DOM environment for testing

## Running Tests

### Run all tests

```bash
npm test
```

### Run tests in watch mode

```bash
npm test -- --watch
```

### Run tests with UI

```bash
npm run test:ui
```

This opens an interactive UI in your browser for running and debugging tests.

### Run tests with coverage

```bash
npm run test:coverage
```

## Test Structure

### Test Files

- **`src/pages/Screenshots.test.tsx`**: Tests for the Screenshots page component
- **`src/test/setup.ts`**: Test setup and configuration

### Writing Tests

Tests use Vitest and React Testing Library. Example:

```typescript
import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import MyComponent from './MyComponent'

describe('MyComponent', () => {
  it('should render correctly', () => {
    render(<MyComponent />)
    expect(screen.getByText('Hello')).toBeInTheDocument()
  })

  it('should handle user interactions', async () => {
    const user = userEvent.setup()
    render(<MyComponent />)
    
    const button = screen.getByRole('button', { name: /click me/i })
    await user.click(button)
    
    await waitFor(() => {
      expect(screen.getByText('Clicked!')).toBeInTheDocument()
    })
  })
})
```

## Mocking

### API Mocking

The tests mock the API module to avoid making real HTTP requests:

```typescript
vi.mock('../utils/api', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}))
```

### Global Mocks

- **`fetch`**: Mocked for snapshot capture and export functionality
- **`FileReader`**: Mocked for image processing
- **`URL.createObjectURL`**: Mocked for file downloads

## Test Coverage

Current test coverage includes:

- ✅ API calls (`fetchCameras`, `fetchScreenshots`)
- ✅ Screenshot capture workflow
- ✅ Screenshot save with validation
- ✅ Screenshot deletion with confirmation
- ✅ Screenshot label updates
- ✅ Dataset export
- ✅ Filter and sort functionality
- ✅ Modal state management
- ✅ Dataset progress calculation
- ✅ Error handling
- ✅ Loading states

## Best Practices

1. **Use `userEvent` over `fireEvent`**: `userEvent` simulates real user interactions better
2. **Wait for async operations**: Use `waitFor` for async state updates
3. **Query by role**: Prefer `getByRole` over `getByText` for better accessibility
4. **Mock external dependencies**: Always mock API calls and browser APIs
5. **Test user flows**: Focus on testing user interactions, not implementation details

## Troubleshooting

### Tests fail with "Cannot find module"

Make sure all dependencies are installed:
```bash
npm install
```

### Tests fail with "ReferenceError: window is not defined"

This usually means jsdom is not configured. Check `vitest.config.ts` has `environment: 'jsdom'`.

### Tests are slow

- Use `vi.useFakeTimers()` for time-dependent tests
- Mock heavy operations
- Use `waitFor` with appropriate timeouts

## Continuous Integration

Tests can be run in CI/CD pipelines:

```bash
npm test -- --run
```

This runs tests once without watch mode, suitable for CI environments.
