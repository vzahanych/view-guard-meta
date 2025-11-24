import axios from 'axios'

const apiClient = axios.create({
  baseURL: '/api',
  headers: {
    'Content-Type': 'application/json',
  },
})

// Response interceptor for error handling
apiClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response) {
      // Server responded with error status
      throw new Error(error.response.data?.error || error.response.statusText)
    } else if (error.request) {
      // Request made but no response
      throw new Error('Network error: No response from server')
    } else {
      // Something else happened
      throw new Error(error.message || 'An unexpected error occurred')
    }
  }
)

export const api = {
  get: <T = any>(url: string, config?: any): Promise<T> => {
    return apiClient.get<T>(url, config) as Promise<T>
  },
  post: <T = any>(url: string, data?: any, config?: any): Promise<T> => {
    return apiClient.post<T>(url, data, config) as Promise<T>
  },
  put: <T = any>(url: string, data?: any, config?: any): Promise<T> => {
    return apiClient.put<T>(url, data, config) as Promise<T>
  },
  delete: <T = any>(url: string, config?: any): Promise<T> => {
    return apiClient.delete<T>(url, config) as Promise<T>
  },
}

