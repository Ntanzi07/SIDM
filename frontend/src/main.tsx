import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
// fontes servidas pelo próprio app (a rede interna pode não alcançar o Google Fonts)
import '@fontsource-variable/figtree'
import '@fontsource-variable/inter-tight'
import './index.css'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
