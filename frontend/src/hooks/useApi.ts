import { useState, useCallback } from 'preact/hooks';
import axios, { type AxiosRequestConfig, type AxiosError } from 'axios';
import { useAuth } from '../context/AuthContext';

// General response format from API
interface ApiResponse<T> {
  success: boolean;
  message: string;
  data: T;
}

// Types of values that the hook will return
interface UseApiResult<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
  request: (config: AxiosRequestConfig) => Promise<T | null>;
  errorData?: any; // For additional data from backend in error situations
}

const api = axios.create({
      baseURL: '/api/v1', // Base URL for all requests
  withCredentials: true,
});

export function useApi<T = any>(): UseApiResult<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState<boolean>(false);
  const [errorData, setErrorData] = useState<any>(null);
  const { ssoSession } = useAuth();

      // We work with SSO session cookie, no need for headers
  api.interceptors.request.use(
    (config) => {
      // Cookie is sent automatically because withCredentials: true
      return config;
    },
    (error) => Promise.reject(error)
  );

  const request = useCallback(
    async (config: AxiosRequestConfig): Promise<T | null> => {
      setLoading(true);
      setError(null);
      setData(null);
      setErrorData(null);

      try {
        const response = await api.request<ApiResponse<T>>(config);
        
        if (response.data && response.data.success) {
          setData(response.data.data);
          return response.data.data;
        } else {
          setError(response.data.message || 'An unexpected error occurred.');
          // Save data even in error situations (for deploy output))
          if (response.data.data) {
            setErrorData(response.data.data);
          }
          return null;
        }
      } catch (err) {
        const axiosError = err as AxiosError<ApiResponse<any>>;
        
        // Extract message from error response
        const errorMessage =
          axiosError.response?.data?.message ||
          axiosError.message ||
          'An unknown error occurred.';
        setError(errorMessage);
        
        // Extract data from error response (for deploy output)
        if (axiosError.response?.data?.data) {
          setErrorData(axiosError.response.data.data);
        }
        
        return null;
      } finally {
        setLoading(false);
      }
    },
    [ssoSession] // Request function is recreated when SSO session changes
  );

  return { data, error, loading, request, errorData };
} 