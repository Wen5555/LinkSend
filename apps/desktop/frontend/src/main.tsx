import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'
import {QueryClientProvider} from '@tanstack/react-query'
import {createQueryClient} from './workspace-cache'
import {ClipboardCopyProgress} from './components/ClipboardCopyProgress'

const container = document.getElementById('root')

const root = createRoot(container!)
const queryClient = createQueryClient()
const clipboardProgress = new URLSearchParams(window.location.search).has('clipboard-progress')
if (clipboardProgress) document.body.classList.add('clipboard-progress-body')

root.render(
    <React.StrictMode>
        <QueryClientProvider client={queryClient}>{clipboardProgress ? <ClipboardCopyProgress/> : <App/>}</QueryClientProvider>
    </React.StrictMode>
)
