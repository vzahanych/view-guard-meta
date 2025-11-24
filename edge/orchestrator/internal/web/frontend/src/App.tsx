import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import ErrorBoundary from './components/ErrorBoundary'
import Dashboard from './pages/Dashboard'
import Cameras from './pages/Cameras'
import CameraManagement from './pages/CameraManagement'
import Events from './pages/Events'
import Configuration from './pages/Configuration'
import Screenshots from './pages/Screenshots'

function App() {
  return (
    <ErrorBoundary>
      <BrowserRouter>
        <Layout>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/cameras" element={<Cameras />} />
            <Route path="/cameras/manage" element={<CameraManagement />} />
            <Route path="/events" element={<Events />} />
            <Route path="/screenshots" element={<Screenshots />} />
            <Route path="/configuration" element={<Configuration />} />
          </Routes>
        </Layout>
      </BrowserRouter>
    </ErrorBoundary>
  )
}

export default App

