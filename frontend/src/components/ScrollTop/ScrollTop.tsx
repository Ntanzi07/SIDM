import { useEffect, useState } from 'react'
import './ScrollTop.css'

// A seta de voltar ao topo: aparece depois de descer um pouco (listas de
// milhares de imagens), em todas as telas.
export function ScrollTop() {
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    const check = () => setVisible(window.scrollY > 600)
    check()
    window.addEventListener('scroll', check, { passive: true })
    return () => window.removeEventListener('scroll', check)
  }, [])

  if (!visible) return null
  return (
    <button aria-label="Voltar ao topo" className="scroll-top" onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })} title="Voltar ao topo" type="button">
      <svg aria-hidden="true" fill="none" height="18" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" viewBox="0 0 24 24" width="18">
        <path d="M12 19V5M5 12l7-7 7 7" />
      </svg>
    </button>
  )
}
