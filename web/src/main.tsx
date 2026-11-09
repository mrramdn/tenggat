import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { ToastProvider } from './components/Toasts'
import { ApiError } from './lib/api'
import './styles/index.css'
import './styles/registri.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      // Retry network and server errors once, never 4xx answers.
      retry: (n, err) => n < 1 && !(err instanceof ApiError && err.status >= 400 && err.status < 500),
    },
    mutations: { retry: false },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <App />
      </ToastProvider>
    </QueryClientProvider>
  </StrictMode>,
)

// Dev only: ?measure=1 prints the control-height audit (see src/dev/measure.ts).
if (import.meta.env.DEV && new URLSearchParams(location.search).has('measure')) {
  void Promise.all([import('./dev/measure'), document.fonts.ready]).then(([m]) => setTimeout(m.ukur, 1500))
}
