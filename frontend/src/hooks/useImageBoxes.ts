import { useCallback, useEffect, useState } from 'react'
import { getImageBoxes, type BoxSet } from '../services/api'

// As caixas da imagem aberta, uma entrada por tarefa (vazio: sem caixas, ou
// ainda carregando). reload() busca de novo (depois de editar os atributos).
export function useImageBoxes(sha1: string | undefined): { sets: BoxSet[]; reload: () => void } {
  const [loaded, setLoaded] = useState<{ sha1?: string; sets: BoxSet[] }>({ sets: [] })
  const [round, setRound] = useState(0)
  useEffect(() => {
    if (!sha1) return
    let active = true
    getImageBoxes(sha1)
      .then((sets) => { if (active) setLoaded({ sha1, sets }) })
      .catch(() => { if (active) setLoaded({ sha1, sets: [] }) })
    return () => { active = false }
  }, [sha1, round])
  const reload = useCallback(() => setRound((n) => n + 1), [])
  return { sets: loaded.sha1 === sha1 ? loaded.sets : [], reload }
}

// "placa-letras: 7 caixas · capacete: sem objetos"
export function describeBoxSets(sets: BoxSet[]): string {
  return sets
    .map((set) => `${set.task}: ${set.boxes.length > 0 ? `${set.boxes.length} caixa(s)` : 'sem objetos'}`)
    .join(' · ')
}
