import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'
import {QueryClientProvider} from '@tanstack/react-query'
import {createQueryClient} from './workspace-cache'

const container = document.getElementById('root')

const root = createRoot(container!)
const queryClient = createQueryClient()

root.render(
    <React.StrictMode>
        <QueryClientProvider client={queryClient}><App/></QueryClientProvider>
    </React.StrictMode>
)
